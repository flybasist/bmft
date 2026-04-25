// Package scheduler — планировщик задач по cron-расписанию.
package scheduler

import (
	"database/sql"
	"fmt"
	"html"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// SchedulerModule реализует модуль планировщика задач.
type SchedulerModule struct {
	db            *sql.DB
	bot           *tele.Bot
	logger        *zap.Logger
	schedulerRepo *repositories.SchedulerRepository
	eventRepo     *repositories.EventRepository
	chatRepo      *repositories.ChatRepository
	cron          *cron.Cron
	taskEntries   map[int64]cron.EntryID // task DB ID → cron entry ID
	mu            sync.Mutex             // защита taskEntries
}

// New создаёт новый инстанс модуля планировщика.
func New(db *sql.DB, schedulerRepo *repositories.SchedulerRepository, eventRepo *repositories.EventRepository, chatRepo *repositories.ChatRepository, logger *zap.Logger, bot *tele.Bot) *SchedulerModule {
	m := &SchedulerModule{
		db:            db,
		schedulerRepo: schedulerRepo,
		eventRepo:     eventRepo,
		chatRepo:      chatRepo,
		logger:        logger,
		bot:           bot,
		cron:          cron.New(),
		taskEntries:   make(map[int64]cron.EntryID),
	}

	logger.Info("scheduler module created")
	return m
}

// Start запускает планировщик задач.
// Явный метод для управления жизненным циклом.
// Загружает активные задачи из БД и запускает cron scheduler.
func (m *SchedulerModule) Start() error {
	m.logger.Info("starting scheduler module")

	// Загружаем активные задачи из БД
	if err := m.loadActiveTasks(); err != nil {
		m.logger.Error("failed to load active tasks", zap.Error(err))
		return fmt.Errorf("failed to load active tasks: %w", err)
	}

	// Запускаем cron scheduler
	m.cron.Start()
	m.logger.Info("cron scheduler started successfully")

	return nil
}

// Shutdown выполняет graceful shutdown модуля.
func (m *SchedulerModule) Shutdown() error {
	m.logger.Info("shutting down scheduler module")
	ctx := m.cron.Stop()
	<-ctx.Done()
	m.logger.Info("cron scheduler stopped")
	return nil
}

func (m *SchedulerModule) RegisterCommands(bot *tele.Bot) {
	// /scheduler — справка по модулю
	bot.Handle("/scheduler", func(c tele.Context) error {
		msg := "⏰ <b>Модуль Scheduler</b> — Запланированные задачи\n\n"
		msg += "Автоматическая отправка сообщений по расписанию (cron).\n\n"
		msg += "<b>Доступные команды:</b>\n\n"

		msg += "🔹 <code>/addtask</code> — Добавить задачу (только админы)\n\n"

		msg += "<b>Способ 1 - Текстовое сообщение:</b>\n"
		msg += "<code>/addtask &lt;имя&gt; \"&lt;cron&gt;\" text \"&lt;текст&gt;\"</code>\n"
		msg += "📌 Пример:\n"
		msg += "<code>/addtask утро \"0 9 * * *\" text \"Доброе утро!\"</code>\n\n"

		msg += "<b>Способ 2 - Медиа (стикер/фото/гифка):</b>\n"
		msg += "Ответьте на стикер/фото/гифку и напишите:\n"
		msg += "<code>/addtask &lt;имя&gt; \"&lt;cron&gt;\"</code>\n"
		msg += "📌 Пример:\n"
		msg += "<code>/addtask стикер \"0 9 * * 1\"</code> (reply на стикер)\n\n"

		msg += "🔹 <code>/listtasks</code> — Список всех активных задач (только админы)\n\n"

		msg += "🔹 <code>/deltask &lt;ID&gt;</code> — Удалить задачу (только админы)\n"
		msg += "   📌 Пример: <code>/deltask 3</code>\n\n"

		msg += "🔹 <code>/runtask &lt;ID&gt;</code> — Запустить задачу немедленно (только админы)\n"
		msg += "   📌 Пример: <code>/runtask 3</code>\n\n"

		msg += "📅 <b>Формат cron:</b> минута час день месяц день_недели\n"
		msg += "• <code>0 9 * * *</code> — каждый день в 9:00\n"
		msg += "• <code>0 */6 * * *</code> — каждые 6 часов\n"
		msg += "• <code>0 9 * * 1</code> — каждый понедельник в 9:00\n"
		msg += "• <code>0 0 1 * *</code> — 1-го числа каждого месяца в 00:00\n\n"

		msg += "<b>⏰ ВРЕМЯ СЕРВЕРА (Europe/Moscow, UTC+3):</b>\n"
		msg += "⚠️ Время указывается по московскому времени (MSK):\n"
		msg += "• Москва: 9:00 MSK = <code>0 9 * * *</code>\n"
		msg += "• Алматы (UTC+5): 9:00 ALMT = <code>0 7 * * *</code> (7:00 MSK)\n"
		msg += "• Владивосток (UTC+10): 9:00 VLAT = <code>0 2 * * *</code> (2:00 MSK)\n\n"

		msg += "⚙️ <b>Работа с топиками:</b>\n"
		msg += "• Команда в топике → задача отправляется только в этот топик\n"
		msg += "• Команда в основном чате → задача для всего чата\n\n"

		msg += "💡 <i>Подсказки:</i>\n"
		msg += "• Проверяйте cron на сайте <b>crontab.guru</b>\n"
		msg += "• Конвертер времени: <b>worldtimebuddy.com</b>"

		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
	})
}

func (m *SchedulerModule) RegisterAdminCommands(bot *tele.Bot) {
	bot.Handle("/listtasks", m.handleListTasks)
	bot.Handle("/addtask", m.handleAddTask)
	bot.Handle("/deltask", m.handleDeleteTask)
	bot.Handle("/runtask", m.handleRunTask)
}

// HandleAddTask — публичная обёртка над handleAddTask для wizard-фолбэка
// (cmd/bot/wizards.go::wrapAddTaskWithWizard). Используется legacy-путём
// при наличии аргументов или другого неподдерживаемого wizard'ом контекста.
func (m *SchedulerModule) HandleAddTask(c tele.Context) error {
	return m.handleAddTask(c)
}

// RegisterTaskByID загружает задачу из БД по ID и регистрирует её в cron.
// Используется wizard'ом /addtask после успешного CreateTask для активации
// только что созданной задачи без перезапуска бота.
//
// Дублирует первые 5 строк handleAddTask::createAndRegisterTask, но возвращает
// ошибку (а не отправляет ответ в чат) — wizard сам формирует сообщение.
func (m *SchedulerModule) RegisterTaskByID(taskID int64) error {
	task, err := m.schedulerRepo.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("failed to get task %d: %w", taskID, err)
	}
	return m.registerTask(task)
}

