package main

import (
	"database/sql"
	"fmt"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/modules/limiter"
	"github.com/flybasist/bmft/internal/modules/maintenance"
	"github.com/flybasist/bmft/internal/modules/reactions"
	"github.com/flybasist/bmft/internal/modules/scheduler"
	"github.com/flybasist/bmft/internal/modules/statistics"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"github.com/flybasist/bmft/internal/wizard"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// Modules содержит все модули бота.
// Явная структура со всеми модулями.
// Нет магии registry, все модули видны напрямую.
// TextFilter и ProfanityFilter объединены в Reactions (v1.1).
type Modules struct {
	Limiter     *limiter.LimiterModule
	Statistics  *statistics.StatisticsModule
	Reactions   *reactions.ReactionsModule
	Scheduler   *scheduler.SchedulerModule
	Maintenance *maintenance.MaintenanceModule
}

// initModules создаёт и инициализирует все модули бота.
// Централизованная инициализация всех модулей.
// Возвращает структуру Modules со всеми готовыми к работе модулями.
func initModules(db *sql.DB, bot *tele.Bot, logger *zap.Logger, cfg *config.Config, wizardMgr *wizard.Manager) (*Modules, error) {
	logger.Info("initializing modules")

	// Создаём репозитории
	eventRepo := repositories.NewEventRepository(db)
	vipRepo := repositories.NewVIPRepository(db)
	contentLimitsRepo := repositories.NewContentLimitsRepository(db)
	schedulerRepo := repositories.NewSchedulerRepository(db)
	messageRepo := repositories.NewMessageRepository(db, logger)
	chatRepo := repositories.NewChatRepository(db)

	// Создаём модули
	// messageRepo — единый экземпляр для всех модулей (statistics, limiter, reactions).
	// Раньше каждый модуль создавал свой NewMessageRepository — 3 одинаковых объекта на одну БД.
	// chatRepo — общий экземпляр; модули используют EnsureExists() перед INSERT с FK на chats.
	modules := &Modules{
		Statistics:  statistics.New(db, eventRepo, messageRepo, logger, bot),
		Limiter:     limiter.New(db, vipRepo, contentLimitsRepo, messageRepo, eventRepo, chatRepo, logger, bot),
		Scheduler:   scheduler.New(db, schedulerRepo, eventRepo, chatRepo, logger, bot),
		Reactions:   reactions.New(db, vipRepo, contentLimitsRepo, messageRepo, eventRepo, chatRepo, logger, bot),
		Maintenance: maintenance.New(db, logger, cfg.DBRetentionMonths),
	}

	// Запускаем scheduler (явный старт жизненного цикла)
	logger.Info("starting scheduler module")
	if err := modules.Scheduler.Start(); err != nil {
		return nil, fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Запускаем maintenance (автоматическая ротация данных)
	logger.Info("starting maintenance module")
	if err := modules.Maintenance.Start(); err != nil {
		return nil, fmt.Errorf("failed to start maintenance: %w", err)
	}

	// Регистрируем команды всех модулей
	logger.Info("registering module commands")

	// Statistics
	modules.Statistics.RegisterCommands(bot)
	modules.Statistics.RegisterAdminCommands(bot)

	// Limiter
	modules.Limiter.RegisterCommands(bot)
	modules.Limiter.RegisterAdminCommands(bot)

	// Scheduler
	modules.Scheduler.RegisterCommands(bot)
	modules.Scheduler.RegisterAdminCommands(bot)

	// Reactions (включает фильтры и мат)
	modules.Reactions.RegisterCommands(bot)
	modules.Reactions.RegisterAdminCommands(bot)

	logger.Info("all modules initialized successfully")

	// Регистрируем pipeline обработки сообщений
	logger.Info("registering message pipeline")
	registerPipeline(bot, modules, vipRepo, db, logger, wizardMgr)

	return modules, nil
}

// shutdownModules выполняет graceful shutdown всех модулей.
// Scheduler и Maintenance требуют явного shutdown (остановка cron).
// Остальные модули stateless и не требуют очистки ресурсов.
func (m *Modules) shutdownModules(logger *zap.Logger) error {
	logger.Info("shutting down modules")

	// Останавливаем все модули последовательно.
	// НЕ прерываемся при ошибке — каждый модуль должен получить шанс на shutdown.
	// Раньше ошибка Scheduler.Shutdown() блокировала остановку Maintenance.
	var firstErr error

	if err := m.Scheduler.Shutdown(); err != nil {
		logger.Error("failed to shutdown scheduler", zap.Error(err))
		firstErr = err
	}

	if err := m.Maintenance.Shutdown(); err != nil {
		logger.Error("failed to shutdown maintenance", zap.Error(err))
		if firstErr == nil {
			firstErr = err
		}
	}

	logger.Info("all modules shutdown complete")
	return firstErr
}

// registerPipeline регистрирует явный pipeline обработки сообщений через bot.Use().
// Каждый модуль регистрируется как middleware в правильном порядке.
// ВАЖНО: Порядок модулей критичен!
// 1. Statistics — записывает все сообщения в таблицу messages
// 2. Limiter — проверяет лимиты контента, может удалить сообщение
// 3. Reactions — фильтры (мат, бан-слова) + автоответы на ключевые слова
//
// ThreadID вычисляется один раз в первом middleware и кешируется через c.Set (−2 SQL-запроса).
// IsVIP проверяется один раз и кешируется (−1 SQL-запрос: Limiter+Reactions раньше дублировали).
// MessageDeleted пропагируется через c.Set: если Limiter удалил сообщение,
// Reactions видит MessageDeleted=true и считает мат без повторного удаления.
func registerPipeline(bot *tele.Bot, modules *Modules, vipRepo *repositories.VIPRepository, db *sql.DB, logger *zap.Logger, wizardMgr *wizard.Manager) {
	// Wizard text-intercept middleware ВСЕГДА первый в pipeline:
	// если пользователь в режиме wizard'а вводит текст (секунды TTL,
	// имя задачи и т.п.) — это сообщение НЕ должно попасть в статистику/
	// limiter/reactions (это служебный ввод, не обычный чат).
	bot.Use(wizardMgr.TextInterceptMiddleware())

	bot.Use(wrapModuleMiddleware(modules.Statistics.OnMessage, "statistics", vipRepo, db, logger))
	bot.Use(wrapModuleMiddleware(modules.Limiter.OnMessage, "limiter", vipRepo, db, logger))
	bot.Use(wrapModuleMiddleware(modules.Reactions.OnMessage, "reactions", vipRepo, db, logger))

	logger.Info("message pipeline registered", zap.Int("modules", 3))
}

// wrapModuleMiddleware конвертирует функцию Module.OnMessage в telebot.MiddlewareFunc.
// Адаптер между core.MessageContext и telebot.Context.
//
// Ключевые гарантии:
// 1. ThreadID вычисляется один раз (первый модуль), кешируется для остальных
// 2. MessageDeleted пропагируется между модулями через c.Set/c.Get
// 3. ВСЕГДА вызывает next(c) — каждый модуль получает шанс обработать сообщение
func wrapModuleMiddleware(
	onMessage func(*core.MessageContext) error,
	moduleName string,
	vipRepo *repositories.VIPRepository,
	db *sql.DB,
	logger *zap.Logger,
) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			msg := c.Message()
			if msg == nil {
				return next(c) // пропускаем не-сообщения
			}

			// Не обрабатываем callback-запросы (нажатия на inline-кнопки меню бота)
			if c.Callback() != nil {
				return next(c)
			}

			// Не обрабатываем сообщения от самого бота
			if msg.Sender != nil && msg.Sender.ID == c.Bot().Me.ID {
				return next(c)
			}

			// ThreadID вычисляется один раз и кешируется для всех модулей.
			// Раньше каждый из 3 модулей вызывал GetThreadIDFromMessage — 3 SQL-запроса.
			var threadID int
			if cached := c.Get("pipelineThreadID"); cached != nil {
				threadID = cached.(int)
			} else {
				threadID = core.GetThreadIDFromMessage(db, msg)
				c.Set("pipelineThreadID", threadID)
			}

			// VIP-статус проверяется один раз и кешируется для всех модулей.
			// Раньше Limiter и Reactions вызывали vipRepo.IsVIP независимо — 2 SQL-запроса.
			// Пропускаем проверку для приватов/команд/нет sender — там VIP не нужен.
			var isVIP bool
			if cached := c.Get("pipelineIsVIP"); cached != nil {
				isVIP = cached.(bool)
			} else if msg.Sender != nil && !msg.Private() {
				v, err := vipRepo.IsVIP(msg.Chat.ID, threadID, msg.Sender.ID)
				if err != nil {
					logger.Warn("VIP check failed in pipeline",
						zap.Int64("chat_id", msg.Chat.ID),
						zap.Int64("user_id", msg.Sender.ID),
						zap.Error(err))
				}
				isVIP = v
				c.Set("pipelineIsVIP", isVIP)
			}

			// MessageDeleted пропагируется между модулями.
			// Если Limiter удалил сообщение, Reactions увидит и скорректирует поведение.
			messageDeleted := false
			if cached := c.Get("messageDeleted"); cached != nil {
				messageDeleted = cached.(bool)
			}

			ctx := &core.MessageContext{
				Message:        msg,
				Chat:           msg.Chat,
				Sender:         msg.Sender,
				Bot:            c.Bot(),
				ThreadID:       threadID,
				IsVIP:          isVIP,
				MessageDeleted: messageDeleted,
			}

			if err := onMessage(ctx); err != nil {
				logger.Error("module failed to process message",
					zap.String("module", moduleName),
					zap.Int64("chat_id", msg.Chat.ID),
					zap.Int("message_id", msg.ID),
					zap.Error(err))
			}

			// Если модуль пометил сообщение как удалённое — сохраняем для следующих
			if ctx.MessageDeleted && !messageDeleted {
				c.Set("messageDeleted", true)
				logger.Debug("message deleted by module",
					zap.String("module", moduleName),
					zap.Int64("chat_id", msg.Chat.ID),
					zap.Int("message_id", msg.ID))
			}

			return next(c) // ВСЕГДА продолжаем — каждый модуль получает шанс
		}
	}
}
