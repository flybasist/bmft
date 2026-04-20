package wizard

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
)

// Manager — координатор всех wizard'ов. Создаётся один на бота.
//
// Жизненный цикл wizard'а:
//
//  1. Команда (/welcome без аргументов) вызывает Manager.Start(c, name, render).
//  2. Manager создаёт State, вызывает render(c, state) — wizard рисует первый шаг.
//  3. Пользователь жмёт inline-кнопку → callback handler вызывает Manager.Guard(c)
//     для проверки прав, затем сам wizard обновляет State.Data, переходит к
//     следующему шагу (Edit сообщения) или вызывает Manager.Complete().
//  4. Manager.Complete(c, key, applyMsg) — удаляет state, редактирует сообщение
//     wizard'а на финальный текст, ставит ScheduleDelete через ConfirmTTL.
//  5. Manager.Cancel(c, key, reason) — то же, что Complete, но текст отмены.
//
// Manager не предписывает конкретные шаги — это делает каждый wizard
// (см. internal/wizard/welcome.go как пример).
type Manager struct {
	store        *stateStore
	bot          *tele.Bot
	db           *sql.DB
	admin        *core.AdminChecker
	logger       *zap.Logger
	textHandlers map[string]TextHandler
}

// NewManager создаёт менеджер wizard'ов.
// db нужен для получения корректного ThreadID в text-intercept middleware.
func NewManager(bot *tele.Bot, db *sql.DB, admin *core.AdminChecker, logger *zap.Logger) *Manager {
	return &Manager{
		store:        newStateStore(),
		bot:          bot,
		db:           db,
		admin:        admin,
		logger:       logger,
		textHandlers: make(map[string]TextHandler),
	}
}

// Start инициализирует wizard для текущего пользователя.
//
// Проверки на старте:
//   - Команда вызвана в группе/супергруппе (в личке wizard'ы запрещены —
//     security model требует, чтобы wizard был привязан к чату).
//   - Sender не является anonymous-админом (нет уникального UserID для FSM).
//
// Если уже есть активный wizard для (chat, thread, user) — он отменяется,
// его сообщение удаляется, новый запускается с чистого листа.
//
// initialData — стартовые значения для State.Data (например, текущие настройки
// чата, чтобы показать их на первом шаге). Может быть nil.
//
// firstStep вызывается после создания state и должен:
//   - построить inline-клавиатуру через Manager.NewMarkup(...)
//   - вызвать c.Send(text, markup) и сохранить msg.ID в state через Manager.SetMessage.
//   - вернуть имя первого шага через state.Step (или присвоить до возврата).
//
// Если firstStep возвращает ошибку — state удаляется, ошибка возвращается наверх.
func (m *Manager) Start(c tele.Context, wizard string, initialData map[string]any, firstStep func(state *State) error) error {
	chat := c.Chat()
	if chat.Type != tele.ChatGroup && chat.Type != tele.ChatSuperGroup {
		return c.Send("🚫 Wizard работает только в группах. Используйте старый синтаксис команды.")
	}

	msg := c.Message()
	if core.IsAnonymousAdmin(msg) {
		return c.Send("🚫 Wizard недоступен в режиме «Remain Anonymous» (нельзя различить пользователей). Используйте старый синтаксис команды — см. /help.")
	}

	if c.Sender() == nil {
		return nil // нет смысла продолжать — некому отвечать
	}

	threadID := core.GetThreadIDFromMessage(m.db, msg)
	key := StateKey{
		ChatID: chat.ID,
		UserID: c.Sender().ID,
	}

	// Если был предыдущий wizard — отменяем его (без сообщения, чтобы не спамить).
	if old := m.store.remove(key); old != nil {
		m.deleteWizardMessage(chat, old.MessageID)
		m.logger.Debug("previous wizard replaced",
			zap.Int64("chat_id", key.ChatID),
			zap.Int64("user_id", key.UserID),
			zap.String("old_wizard", old.Wizard),
		)
	}

	if initialData == nil {
		initialData = make(map[string]any)
	}
	initialData[DataThreadID] = threadID

	now := timeNow()
	state := &State{
		Key:       key,
		Wizard:    wizard,
		Data:      initialData,
		StartedAt: now,
		UpdatedAt: now,
	}
	m.store.set(state)
	m.armIdleTimer(state)

	if err := firstStep(state); err != nil {
		m.store.remove(key)
		return err
	}
	return nil
}

