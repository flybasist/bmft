package wizard

import (
	"fmt"
	"html"
	"strconv"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// Конвенция для setlimit wizard'а:
//   - имя wizard'а: "setlimit"
//   - 2-3 шага:
//     step1_type   — выбор типа контента (12 кнопок).
//     step2_value  — выбор значения (пресеты + ✏ Свой + 🚫 Запретить).
//     step3_custom — ожидание текста: целое число ≥ -1.
//   - данные в State.Data:
//     DataThreadID    — ставится Manager.Start.
//     "userID"        — *int64, target для персонального лимита (если был ReplyTo).
//     "userName"      — string, отображаемое имя target (для UI).
//     "contentType"   — string, выбранный тип на шаге 1.
//   - unique кнопок: "wiz_l_type", "wiz_l_val", "wiz_l_back".
//
// Семантика значений (повторяет logic в limiter.go::OnMessage):
//   - 0   — без лимита (не используется, просто эквивалентно «нет записи»).
//   - >0  — N сообщений в день.
//   - -1  — полный запрет (любое сообщение этого типа удаляется).
//
// Дублирования SQL нет: используется ContentLimitsRepository.SetLimit,
// тот же что и в legacy handleSetLimit (limiter.go).
const (
	wizardSetLimitName = "setlimit"

	stepLimitType   = "step1_type"
	stepLimitValue  = "step2_value"
	stepLimitCustom = "step3_custom"

	uniqueLimitType = "wiz_l_type"
	uniqueLimitVal  = "wiz_l_val"
	uniqueLimitBack = "wiz_l_back"

	dataKeyUserID      = "userID"
	dataKeyUserName    = "userName"
	dataKeyContentType = "contentType"

	// Лимит на «свой» ввод. Защита от случайных миллиардов.
	// 100000 в день более чем достаточно для любого реального чата.
	limitMaxValue = 100000
)

// limitContentType — описание одного типа контента: код для БД,
// emoji-метка для кнопки и человекочитаемое название для UI.
type limitContentType struct {
	code  string // должен совпадать с validContentTypes в limiter.handleSetLimit
	emoji string
	name  string
}

// limitContentTypes — порядок и набор типов в кнопочной сетке шага 1.
// Список синхронизирован с validContentTypes в limiter.handleSetLimit.
// При добавлении нового типа в БД нужно править оба места.
var limitContentTypes = []limitContentType{
	{"text", "📝", "Текст"},
	{"photo", "🖼", "Фото"},
	{"video", "🎬", "Видео"},
	{"sticker", "🎨", "Стикеры"},
	{"animation", "🎞", "Гифки"},
	{"voice", "🎤", "Голос"},
	{"video_note", "📹", "Кружки"},
	{"audio", "🎵", "Аудио"},
	{"document", "📎", "Файлы"},
	{"location", "📍", "Локации"},
	{"contact", "👤", "Контакты"},
	{"banned_words", "🚫", "Запр.слова"},
	{"via_bot", "🤖", "Inline-боты"},
}

// limitPresets — пресеты значений для шага 2. -1 и «Свой» добавляются отдельно.
var limitPresets = []int{5, 10, 20, 50, 100}

type setLimitWizard struct {
	mgr               *Manager
	contentLimitsRepo *repositories.ContentLimitsRepository
	chatRepo          *repositories.ChatRepository
	eventRepo         *repositories.EventRepository
	logger            *zap.Logger
}

// RegisterSetLimit подключает /setlimit wizard к боту.
func RegisterSetLimit(
	bot *tele.Bot,
	mgr *Manager,
	contentLimitsRepo *repositories.ContentLimitsRepository,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(c tele.Context) error {
	w := &setLimitWizard{
		mgr:               mgr,
		contentLimitsRepo: contentLimitsRepo,
		chatRepo:          chatRepo,
		eventRepo:         eventRepo,
		logger:            logger,
	}

	btnType := tele.Btn{Unique: uniqueLimitType}
	bot.Handle(&btnType, w.handleType)

	btnVal := tele.Btn{Unique: uniqueLimitVal}
	bot.Handle(&btnVal, w.handleValue)

	btnBack := tele.Btn{Unique: uniqueLimitBack, Text: "⬅ Назад"}
	bot.Handle(&btnBack, w.handleBack)

	mgr.RegisterTextHandler(wizardSetLimitName, w.handleCustomText)

	return w.start
}

// start — точка входа. Если есть ReplyTo, сохраняет target в state.
func (w *setLimitWizard) start(c tele.Context) error {
	initialData := map[string]any{}

	if msg := c.Message(); msg != nil && msg.ReplyTo != nil && msg.ReplyTo.Sender != nil {
		target := msg.ReplyTo.Sender
		if target.IsBot {
			return c.Send("❌ Нельзя установить персональный лимит боту.")
		}
		if target.ID == c.Bot().Me.ID {
			return c.Send("❌ Нельзя установить лимит самому боту.")
		}
		initialData[dataKeyUserID] = target.ID
		initialData[dataKeyUserName] = core.DisplayName(target)
	}

	return w.mgr.Start(c, wizardSetLimitName, initialData, func(state *State) error {
		state.Step = stepLimitType
		text, markup := w.renderStep1(state)
		sent, err := c.Bot().Send(c.Chat(), text, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: markup})
		if err != nil {
			return err
		}
		w.mgr.SetMessage(state, sent)
		return nil
	})
}

// scopeText формирует строку «область действия» по threadID и userID.
func (w *setLimitWizard) scopeText(state *State) string {
	threadID, _ := state.Data[DataThreadID].(int)
	userID, hasUser := state.Data[dataKeyUserID].(int64)
	userName, _ := state.Data[dataKeyUserName].(string)

	scope := "<b>всего чата</b>"
	if threadID != 0 {
		scope = "<b>этого топика</b>"
	}
	if hasUser {
		return fmt.Sprintf("%s для %s (<code>%d</code>)", scope, html.EscapeString(userName), userID)
	}
	return scope
}

// renderStep1 — выбор типа контента: 4 ряда по 3 кнопки.
func (w *setLimitWizard) renderStep1(state *State) (string, *tele.ReplyMarkup) {
	text := fmt.Sprintf(
		"<b>📊 Установка лимита</b>\n\n"+
			"Область: %s\n\n"+
			"Выберите тип контента:",
		w.scopeText(state),
	)

	markup := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, 5)
	var row []tele.Btn
	for i, ct := range limitContentTypes {
		row = append(row, tele.Btn{
			Unique: uniqueLimitType,
			Text:   ct.emoji + " " + ct.name,
			Data:   ct.code,
		})
		if (i+1)%3 == 0 {
			rows = append(rows, markup.Row(row...))
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, markup.Row(row...))
	}
	rows = append(rows, markup.Row(CancelButton()))
	markup.Inline(rows...)
	return text, markup
}

