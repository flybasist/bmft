package wizard

import (
	"fmt"
	"strconv"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// Конвенция для welcome wizard'а:
//   - имя wizard'а: "welcome"
//   - шаги: "step1_toggle" → "step2_ttl" → (опционально) "step2_manual_ttl" → завершение
//   - данные в State.Data:
//     "enabled" bool — текущая настройка on/off
//     "ttl"     int  — текущий TTL в секундах
//   - unique кнопок: "wiz_w_on", "wiz_w_off", "wiz_w_ttl" (data=сек или "m"),
//     "wiz_w_back". Кнопка отмены — общая (UniqueCancel).
const (
	wizardWelcomeName = "welcome"

	stepWelcomeToggle    = "step1_toggle"
	stepWelcomeTTL       = "step2_ttl"
	stepWelcomeManualTTL = "step2_manual_ttl"

	uniqueWelcomeOn   = "wiz_w_on"
	uniqueWelcomeOff  = "wiz_w_off"
	uniqueWelcomeTTL  = "wiz_w_ttl"
	uniqueWelcomeBack = "wiz_w_back"

	dataKeyEnabled = "enabled"
	dataKeyTTL     = "ttl"
)

// welcomeWizard — состояние wizard'а с зависимостями.
// Регистрируется через RegisterWelcome при старте бота.
type welcomeWizard struct {
	mgr      *Manager
	chatRepo *repositories.ChatRepository
	logger   *zap.Logger
}

// RegisterWelcome подключает /welcome wizard к боту.
//
// Регистрирует callback handler'ы для всех кнопок wizard'а. Также
// возвращает функцию StartWelcome, которую вызывает /welcome handler
// (см. cmd/bot/handlers.go) при пустых аргументах команды.
//
// Возвращаемая функция — точка входа: вызывается из /welcome без аргументов
// (с проверками на anonymous-админа и личку — внутри Manager.Start).
func RegisterWelcome(bot *tele.Bot, mgr *Manager, chatRepo *repositories.ChatRepository, logger *zap.Logger) func(c tele.Context) error {
	w := &welcomeWizard{mgr: mgr, chatRepo: chatRepo, logger: logger}

	// Кнопки шага 1 (toggle on/off).
	btnOn := tele.Btn{Unique: uniqueWelcomeOn, Text: "✅ Включить"}
	btnOff := tele.Btn{Unique: uniqueWelcomeOff, Text: "⛔ Выключить"}
	bot.Handle(&btnOn, w.handleToggleOn)
	bot.Handle(&btnOff, w.handleToggleOff)

	// Кнопки шага 2 (выбор TTL): один handler, разные data.
	btnTTL := tele.Btn{Unique: uniqueWelcomeTTL}
	bot.Handle(&btnTTL, w.handleTTLChoice)

	// Кнопка «Назад» с шага 2 на шаг 1.
	btnBack := tele.Btn{Unique: uniqueWelcomeBack, Text: "⬅ Назад"}
	bot.Handle(&btnBack, w.handleBack)

	// Регистрируем text-handler для шага manual TTL.
	mgr.RegisterTextHandler(wizardWelcomeName, w.handleWelcomeTextInput)

	return w.start
}

// start — точка входа: загружает текущие настройки и рисует шаг 1.
func (w *welcomeWizard) start(c tele.Context) error {
	chatID := c.Chat().ID
	settings, err := w.chatRepo.GetWelcomeSettings(chatID)
	if err != nil {
		w.logger.Error("welcome wizard: get settings", zap.Error(err), zap.Int64("chat_id", chatID))
		return c.Send("Не удалось прочитать настройки.")
	}

	initialData := map[string]any{
		dataKeyEnabled: settings.Enabled,
		dataKeyTTL:     settings.TTLSeconds,
	}

	return w.mgr.Start(c, wizardWelcomeName, initialData, func(state *State) error {
		state.Step = stepWelcomeToggle
		text, markup := w.renderStep1(state)
		msg, err := c.Bot().Send(c.Chat(), text, markup, &tele.SendOptions{ParseMode: tele.ModeHTML})
		if err != nil {
			return err
		}
		w.mgr.SetMessage(state, msg)
		return nil
	})
}

// renderStep1 строит текст и клавиатуру первого шага.
func (w *welcomeWizard) renderStep1(state *State) (string, *tele.ReplyMarkup) {
	enabled, _ := state.Data[dataKeyEnabled].(bool)
	ttl, _ := state.Data[dataKeyTTL].(int)

	currentState := "⛔ выключено"
	if enabled {
		currentState = "✅ включено"
	}
	ttlNote := fmt.Sprintf("%d сек", ttl)
	if ttl == 0 {
		ttlNote = "без авто-удаления"
	}

	text := fmt.Sprintf(
		"<b>⚙ Настройка приветствия</b>\n\n"+
			"Сейчас: <b>%s</b>\nTTL: <b>%s</b>\n\n"+
			"Включить или выключить?",
		currentState, ttlNote,
	)

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: uniqueWelcomeOn, Text: "✅ Включить"},
			tele.Btn{Unique: uniqueWelcomeOff, Text: "⛔ Выключить"},
		),
		markup.Row(CancelButton()),
	)
	return text, markup
}

