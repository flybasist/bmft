package wizard

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
)

// Константы inline-меню.
const (
	// MenuTimeout — таймаут бездействия для inline-меню (5 мин, как у wizard'а).
	MenuTimeout = 5 * time.Minute

	// Уникальные callback'и кнопок меню.
	// Формат: "m_<действие>". Префикс "m_" отделяет меню от wizard'ов ("wiz_").
	UniqueMenuClose = "m_close"
	UniqueMenuBack  = "m_back"
)

// CloseButton — стандартная кнопка закрытия меню.
func CloseButton() tele.Btn {
	return tele.Btn{Unique: UniqueMenuClose, Text: "❌ Закрыть"}
}

// BackButton — стандартная кнопка «Назад».
func BackButton() tele.Btn {
	return tele.Btn{Unique: UniqueMenuBack, Text: "⬅ Назад"}
}

// StartMenu создаёт inline-меню для текущего пользователя.
//
// В отличие от Start (wizard):
//   - Работает и в личке, и в группе.
//   - Не требует admin-прав на старте (проверки per-button).
//   - Удаляет команду пользователя из чата (только в группах).
//   - State.IsMenu = true — Guard пропускает admin-recheck.
//
// Если уже есть активная сессия (wizard или меню) для (chat, user) —
// она заменяется (старое сообщение удаляется).
func (m *Manager) StartMenu(c tele.Context, menuName string, initialData map[string]any, render func(state *State) error) error {
	chat := c.Chat()
	if chat == nil || c.Sender() == nil {
		return nil
	}

	key := StateKey{
		ChatID: chat.ID,
		UserID: c.Sender().ID,
	}

	// Если был предыдущий wizard/меню — отменяем.
	if old := m.store.remove(key); old != nil {
		m.deleteWizardMessage(chat, old.MessageID)
		m.logger.Debug("previous session replaced by menu",
			zap.Int64("chat_id", key.ChatID),
			zap.Int64("user_id", key.UserID),
			zap.String("old", old.Wizard),
		)
	}

	if initialData == nil {
		initialData = make(map[string]any)
	}

	// В группах сохраняем threadID и удаляем команду пользователя.
	if chat.Type == tele.ChatGroup || chat.Type == tele.ChatSuperGroup {
		msg := c.Message()
		if msg != nil {
			initialData[DataThreadID] = core.GetThreadIDFromMessage(m.db, msg)
			// Удаляем команду пользователя из чата.
			if err := m.bot.Delete(msg); err != nil {
				m.logger.Debug("failed to delete user command for menu",
					zap.Int("message_id", msg.ID),
					zap.Error(err))
			}
		}
	}

	now := timeNow()
	state := &State{
		Key:       key,
		Wizard:    menuName,
		Data:      initialData,
		StartedAt: now,
		UpdatedAt: now,
		IsMenu:    true,
	}
	m.store.set(state)
	m.armIdleTimer(state)

	if err := render(state); err != nil {
		m.store.remove(key)
		return err
	}
	return nil
}

// GuardMenu проверяет callback для inline-меню.
//
// В отличие от Guard (wizard):
//   - НЕ проверяет admin-права (каждая кнопка решает сама).
//   - Проверяет что нажимает владелец сессии (ownerID).
//   - Сбрасывает idle-таймер.
//
// Если нажал не владелец — показывает alert и возвращает ошибку.
func (m *Manager) GuardMenu(c tele.Context, expectedMenu string) (*State, error) {
	cb := c.Callback()
	if cb == nil || cb.Message == nil || c.Sender() == nil {
		return nil, fmt.Errorf("guard menu: not a callback context")
	}

	key := StateKey{
		ChatID: cb.Message.Chat.ID,
		UserID: c.Sender().ID,
	}

	state := m.store.get(key)
	if state == nil {
		_ = c.Respond(&tele.CallbackResponse{
			Text:      "Меню устарело. Вызовите команду заново.",
			ShowAlert: false,
		})
		return nil, fmt.Errorf("menu not active")
	}

	if state.Wizard != expectedMenu {
		_ = c.Respond(&tele.CallbackResponse{
			Text:      "Кнопка от другого меню.",
			ShowAlert: false,
		})
		return nil, fmt.Errorf("menu mismatch: have %s, want %s", state.Wizard, expectedMenu)
	}

	// Сбрасываем idle-таймер.
	m.armIdleTimer(state)
	return state, nil
}

// GuardMenuAdmin — как GuardMenu, но дополнительно проверяет admin-права.
// Используется для кнопок, которые выполняют админские действия.
func (m *Manager) GuardMenuAdmin(c tele.Context, expectedMenu string) (*State, error) {
	state, err := m.GuardMenu(c, expectedMenu)
	if err != nil {
		return nil, err
	}

	cb := c.Callback()
	isAdmin, apiErr := m.admin.IsAdmin(cb.Message.Chat, c.Sender().ID)
	if apiErr != nil {
		_ = c.Respond(&tele.CallbackResponse{
			Text:      "🚫 Не удалось проверить права.",
			ShowAlert: true,
		})
		return nil, apiErr
	}
	if !isAdmin {
		_ = c.Respond(&tele.CallbackResponse{
			Text:      "🚫 Только для администраторов.",
			ShowAlert: true,
		})
		return nil, fmt.Errorf("not admin")
	}
	return state, nil
}

// CloseMenu закрывает меню: удаляет сообщение и очищает state.
func (m *Manager) CloseMenu(c tele.Context, key StateKey) {
	state := m.store.remove(key)
	if state == nil {
		return
	}
	// Удаляем сообщение меню (не Edit→текст, а полное удаление).
	if state.MessageID != 0 {
		m.deleteWizardMessage(&tele.Chat{ID: key.ChatID}, state.MessageID)
	}
}

// EditMenu редактирует сообщение меню на новый экран.
// Возвращает ошибку telebot'а (для логирования).
func (m *Manager) EditMenu(state *State, text string, markup *tele.ReplyMarkup) error {
	if state.MessageID == 0 {
		return fmt.Errorf("no message to edit")
	}
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	_, err := m.bot.Edit(editable, text, &tele.SendOptions{
		ParseMode:   tele.ModeHTML,
		ReplyMarkup: markup,
	})
	return err
}

// RegisterMenuHandlers регистрирует общие кнопки меню: «Закрыть» и «Назад».
// «Назад» вызывает backHandler, который каждый модуль передаёт при регистрации.
//
// Вызывается один раз при инициализации.
func (m *Manager) RegisterMenuHandlers(bot *tele.Bot) {
	// Кнопка «Закрыть» — универсальная для всех меню.
	btnClose := CloseButton()
	bot.Handle(&btnClose, func(c tele.Context) error {
		_ = c.Respond()
		cb := c.Callback()
		if cb == nil || cb.Message == nil || c.Sender() == nil {
			return nil
		}
		key := StateKey{
			ChatID: cb.Message.Chat.ID,
			UserID: c.Sender().ID,
		}
		m.CloseMenu(c, key)
		return nil
	})
}
