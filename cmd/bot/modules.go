package main

import (
	"database/sql"
	"fmt"

	"github.com/flybasist/bmft/internal/modules/limiter"
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
	Limiter    *limiter.LimiterModule
	Statistics *statistics.StatisticsModule
	Reactions  *reactions.ReactionsModule
	Scheduler  *scheduler.SchedulerModule
	TextFilter *textfilter.TextFilterModule
}

// initModules создаёт и инициализирует все модули бота.
// Русский комментарий: Централизованная инициализация всех модулей.
// Возвращает структуру Modules со всеми готовыми к работе модулями.
func initModules(db *sql.DB, bot *tele.Bot, logger *zap.Logger) (*Modules, error) {
	logger.Info("initializing modules")

	// Создаём репозитории
	moduleRepo := repositories.NewModuleRepository(db)
	eventRepo := repositories.NewEventRepository(db)
	vipRepo := repositories.NewVIPRepository(db)
	contentLimitsRepo := repositories.NewContentLimitsRepository(db)
	statsRepo := repositories.NewStatisticsRepository(db)
	schedulerRepo := repositories.NewSchedulerRepository(db)

	// Создаём модули
	modules := &Modules{
		Statistics: statistics.New(db, statsRepo, moduleRepo, eventRepo, logger, bot),
		Limiter:    limiter.New(vipRepo, contentLimitsRepo, moduleRepo, logger, bot),
		Scheduler:  scheduler.New(db, schedulerRepo, moduleRepo, eventRepo, logger, bot),
		Reactions:  reactions.New(db, vipRepo, logger, bot),
		TextFilter: textfilter.New(db, vipRepo, contentLimitsRepo, logger, bot),
	}

	// Запускаем scheduler (явный старт жизненного цикла)
	logger.Info("starting scheduler module")
	if err := modules.Scheduler.Start(); err != nil {
		return nil, fmt.Errorf("failed to start scheduler: %w", err)
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

	logger.Info("all modules initialized successfully")

	return modules, nil
}

// shutdownModules выполняет graceful shutdown всех модулей.
// Русский комментарий: Только scheduler требует явного shutdown (остановка cron).
// Остальные модули stateless и не требуют очистки ресурсов.
func (m *Modules) shutdownModules(logger *zap.Logger) error {
	logger.Info("shutting down modules")

	// Только Scheduler требует shutdown (остановка cron)
	if err := m.Scheduler.Shutdown(); err != nil {
		logger.Error("failed to shutdown scheduler", zap.Error(err))
		return err
	}

	logger.Info("all modules shutdown complete")
	return nil
}
