package wizard

import (
	"fmt"
	"html"
	"strings"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// Конвенция для addtask wizard'а:
//   - имя wizard'а: "addtask"
//   - 3 шага:
//     step1_name    — ожидание текста имени задачи (1..200 символов).
//     step2_cron    — пресеты cron + ✏ Свой → ожидание текста cron-выражения.
//     Валидация через cron.ParseStandard (стандартный 5-полевой формат).
//     step3_text    — только если на старте НЕ было ReplyTo с медиа: ввод текста (1..4000 символов).
//     Если было ReplyTo с медиа — этот шаг пропускается, применяем сразу.
//   - данные в State.Data:
//     DataThreadID  — ставится Manager.Start.
//     "name"        — string, валидированное имя.
//     "cronExpr"    — string, валидированное cron-выражение.
//     "taskType"    — string, тип контента (text/sticker/photo/...).
//     "taskData"    — string, текст или file_id.
//     "fromReply"   — bool, true если type/data взяты из ReplyTo.
//   - unique кнопок: "wiz_t_cron" (data=preset|"custom"), "wiz_t_back".
//
// Поведение со стартовым ReplyTo:
//   - Если ReplyTo содержит медиа (sticker/photo/animation/video/voice/document/audio),
//     извлекаем (type, file_id) и сохраняем в state, fromReply=true. Шаг step3_text
//     пропускается.
//   - Если ReplyTo пустое или содержит только текст — wizard собирает всё через шаги
//     (для текстовой задачи это естественно; для медиа — в legacy ничего бы и так
//     не получилось).
//
// Дублирование SQL: отсутствует — используется SchedulerRepository.CreateTask
// (тот же что и в handleAddTask). Регистрация в cron делается через
// SchedulerModule.RegisterTask (публичная обёртка).
const (
	wizardAddTaskName = "addtask"

	stepTaskName = "step1_name"
	stepTaskCron = "step2_cron"
	stepTaskText = "step3_text"

	uniqueTaskCron = "wiz_t_cron"
	uniqueTaskBack = "wiz_t_back"

	dataKeyTaskName = "name"
	dataKeyCronExpr = "cronExpr"
	dataKeyTaskType = "taskType"
	dataKeyTaskData = "taskData"
	dataKeyFromRpl  = "fromReply"

	taskNameMaxLen = 200
	taskTextMaxLen = 4000
	cronExprMaxLen = 100
)

// cronPresets — короткие пресеты cron. Хранится индекс (0..N-1) в Data
// callback'а (≤64 байт), а не сама строка, чтобы избежать любых проблем
// с экранированием пробелов.
var cronPresets = []struct {
	expr  string
	label string
}{
	{"0 9 * * *", "Каждый день 9:00"},
	{"0 12 * * *", "Каждый день 12:00"},
	{"0 9 * * 1", "Понедельник 9:00"},
	{"0 18 * * 5", "Пятница 18:00"},
	{"0 * * * *", "Каждый час"},
	{"0 9 1 * *", "1-е число 9:00"},
}

// taskRegistrar — минимальный интерфейс к SchedulerModule, нужный wizard'у:
// зарегистрировать только что созданную в БД задачу в cron-планировщике.
// Реализуется методом SchedulerModule.RegisterTaskByID.
type taskRegistrar interface {
	RegisterTaskByID(taskID int64) error
}

type addTaskWizard struct {
	mgr           *Manager
	schedulerRepo *repositories.SchedulerRepository
	chatRepo      *repositories.ChatRepository
	eventRepo     *repositories.EventRepository
	registrar     taskRegistrar
	logger        *zap.Logger
}

// RegisterAddTask подключает /addtask wizard к боту.
func RegisterAddTask(
	bot *tele.Bot,
	mgr *Manager,
	schedulerRepo *repositories.SchedulerRepository,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	registrar taskRegistrar,
	logger *zap.Logger,
) func(c tele.Context) error {
	w := &addTaskWizard{
		mgr:           mgr,
		schedulerRepo: schedulerRepo,
		chatRepo:      chatRepo,
		eventRepo:     eventRepo,
		registrar:     registrar,
		logger:        logger,
	}

	btnCron := tele.Btn{Unique: uniqueTaskCron}
	bot.Handle(&btnCron, w.handleCronChoice)

	btnBack := tele.Btn{Unique: uniqueTaskBack, Text: "⬅ Назад"}
	bot.Handle(&btnBack, w.handleBack)

	mgr.RegisterTextHandler(wizardAddTaskName, w.handleText)

	return w.start
}

// start — точка входа. Если есть ReplyTo с медиа, извлекаем тип/file_id.
func (w *addTaskWizard) start(c tele.Context) error {
	initialData := map[string]any{}

	if msg := c.Message(); msg != nil && msg.ReplyTo != nil {
		taskType, taskData := extractTaskTypeFromReply(msg.ReplyTo)
		// taskType=="text" с пустым taskData нам не нужен — это эквивалентно
		// «ReplyTo без полезной нагрузки», такой случай обрабатываем как
		// обычный wizard без media (соберём text через step3_text).
		if !(taskType == "text" && taskData == "") {
			initialData[dataKeyTaskType] = taskType
			initialData[dataKeyTaskData] = taskData
			initialData[dataKeyFromRpl] = true
		}
	}

	return w.mgr.Start(c, wizardAddTaskName, initialData, func(state *State) error {
		state.Step = stepTaskName
		w.mgr.AwaitText(state, stepTaskName)

		text := w.renderStep1Text(state)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))

		sent, err := c.Bot().Send(c.Chat(), text, markup, &tele.SendOptions{ParseMode: tele.ModeHTML})
		if err != nil {
			return err
		}
		w.mgr.SetMessage(state, sent)
		return nil
	})
}