// Guard вызывается в начале КАЖДОГО callback-обработчика wizard'а.
// Проверяет: state существует, нажимает инициатор, права админа не отозваны.
//
// Возвращает (state, nil) если всё ок — wizard может продолжать.
// Возвращает (nil, error) если запрос отклонён — handler должен сделать
// `return err`. Manager уже отправил пользователю popup-уведомление
// и (при необходимости) отменил wizard.
//
// Передавайте сюда контекст callback'а (c.Callback() != nil).
func (m *Manager) Guard(c tele.Context, expectedWizard string) (*State, error) {
	cb := c.Callback()
	if cb == nil || cb.Message == nil || c.Sender() == nil {
		return nil, fmt.Errorf("guard: not a callback context")
	}

	key := StateKey{
		ChatID: cb.Message.Chat.ID,
		UserID: c.Sender().ID,
	}

	state := m.store.get(key)
	if state == nil {
		// Нажали кнопку устаревшего wizard'а (его уже нет в store).
		// Это нормально: idle-таймаут или пользователь начал заново.
		_ = c.Respond(&tele.CallbackResponse{Text: "Этот wizard уже завершён.", ShowAlert: false})
		return nil, fmt.Errorf("wizard not active")
	}

	if state.Wizard != expectedWizard {
		_ = c.Respond(&tele.CallbackResponse{Text: "Кнопка не от этого wizard'а.", ShowAlert: false})
		return nil, fmt.Errorf("wizard mismatch: have %s, want %s", state.Wizard, expectedWizard)
	}

	// Per-step admin recheck. Сценарий: админа лишили прав за время wizard'а.
	isAdmin, err := m.admin.IsAdmin(cb.Message.Chat, c.Sender().ID)
	if err != nil {
		// При ошибке API — deny (consistent с AdminOnlyMiddleware).
		m.logger.Warn("wizard admin recheck failed",
			zap.Int64("chat_id", key.ChatID),
			zap.Int64("user_id", key.UserID),
			zap.Error(err))
		m.cancelWithText(c, key, "🚫 Не удалось проверить права. Wizard отменён.")
		return nil, err
	}
	if !isAdmin {
		m.cancelWithText(c, key, "🚫 Вы больше не админ этого чата. Wizard отменён.")
		return nil, fmt.Errorf("user lost admin rights")
	}

	// Сбрасываем idle-таймер (пользователь активен).
	m.armIdleTimer(state)
	return state, nil
}

// SetMessage запоминает ID отправленного сообщения wizard'а.
// Вызывается после Start/firstStep, когда wizard отправил первое сообщение.
func (m *Manager) SetMessage(state *State, msg *tele.Message) {
	if msg == nil {
		return
	}
	state.MessageID = msg.ID
}

// AwaitText переводит wizard в режим ожидания текстового ввода.
// Pipeline middleware (TextInterceptMiddleware) увидит флаг и поглотит
// следующее сообщение пользователя в этом (chat, thread, user).
//
// step — имя шага, на котором ждём текст (для логов и роутинга onText).
func (m *Manager) AwaitText(state *State, step string) {
	state.AwaitText = true
	state.Step = step
	state.UpdatedAt = timeNow()
}

