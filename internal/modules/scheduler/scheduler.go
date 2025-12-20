package scheduler

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
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
	cron          *cron.Cron
}

// New создаёт новый инстанс модуля планировщика.
func New(db *sql.DB, schedulerRepo *repositories.SchedulerRepository, eventRepo *repositories.EventRepository, logger *zap.Logger, bot *tele.Bot) *SchedulerModule {
	m := &SchedulerModule{
		db:            db,
		schedulerRepo: schedulerRepo,
		eventRepo:     eventRepo,
		logger:        logger,
		bot:           bot,
		cron:          cron.New(),
	}

	logger.Info("scheduler module created")
	return m
}

// Start запускает планировщик задач.
// Русский комментарий: Явный метод для управления жизненным циклом.
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

// OnMessage обрабатывает входящие сообщения.
func (m *SchedulerModule) OnMessage(ctx *core.MessageContext) error {
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

		msg += "🔹 <code>/listtasks</code> — Список всех активных задач\n\n"

		msg += "🔹 <code>/removetask &lt;ID&gt;</code> — Удалить задачу\n"
		msg += "   📌 Пример: <code>/removetask 3</code>\n\n"

		msg += "📅 <b>Формат cron:</b> минута час день месяц день_недели\n"
		msg += "• <code>0 9 * * *</code> — каждый день в 9:00\n"
		msg += "• <code>0 */6 * * *</code> — каждые 6 часов\n"
		msg += "• <code>0 9 * * 1</code> — каждый понедельник в 9:00\n"
		msg += "• <code>0 0 1 * *</code> — 1-го числа каждого месяца\n\n"

		msg += "⚙️ <b>Топики:</b> команда в топике → задача для топика\n\n"

		msg += "💡 <i>Подсказка:</i> Проверяйте cron на сайте crontab.guru"

		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
	})
}