// renderStep2 — выбор TTL после включения приветствия.
func (w *welcomeWizard) renderStep2() (string, *tele.ReplyMarkup) {
	text := "<b>⚙ Авто-удаление приветствия</b>\n" +
		"Через сколько удалять сообщение \xabПривет, @user\xbb?"

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: uniqueWelcomeTTL, Text: "Не удалять", Data: "0"},
			tele.Btn{Unique: uniqueWelcomeTTL, Text: "1 мин", Data: "60"},
			tele.Btn{Unique: uniqueWelcomeTTL, Text: "5 мин", Data: "300"},
		),
		markup.Row(
			tele.Btn{Unique: uniqueWelcomeTTL, Text: "10 мин", Data: "600"},
			tele.Btn{Unique: uniqueWelcomeTTL, Text: "1 час", Data: "3600"},
			tele.Btn{Unique: uniqueWelcomeTTL, Text: "✏ Вручную", Data: "m"},
		),
		markup.Row(
			tele.Btn{Unique: uniqueWelcomeBack, Text: "⬅ Назад"},
			CancelButton(),
		),
	)
	return text, markup
}

// editToStep редактирует сообщение wizard'а на новый шаг.
func (w *welcomeWizard) editToStep(c tele.Context, state *State, text string, markup *tele.ReplyMarkup) error {
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	_, err := w.mgr.bot.Edit(editable, text, markup, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		w.logger.Warn("welcome wizard: edit failed", zap.Error(err))
	}
	return err
}

// handleToggleOn — пользователь включил приветствие. Переходим к выбору TTL.
func (w *welcomeWizard) handleToggleOn(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardWelcomeName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	state.Data[dataKeyEnabled] = true
	state.Step = stepWelcomeTTL

	text, markup := w.renderStep2()
	return w.editToStep(c, state, text, markup)
}

// handleToggleOff — пользователь выключил приветствие. Применяем и завершаем.
func (w *welcomeWizard) handleToggleOff(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardWelcomeName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	chatID := state.Key.ChatID
	if err := w.chatRepo.SetWelcomeEnabled(chatID, false); err != nil {
		w.logger.Error("welcome wizard: SetWelcomeEnabled false", zap.Error(err), zap.Int64("chat_id", chatID))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось сохранить настройку.")
		return nil
	}
	w.mgr.Complete(c, state.Key, "✅ Приветствие выключено.")
	return nil
}

// handleTTLChoice обрабатывает выбор пресета TTL или переход в режим ручного ввода.
func (w *welcomeWizard) handleTTLChoice(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardWelcomeName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	data := c.Callback().Data
	if data == "m" {
		// Переходим в режим ожидания текстового ввода.
		state.Step = stepWelcomeManualTTL
		w.mgr.AwaitText(state, stepWelcomeManualTTL)

		text := "<b>✏ Введите TTL в секундах</b>\n\n" +
			"Допустимо: <b>0</b> (не удалять) либо число от <b>10</b> до <b>86400</b> (24 часа).\n\n" +
			"Отправьте число сообщением. Команда (например /cancel) отменит wizard."
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, text, markup)
	}

	ttl, parseErr := strconv.Atoi(data)
	if parseErr != nil || !isValidWelcomeTTL(ttl) {
		w.logger.Warn("welcome wizard: invalid ttl in callback", zap.String("data", data))
		return nil
	}
	return w.applyTTL(c, state, ttl)
}

// handleBack — возврат с шага 2 на шаг 1.
func (w *welcomeWizard) handleBack(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardWelcomeName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	state.Step = stepWelcomeToggle
	state.AwaitText = false
	text, markup := w.renderStep1(state)
	return w.editToStep(c, state, text, markup)
}

// applyTTL — финальное применение enabled=true + указанный TTL.
func (w *welcomeWizard) applyTTL(c tele.Context, state *State, ttl int) error {
	chatID := state.Key.ChatID
	if err := w.chatRepo.SetWelcomeEnabled(chatID, true); err != nil {
		w.logger.Error("welcome wizard: SetWelcomeEnabled true", zap.Error(err), zap.Int64("chat_id", chatID))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось сохранить настройку.")
		return nil
	}
	if err := w.chatRepo.SetWelcomeTTL(chatID, ttl); err != nil {
		w.logger.Error("welcome wizard: SetWelcomeTTL", zap.Error(err), zap.Int64("chat_id", chatID))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось сохранить TTL.")
		return nil
	}

	var finalText string
	if ttl == 0 {
		finalText = "✅ Приветствие включено. Сообщения не удаляются автоматически."
	} else {
		finalText = fmt.Sprintf("✅ Приветствие включено. TTL: %d сек.", ttl)
	}
	w.mgr.Complete(c, state.Key, finalText)
	return nil
}

// isValidWelcomeTTL — те же правила, что в handleWelcome (см. handlers.go):
// 0 (не удалять) или 10..86400 секунд.
func isValidWelcomeTTL(ttl int) bool {
	return ttl == 0 || (ttl >= 10 && ttl <= 86400)
}

// handleWelcomeTextInput — обработчик текстового ввода для шага manual TTL.
// Регистрируется в общем text-router (см. RegisterTextRouter).
func (w *welcomeWizard) handleWelcomeTextInput(c tele.Context, state *State, text string) error {
	if state.Step != stepWelcomeManualTTL {
		// На случай неожиданного шага — просто отменяем wizard.
		w.mgr.Cancel(c, state.Key, "🚫 Wizard отменён (неожиданный шаг).")
		return nil
	}

	ttl, err := strconv.Atoi(text)
	if err != nil || !isValidWelcomeTTL(ttl) {
		// Возвращаем шаг в режим ожидания текста ещё раз с подсказкой об ошибке.
		w.mgr.AwaitText(state, stepWelcomeManualTTL)
		errText := "❌ <b>Неверное значение.</b>\n\n" +
			"Введите <b>0</b> (не удалять) либо число от <b>10</b> до <b>86400</b> секунд.\n\n" +
			"Отправьте число сообщением или нажмите Отмена."
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, errText, markup)
	}
	return w.applyTTL(c, state, ttl)
}
