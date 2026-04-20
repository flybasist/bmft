package wizard

import (
	"fmt"
	"html"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// Конвенция для setvip wizard'а:
//   - имя wizard'а: "setvip"
//   - запускается ТОЛЬКО как reply: target пользователь определяется через
//     msg.ReplyTo.Sender.ID. Без reply wrapper отдаёт команду в legacy
//     (тот покажет «Ответьте на сообщение»).
//   - данные в State.Data:
//     DataThreadID    — ставится Manager.Start.
//     "targetID"      — int64, целевой UserID.
//     "targetName"    — string, отображаемое имя (для UI и подтверждений).
//   - шаги:
//     step1_confirm — кнопки: [✅ Без причины] [✏ Указать причину] [❌ Отмена]
//     step2_reason  — ожидаем текст причины, валидация 1..200 символов.
//   - unique кнопок: "wiz_v_grant" (data="" или "ask"), без back.
//
// Дублирование SQL: вызовы VIPRepository.GrantVIP / EnsureExists совпадают
// с handleSetVIP в limiter.go. Это нормально — мы используем тот же
// репозиторий, никакой свой SQL не пишем.
const (
	wizardSetVIPName = "setvip"

	stepVIPConfirm = "step1_confirm"
	stepVIPReason  = "step2_reason"

	uniqueVIPGrant = "wiz_v_grant"

	dataKeyTargetID   = "targetID"
	dataKeyTargetName = "targetName"

	// Лимит длины причины. 200 символов — компромисс: достаточно для
	// «Постоянный участник, доверенный пользователь, не спамит» и
	// помещается в одну колонку chat_vips.reason без обрезания UI.
	vipReasonMaxLen = 200

	// Дефолтная причина — повторяет legacy handleSetVIP (limiter.go),
	// чтобы поведение wizard'а и старого синтаксиса было идентичным.
	vipDefaultReason = "Установлено администратором"
)

type setVIPWizard struct {
	mgr       *Manager
	vipRepo   *repositories.VIPRepository
	chatRepo  *repositories.ChatRepository
	eventRepo *repositories.EventRepository
	logger    *zap.Logger
}

// RegisterSetVIP подключает /setvip wizard к боту.
//
// Возвращает функцию-точку входа для cmd/bot/wizards.go: вызывается
// wrapper'ом, когда args пусто И есть ReplyTo (target определён).
func RegisterSetVIP(
	bot *tele.Bot,
	mgr *Manager,
	vipRepo *repositories.VIPRepository,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(c tele.Context) error {
	w := &setVIPWizard{
		mgr:       mgr,
		vipRepo:   vipRepo,
		chatRepo:  chatRepo,
		eventRepo: eventRepo,
		logger:    logger,
	}

	btnGrant := tele.Btn{Unique: uniqueVIPGrant}
	bot.Handle(&btnGrant, w.handleGrant)

	// Регистрируем text-handler для шага step2_reason.
	mgr.RegisterTextHandler(wizardSetVIPName, w.handleVIPText)

	return w.start
}

// handleVIPText — обработчик текстового ввода для шага step2_reason.
func (w *setVIPWizard) handleVIPText(c tele.Context, state *State, text string) error {
	if state.Step != stepVIPReason {
		w.mgr.Cancel(c, state.Key, "🚫 Wizard отменён (неожиданный шаг).")
		return nil
	}
	reason := text
	if len(reason) > vipReasonMaxLen {
		// Возвращаем шаг в режим ожидания текста с подсказкой об ошибке.
		w.mgr.AwaitText(state, stepVIPReason)
		errText := fmt.Sprintf(
			"❌ <b>Слишком длинная причина.</b>\n\n"+
				"Максимум %d символов. Сократите и отправьте снова, "+
				"или нажмите Отмена.",
			vipReasonMaxLen,
		)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, errText, markup)
	}
	if reason == "" {
		reason = vipDefaultReason
	}
	return w.applyGrant(c, state, reason)
}

// start — точка входа. Сохраняет target в state и рисует шаг 1.
// Wrapper уже проверил наличие ReplyTo — здесь просто разыменовываем.
func (w *setVIPWizard) start(c tele.Context) error {
	msg := c.Message()
	if msg == nil || msg.ReplyTo == nil || msg.ReplyTo.Sender == nil {
		// Защита от гонок: wrapper мог пропустить, но повторно проверяем.
		return c.Send("❌ Ответьте этой командой на сообщение пользователя.")
	}
	target := msg.ReplyTo.Sender
	if target.ID == c.Bot().Me.ID {
		return c.Send("❌ Нельзя выдать VIP-статус самому боту.")
	}
	if target.IsBot {
		return c.Send("❌ Нельзя выдать VIP-статус боту.")
	}

	initialData := map[string]any{
		dataKeyTargetID:   target.ID,
		dataKeyTargetName: core.DisplayName(target),
	}

	return w.mgr.Start(c, wizardSetVIPName, initialData, func(state *State) error {
		state.Step = stepVIPConfirm
		text, markup := w.renderStep1(state)
		sent, err := c.Bot().Send(c.Chat(), text, markup, &tele.SendOptions{ParseMode: tele.ModeHTML})
		if err != nil {
			return err
		}
		w.mgr.SetMessage(state, sent)
		return nil
	})
}

// renderStep1 — подтверждение цели + выбор «без причины» / «указать причину».
func (w *setVIPWizard) renderStep1(state *State) (string, *tele.ReplyMarkup) {
	threadID, _ := state.Data[DataThreadID].(int)
	name, _ := state.Data[dataKeyTargetName].(string)
	targetID, _ := state.Data[dataKeyTargetID].(int64)

	scope := "<b>этого топика</b>"
	if threadID == 0 {
		scope = "<b>всего чата</b>"
	}

	text := fmt.Sprintf(
		"<b>👑 Выдача VIP-статуса</b>\n\n"+
			"Пользователь: %s (<code>%d</code>)\n"+
			"Область: %s\n\n"+
			"VIP игнорирует лимиты и фильтры. Указать причину выдачи?",
		html.EscapeString(name), targetID, scope,
	)

	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(
			tele.Btn{Unique: uniqueVIPGrant, Text: "✅ Без причины", Data: ""},
			tele.Btn{Unique: uniqueVIPGrant, Text: "✏ Указать причину", Data: "ask"},
		),
		markup.Row(CancelButton()),
	)
	return text, markup
}