// extractTaskTypeFromReply — копия логики из scheduler.extractTaskTypeFromReply.
// Дублируем, чтобы wizard package не зависел от модуля scheduler. Списки типов
// синхронизированы (sticker/photo/animation/video/voice/document/audio + text).
func extractTaskTypeFromReply(replyMsg *tele.Message) (string, string) {
	switch {
	case replyMsg.Sticker != nil:
		return "sticker", replyMsg.Sticker.FileID
	case replyMsg.Photo != nil:
		return "photo", replyMsg.Photo.FileID
	case replyMsg.Animation != nil:
		return "animation", replyMsg.Animation.FileID
	case replyMsg.Video != nil:
		return "video", replyMsg.Video.FileID
	case replyMsg.Voice != nil:
		return "voice", replyMsg.Voice.FileID
	case replyMsg.Document != nil:
		return "document", replyMsg.Document.FileID
	case replyMsg.Audio != nil:
		return "audio", replyMsg.Audio.FileID
	default:
		return "text", replyMsg.Text
	}
}

// scopeText — область действия (чат vs топик).
func (w *addTaskWizard) scopeText(state *State) string {
	threadID, _ := state.Data[DataThreadID].(int)
	if threadID != 0 {
		return "<b>этого топика</b>"
	}
	return "<b>всего чата</b>"
}

// contentHint — подсказка о типе контента (для шагов 1-2).
func (w *addTaskWizard) contentHint(state *State) string {
	taskType, _ := state.Data[dataKeyTaskType].(string)
	fromReply, _ := state.Data[dataKeyFromRpl].(bool)
	if !fromReply {
		return "Контент: <b>текстовое сообщение</b> (введёте на шаге 3)"
	}
	return fmt.Sprintf("Контент: <b>%s</b> из reply-сообщения", html.EscapeString(taskType))
}

// renderStep1Text — приглашение ввести имя задачи.
func (w *addTaskWizard) renderStep1Text(state *State) string {
	return fmt.Sprintf(
		"<b>📅 Создание задачи планировщика</b>\n\n"+
			"Область: %s\n"+
			"%s\n\n"+
			"Шаг 1/%d: отправьте <b>имя задачи</b> (1..%d символов).\n"+
			"Например: <code>morning_greeting</code>",
		w.scopeText(state), w.contentHint(state), w.totalSteps(state), taskNameMaxLen,
	)
}