// UniqueDeleteTask — Unique значение inline-кнопки 🗑 для /listtasks.
// cmd/bot/main.go регистрирует callback с обёрткой core.AdminOnlyCallback
// (без неё любой пользователь смог бы нажать кнопку и удалить задачу).
const UniqueDeleteTask = "del_task"

// listTasksMaxButtons — максимум inline-кнопок 🗑 в одном сообщении /listtasks.
// Telegram допускает до 100 кнопок на сообщение; берём с большим запасом.
// При превышении кнопки не добавляются, пользователь использует /deltask <id>.
const listTasksMaxButtons = 50

// HandleDeleteTaskCallback обрабатывает нажатие inline-кнопки 🗑 в /listtasks.
// Data callback'а — taskID (int64) в виде строки.
//
// Безопасность:
//   - admin-проверка делается в AdminOnlyCallback на уровне регистрации;
//   - дополнительно проверяем что task.ChatID == c.Chat().ID (защита от
//     callback'а из чужого чата при пересылке сообщений).
//
// Поведение: удаляет задачу из БД и cron, отвечает alert'ом «✅ Удалено».
// Сообщение со списком НЕ перерисовывается — повторное нажатие вернёт
// «Не найдено».
func (m *SchedulerModule) HandleDeleteTaskCallback(c tele.Context) error {
	cb := c.Callback()
	if cb == nil {
		return nil
	}
	taskID, err := strconv.ParseInt(strings.TrimSpace(cb.Data), 10, 64)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "❌ Некорректный ID", ShowAlert: true})
	}

	task, err := m.schedulerRepo.GetTask(taskID)
	if err != nil {
		return c.Respond(&tele.CallbackResponse{Text: "ℹ️ Задача не найдена", ShowAlert: true})
	}
	if task.ChatID != c.Chat().ID {
		// Защита от ситуации с пересланным сообщением: задача чужого чата.
		return c.Respond(&tele.CallbackResponse{Text: "🚫 Задача из другого чата", ShowAlert: true})
	}

	if err := m.schedulerRepo.DeleteTask(taskID); err != nil {
		m.logger.Error("delete task callback: db delete", zap.Error(err), zap.Int64("task_id", taskID))
		return c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка БД", ShowAlert: true})
	}

	m.mu.Lock()
	if entryID, ok := m.taskEntries[taskID]; ok {
		m.cron.Remove(entryID)
		delete(m.taskEntries, taskID)
	}
	m.mu.Unlock()

	_ = m.eventRepo.Log(c.Chat().ID, c.Sender().ID, "scheduler", "task_deleted",
		fmt.Sprintf("Task %d deleted via inline button", taskID))

	return c.Respond(&tele.CallbackResponse{
		Text:      fmt.Sprintf("✅ Задача %d удалена", taskID),
		ShowAlert: true,
	})
}