// renderStep2 — выбор значения для уже выбранного типа.
func (w *setLimitWizard) renderStep2(state *State) (string, *tele.ReplyMarkup) {
	contentType, _ := state.Data[dataKeyContentType].(string)
	typeName := contentTypeName(contentType)

	text := fmt.Sprintf(
		"<b>📊 Лимит на %s</b>\n"+
			"Выберите значение (сообщений в сутки):",
		html.EscapeString(typeName),
	)

	markup := &tele.ReplyMarkup{}
	presetRow := make([]tele.Btn, 0, len(limitPresets))
	for _, n := range limitPresets {
		presetRow = append(presetRow, tele.Btn{
			Unique: uniqueLimitVal,
			Text:   strconv.Itoa(n),
			Data:   strconv.Itoa(n),
		})
	}
	markup.Inline(
		markup.Row(presetRow...),
		markup.Row(
			tele.Btn{Unique: uniqueLimitVal, Text: "✏ Свой", Data: "custom"},
			tele.Btn{Unique: uniqueLimitVal, Text: "🚫 Запретить", Data: "-1"},
		),
		markup.Row(
			tele.Btn{Unique: uniqueLimitBack, Text: "⬅ Назад"},
			CancelButton(),
		),
	)
	return text, markup
}

// handleType — клик по кнопке типа контента.
func (w *setLimitWizard) handleType(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardSetLimitName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	code := c.Callback().Data
	if !isValidContentType(code) {
		w.logger.Warn("setlimit wizard: unknown content type", zap.String("code", code))
		return nil
	}

	state.Data[dataKeyContentType] = code
	state.Step = stepLimitValue

	text, markup := w.renderStep2(state)
	return w.editToStep(c, state, text, markup)
}

// handleValue — клик по пресету / -1 / custom.
func (w *setLimitWizard) handleValue(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardSetLimitName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	data := c.Callback().Data
	if data == "custom" {
		state.Step = stepLimitCustom
		w.mgr.AwaitText(state, stepLimitCustom)
		text := fmt.Sprintf(
			"<b>✏ Введите лимит</b>\n"+
				"Целое число от 0 до %d (или -1 = запретить).",
			limitMaxValue,
		)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, text, markup)
	}

	value, err := strconv.Atoi(data)
	if err != nil {
		w.logger.Warn("setlimit wizard: bad value data", zap.String("data", data))
		return nil
	}
	return w.applyLimit(c, state, value)
}

