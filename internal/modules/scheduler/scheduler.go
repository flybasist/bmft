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
	moduleRepo    *repositories.ModuleRepository
	eventRepo     *repositories.EventRepository
	adminUsers    []int64
	cron          *cron.Cron
}

// New создаёт новый инстанс модуля планировщика.
func New(db *sql.DB, schedulerRepo *repositories.SchedulerRepository, moduleRepo *repositories.ModuleRepository, eventRepo *repositories.EventRepository, logger *zap.Logger) *SchedulerModule {
	return &SchedulerModule{
		db:            db,
		schedulerRepo: schedulerRepo,
		moduleRepo:    moduleRepo,
		eventRepo:     eventRepo,
		logger:        logger,
		adminUsers:    []int64{},
		cron:          cron.New(),
	}
}

// SetAdminUsers устанавливает список администраторов.
func (m *SchedulerModule) SetAdminUsers(adminUsers []int64) {
	m.adminUsers = adminUsers
}

// Init инициализирует модуль планировщика.
func (m *SchedulerModule) Init(deps core.ModuleDependencies) error {
	m.bot = deps.Bot
	m.logger.Info("initializing scheduler module")

	if err := m.loadActiveTasks(); err != nil {
		return fmt.Errorf("failed to load active tasks: %w", err)
	}

	m.cron.Start()
	m.logger.Info("cron scheduler started")

	return nil
}

// OnMessage обрабатывает входящие сообщения.
func (m *SchedulerModule) OnMessage(ctx *core.MessageContext) error {
	return nil
}

// Commands возвращает список команд модуля.
func (m *SchedulerModule) Commands() []core.BotCommand {
	return []core.BotCommand{
		{Command: "/listtasks", Description: "Список задач планировщика"},
	}
}

// Enabled проверяет, включен ли модуль для данного чата.
func (m *SchedulerModule) Enabled(chatID int64) (bool, error) {
	enabled, err := m.moduleRepo.IsEnabled(chatID, "scheduler")
	if err != nil {
		return false, err
	}
	return enabled, nil
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
	bot.Handle("/listtasks", m.handleListTasks)
}

func (m *SchedulerModule) RegisterAdminCommands(bot *tele.Bot) {
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

	enabled, err := m.Enabled(task.ChatID)
	if err != nil {
		m.logger.Error("failed to check if module enabled", zap.Error(err))
		return
	}
	if !enabled {
		m.logger.Info("scheduler module disabled for chat", zap.Int64("chat_id", task.ChatID))
		return
	}

	chat := &tele.Chat{ID: task.ChatID}

	switch task.TaskType {
	case "sticker":
		sticker := &tele.Sticker{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, sticker); err != nil {
			m.logger.Error("failed to send sticker", zap.Error(err))
			return
		}

	case "text":
		if _, err := m.bot.Send(chat, task.TaskData); err != nil {
			m.logger.Error("failed to send text", zap.Error(err))
			return
		}

	case "photo":
		photo := &tele.Photo{File: tele.File{FileID: task.TaskData}}
		if _, err := m.bot.Send(chat, photo); err != nil {
			m.logger.Error("failed to send photo", zap.Error(err))
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
	chatID := c.Chat().ID

	tasks, err := m.schedulerRepo.GetChatTasks(chatID)
	if err != nil {
		m.logger.Error("failed to get chat tasks", zap.Error(err))
		return c.Send("❌ Ошибка при получении списка задач")
	}

	if len(tasks) == 0 {
		return c.Send("📋 Нет задач планировщика\n\nИспользуйте /addtask для создания новой задачи")
	}

	var msg strings.Builder
	msg.WriteString("📋 Задачи планировщика:\n\n")

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
	msg.WriteString("/runtask <id> - запустить сейчас")

	return c.Send(msg.String())
}

func (m *SchedulerModule) handleAddTask(c tele.Context) error {
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Эта команда доступна только администраторам")
	}

	args := strings.Fields(c.Text())
	if len(args) < 5 {
		return c.Send("❌ Неверный формат команды\n\n" +
			"Использование:\n" +
			"/addtask <name> \"<cron>\" <type> <data>\n\n" +
			"Примеры:\n" +
			"/addtask monday_sticker \"0 9 * * 1\" sticker CAACAgIAA...\n" +
			"/addtask morning_text \"0 9 * * *\" text \"Доброе утро!\"\n\n" +
			"Cron формат: минута час день месяц день_недели\n" +
			"0 9 * * 1 - каждый понедельник в 9:00\n" +
			"0 9 * * 1-5 - пн-пт в 9:00")
	}

	name := args[1]
	cronExpr := strings.Trim(args[2], "\"")
	taskType := args[3]
	taskData := strings.Join(args[4:], " ")
	taskData = strings.Trim(taskData, "\"")

	if _, err := cron.ParseStandard(cronExpr); err != nil {
		return c.Send(fmt.Sprintf("❌ Неверное cron выражение: %v", err))
	}

	if taskType != "sticker" && taskType != "text" && taskType != "photo" {
		return c.Send("❌ Неверный тип задачи. Доступны: sticker, text, photo")
	}

	chatID := c.Chat().ID

	taskID, err := m.schedulerRepo.CreateTask(chatID, name, cronExpr, taskType, taskData)
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

	return c.Send(fmt.Sprintf("✅ Задача создана\n\n"+
		"ID: %d\n"+
		"Название: %s\n"+
		"Расписание: %s\n"+
		"Тип: %s", taskID, name, cronExpr, taskType))
}

func (m *SchedulerModule) handleDeleteTask(c tele.Context) error {
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Эта команда доступна только администраторам")
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
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Эта команда доступна только администраторам")
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

func (m *SchedulerModule) isAdmin(userID int64) bool {
	for _, adminID := range m.adminUsers {
		if adminID == userID {
			return true
		}
	}
	return false
}