// handleGrant обрабатывает оба варианта шага 1.
//   - data == "":    моментальная выдача с дефолтной причиной.
//   - data == "ask": переход в режим ожидания текста причины.
func (w *setVIPWizard) handleGrant(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardSetVIPName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	switch c.Callback().Data {
	case "":
		return w.applyGrant(c, state, vipDefaultReason)
	case "ask":
		state.Step = stepVIPReason
		w.mgr.AwaitText(state, stepVIPReason)
		text := fmt.Sprintf(
			"<b>✏ Введите причину выдачи VIP</b>\n\n"+
				"Максимум %d символов. Отправьте текст сообщением.\n"+
				"Команда (например /cancel) отменит wizard.",
			vipReasonMaxLen,
		)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, text, markup)
	default:
		w.logger.Warn("setvip wizard: unknown grant data", zap.String("data", c.Callback().Data))
		return nil
	}
}

// applyGrant — финальное применение GrantVIP.
func (w *setVIPWizard) applyGrant(c tele.Context, state *State, reason string) error {
	chatID := state.Key.ChatID
	threadID, _ := state.Data[DataThreadID].(int)
	targetID, _ := state.Data[dataKeyTargetID].(int64)
	targetName, _ := state.Data[dataKeyTargetName].(string)

	// FK: chat_vips.chat_id → chats(chat_id). EnsureExists повторяет
	// поведение handleSetVIP в limiter.go.
	if err := w.chatRepo.EnsureExists(chatID); err != nil {
		w.logger.Error("setvip wizard: ensure chat", zap.Error(err))
	}

	if err := w.vipRepo.GrantVIP(chatID, threadID, targetID, c.Sender().ID, reason); err != nil {
		w.logger.Error("setvip wizard: GrantVIP",
			zap.Error(err),
			zap.Int64("chat_id", chatID),
			zap.Int64("target_id", targetID))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось выдать VIP-статус.")
		return nil
	}

	_ = w.eventRepo.Log(chatID, c.Sender().ID, "limiter", "grant_vip",
		fmt.Sprintf("Granted VIP via wizard to user %d (chat=%d, thread=%d, reason: %s)",
			targetID, chatID, threadID, reason))

	scope := "этого топика"
	if threadID == 0 {
		scope = "всего чата"
	}
	finalText := fmt.Sprintf(
		"✅ VIP-статус выдан %s для %s.\nПричина: %s",
		html.EscapeString(targetName), scope, html.EscapeString(reason),
	)
	w.mgr.Complete(c, state.Key, finalText)
	return nil
}

// editToStep редактирует сообщение wizard'а на новый шаг.
func (w *setVIPWizard) editToStep(c tele.Context, state *State, text string, markup *tele.ReplyMarkup) error {
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	_, err := w.mgr.bot.Edit(editable, text, markup, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		w.logger.Warn("setvip wizard: edit failed", zap.Error(err))
	}
	return err
}