// totalSteps — 2 при reply-media, 3 при текстовой задаче.
func (w *addTaskWizard) totalSteps(state *State) int {
	if fromReply, _ := state.Data[dataKeyFromRpl].(bool); fromReply {
		return 2
	}
	return 3
}

// renderStep2 — выбор cron (пресеты + ✏ Свой).
func (w *addTaskWizard) renderStep2(state *State) (string, *tele.ReplyMarkup) {
	text := fmt.Sprintf(
		"<b>📅 Шаг 2/%d: расписание</b>\n"+
			"Выберите пресет или введите своё cron-выражение.",
		w.totalSteps(state),
	)

	markup := &tele.ReplyMarkup{}
	rows := make([]tele.Row, 0, len(cronPresets)/2+2)
	row := make([]tele.Btn, 0, 2)
	for i, p := range cronPresets {
		row = append(row, tele.Btn{
			Unique: uniqueTaskCron,
			Text:   p.label,
			Data:   fmt.Sprintf("p%d", i), // индекс — не строка, чтобы влезть в 64 байт
		})
		if (i+1)%2 == 0 {
			rows = append(rows, markup.Row(row...))
			row = row[:0]
		}
	}
	if len(row) > 0 {
		rows = append(rows, markup.Row(row...))
	}
	rows = append(rows,
		markup.Row(tele.Btn{Unique: uniqueTaskCron, Text: "✏ Свой cron", Data: "custom"}),
		markup.Row(tele.Btn{Unique: uniqueTaskBack, Text: "⬅ Назад"}, CancelButton()),
	)
	markup.Inline(rows...)
	return text, markup
}

// renderStep3Text — приглашение ввести текст сообщения.
func (w *addTaskWizard) renderStep3Text(state *State) string {
	return fmt.Sprintf(
		"<b>📅 Шаг 3/3: текст</b>\n"+
			"Отправьте <b>текст сообщения</b> (1..%d символов).",
		taskTextMaxLen,
	)
}

// handleText — обработчик текстового ввода для всех шагов.
func (w *addTaskWizard) handleText(c tele.Context, state *State, text string) error {
	switch state.Step {
	case stepTaskName:
		return w.handleNameInput(c, state, text)
	case stepTaskCron:
		return w.handleCronInput(c, state, text)
	case stepTaskText:
		return w.handleTextInput(c, state, text)
	default:
		w.mgr.Cancel(c, state.Key, "🚫 Wizard отменён (неожиданный шаг).")
		return nil
	}
}

// handleNameInput — приём имени задачи на шаге 1.
func (w *addTaskWizard) handleNameInput(c tele.Context, state *State, text string) error {
	name := strings.TrimSpace(text)
	if name == "" {
		return w.rejectStep1(c, state, "❌ Имя задачи не может быть пустым.")
	}
	if len(name) > taskNameMaxLen {
		return w.rejectStep1(c, state,
			fmt.Sprintf("❌ Имя слишком длинное (макс. %d символов).", taskNameMaxLen))
	}
	// Запретим пробелы внутри имени — оно используется в /deltask и в логах.
	// Это совпадает с фактическим парсером legacy parseAddTaskArgs: name = первое слово.
	if strings.ContainsAny(name, " \t\n") {
		return w.rejectStep1(c, state, "❌ Имя задачи не должно содержать пробелов.")
	}

	state.Data[dataKeyTaskName] = name
	state.Step = stepTaskCron
	state.AwaitText = false

	text2, markup := w.renderStep2(state)
	return w.editToStep(c, state, text2, markup)
}

func (w *addTaskWizard) rejectStep1(c tele.Context, state *State, errMsg string) error {
	w.mgr.AwaitText(state, stepTaskName)
	text := errMsg + "\n\n" + w.renderStep1Text(state)
	markup := &tele.ReplyMarkup{}
	markup.Inline(markup.Row(CancelButton()))
	return w.editToStep(c, state, text, markup)
}