func (m *SchedulerModule) loadActiveTasks() error {
	tasks, err := m.schedulerRepo.GetActiveTasks()
	if err != nil {
		return err
	}

	m.logger.Info("loading active tasks", zap.Int("count", len(tasks)))

	for _, task := range tasks {
		if err := m.registerTask(task); err != nil {
			m.logger.Error("failed to register task",
				zap.Int64("task_id", task.ID),
				zap.Error(err))
			continue
		}
	}

	return nil
}

func (m *SchedulerModule) registerTask(task *repositories.ScheduledTask) error {
	entryID, err := m.cron.AddFunc(task.CronExpr, func() {
		m.executeTask(task)
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	// Сохраняем связь task DB ID → cron entry ID для возможности удаления
	m.mu.Lock()
	m.taskEntries[task.ID] = entryID
	m.mu.Unlock()

	m.logger.Info("registered cron task",
		zap.Int64("task_id", task.ID),
		zap.Int("cron_entry_id", int(entryID)),
		zap.String("cron_expr", task.CronExpr),
		zap.String("task_name", task.TaskName),
	)

	return nil
}

func (m *SchedulerModule) executeTask(task *repositories.ScheduledTask) {
	m.logger.Info("executing scheduled task",
		zap.Int64("task_id", task.ID),
		zap.Int64("chat_id", task.ChatID),
		zap.String("task_type", task.TaskType),
	)

	chat := &tele.Chat{ID: task.ChatID}

	// Создаем опции для отправки в топик если нужно
	sendOpts := &tele.SendOptions{}
	if task.ThreadID != 0 {
		sendOpts.ThreadID = int(task.ThreadID)
	}

	switch task.TaskType {
	case "sticker":
		sticker := &tele.Sticker{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, sticker, sendOpts); err != nil {
			m.logger.Error("failed to send sticker", zap.Error(err))
			return
		}

	case "text":
		if _, err := m.bot.Send(chat, task.TaskData, sendOpts); err != nil {
			m.logger.Error("failed to send text", zap.Error(err))
			return
		}

	case "photo":
		photo := &tele.Photo{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, photo, sendOpts); err != nil {
			m.logger.Error("failed to send photo", zap.Error(err))
			return
		}

	case "animation":
		animation := &tele.Animation{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, animation, sendOpts); err != nil {
			m.logger.Error("failed to send animation", zap.Error(err))
			return
		}

	case "video":
		video := &tele.Video{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, video, sendOpts); err != nil {
			m.logger.Error("failed to send video", zap.Error(err))
			return
		}

	case "voice":
		voice := &tele.Voice{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, voice, sendOpts); err != nil {
			m.logger.Error("failed to send voice", zap.Error(err))
			return
		}

	case "document":
		document := &tele.Document{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, document, sendOpts); err != nil {
			m.logger.Error("failed to send document", zap.Error(err))
			return
		}

	case "audio":
		audio := &tele.Audio{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, audio, sendOpts); err != nil {
			m.logger.Error("failed to send audio", zap.Error(err))
			return
		}

	default:
		m.logger.Error("unknown task type", zap.String("task_type", task.TaskType))
		return
	}

	if err := m.schedulerRepo.UpdateLastRun(task.ID, time.Now()); err != nil {
		m.logger.Error("failed to update last run", zap.Error(err))
	}

	_ = m.eventRepo.Log(task.ChatID, 0, "scheduler", "task_executed",
		fmt.Sprintf("Task %s executed", task.TaskName))
}

func (m *SchedulerModule) handleListTasks(c tele.Context) error {
	m.logger.Info("handleListTasks called", zap.Int64("chat_id", c.Chat().ID), zap.Int64("user_id", c.Sender().ID))

	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "scheduler", "list_tasks",
		fmt.Sprintf("Admin viewed tasks list (chat=%d, thread=%d)", chatID, threadID))

	tasks, err := m.schedulerRepo.GetChatTasks(chatID, threadID)
	if err != nil {
		m.logger.Error("failed to get chat tasks", zap.Error(err))
		return c.Send("❌ Ошибка при получении списка задач")
	}

	if len(tasks) == 0 {
		if threadID != 0 {
			return c.Send("📋 Нет задач планировщика для этого топика\n\nИспользуйте /addtask для создания новой задачи")
		}
		return c.Send("📋 Нет задач планировщика для всего чата\n\nИспользуйте /addtask для создания новой задачи")
	}

	var msg strings.Builder
	if threadID != 0 {
		msg.WriteString("📋 <b>Задачи планировщика (для этого топика):</b>\n\n")
	} else {
		msg.WriteString("📋 <b>Задачи планировщика (для всего чата):</b>\n\n")
	}

	for i, task := range tasks {
		status := "✅"
		if !task.IsActive {
			status = "⏸️"
		}

		msg.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, status, html.EscapeString(task.TaskName)))
		msg.WriteString(fmt.Sprintf("   ID: %d\n", task.ID))
		msg.WriteString(fmt.Sprintf("   Расписание: <code>%s</code>\n", html.EscapeString(task.CronExpr)))
		msg.WriteString(fmt.Sprintf("   Тип: %s\n", task.TaskType))

		if task.LastRun != nil {
			msg.WriteString(fmt.Sprintf("   Последний запуск: %s\n", task.LastRun.Format(core.DateTimeFormat)))
		}
		msg.WriteString("\n")
	}

	msg.WriteString("━━━━━━━━━━━━━━━\n")
	msg.WriteString("Команды:\n")
	msg.WriteString("/addtask - добавить задачу\n")
	msg.WriteString("/deltask <id> - удалить задачу\n")
	msg.WriteString("/runtask <id> - запустить сейчас\n\n")
	msg.WriteString("Поддерживаемые типы: text, sticker, photo, animation, video, voice, document, audio\n")
	msg.WriteString("Reply на сообщение для автоматического определения типа")

	// Inline-кнопки 🗑 для каждой задачи (по 2 в ряд) — только если задач немного.
	// Регистрация callback'а: cmd/bot/main.go через core.AdminOnlyCallback.
	opts := &tele.SendOptions{ParseMode: tele.ModeHTML}
	if len(tasks) <= listTasksMaxButtons {
		markup := &tele.ReplyMarkup{}
		rows := make([]tele.Row, 0, (len(tasks)+1)/2)
		row := make([]tele.Btn, 0, 2)
		for _, task := range tasks {
			row = append(row, tele.Btn{
				Unique: UniqueDeleteTask,
				Text:   fmt.Sprintf("🗑 #%d %s", task.ID, task.TaskName),
				Data:   strconv.FormatInt(task.ID, 10),
			})
			if len(row) == 2 {
				rows = append(rows, markup.Row(row...))
				row = row[:0]
			}
		}
		if len(row) > 0 {
			rows = append(rows, markup.Row(row...))
		}
		markup.Inline(rows...)
		opts.ReplyMarkup = markup
	}

	return c.Send(msg.String(), opts)
}

