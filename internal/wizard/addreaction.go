package wizard

import (
	"database/sql"
	"fmt"
	"html"
	"strings"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// Конвенция для addreaction wizard'а:
//   - имя wizard'а: "addreaction"
//   - 3 шага:
//     step1_pattern  — паттерн срабатывания (1..1000 символов).
//     step2_descr    — описание (1..500 символов, для /listreactions).
//     step3_response — ответ. Если на старте был ReplyTo с медиа,
//     этот шаг пропускается (response_type/content
//     берутся из reply). Иначе ввод текста (1..5000).
//   - данные в State.Data:
//     DataThreadID            — ставится Manager.Start.
//     "pattern"               — string.
//     "description"           — string.
//     "responseType"          — string (text/sticker/photo/...).
//     "responseContent"       — string (текст или file_id).
//     "fromReply"             — bool, true если response из ReplyTo media.
//
// Wizard НЕ покрывает расширенные опции legacy-команды:
//   - user:<id> (персональные реакции)
//   - cooldown (всегда 30 сек)
//   - daily_limit (всегда 0 = безлимит)
//   - delete-on-limit (всегда false)
//   - trigger_content_type (всегда NULL = любой контент)
//
// Для этих случаев используется legacy «/addreaction <args...>».
//
// Wizard использует прямой INSERT в keyword_reactions (как handleAddBan
// и как handleAddReaction в reactions.go); при изменении схемы нужно
// править оба места.
const (
	wizardAddReactionName = "addreaction"

	stepReactPattern  = "step1_pattern"
	stepReactDescr    = "step2_descr"
	stepReactResponse = "step3_response"

	dataKeyReactPattern  = "pattern"
	dataKeyReactDescr    = "description"
	dataKeyReactRespType = "responseType"
	dataKeyReactRespCont = "responseContent"
	dataKeyReactFromRpl  = "fromReply"

	uniqueReactBack = "wiz_r_back"

	reactPatternMaxLen     = 1000
	reactDescriptionMaxLen = 500
	reactResponseMaxLen    = 5000

	// Дефолты для wizard'а (расширенные опции — через legacy).
	reactDefaultCooldown   = 30
	reactDefaultDailyLimit = 0
	reactDefaultDelete     = false
)

type addReactionWizard struct {
	mgr       *Manager
	db        *sql.DB
	chatRepo  *repositories.ChatRepository
	eventRepo *repositories.EventRepository
	logger    *zap.Logger
}

// RegisterAddReaction подключает /addreaction wizard к боту.
func RegisterAddReaction(
	bot *tele.Bot,
	mgr *Manager,
	db *sql.DB,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(c tele.Context) error {
	w := &addReactionWizard{
		mgr:       mgr,
		db:        db,
		chatRepo:  chatRepo,
		eventRepo: eventRepo,
		logger:    logger,
	}

	btnBack := tele.Btn{Unique: uniqueReactBack, Text: "⬅ Назад"}
	bot.Handle(&btnBack, w.handleBack)

	mgr.RegisterTextHandler(wizardAddReactionName, w.handleText)

	return w.start
}

// start — точка входа. Если есть ReplyTo, извлекаем тип ответа.
func (w *addReactionWizard) start(c tele.Context) error {
	initialData := map[string]any{}

	if msg := c.Message(); msg != nil && msg.ReplyTo != nil {
		respType, respContent := extractTaskTypeFromReply(msg.ReplyTo)
		// Если ReplyTo пустое (text без содержимого) — игнорируем,
		// wizard всё равно спросит ответ на шаге 3.
		if !(respType == "text" && respContent == "") {
			initialData[dataKeyReactRespType] = respType
			initialData[dataKeyReactRespCont] = respContent
			initialData[dataKeyReactFromRpl] = true
		}
	}

	return w.mgr.Start(c, wizardAddReactionName, initialData, func(state *State) error {
		state.Step = stepReactPattern
		w.mgr.AwaitText(state, stepReactPattern)

		text := w.renderStep1(state)
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
func (w *addReactionWizard) scopeText(state *State) string {
	threadID, _ := state.Data[DataThreadID].(int)
	if threadID != 0 {
		return "<b>этого топика</b>"
	}
	return "<b>всего чата</b>"
}

// responseHint — тип ответа (для шагов 1-2).
func (w *addReactionWizard) responseHint(state *State) string {
	fromReply, _ := state.Data[dataKeyReactFromRpl].(bool)
	if !fromReply {
		return "Ответ: <b>текст</b> (введёте на шаге 3)"
	}
	respType, _ := state.Data[dataKeyReactRespType].(string)
	return fmt.Sprintf("Ответ: <b>%s</b> из reply-сообщения", html.EscapeString(respType))
}

// totalSteps — 2 при reply-media, 3 при текстовом ответе.
func (w *addReactionWizard) totalSteps(state *State) int {
	if fromReply, _ := state.Data[dataKeyReactFromRpl].(bool); fromReply {
		return 2
	}
	return 3
}

// renderStep1 — приглашение ввести паттерн.
func (w *addReactionWizard) renderStep1(state *State) string {
	return fmt.Sprintf(
		"<b>💬 Шаг 1/%d: паттерн</b>\n"+
			"Область: %s · %s\n\n"+
			"Отправьте слово или фразу-триггер (1..%d символов).\n"+
			"Например: <code>привет</code>",
		w.totalSteps(state), w.scopeText(state), w.responseHint(state), reactPatternMaxLen,
	)
}

// renderStep2 — приглашение ввести описание.
func (w *addReactionWizard) renderStep2(state *State) string {
	return fmt.Sprintf(
		"<b>💬 Шаг 2/%d: описание</b>\n"+
			"Отправьте короткое описание (1..%d символов) — оно показывается в /listreactions.",
		w.totalSteps(state), reactDescriptionMaxLen,
	)
}

// renderStep3 — приглашение ввести текстовый ответ.
func (w *addReactionWizard) renderStep3(state *State) string {
	return fmt.Sprintf(
		"<b>💬 Шаг 3/3: текст ответа</b>\n"+
			"Отправьте текст, которым бот будет отвечать (1..%d символов).",
		reactResponseMaxLen,
	)
}

// handleText — обработчик текстового ввода для всех шагов.
func (w *addReactionWizard) handleText(c tele.Context, state *State, text string) error {
	switch state.Step {
	case stepReactPattern:
		return w.handlePatternInput(c, state, text)
	case stepReactDescr:
		return w.handleDescrInput(c, state, text)
	case stepReactResponse:
		return w.handleResponseInput(c, state, text)
	default:
		w.mgr.Cancel(c, state.Key, "🚫 Wizard отменён (неожиданный шаг).")
		return nil
	}
}

func (w *addReactionWizard) handlePatternInput(c tele.Context, state *State, text string) error {
	pattern := strings.TrimSpace(text)
	if pattern == "" {
		return w.rejectStep(c, state, stepReactPattern, "❌ Паттерн не может быть пустым.")
	}
	if len(pattern) > reactPatternMaxLen {
		return w.rejectStep(c, state, stepReactPattern,
			fmt.Sprintf("❌ Паттерн слишком длинный (макс. %d).", reactPatternMaxLen))
	}
	state.Data[dataKeyReactPattern] = pattern
	state.Step = stepReactDescr
	w.mgr.AwaitText(state, stepReactDescr)
	return w.editToStep(c, state, w.renderStep2(state), markupReactBackCancel())
}

func (w *addReactionWizard) handleDescrInput(c tele.Context, state *State, text string) error {
	descr := strings.TrimSpace(text)
	if descr == "" {
		return w.rejectStep(c, state, stepReactDescr, "❌ Описание не может быть пустым.")
	}
	if len(descr) > reactDescriptionMaxLen {
		return w.rejectStep(c, state, stepReactDescr,
			fmt.Sprintf("❌ Описание слишком длинное (макс. %d).", reactDescriptionMaxLen))
	}
	state.Data[dataKeyReactDescr] = descr

	// Если ответ был задан reply-медиа — сразу применяем.
	if fromReply, _ := state.Data[dataKeyReactFromRpl].(bool); fromReply {
		return w.applyReaction(c, state)
	}

	state.Step = stepReactResponse
	w.mgr.AwaitText(state, stepReactResponse)
	return w.editToStep(c, state, w.renderStep3(state), markupReactBackCancel())
}

func (w *addReactionWizard) handleResponseInput(c tele.Context, state *State, text string) error {
	body := text // не TrimSpace — переносы строк могут быть осмысленными
	if strings.TrimSpace(body) == "" {
		return w.rejectStep(c, state, stepReactResponse, "❌ Текст ответа не может быть пустым.")
	}
	if len(body) > reactResponseMaxLen {
		return w.rejectStep(c, state, stepReactResponse,
			fmt.Sprintf("❌ Текст ответа слишком длинный (макс. %d).", reactResponseMaxLen))
	}
	state.Data[dataKeyReactRespType] = "text"
	state.Data[dataKeyReactRespCont] = body
	return w.applyReaction(c, state)
}

func (w *addReactionWizard) rejectStep(c tele.Context, state *State, step, errMsg string) error {
	w.mgr.AwaitText(state, step)
	var body string
	switch step {
	case stepReactPattern:
		body = errMsg + "\n\n" + w.renderStep1(state)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, body, markup)
	case stepReactDescr:
		body = errMsg + "\n\n" + w.renderStep2(state)
	case stepReactResponse:
		body = errMsg + "\n\n" + w.renderStep3(state)
	default:
		body = errMsg
	}
	return w.editToStep(c, state, body, markupReactBackCancel())
}

func markupReactBackCancel() *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(tele.Btn{Unique: uniqueReactBack, Text: "⬅ Назад"}),
		markup.Row(CancelButton()),
	)
	return markup
}

// handleBack — возврат на предыдущий шаг.
func (w *addReactionWizard) handleBack(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardAddReactionName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	switch state.Step {
	case stepReactDescr:
		// step2 → step1: чистим паттерн.
		delete(state.Data, dataKeyReactPattern)
		state.Step = stepReactPattern
		w.mgr.AwaitText(state, stepReactPattern)

		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, w.renderStep1(state), markup)

	case stepReactResponse:
		// step3 → step2: чистим описание.
		delete(state.Data, dataKeyReactDescr)
		state.Step = stepReactDescr
		w.mgr.AwaitText(state, stepReactDescr)
		return w.editToStep(c, state, w.renderStep2(state), markupReactBackCancel())

	default:
		return nil
	}
}

// applyReaction — финальный INSERT в keyword_reactions.
//
// Schema: keyword_reactions(chat_id, thread_id, user_id, pattern, response_type,
//
//	response_content, description, is_regex, trigger_content_type, cooldown,
//	daily_limit, delete_on_limit, is_active).
//
// Wizard всегда вставляет:
//   - user_id = NULL (общая реакция);
//   - is_regex = false;
//   - trigger_content_type = NULL (любой контент);
//   - cooldown = 30, daily_limit = 0, delete_on_limit = false, is_active = true.
//
// Дублирует INSERT из reactions.handleAddReaction; при изменении схемы
// синхронизировать оба места.
func (w *addReactionWizard) applyReaction(c tele.Context, state *State) error {
	chatID := state.Key.ChatID
	threadID, _ := state.Data[DataThreadID].(int)
	pattern, _ := state.Data[dataKeyReactPattern].(string)
	descr, _ := state.Data[dataKeyReactDescr].(string)
	respType, _ := state.Data[dataKeyReactRespType].(string)
	respContent, _ := state.Data[dataKeyReactRespCont].(string)

	if err := w.chatRepo.EnsureExists(chatID); err != nil {
		w.logger.Error("addreaction wizard: ensure chat", zap.Error(err))
	}

	_, err := w.db.Exec(`
		INSERT INTO keyword_reactions
			(chat_id, thread_id, user_id, pattern, response_type, response_content,
			 description, is_regex, trigger_content_type, cooldown, daily_limit,
			 delete_on_limit, is_active)
		VALUES ($1, $2, NULL, $3, $4, $5, $6, false, NULL, $7, $8, $9, true)
	`, chatID, threadID, pattern, respType, respContent, descr,
		reactDefaultCooldown, reactDefaultDailyLimit, reactDefaultDelete)
	if err != nil {
		w.logger.Error("addreaction wizard: insert",
			zap.Error(err), zap.Int64("chat_id", chatID), zap.String("pattern", pattern))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось добавить реакцию в БД.")
		return nil
	}

	_ = w.eventRepo.Log(chatID, c.Sender().ID, "reactions", "add_reaction",
		fmt.Sprintf("Added reaction via wizard: pattern=%q, type=%s, thread=%d",
			pattern, respType, threadID))

	finalText := fmt.Sprintf(
		"✅ Реакция добавлена для %s.\n\n"+
			"Паттерн: <code>%s</code>\n"+
			"Описание: <code>%s</code>\n"+
			"Тип ответа: %s\n"+
			"Кулдаун: %d сек (по умолчанию)",
		w.scopeText(state),
		html.EscapeString(pattern), html.EscapeString(descr),
		html.EscapeString(respType), reactDefaultCooldown,
	)
	w.mgr.Complete(c, state.Key, finalText)
	return nil
}

// editToStep редактирует сообщение wizard'а на новый шаг.
func (w *addReactionWizard) editToStep(c tele.Context, state *State, text string, markup *tele.ReplyMarkup) error {
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	_, err := w.mgr.bot.Edit(editable, text, &tele.SendOptions{ParseMode: tele.ModeHTML, ReplyMarkup: markup})
	if err != nil {
		w.logger.Warn("addreaction wizard: edit failed", zap.Error(err))
	}
	return err
}
