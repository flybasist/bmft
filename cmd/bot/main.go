package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq" // PostgreSQL driver
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/logx"
	"github.com/flybasist/bmft/internal/migrations"
	"github.com/flybasist/bmft/internal/postgresql"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"github.com/flybasist/bmft/internal/profanity"
)

func main() {
	// Загружаем .env для локальной разработки (в проде файл не требуется)
	_ = godotenv.Load()

	// Русский комментарий: Главная точка входа бота.
	// 1. Загружаем конфиг
	// 2. Инициализируем логгер
	// 3. Подключаемся к PostgreSQL
	// 4. Автоматически применяем миграции (001_initial_schema.sql)
	// 5. Создаём telebot.v3 бота с Long Polling
	// 6. Создаём Module Registry
	// 7. Регистрируем модули (limiter, reactions, statistics, scheduler)
	// 8. Инициализируем модули
	// 9. Регистрируем хендлеры команд
	// 10. Запускаем бота
	// 11. Ждём SIGINT/SIGTERM для graceful shutdown

	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Загружаем конфигурацию из .env
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Устанавливаем временную зону для Go-приложения
	tz := os.Getenv("TZ")
	if tz == "" {
		tz = "Europe/Moscow"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return fmt.Errorf("failed to load timezone %s: %w", tz, err)
	}
	time.Local = loc

	// Инициализируем structured logger (zap) с ротацией файлов
	logger, err := logx.NewLogger(cfg.LogLevel, cfg.LogPretty, logx.LogRotationConfig{
		MaxSizeMB:  cfg.LogMaxSizeMB,
		MaxBackups: cfg.LogMaxBackups,
		MaxAgeDays: cfg.LogMaxAgeDays,
	})
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("starting bmft bot",
		zap.String("log_level", cfg.LogLevel),
		zap.Bool("log_pretty", cfg.LogPretty),
		zap.String("timezone", tz),
		zap.Duration("shutdown_timeout", cfg.ShutdownTimeout),
		zap.Int("polling_timeout", cfg.PollingTimeout),
		zap.Int("log_max_size_mb", cfg.LogMaxSizeMB),
		zap.Int("log_max_backups", cfg.LogMaxBackups),
		zap.Int("log_max_age_days", cfg.LogMaxAgeDays),
		zap.Int("db_retention_months", cfg.DBRetentionMonths),
	)

	// Подключаемся к PostgreSQL
	db, err := sql.Open("postgres", cfg.PostgresDSN)
	if err != nil {
		return fmt.Errorf("failed to connect to postgres: %w", err)
	}
	defer db.Close()

	// Проверяем подключение
	if err := postgresql.PingWithRetry(db, 10, 2*time.Second, logger); err != nil {
		return fmt.Errorf("failed to ping postgres: %w", err)
	}
	logger.Info("connected to postgresql")

	// Явно устанавливаем timezone для PostgreSQL-сессии
	_, err = db.Exec("SET TIME ZONE 'Europe/Moscow';")
	if err != nil {
		logger.Warn("failed to set timezone in PostgreSQL session", zap.Error(err))
	} else {
		logger.Info("PostgreSQL session timezone set to Europe/Moscow")
	}

	// Автоматически применяем миграции (или валидируем существующую схему)
	if err := migrations.RunMigrationsIfNeeded(db, logger); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}
	logger.Info("database schema ready")

	// Загружаем словарь мата (если настроено)
	ctx := context.Background()
	if err := profanity.EnsureDictionary(ctx, db, logger); err != nil {
		logger.Warn("failed to load profanity dictionary", zap.Error(err))
	}

	// Создаём telebot.v3 бота с Long Polling
	pref := tele.Settings{
		Token:  cfg.TelegramBotToken,
		Poller: &tele.LongPoller{Timeout: time.Duration(cfg.PollingTimeout) * time.Second},
	}
	bot, err := tele.NewBot(pref)
	if err != nil {
		return fmt.Errorf("failed to create bot: %w", err)
	}

	logger.Info("bot created successfully",
		zap.String("bot_username", bot.Me.Username),
		zap.Int64("bot_id", bot.Me.ID),
	)

	// Регистрируем middleware ПЕРВЫМИ (до регистрации любых команд)
	bot.Use(core.LoggerMiddleware(logger))
	bot.Use(core.PanicRecoveryMiddleware(logger))

	// Создаём все модули
	modules, err := initModules(db, bot, logger, cfg)
	if err != nil {
		return fmt.Errorf("failed to init modules: %w", err)
	}

	// Создаём репозитории для хендлеров
	chatRepo := repositories.NewChatRepository(db)
	eventRepo := repositories.NewEventRepository(db)
	settingsRepo := repositories.NewSettingsRepository(db)

	// Получаем версию бота из БД
	botVersion, err := settingsRepo.GetVersion()
	if err != nil {
		logger.Warn("failed to get bot version from DB, using default",
			zap.Error(err),
		)
		botVersion = "1.0"
	}

	// Регистрируем базовые команды
	registerCommands(bot, modules, chatRepo, eventRepo, logger, db, botVersion)

	// Создаём контекст для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Слушаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Запускаем бота в отдельной горутине
	go func() {
		logger.Info("bot started, polling for updates...")
		bot.Start()
	}()

	// Ждём сигнала остановки
	sig := <-sigChan
	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer shutdownCancel()

	logger.Info("shutting down bot...")
	bot.Stop()

	logger.Info("shutting down modules...")
	if err := modules.shutdownModules(logger); err != nil {
		logger.Error("failed to shutdown modules", zap.Error(err))
	}

	logger.Info("closing database connection...")
	if err := db.Close(); err != nil {
		logger.Error("failed to close database", zap.Error(err))
	}

	select {
	case <-shutdownCtx.Done():
		logger.Warn("shutdown timeout exceeded")
		return fmt.Errorf("shutdown timeout exceeded")
	default:
		logger.Info("bot shutdown complete")
		return nil
	}
}