// validTaskTypes — разрешённые типы данных для задачи планировщика.
var validTaskTypes = map[string]bool{
	"sticker": true, "text": true, "photo": true, "animation": true,
	"video": true, "voice": true, "document": true, "audio": true,
}

// extractTaskTypeFromReply определяет тип и file_id из медиа в reply-сообщении.
// Если медиа нет — возвращает ("text", msg.Text).
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

// parseAddTaskArgs разбирает аргументы /addtask: имя + "cron" в кавычках.
// Возвращает name, cronExpr, остаток после закрывающей кавычки и сообщение об ошибке (пустое если OK).
func parseAddTaskArgs(rawText string) (name, cronExpr, rest, errMsg string) {
	text := strings.TrimSpace(rawText)
	if !strings.HasPrefix(text, "/addtask ") {
		return "", "", "", "❌ Неверный формат команды"
	}
	text = strings.TrimSpace(text[len("/addtask "):])

	parts := strings.SplitN(text, " ", 2)
	if len(parts) < 2 {
		return "", "", "", "❌ Не указано имя или cron-выражение"
	}
	name = parts[0]
	remaining := strings.TrimSpace(parts[1])

	if len(name) == 0 {
		return "", "", "", "❌ Имя задачи не может быть пустым"
	}
	if len(name) > 200 {
		return "", "", "", "❌ Имя задачи слишком длинное (макс. 200 символов)"
	}

	if !strings.HasPrefix(remaining, "\"") {
		return "", "", "", "❌ Cron выражение должно быть в кавычках"
	}
	endQuote := strings.Index(remaining[1:], "\"")
	if endQuote == -1 {
		return "", "", "", "❌ Неверный формат cron выражения"
	}
	cronExpr = remaining[1 : endQuote+1]

	if _, err := cron.ParseStandard(cronExpr); err != nil {
		return "", "", "", fmt.Sprintf("❌ Неверное cron выражение: %v", err)
	}

	rest = strings.TrimSpace(remaining[endQuote+2:])
	return name, cronExpr, rest, ""
}