// handleCronChoice — клик по кнопке пресета или «✏ Свой».
func (w *addTaskWizard) handleCronChoice(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardAddTaskName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	data := c.Callback().Data
	if data == "custom" {
		w.mgr.AwaitText(state, stepTaskCron)
		text := fmt.Sprintf(
			"<b>✏ Введите cron-выражение</b>\n\n" +
				"Стандартный 5-полевой формат: <code>минута час день месяц день_недели</code>\n" +
				"Примеры:\n" +
				"<code>0 9 * * *</code> — ежедневно в 9:00\n" +
				"<code>*/30 * * * *</code> — каждые 30 минут\n" +
				"<code>0 9 * * 1-5</code> — по будням в 9:00\n\n" +
				"Команда (например /cancel) отменит wizard.",
		)
		markup := &tele.ReplyMarkup{}
		markup.Inline(
			markup.Row(tele.Btn{Unique: uniqueTaskBack, Text: "⬅ Назад"}),
			markup.Row(CancelButton()),
		)
		return w.editToStep(c, state, text, markup)
	}

	// Пресет: data вида "pN".
	if !strings.HasPrefix(data, "p") {
		w.logger.Warn("addtask wizard: unknown cron data", zap.String("data", data))
		return nil
	}
	idx := 0
	if _, err := fmt.Sscanf(data[1:], "%d", &idx); err != nil || idx < 0 || idx >= len(cronPresets) {
		w.logger.Warn("addtask wizard: bad preset index", zap.String("data", data))
		return nil
	}
	state.Data[dataKeyCronExpr] = cronPresets[idx].expr
	return w.advanceAfterCron(c, state)
}

// handleCronInput — приём произвольного cron-выражения.
func (w *addTaskWizard) handleCronInput(c tele.Context, state *State, text string) error {
	expr := strings.TrimSpace(text)
	if expr == "" || len(expr) > cronExprMaxLen {
		return w.rejectCron(c, state, "❌ Пустое или слишком длинное cron-выражение.")
	}
	if _, err := cron.ParseStandard(expr); err != nil {
		return w.rejectCron(c, state, fmt.Sprintf("❌ Неверное cron-выражение: %v", err))
	}
	state.Data[dataKeyCronExpr] = expr
	return w.advanceAfterCron(c, state)
}

func (w *addTaskWizard) rejectCron(c tele.Context, state *State, errMsg string) error {
	w.mgr.AwaitText(state, stepTaskCron)
	text := errMsg + "\n\nОтправьте корректное cron-выражение или нажмите Назад."
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(tele.Btn{Unique: uniqueTaskBack, Text: "⬅ Назад"}),
		markup.Row(CancelButton()),
	)
	return w.editToStep(c, state, text, markup)
}

// advanceAfterCron — после сохранения cronExpr решаем: переходим к step3_text
// (если контент не из reply) или сразу применяем (если есть reply-media).
func (w *addTaskWizard) advanceAfterCron(c tele.Context, state *State) error {
	state.AwaitText = false
	if fromReply, _ := state.Data[dataKeyFromRpl].(bool); fromReply {
		return w.applyTask(c, state)
	}
	state.Step = stepTaskText
	w.mgr.AwaitText(state, stepTaskText)

	text := w.renderStep3Text(state)
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(tele.Btn{Unique: uniqueTaskBack, Text: "⬅ Назад"}),
		markup.Row(CancelButton()),
	)
	return w.editToStep(c, state, text, markup)
}

// handleTextInput — приём текста сообщения на шаге 3.
func (w *addTaskWizard) handleTextInput(c tele.Context, state *State, text string) error {
	body := text // не TrimSpace: текст может намеренно содержать переносы строк
	if strings.TrimSpace(body) == "" {
		w.mgr.AwaitText(state, stepTaskText)
		return w.editToStep(c, state,
			"❌ Текст не может быть пустым.\n\n"+w.renderStep3Text(state),
			markupBackCancel())
	}
	if len(body) > taskTextMaxLen {
		w.mgr.AwaitText(state, stepTaskText)
		return w.editToStep(c, state,
			fmt.Sprintf("❌ Текст слишком длинный (макс. %d символов).\n\n", taskTextMaxLen)+
				w.renderStep3Text(state),
			markupBackCancel())
	}
	state.Data[dataKeyTaskType] = "text"
	state.Data[dataKeyTaskData] = body
	return w.applyTask(c, state)
}

