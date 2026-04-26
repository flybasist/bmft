package wizard

import (
	"database/sql"
	"fmt"
	"html"
	"regexp"
	"strings"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// Конвенция для addban wizard'а:
//   - имя wizard'а: "addban"
//   - 2 шага:
//     step1_pattern — ожидание текста паттерна (1..500 символов).
//     Спецсимволы regex автодетектятся, regex валидируется.
//     step2_action  — выбор действия [🗑 delete] [⚠ warn] [🗑+⚠ delete_warn] [⬅ Назад] [❌ Отмена].
//   - данные в State.Data:
//     DataThreadID   — ставится Manager.Start.
//     "pattern"      — string, паттерн (валидированный).
//     "isRegex"      — bool, автодетект.
//   - unique кнопок: "wiz_b_act", "wiz_b_back".
//
// Семантика action (повторяет logic в handleAddBan):
//   - "delete"       — молча удалять сообщение с паттерном.
//   - "warn"         — отвечать предупреждением, не удалять.
//   - "delete_warn"  — удалять и предупреждать.
//
// Дублирование SQL: вставка в keyword_reactions повторяет logic в
// handleAddBan (filters.go). Это осознано — wizard package не зависит
// от модулей. При изменении схемы keyword_reactions нужно править оба места.
const (
	wizardAddBanName = "addban"

	stepBanPattern = "step1_pattern"
	stepBanAction  = "step2_action"

	uniqueBanAction = "wiz_b_act"
	uniqueBanBack   = "wiz_b_back"

	dataKeyPattern = "pattern"
	dataKeyIsRegex = "isRegex"

	banPatternMaxLen = 500
)

// banRegexChars — спецсимволы, при наличии которых паттерн считается regex.
// Список идентичен handleAddBan в filters.go.
var banRegexChars = []string{"|", "(", ")", "[", "]", ".", "*", "+", "?", "^", "$"}

type addBanWizard struct {
	mgr       *Manager
	db        *sql.DB
	chatRepo  *repositories.ChatRepository
	eventRepo *repositories.EventRepository
	logger    *zap.Logger
}

// RegisterAddBan подключает /addban wizard к боту.
func RegisterAddBan(
	bot *tele.Bot,
	mgr *Manager,
	db *sql.DB,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(c tele.Context) error {
	w := &addBanWizard{
		mgr:       mgr,
		db:        db,
		chatRepo:  chatRepo,
		eventRepo: eventRepo,
		logger:    logger,
	}

	btnAct := tele.Btn{Unique: uniqueBanAction}
	bot.Handle(&btnAct, w.handleAction)

	btnBack := tele.Btn{Unique: uniqueBanBack, Text: "⬅ Назад"}
	bot.Handle(&btnBack, w.handleBack)

	mgr.RegisterTextHandler(wizardAddBanName, w.handlePatternText)

	return w.start
}

// start — точка входа: сразу переводим в режим ожидания паттерна.
func (w *addBanWizard) start(c tele.Context) error {
	return w.mgr.Start(c, wizardAddBanName, nil, func(state *State) error {
		state.Step = stepBanPattern
		w.mgr.AwaitText(state, stepBanPattern)

		text := w.renderStep1Text(state)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))

		sent, err := c.Bot().Send(c.Chat(), text, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: markup})
		if err != nil {
			return err
		}
		w.mgr.SetMessage(state, sent)
		return nil
	})
}

// scopeText — область действия (чат vs топик).
func (w *addBanWizard) scopeText(state *State) string {
	threadID, _ := state.Data[DataThreadID].(int)
	if threadID != 0 {
		return "<b>этого топика</b>"
	}
	return "<b>всего чата</b>"
}

// renderStep1Text — приглашение ввести паттерн.
func (w *addBanWizard) renderStep1Text(state *State) string {
	return fmt.Sprintf(
		"<b>🚫 Добавление запрещённого слова</b>\n\n"+
			"Область: %s\n\n"+
			"Отправьте паттерн сообщением (1..%d символов).\n"+
			"Спецсимволы (<code>|()[].*+?^$</code>) автоматически "+
			"включают режим regex.\n\n"+
			"Пример: <code>спам</code> или <code>спам|реклама|продам</code>",
		w.scopeText(state), banPatternMaxLen,
	)
}

// handlePatternText — обработчик текста на шаге step1_pattern.
func (w *addBanWizard) handlePatternText(c tele.Context, state *State, text string) error {
	if state.Step != stepBanPattern {
		w.mgr.Cancel(c, state.Key, "🚫 Wizard отменён (неожиданный шаг).")
		return nil
	}

	pattern := strings.TrimSpace(text)
	if pattern == "" {
		return w.rejectPattern(c, state, "❌ Паттерн не может быть пустым.")
	}
	if len(pattern) > banPatternMaxLen {
		return w.rejectPattern(c, state,
			fmt.Sprintf("❌ Паттерн слишком длинный (макс. %d символов).", banPatternMaxLen))
	}

	// Автодетект regex.
	isRegex := false
	for _, ch := range banRegexChars {
		if strings.Contains(pattern, ch) {
			isRegex = true
			break
		}
	}
	if isRegex {
		if _, err := regexp.Compile(pattern); err != nil {
			return w.rejectPattern(c, state,
				fmt.Sprintf("❌ Некорректное regex-выражение: %v", err))
		}
	}

	state.Data[dataKeyPattern] = pattern
	state.Data[dataKeyIsRegex] = isRegex
	state.Step = stepBanAction

	text2, markup := w.renderStep2(state)
	return w.editToStep(c, state, text2, markup)
}