// createAndRegisterTask создаёт задачу в БД, регистрирует в cron, логирует событие и отправляет
// пользователю подтверждение. Общий хвост для reply-режима и text-режима /addtask.
func (m *SchedulerModule) createAndRegisterTask(c tele.Context, chatID int64, threadID int, name, cronExpr, taskType, taskData string) error {
	taskID, err := m.schedulerRepo.CreateTask(chatID, threadID, name, cronExpr, taskType, taskData)
	if err != nil {
		m.logger.Error("failed to create task", zap.Error(err))
		return c.Send("❌ Ошибка при создании задачи")
	}

	task, err := m.schedulerRepo.GetTask(taskID)
	if err != nil {
		m.logger.Error("failed to get task", zap.Error(err))
		return c.Send("❌ Ошибка при получении задачи")
	}

	if err := m.registerTask(task); err != nil {
		m.logger.Error("failed to register task in cron", zap.Error(err))
		return c.Send("❌ Ошибка при регистрации задачи в планировщике")
	}

	_ = m.eventRepo.Log(chatID, c.Sender().ID, "scheduler", "task_created",
		fmt.Sprintf("Task %s created", name))

	scopeLabel := "для всего чата"
	scopeHint := "💡 Для создания задачи для топика используйте команду внутри топика"
	if threadID != 0 {
		scopeLabel = "для этого топика"
		scopeHint = "💡 Для создания задачи для всего чата используйте команду в основном чате"
	}

	scopeMsg := fmt.Sprintf("✅ Задача создана <b>%s</b>\n\n%s\n\n"+
		"ID: %d\n"+
		"Название: <code>%s</code>\n"+
		"Расписание: <code>%s</code>\n"+
		"Тип: %s", scopeLabel, scopeHint, taskID,
		html.EscapeString(name),     // пользовательский ввод — escape обязателен
		html.EscapeString(cronExpr), // cron-выражение валидировано парсером, но escape для единообразия
		taskType)                    // taskType — enum из белого списка, безопасен

	return c.Send(scopeMsg, &tele.SendOptions{ParseMode: tele.ModeHTML})
}