// Complete завершает wizard успехом.
//   - Удаляет state.
//   - Редактирует сообщение wizard'а на finalText (без клавиатуры).
//   - Планирует удаление сообщения через ConfirmTTL (если ttl > 0; 0 = не удалять).
func (m *Manager) Complete(c tele.Context, key StateKey, finalText string, ttl ...interface{}) {
	state := m.store.remove(key)
	if state == nil {
		return
	}
	useTTL := ConfirmTTL
	if len(ttl) > 0 {
		switch v := ttl[0].(type) {
		case bool:
			if !v {
				useTTL = 0
			}
		}
	}
	editedMsg := m.editWizardMessage(c, state, finalText)
	if useTTL > 0 && editedMsg != nil {
		core.ScheduleDelete(m.bot, editedMsg, useTTL, m.logger)
	}
}

// Cancel отменяет wizard по инициативе пользователя или системы.
// Сообщение wizard'а заменяется на reason и удаляется через ConfirmTTL.
func (m *Manager) Cancel(c tele.Context, key StateKey, reason string) {
	m.cancelWithText(c, key, reason)
}

// cancelWithText — внутренняя версия Cancel, может быть вызвана из Guard.
// Отделена для ясности кода: «системная» отмена внутри Manager vs пользовательская.
func (m *Manager) cancelWithText(c tele.Context, key StateKey, reason string) {
	state := m.store.remove(key)
	if state == nil {
		return
	}
	editedMsg := m.editWizardMessage(c, state, reason)
	if editedMsg != nil {
		core.ScheduleDelete(m.bot, editedMsg, ConfirmTTL, m.logger)
	}
}

// editWizardMessage редактирует сообщение wizard'а на новый текст без кнопок.
// Если редактирование не удалось (например, сообщение удалено) — возвращает nil.
// Если удалось — возвращает обновлённое Message для последующего ScheduleDelete.
func (m *Manager) editWizardMessage(c tele.Context, state *State, text string) *tele.Message {
	if state.MessageID == 0 {
		return nil
	}
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	updated, err := m.bot.Edit(editable, text, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		m.logger.Debug("failed to edit wizard message on finalize",
			zap.Int("message_id", state.MessageID),
			zap.Int64("chat_id", state.Key.ChatID),
			zap.Error(err))
		return nil
	}
	return updated
}

// deleteWizardMessage удаляет сообщение wizard'а (без редактирования).
// Используется при «тихой» замене старого wizard'а на новый.
func (m *Manager) deleteWizardMessage(chat *tele.Chat, messageID int) {
	if messageID == 0 {
		return
	}
	editable := &tele.Message{ID: messageID, Chat: chat}
	if err := m.bot.Delete(editable); err != nil {
		m.logger.Debug("failed to delete wizard message",
			zap.Int("message_id", messageID),
			zap.Int64("chat_id", chat.ID),
			zap.Error(err))
	}
}

// armIdleTimer запускает (или перезапускает) idle-таймер wizard'а.
// При срабатывании — wizard отменяется, сообщение получает текст-explanation.
func (m *Manager) armIdleTimer(state *State) {
	if state.timer != nil {
		state.timer.Stop()
	}
	key := state.Key
	state.timer = time.AfterFunc(IdleTimeout, func() {
		// Через IdleTimeout проверяем что state ещё актуален (не был удалён).
		current := m.store.get(key)
		if current == nil || current != state {
			return
		}
		m.store.remove(key)
		m.logger.Debug("wizard expired by idle timeout",
			zap.String("wizard", state.Wizard),
			zap.Int64("chat_id", key.ChatID),
			zap.Int64("user_id", key.UserID))

		editable := &tele.Message{
			ID:   state.MessageID,
			Chat: &tele.Chat{ID: key.ChatID},
		}
		updated, err := m.bot.Edit(editable, "⏱ Wizard отменён по таймауту бездействия (5 минут).")
		if err == nil && updated != nil {
			core.ScheduleDelete(m.bot, updated, ConfirmTTL, m.logger)
		}
	})
}

// timeNow вынесено для возможной подмены в тестах.
var timeNow = func() (t time.Time) { return time.Now() }
