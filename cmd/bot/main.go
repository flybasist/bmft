package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
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
	"github.com/flybasist/bmft/internal/wizard"
)

func main() {
	// Загружаем .env для локальной разработки (в проде файл не требуется)
	_ = godotenv.Load()

	// Главная точка входа бота.
	// 1. Загружаем конфиг
	// 2. Инициализируем логгер
	// 3. Подключаемся к PostgreSQL
	// 4. Автоматически применяем миграции
	// 5. Создаём telebot.v3 бота с Long Polling
	// 6. Создаём и инициализируем модули
	// 7. Регистрируем команды модулей
	// 8. Регистрируем pipeline обработки сообщений
	// 9. Запускаем бота
	// 10. Ждём SIGINT/SIGTERM для graceful shutdown

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

	// Явно устанавливаем timezone для PostgreSQL-сессии (берём из TZ)
	_, err = db.Exec(fmt.Sprintf("SET TIME ZONE '%s';", tz))
	if err != nil {
		logger.Warn("failed to set timezone in PostgreSQL session", zap.Error(err))
	} else {
		logger.Info("PostgreSQL session timezone set", zap.String("timezone", tz))
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
	// Порядок: CommandCooldown → AdminOnly → Logger → PanicRecovery
	adminChecker := core.NewAdminChecker(bot, 60*time.Second)
	bot.Use(core.CommandCooldownMiddleware(5*time.Second, logger))
	bot.Use(core.AdminOnlyMiddleware(adminChecker, logger))
	bot.Use(core.LoggerMiddleware(logger))
	bot.Use(core.PanicRecoveryMiddleware(logger))

	// Создаём менеджер wizard'ов (FSM для интерактивных команд).
	// Manager передаётся в initModules — там TextInterceptMiddleware встанет
	// ПЕРЕД statistics/limiter/reactions, чтобы поглощать wizard-ввод.
	wizardMgr := wizard.NewManager(bot, db, adminChecker, logger)
	wizardMgr.RegisterCancelHandler(bot)

	// Создаём все модули
	modules, err := initModules(db, bot, logger, cfg, wizardMgr)
	if err != nil {
		return fmt.Errorf("failed to init modules: %w", err)
	}

	// Создаём репозитории для хендлеров
	chatRepo := repositories.NewChatRepository(db)
	eventRepo := repositories.NewEventRepository(db)
	settingsRepo := repositories.NewSettingsRepository(db)

	// Получаем версию бота из БД. Источник истины — bot_settings.bot_version,
	// который выставляется миграциями. Fallback "unknown" должен срабатывать
	// только при повреждённой БД — в этом случае выводится warning выше.
	botVersion, err := settingsRepo.GetVersion()
	if err != nil {
		logger.Warn("failed to get bot version from DB, using fallback",
			zap.Error(err),
		)
		botVersion = "unknown"
	}

	// Регистрируем wizard'ы (инлайн-кнопки и text-router).
	// startWelcomeWizard — точка входа для /welcome без аргументов.
	startWelcomeWizard := wizard.RegisterWelcome(bot, wizardMgr, chatRepo, logger)

	// startSetProfanityWizard — точка входа для /setprofanity без аргументов.
	// Wizard package не знает про reactions, поэтому SQL для profanity_settings
	// дублируется (см. internal/wizard/profanity.go комментарий о синхронизации).
	startSetProfanityWizard := wizard.RegisterSetProfanity(bot, wizardMgr, db, chatRepo, eventRepo, logger)

	// Регистрируем базовые команды
	registerCommands(bot, chatRepo, eventRepo, logger, botVersion, startWelcomeWizard)

	// Перерегистрируем /setprofanity на двухрежимный handler (wizard | legacy).
	// initModules уже зарегистрировал legacy handler через
	// modules.Reactions.RegisterAdminCommands; здесь мы перетираем endpoint
	// на обёртку, которая запускает wizard при пустых аргументах в группе
	// и не от anonymous-админа, а в остальных случаях вызывает legacy.
	bot.Handle("/setprofanity", wrapSetProfanityWithWizard(modules.Reactions.HandleSetProfanity, startSetProfanityWizard))

	// Создаём контекст для graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Слушаем сигналы для graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Запускаем HTTP health-сервер для Docker healthcheck
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	healthServer := &http.Server{Addr: cfg.MetricsAddr, Handler: healthMux}
	go func() {
		logger.Info("health server started", zap.String("addr", cfg.MetricsAddr))
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server failed", zap.Error(err))
		}
	}()

	// Запускаем бота в отдельной горутине
	go func() {
		logger.Info("bot started, polling for updates...")
		bot.Start()
	}()

	// Ждём сигнала остановки
	sig := <-sigChan
	logger.Info("received shutdown signal", zap.String("signal", sig.String()))

	// Graceful shutdown с реальным таймаутом
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, cfg.ShutdownTimeout)
	defer shutdownCancel()

	// Канал для отслеживания завершения shutdown
	done := make(chan struct{})
	go func() {
		logger.Info("shutting down health server...")
		if err := healthServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("failed to shutdown health server", zap.Error(err))
		}

		logger.Info("shutting down bot...")
		bot.Stop()

		logger.Info("shutting down modules...")
		if err := modules.shutdownModules(logger); err != nil {
			logger.Error("failed to shutdown modules", zap.Error(err))
		}

		// db.Close() вызывается через defer в run() — дублировать не нужно

		close(done)
	}()

	select {
	case <-shutdownCtx.Done():
		logger.Warn("shutdown timeout exceeded")
		return fmt.Errorf("shutdown timeout exceeded")
	case <-done:
		logger.Info("bot shutdown complete")
		return nil
	}
}