// handleBack — возврат с шага 2 на шаг 1.
func (w *setLimitWizard) handleBack(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardSetLimitName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	if state.Step != stepLimitValue && state.Step != stepLimitCustom {
		return nil
	}

	delete(state.Data, dataKeyContentType)
	state.Step = stepLimitType
	state.AwaitText = false

	text, markup := w.renderStep1(state)
	return w.editToStep(c, state, text, markup)
}

// handleCustomText — обработчик текста на шаге step3_custom.
func (w *setLimitWizard) handleCustomText(c tele.Context, state *State, text string) error {
	if state.Step != stepLimitCustom {
		w.mgr.Cancel(c, state.Key, "🚫 Wizard отменён (неожиданный шаг).")
		return nil
	}

	value, err := strconv.Atoi(text)
	if err != nil || value < -1 || value > limitMaxValue {
		// Возврат в режим ожидания с подсказкой.
		w.mgr.AwaitText(state, stepLimitCustom)
		errText := fmt.Sprintf(
			"❌ <b>Неверное значение.</b>\n\n"+
				"Введите целое число от 0 до %d или -1.\n"+
				"Отправьте новое значение или нажмите Отмена.",
			limitMaxValue,
		)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, errText, markup)
	}

	return w.applyLimit(c, state, value)
}

// applyLimit — финальная запись лимита и завершение wizard'а.
func (w *setLimitWizard) applyLimit(c tele.Context, state *State, value int) error {
	chatID := state.Key.ChatID
	threadID, _ := state.Data[DataThreadID].(int)
	contentType, _ := state.Data[dataKeyContentType].(string)
	typeName := contentTypeName(contentType)

	var userPtr *int64
	var userName string
	if uid, ok := state.Data[dataKeyUserID].(int64); ok {
		userPtr = &uid
		userName, _ = state.Data[dataKeyUserName].(string)
	}

	if err := w.chatRepo.EnsureExists(chatID); err != nil {
		w.logger.Error("setlimit wizard: ensure chat", zap.Error(err))
	}

	if err := w.contentLimitsRepo.SetLimit(chatID, threadID, userPtr, contentType, value); err != nil {
		w.logger.Error("setlimit wizard: SetLimit",
			zap.Error(err),
			zap.Int64("chat_id", chatID),
			zap.String("type", contentType),
			zap.Int("value", value))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось установить лимит.")
		return nil
	}

	details := fmt.Sprintf("Set limit via wizard: %s=%d (chat=%d, thread=%d)", contentType, value, chatID, threadID)
	if userPtr != nil {
		details = fmt.Sprintf("Set limit via wizard: %s=%d for user %d (chat=%d, thread=%d)",
			contentType, value, *userPtr, chatID, threadID)
	}
	_ = w.eventRepo.Log(chatID, c.Sender().ID, "limiter", "set_limit", details)

	// Финальный текст: повторяет смысл legacy handleSetLimit, но короче.
	valueText := fmt.Sprintf("%d в сутки", value)
	switch value {
	case -1:
		valueText = "🚫 запрещено"
	case 0:
		valueText = "∞ без лимита"
	}

	finalText := fmt.Sprintf(
		"✅ Лимит обновлён.\n\nТип: <b>%s</b>\nОбласть: %s\nЗначение: %s",
		html.EscapeString(typeName),
		w.scopeText(state),
		valueText,
	)

	// Дополнительные предупреждения (как в legacy).
	if contentType == "banned_words" && value > 0 {
		finalText += "\n\n⚠️ Для работы этого лимита нужен фильтр мата: /setprofanity"
	}
	if contentType == "text" && value == -1 {
		finalText += "\n\n⚠️ При полном запрете текста /addban и /setprofanity не смогут проверять текст — Limiter удаляет его до фильтров."
	}

	_ = userName // userName уже включён в scopeText
	w.mgr.Complete(c, state.Key, finalText)
	return nil
}

// editToStep редактирует сообщение wizard'а на новый шаг.
func (w *setLimitWizard) editToStep(c tele.Context, state *State, text string, markup *tele.ReplyMarkup) error {
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	_, err := w.mgr.bot.Edit(editable, text, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: markup})
	if err != nil {
		w.logger.Warn("setlimit wizard: edit failed", zap.Error(err))
	}
	return err
}

// isValidContentType проверяет, что код есть в limitContentTypes.
func isValidContentType(code string) bool {
	for _, ct := range limitContentTypes {
		if ct.code == code {
			return true
		}
	}
	return false
}

// contentTypeName возвращает «emoji + Имя» по коду; для unknown — сам код.
func contentTypeName(code string) string {
	for _, ct := range limitContentTypes {
		if ct.code == code {
			return ct.emoji + " " + ct.name
		}
	}
	return code
}
