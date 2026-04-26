package wizard

import (
	"database/sql"
	"fmt"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// Конвенция для setprofanity wizard'а:
//   - имя wizard'а: "setprofanity"
//   - один шаг (выбор действия), без back/manual.
//   - данные в State.Data:
//     DataThreadID    — ставится Manager.Start (thread_id чата).
//     "currentAction" — string ("delete"|"warn"|"delete_warn"|""),
//     "" = фильтр не настроен.
//   - unique кнопок: "wiz_p_set" (data="delete"|"warn"|"delete_warn"),
//     "wiz_p_off" (без data).
//
// Нота про дублирование SQL: вставка/удаление здесь повторяет логику
// reactions.handleSetProfanity / handleRemoveProfanity (см. соответствующий
// файл). Это сделано осознанно, чтобы wizard package оставался
// независимым от модулей. При изменении схемы profanity_settings нужно
// синхронно править оба места.
const (
	wizardProfanityName = "setprofanity"

	stepProfanitySelect = "step1_select"

	uniqueProfanitySet = "wiz_p_set"
	uniqueProfanityOff = "wiz_p_off"

	dataKeyCurrentAction = "currentAction"
)

type profanityWizard struct {
	mgr       *Manager
	db        *sql.DB
	chatRepo  *repositories.ChatRepository
	eventRepo *repositories.EventRepository
	logger    *zap.Logger
}

// RegisterSetProfanity регистрирует wizard для /setprofanity без аргументов.
//
// Возвращает функцию-точку входа: вызывается из reactions.handleSetProfanity
// (или из wrapper'а в cmd/bot), когда args пусто и контекст безопасен
// (группа, не anonymous).
//
// Сам Manager.Start ещё раз проверит все security-условия и при их
// нарушении просто отправит explanation вместо запуска wizard'а.
func RegisterSetProfanity(
	bot *tele.Bot,
	mgr *Manager,
	db *sql.DB,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(c tele.Context) error {
	w := &profanityWizard{
		mgr:       mgr,
		db:        db,
		chatRepo:  chatRepo,
		eventRepo: eventRepo,
		logger:    logger,
	}

	btnSet := tele.Btn{Unique: uniqueProfanitySet}
	bot.Handle(&btnSet, w.handleSetAction)

	btnOff := tele.Btn{Unique: uniqueProfanityOff, Text: "⛔ Отключить фильтр"}
	bot.Handle(&btnOff, w.handleOff)

	return w.start
}

// start — точка входа: загружает текущие настройки (для текущего thread_id)
// и рисует единственный шаг wizard'а.
func (w *profanityWizard) start(c tele.Context) error {
	chatID := c.Chat().ID
	// ThreadID считаем сейчас, чтобы показать настройки именно этого scope.
	// Manager.Start позже снова положит его в state.Data[DataThreadID].
	threadID := 0
	if msg := c.Message(); msg != nil {
		threadID = core.GetThreadIDFromMessage(w.db, msg)
	}

	current, err := w.queryAction(chatID, threadID)
	if err != nil {
		w.logger.Error("setprofanity wizard: load settings", zap.Error(err), zap.Int64("chat_id", chatID))
		return c.Send("Не удалось прочитать настройки фильтра мата.")
	}

	initialData := map[string]any{
		dataKeyCurrentAction: current,
	}

	return w.mgr.Start(c, wizardProfanityName, initialData, func(state *State) error {
		state.Step = stepProfanitySelect
		text, markup := w.renderStep(state)
		msg, err := c.Bot().Send(c.Chat(), text, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: markup})
		if err != nil {
			return err
		}
		w.mgr.SetMessage(state, msg)
		return nil
	})
}

// renderStep — единственный шаг: показ статуса + выбор действия.
func (w *profanityWizard) renderStep(state *State) (string, *tele.ReplyMarkup) {
	threadID, _ := state.Data[DataThreadID].(int)
	current, _ := state.Data[dataKeyCurrentAction].(string)

	scope := "<b>топика</b>"
	if threadID == 0 {
		scope = "<b>всего чата</b>"
	}

	statusLine := "Сейчас: <b>не настроен</b>"
	if current != "" {
		statusLine = fmt.Sprintf("Сейчас: <b>включён</b> (действие: <code>%s</code>)", current)
	}

	text := fmt.Sprintf(
		"<b>🚫 Фильтр мата</b>\n\n"+
			"Область: %s\n%s\n\n"+
			"Выберите действие фильтра (не распространяется на VIP):",
		scope, statusLine,
	)

	markup := &tele.ReplyMarkup{}
	rows := []tele.Row{
		markup.Row(
			tele.Btn{Unique: uniqueProfanitySet, Text: "🗑 delete", Data: "delete"},
			tele.Btn{Unique: uniqueProfanitySet, Text: "⚠ warn", Data: "warn"},
		),
		markup.Row(
			tele.Btn{Unique: uniqueProfanitySet, Text: "🗑+⚠ delete_warn", Data: "delete_warn"},
		),
	}
	// Кнопка «Отключить» только если фильтр уже включён — иначе бесполезна.
	if current != "" {
		rows = append(rows, markup.Row(tele.Btn{Unique: uniqueProfanityOff, Text: "⛔ Отключить фильтр"}))
	}
	rows = append(rows, markup.Row(CancelButton()))
	markup.Inline(rows...)
	return text, markup
}

// handleSetAction применяет выбранное действие фильтра.
func (w *profanityWizard) handleSetAction(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardProfanityName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	action := c.Callback().Data
	if action != "delete" && action != "warn" && action != "delete_warn" {
		w.logger.Warn("setprofanity wizard: invalid action in callback", zap.String("data", action))
		return nil
	}

	chatID := state.Key.ChatID
	threadID, _ := state.Data[DataThreadID].(int)

	// FK: profanity_settings.chat_id → chats(chat_id). EnsureExists гарантирует
	// строку chats до INSERT (см. handleSetProfanity в reactions/filters.go).
	if err := w.chatRepo.EnsureExists(chatID); err != nil {
		w.logger.Error("setprofanity wizard: ensure chat", zap.Error(err))
	}

	_, err = w.db.Exec(`
		INSERT INTO profanity_settings (chat_id, thread_id, action, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (chat_id, thread_id)
		DO UPDATE SET action = $3, updated_at = NOW()
	`, chatID, threadID, action)
	if err != nil {
		w.logger.Error("setprofanity wizard: insert", zap.Error(err), zap.Int64("chat_id", chatID))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось сохранить настройку.")
		return nil
	}

	_ = w.eventRepo.Log(chatID, c.Sender().ID, "reactions", "set_profanity",
		fmt.Sprintf("Set profanity filter via wizard: action=%s (chat=%d, thread=%d)", action, chatID, threadID))

	scope := "этого топика"
	if threadID == 0 {
		scope = "всего чата"
	}
	w.mgr.Complete(c, state.Key,
		fmt.Sprintf("✅ Фильтр мата включён для %s.\nДействие: <code>%s</code>", scope, action))
	return nil
}

// handleOff отключает фильтр мата для текущего scope.
func (w *profanityWizard) handleOff(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardProfanityName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	chatID := state.Key.ChatID
	threadID, _ := state.Data[DataThreadID].(int)

	res, err := w.db.Exec(`
		DELETE FROM profanity_settings
		WHERE chat_id = $1 AND thread_id = $2
	`, chatID, threadID)
	if err != nil {
		w.logger.Error("setprofanity wizard: delete", zap.Error(err), zap.Int64("chat_id", chatID))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось отключить фильтр.")
		return nil
	}

	scope := "этого топика"
	if threadID == 0 {
		scope = "всего чата"
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		w.mgr.Complete(c, state.Key, fmt.Sprintf("ℹ️ Фильтр мата для %s не был настроен.", scope))
		return nil
	}

	_ = w.eventRepo.Log(chatID, c.Sender().ID, "reactions", "remove_profanity",
		fmt.Sprintf("Removed profanity filter via wizard (chat=%d, thread=%d)", chatID, threadID))

	w.mgr.Complete(c, state.Key, fmt.Sprintf("✅ Фильтр мата отключён для %s.", scope))
	return nil
}

// queryAction возвращает текущий action для (chat, thread) — пустую строку,
// если фильтр не настроен. Без fallback на chat_id+thread_id=0:
// wizard показывает настройки именно текущего scope, чтобы изменения
// были предсказуемыми (а не «вижу настройки чата, меняю — а оказывается
// настройки топика создались»).
func (w *profanityWizard) queryAction(chatID int64, threadID int) (string, error) {
	var action string
	err := w.db.QueryRow(`
		SELECT action FROM profanity_settings
		WHERE chat_id = $1 AND thread_id = $2
	`, chatID, threadID).Scan(&action)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return action, nil
}