func (m *SchedulerModule) RegisterAdminCommands(bot *tele.Bot) {
	bot.Handle("/listtasks", m.handleListTasks)
	bot.Handle("/addtask", m.handleAddTask)
	bot.Handle("/deltask", m.handleDeleteTask)
	bot.Handle("/runtask", m.handleRunTask)
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
	_, err := m.cron.AddFunc(task.CronExpr, func() {
		m.executeTask(task)
	})
	if err != nil {
		return fmt.Errorf("failed to add cron job: %w", err)
	}

	m.logger.Info("registered cron task",
		zap.Int64("task_id", task.ID),
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
	// Проверка прав администратора
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		m.logger.Error("failed to check user admin status", zap.Error(err))
		return c.Send("❌ Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	chatID := c.Chat().ID
	threadID := 0
	if c.Message().ThreadID != 0 {
		threadID = c.Message().ThreadID
	}

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
		msg.WriteString("📋 *Задачи планировщика (для этого топика):*\n\n")
	} else {
		msg.WriteString("📋 *Задачи планировщика (для всего чата):*\n\n")
	}

	for i, task := range tasks {
		status := "✅"
		if !task.IsActive {
			status = "⏸️"
		}

		msg.WriteString(fmt.Sprintf("%d. %s %s\n", i+1, status, task.TaskName))
		msg.WriteString(fmt.Sprintf("   ID: %d\n", task.ID))
		msg.WriteString(fmt.Sprintf("   Расписание: %s\n", task.CronExpr))
		msg.WriteString(fmt.Sprintf("   Тип: %s\n", task.TaskType))

		if task.LastRun != nil {
			msg.WriteString(fmt.Sprintf("   Последний запуск: %s\n", task.LastRun.Format("02.01.2006 15:04")))
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

	return c.Send(msg.String(), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

func (m *SchedulerModule) handleAddTask(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := int(c.Message().ThreadID)

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		m.logger.Error("failed to check user admin status", zap.Error(err))
		return c.Send("❌ Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	var taskType, taskData string

	if c.Message().ReplyTo != nil {
		// Reply mode: get content from replied message
		// Русский комментарий: Парсим команду /addtask <name> "<cron>" с учётом кавычек
		text := strings.TrimSpace(c.Text())
		if !strings.HasPrefix(text, "/addtask ") {
			return c.Send("❌ Неверный формат команды")
		}
		text = text[len("/addtask "):]

		parts := strings.SplitN(text, " ", 2)
		if len(parts) < 2 {
			return c.Send("Использование: /addtask <name> \"<cron>\" (reply на сообщение со стикером/фото/гифкой/etc.)\nПример: /addtask monday_sticker \"0 9 * * 1\"")
		}

		name := parts[0]
		remaining := strings.TrimSpace(parts[1])

		// Parse cron expression in quotes
		if !strings.HasPrefix(remaining, "\"") {
			return c.Send("❌ Cron выражение должно быть в кавычках")
		}
		endQuote := strings.Index(remaining[1:], "\"")
		if endQuote == -1 {
			return c.Send("❌ Неверный формат cron выражения")
		}
		cronExpr := remaining[1 : endQuote+1]

		if _, err := cron.ParseStandard(cronExpr); err != nil {
			return c.Send(fmt.Sprintf("❌ Неверное cron выражение: %v", err))
		}

		replyMsg := c.Message().ReplyTo
		if replyMsg.Sticker != nil {
			taskType = "sticker"
			taskData = replyMsg.Sticker.FileID
		} else if replyMsg.Photo != nil {
			taskType = "photo"
			taskData = replyMsg.Photo.FileID
		} else if replyMsg.Animation != nil {
			taskType = "animation"
			taskData = replyMsg.Animation.FileID
		} else if replyMsg.Video != nil {
			taskType = "video"
			taskData = replyMsg.Video.FileID
		} else if replyMsg.Voice != nil {
			taskType = "voice"
			taskData = replyMsg.Voice.FileID
		} else if replyMsg.Document != nil {
			taskType = "document"
			taskData = replyMsg.Document.FileID
		} else if replyMsg.Audio != nil {
			taskType = "audio"
			taskData = replyMsg.Audio.FileID
		} else {
			taskType = "text"
			taskData = replyMsg.Text
		}

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

		var scopeMsg string
		if threadID != 0 {
			scopeMsg = fmt.Sprintf("✅ Задача создана **для этого топика**\n\n💡 Для создания задачи для всего чата используйте команду в основном чате\n\n"+
				"ID: %d\n"+
				"Название: %s\n"+
				"Расписание: %s\n"+
				"Тип: %s", taskID, name, cronExpr, taskType)
		} else {
			scopeMsg = fmt.Sprintf("✅ Задача создана **для всего чата**\n\n💡 Для создания задачи для топика используйте команду внутри топика\n\n"+
				"ID: %d\n"+
				"Название: %s\n"+
				"Расписание: %s\n"+
				"Тип: %s", taskID, name, cronExpr, taskType)
		}

		return c.Send(scopeMsg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	} else {
		// Text mode
		text := strings.TrimSpace(c.Text())
		if !strings.HasPrefix(text, "/addtask ") {
			return c.Send("❌ Неверный формат команды")
		}
		text = text[len("/addtask "):]

		parts := strings.SplitN(text, " ", 2)
		if len(parts) < 2 {
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

		name := parts[0]
		remaining := parts[1]

		// Parse cron expression in quotes
		if !strings.HasPrefix(remaining, "\"") {
			return c.Send("❌ Cron выражение должно быть в кавычках")
		}
		endQuote := strings.Index(remaining[1:], "\"")
		if endQuote == -1 {
			return c.Send("❌ Неверный формат cron выражения")
		}
		cronExpr := remaining[1 : endQuote+1]
		remaining = remaining[endQuote+2:] // skip "
		remaining = strings.TrimSpace(remaining)

		parts = strings.SplitN(remaining, " ", 2)
		if len(parts) < 2 {
			return c.Send("❌ Неверный формат команды")
		}

		taskType = parts[0]
		taskData = strings.Trim(parts[1], "\"")

		if _, err := cron.ParseStandard(cronExpr); err != nil {
			return c.Send(fmt.Sprintf("❌ Неверное cron выражение: %v", err))
		}

		if taskType != "sticker" && taskType != "text" && taskType != "photo" && taskType != "animation" && taskType != "video" && taskType != "voice" && taskType != "document" && taskType != "audio" {
			return c.Send("❌ Неверный тип задачи. Доступны: sticker, text, photo, animation, video, voice, document, audio")
		}

		chatID := c.Chat().ID
		threadID := 0
		if c.Message().ThreadID != 0 {
			threadID = c.Message().ThreadID
		}

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

		var scopeMsg string
		if threadID != 0 {
			scopeMsg = fmt.Sprintf("✅ Задача создана **для этого топика**\n\n💡 Для создания задачи для всего чата используйте команду в основном чате\n\n"+
				"ID: %d\n"+
				"Название: %s\n"+
				"Расписание: %s\n"+
				"Тип: %s", taskID, name, cronExpr, taskType)
		} else {
			scopeMsg = fmt.Sprintf("✅ Задача создана **для всего чата**\n\n💡 Для создания задачи для топика используйте команду внутри топика\n\n"+
				"ID: %d\n"+
				"Название: %s\n"+
				"Расписание: %s\n"+
				"Тип: %s", taskID, name, cronExpr, taskType)
		}

		return c.Send(scopeMsg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}
}

func (m *SchedulerModule) handleDeleteTask(c tele.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		m.logger.Error("failed to check user admin status", zap.Error(err))
		return c.Send("❌ Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

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

	_ = m.eventRepo.Log(c.Chat().ID, c.Sender().ID, "scheduler", "task_deleted",
		fmt.Sprintf("Task %d deleted", taskID))

	return c.Send(fmt.Sprintf("✅ Задача %d удалена", taskID))
}

func (m *SchedulerModule) handleRunTask(c tele.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		m.logger.Error("failed to check user admin status", zap.Error(err))
		return c.Send("❌ Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

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