func (m *SchedulerModule) handleAddTask(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleAddTask called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	// scheduled_tasks имеет FK на chats(chat_id) — без записи в chats INSERT упадёт.
	if err := m.chatRepo.EnsureExists(chatID); err != nil {
		m.logger.Error("failed to ensure chat exists", zap.Error(err))
	}

	name, cronExpr, rest, errMsg := parseAddTaskArgs(c.Text())
	if errMsg != "" {
		// В reply-режиме показываем короткую справку, в text-режиме — полную.
		if c.Message().ReplyTo != nil && errMsg == "❌ Не указано имя или cron-выражение" {
			return c.Send("Использование: /addtask <name> \"<cron>\" (reply на сообщение со стикером/фото/гифкой/etc.)\nПример: /addtask monday_sticker \"0 9 * * 1\"")
		}
		if c.Message().ReplyTo == nil && errMsg == "❌ Не указано имя или cron-выражение" {
			return c.Send("❌ Неверный формат команды\n\n" +
				"Использование:\n" +
				"/addtask <name> \"<cron>\" <type> <data>\n" +
				"Или reply на сообщение со стикером/фото/etc.\n\n" +
				"Примеры:\n" +
				"/addtask monday_sticker \"0 9 * * 1\" sticker CAACAgIAA...\n" +
				"/addtask morning_text \"0 9 * * *\" text \"Доброе утро!\"\n\n" +
				"Cron формат: минута час день месяц день_недели\n" +
				"0 9 * * 1 - каждый понедельник в 9:00\n" +
				"0 9 * * 1-5 - пн-пт в 9:00")
		}
		return c.Send(errMsg)
	}

	var taskType, taskData string
	if c.Message().ReplyTo != nil {
		// Reply-режим: тип и данные берём из reply-сообщения.
		taskType, taskData = extractTaskTypeFromReply(c.Message().ReplyTo)
	} else {
		// Text-режим: парсим тип и данные из остатка после "cron".
		typeParts := strings.SplitN(rest, " ", 2)
		if len(typeParts) < 2 {
			return c.Send("❌ Неверный формат команды")
		}
		taskType = typeParts[0]
		taskData = strings.Trim(typeParts[1], "\"")

		if !validTaskTypes[taskType] {
			return c.Send("❌ Неверный тип задачи. Доступны: sticker, text, photo, animation, video, voice, document, audio")
		}
	}

	return m.createAndRegisterTask(c, chatID, threadID, name, cronExpr, taskType, taskData)
}

func (m *SchedulerModule) handleDeleteTask(c tele.Context) error {
	m.logger.Info("handleDeleteTask called", zap.Int64("chat_id", c.Chat().ID), zap.Int64("user_id", c.Sender().ID))

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("❌ Использование: /deltask <task_id>")
	}

	taskID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный ID задачи")
	}

	task, err := m.schedulerRepo.GetTask(taskID)
	if err != nil {
		return c.Send("❌ Задача не найдена")
	}

	if task.ChatID != c.Chat().ID {
		return c.Send("❌ Задача не найдена в этом чате")
	}

	if err := m.schedulerRepo.DeleteTask(taskID); err != nil {
		m.logger.Error("failed to delete task", zap.Error(err))
		return c.Send("❌ Ошибка при удалении задачи")
	}

	// Удаляем задачу из cron в памяти
	m.mu.Lock()
	if entryID, ok := m.taskEntries[taskID]; ok {
		m.cron.Remove(entryID)
		delete(m.taskEntries, taskID)
		m.logger.Info("removed cron entry",
			zap.Int64("task_id", taskID),
			zap.Int("cron_entry_id", int(entryID)),
		)
	}
	m.mu.Unlock()

	_ = m.eventRepo.Log(c.Chat().ID, c.Sender().ID, "scheduler", "task_deleted",
		fmt.Sprintf("Task %d deleted", taskID))

	return c.Send(fmt.Sprintf("✅ Задача %d удалена", taskID))
}

func (m *SchedulerModule) handleRunTask(c tele.Context) error {
	m.logger.Info("handleRunTask called", zap.Int64("chat_id", c.Chat().ID), zap.Int64("user_id", c.Sender().ID))

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("❌ Использование: /runtask <task_id>")
	}

	taskID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный ID задачи")
	}

	task, err := m.schedulerRepo.GetTask(taskID)
	if err != nil {
		return c.Send("❌ Задача не найдена")
	}

	if task.ChatID != c.Chat().ID {
		return c.Send("❌ Задача не найдена в этом чате")
	}

	go m.executeTask(task)

	return c.Send(fmt.Sprintf("✅ Задача %s запущена", task.TaskName))
}