// rejectPattern — оставляем wizard на шаге step1_pattern, показываем ошибку.
func (w *addBanWizard) rejectPattern(c tele.Context, state *State, errMsg string) error {
	w.mgr.AwaitText(state, stepBanPattern)
	text := errMsg + "\n\n" + w.renderStep1Text(state)
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(CancelButton()))
	return w.editToStep(c, state, text, markup)
}

// renderStep2 — выбор действия для уже сохранённого паттерна.
func (w *addBanWizard) renderStep2(state *State) (string, *tele.ReplyMarkup) {
	pattern, _ := state.Data[dataKeyPattern].(string)
	isRegex, _ := state.Data[dataKeyIsRegex].(bool)

	regexLabel := "обычный текст"
	if isRegex {
		regexLabel = "<b>regex</b>"
	}

	text := fmt.Sprintf(
		"<b>🚫 Выбор действия</b>\n\n"+
			"Паттерн: <code>%s</code> (%s)\n"+
			"Область: %s\n\n"+
			"Что делать с сообщениями, содержащими этот паттерн?",
		html.EscapeString(pattern), regexLabel, w.scopeText(state),
	)

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: uniqueBanAction, Text: "🗑 Удалить", Data: "delete"},
			tele.Btn{Unique: uniqueBanAction, Text: "⚠ Предупредить", Data: "warn"},
		),
		markup.Row(
			tele.Btn{Unique: uniqueBanAction, Text: "🗑+⚠ Удалить и предупредить", Data: "delete_warn"},
		),
		markup.Row(
			tele.Btn{Unique: uniqueBanBack, Text: "⬅ Назад"},
			CancelButton(),
		),
	)
	return text, markup
}

// handleAction — клик по кнопке выбора действия.
func (w *addBanWizard) handleAction(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardAddBanName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	action := c.Callback().Data
	if action != "delete" && action != "warn" && action != "delete_warn" {
		w.logger.Warn("addban wizard: unknown action", zap.String("data", action))
		return nil
	}
	return w.applyBan(c, state, action)
}

// handleBack — возврат с шага 2 на шаг 1 (новый ввод паттерна).
func (w *addBanWizard) handleBack(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardAddBanName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	if state.Step != stepBanAction {
		return nil
	}

	delete(state.Data, dataKeyPattern)
	delete(state.Data, dataKeyIsRegex)
	state.Step = stepBanPattern
	w.mgr.AwaitText(state, stepBanPattern)

	text := w.renderStep1Text(state)
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(CancelButton()))
	return w.editToStep(c, state, text, markup)
}

// applyBan — финальный INSERT в keyword_reactions и завершение.
func (w *addBanWizard) applyBan(c tele.Context, state *State, action string) error {
	chatID := state.Key.ChatID
	threadID, _ := state.Data[DataThreadID].(int)
	pattern, _ := state.Data[dataKeyPattern].(string)
	isRegex, _ := state.Data[dataKeyIsRegex].(bool)

	if err := w.chatRepo.EnsureExists(chatID); err != nil {
		w.logger.Error("addban wizard: ensure chat", zap.Error(err))
	}

	// SQL идентичен handleAddBan в reactions/filters.go.
	_, err := w.db.Exec(`
		INSERT INTO keyword_reactions (chat_id, thread_id, pattern, is_regex, response_type, response_content, description, action, is_active)
		VALUES ($1, $2, $3, $4, 'none', '', '', $5, true)
	`, chatID, threadID, pattern, isRegex, action)
	if err != nil {
		w.logger.Error("addban wizard: insert keyword_reactions",
			zap.Error(err),
			zap.Int64("chat_id", chatID),
			zap.String("pattern", pattern))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось добавить запрещённое слово.")
		return nil
	}

	_ = w.eventRepo.Log(chatID, c.Sender().ID, "reactions", "add_filter",
		fmt.Sprintf("Added filter via wizard: pattern='%s', action=%s (chat=%d, thread=%d)",
			pattern, action, chatID, threadID))

	finalText := fmt.Sprintf(
		"✅ Запрещённое слово добавлено для %s.\n\n"+
			"Паттерн: <code>%s</code>\nДействие: %s",
		w.scopeText(state), html.EscapeString(pattern), action,
	)
	w.mgr.Complete(c, state.Key, finalText)
	return nil
}

// editToStep редактирует сообщение wizard'а на новый шаг.
func (w *addBanWizard) editToStep(c tele.Context, state *State, text string, markup *tele.ReplyMarkup) error {
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	_, err := w.mgr.bot.Edit(editable, text, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: markup})
	if err != nil {
		w.logger.Warn("addban wizard: edit failed", zap.Error(err))
	}
	return err
}
