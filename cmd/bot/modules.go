package main

import (
	"database/sql"
	"fmt"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/modules/limiter"
	"github.com/flybasist/bmft/internal/modules/maintenance"
	"github.com/flybasist/bmft/internal/modules/profanityfilter"
	"github.com/flybasist/bmft/internal/modules/reactions"
	"github.com/flybasist/bmft/internal/modules/scheduler"
	"github.com/flybasist/bmft/internal/modules/statistics"
	"github.com/flybasist/bmft/internal/modules/textfilter"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// Modules содержит все модули бота.
// Русский комментарий: Явная структура со всеми модулями.
// Нет магии registry, все модули видны напрямую.
type Modules struct {
	Limiter         *limiter.LimiterModule
	Statistics      *statistics.StatisticsModule
	Reactions       *reactions.ReactionsModule
	Scheduler       *scheduler.SchedulerModule
	TextFilter      *textfilter.TextFilterModule
	ProfanityFilter *profanityfilter.ProfanityFilterModule
	Maintenance     *maintenance.MaintenanceModule
}

// initModules создаёт и инициализирует все модули бота.
// Русский комментарий: Централизованная инициализация всех модулей.
// Возвращает структуру Modules со всеми готовыми к работе модулями.
func initModules(db *sql.DB, bot *tele.Bot, logger *zap.Logger, cfg *config.Config) (*Modules, error) {
	logger.Info("initializing modules")

	// Создаём репозитории
	eventRepo := repositories.NewEventRepository(db)
	vipRepo := repositories.NewVIPRepository(db)
	contentLimitsRepo := repositories.NewContentLimitsRepository(db)
	schedulerRepo := repositories.NewSchedulerRepository(db)

	// Создаём модули
	modules := &Modules{
		Statistics:      statistics.New(db, eventRepo, logger, bot),
		Limiter:         limiter.New(db, vipRepo, contentLimitsRepo, eventRepo, logger, bot),
		Scheduler:       scheduler.New(db, schedulerRepo, eventRepo, logger, bot),
		Reactions:       reactions.New(db, vipRepo, eventRepo, logger, bot),
		TextFilter:      textfilter.New(db, vipRepo, contentLimitsRepo, eventRepo, logger, bot),
		ProfanityFilter: profanityfilter.New(db, vipRepo, contentLimitsRepo, eventRepo, logger, bot),
		Maintenance:     maintenance.New(db, logger, cfg.DBRetentionMonths),
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

	// Reactions
	modules.Reactions.RegisterCommands(bot)
	modules.Reactions.RegisterAdminCommands(bot)

	// TextFilter
	modules.TextFilter.RegisterCommands(bot)
	modules.TextFilter.RegisterAdminCommands(bot)

	// ProfanityFilter
	modules.ProfanityFilter.RegisterCommands(bot)
	modules.ProfanityFilter.RegisterAdminCommands(bot)

	logger.Info("all modules initialized successfully")

	return modules, nil
}

// shutdownModules выполняет graceful shutdown всех модулей.
// Русский комментарий: Scheduler и Maintenance требуют явного shutdown (остановка cron).
// Остальные модули stateless и не требуют очистки ресурсов.
func (m *Modules) shutdownModules(logger *zap.Logger) error {
	logger.Info("shutting down modules")

	// Останавливаем Scheduler
	if err := m.Scheduler.Shutdown(); err != nil {
		logger.Error("failed to shutdown scheduler", zap.Error(err))
		return err
	}

	// Останавливаем Maintenance
	if err := m.Maintenance.Shutdown(); err != nil {
		logger.Error("failed to shutdown maintenance", zap.Error(err))
		return err
	}

	logger.Info("all modules shutdown complete")
	return nil
}