// markupBackCancel — общая клавиатура для шагов с back+cancel.
func markupBackCancel() *tele.ReplyMarkup {
	markup := &tele.ReplyMarkup{}
	markup.Inline(
		markup.Row(tele.Btn{Unique: uniqueTaskBack, Text: "⬅ Назад"}),
		markup.Row(CancelButton()),
	)
	return markup
}

// handleBack — возврат на предыдущий шаг.
func (w *addTaskWizard) handleBack(c tele.Context) error {
	state, err := w.mgr.Guard(c, wizardAddTaskName)
	if err != nil {
		return nil
	}
	_ = c.Respond()

	switch state.Step {
	case stepTaskCron:
		// step2 → step1: чистим имя.
		delete(state.Data, dataKeyTaskName)
		state.Step = stepTaskName
		w.mgr.AwaitText(state, stepTaskName)

		text := w.renderStep1Text(state)
		markup := &tele.ReplyMarkup{}
		markup.Inline(markup.Row(CancelButton()))
		return w.editToStep(c, state, text, markup)

	case stepTaskText:
		// step3 → step2: чистим cron.
		delete(state.Data, dataKeyCronExpr)
		state.Step = stepTaskCron
		state.AwaitText = false

		text, markup := w.renderStep2(state)
		return w.editToStep(c, state, text, markup)

	default:
		return nil
	}
}

// applyTask — финальное создание задачи в БД и регистрация в cron.
func (w *addTaskWizard) applyTask(c tele.Context, state *State) error {
	chatID := state.Key.ChatID
	threadID, _ := state.Data[DataThreadID].(int)
	name, _ := state.Data[dataKeyTaskName].(string)
	cronExpr, _ := state.Data[dataKeyCronExpr].(string)
	taskType, _ := state.Data[dataKeyTaskType].(string)
	taskData, _ := state.Data[dataKeyTaskData].(string)

	if err := w.chatRepo.EnsureExists(chatID); err != nil {
		w.logger.Error("addtask wizard: ensure chat", zap.Error(err))
	}

	taskID, err := w.schedulerRepo.CreateTask(chatID, threadID, name, cronExpr, taskType, taskData)
	if err != nil {
		w.logger.Error("addtask wizard: CreateTask",
			zap.Error(err),
			zap.Int64("chat_id", chatID),
			zap.String("name", name))
		w.mgr.Cancel(c, state.Key, "❌ Не удалось создать задачу в БД.")
		return nil
	}

	if err := w.registrar.RegisterTaskByID(taskID); err != nil {
		w.logger.Error("addtask wizard: RegisterTaskByID",
			zap.Error(err),
			zap.Int64("task_id", taskID))
		// Задача в БД есть, но в cron не зарегистрирована — сообщаем пользователю.
		w.mgr.Cancel(c, state.Key,
			fmt.Sprintf("⚠️ Задача %d создана в БД, но не зарегистрирована в планировщике. Перезапустите бот.", taskID))
		return nil
	}

	_ = w.eventRepo.Log(chatID, c.Sender().ID, "scheduler", "task_created",
		fmt.Sprintf("Task %s created via wizard (id=%d)", name, taskID))

	finalText := fmt.Sprintf(
		"✅ Задача создана для %s.\n\n"+
			"ID: <code>%d</code>\n"+
			"Имя: <code>%s</code>\n"+
			"Расписание: <code>%s</code>\n"+
			"Тип: %s",
		w.scopeText(state), taskID,
		html.EscapeString(name), html.EscapeString(cronExpr), html.EscapeString(taskType),
	)
	w.mgr.Complete(c, state.Key, finalText)
	return nil
}

// editToStep редактирует сообщение wizard'а на новый шаг.
func (w *addTaskWizard) editToStep(c tele.Context, state *State, text string, markup *tele.ReplyMarkup) error {
	editable := &tele.Message{
		ID:   state.MessageID,
		Chat: &tele.Chat{ID: state.Key.ChatID},
	}
	_, err := w.mgr.bot.Edit(editable, text, markup, &tele.SendOptions{ParseMode: tele.ModeHTML})
	if err != nil {
		w.logger.Warn("addtask wizard: edit failed", zap.Error(err))
	}
	return err
}
