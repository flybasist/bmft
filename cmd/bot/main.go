package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/logx"
	"github.com/flybasist/bmft/internal/migrations"
	"github.com/flybasist/bmft/internal/modules/limiter"
	"github.com/flybasist/bmft/internal/modules/reactions"
	"github.com/flybasist/bmft/internal/modules/scheduler"
	"github.com/flybasist/bmft/internal/modules/statistics"
	"github.com/flybasist/bmft/internal/postgresql"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

func main() {
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

	// Инициализируем structured logger (zap)
	logger, err := logx.NewLogger(cfg.LogLevel, cfg.LogPretty)
	if err != nil {
		return fmt.Errorf("failed to init logger: %w", err)
	}
	defer logger.Sync()

	logger.Info("starting bmft bot",
		zap.String("log_level", cfg.LogLevel),
		zap.Bool("log_pretty", cfg.LogPretty),
		zap.Duration("shutdown_timeout", cfg.ShutdownTimeout),
		zap.Int("polling_timeout", cfg.PollingTimeout),
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

	// Автоматически применяем миграции (или валидируем существующую схему)
	if err := migrations.RunMigrationsIfNeeded(db, logger); err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}
	logger.Info("database schema ready")

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

	// Создаём зависимости для модулей (DI контейнер)
	deps := core.ModuleDependencies{
		DB:     db,
		Bot:    bot,
		Logger: logger,
		Config: cfg,
	}

	// Создаём Module Registry
	registry := core.NewRegistry(deps)

	// Инициализируем все зарегистрированные модули
	if err := registry.InitAll(); err != nil {
		return fmt.Errorf("failed to init modules: %w", err)
	}

	// Создаём репозитории для использования в хендлерах
	chatRepo := repositories.NewChatRepository(db)
	moduleRepo := repositories.NewModuleRepository(db)
	eventRepo := repositories.NewEventRepository(db)
	limitRepo := repositories.NewLimitRepository(db, logger)

	// Создаём и регистрируем модуль лимитов
	limiterModule := limiter.New(limitRepo, logger)
	// TODO: Загружать список админов из конфига
	adminUsers := []int64{} // Пока пустой список, заполнить потом
	limiterModule.SetAdminUsers(adminUsers)

	registry.Register("limiter", limiterModule)

	// Регистрируем команды модуля лимитов
	limiterModule.RegisterCommands(bot)
	limiterModule.RegisterAdminCommands(bot)

	// Создаём и регистрируем модуль реакций (Phase 3)
	reactionsModule := reactions.New(db, moduleRepo, eventRepo, logger)
	reactionsModule.SetAdminUsers(adminUsers)

	registry.Register("reactions", reactionsModule)

	// Регистрируем команды модуля реакций
	reactionsModule.RegisterAdminCommands(bot)

	// Создаём и регистрируем модуль статистики (Phase 4)
	statsRepo := repositories.NewStatisticsRepository(db, logger)
	statisticsModule := statistics.New(db, statsRepo, moduleRepo, eventRepo, logger)
	statisticsModule.SetAdminUsers(adminUsers)

	registry.Register("statistics", statisticsModule)

	// Регистрируем команды модуля статистики
	statisticsModule.RegisterCommands(bot)
	statisticsModule.RegisterAdminCommands(bot)

	// Создаём и регистрируем модуль планировщика (Phase 5)
	schedulerRepo := repositories.NewSchedulerRepository(db, logger)
	schedulerModule := scheduler.New(db, schedulerRepo, moduleRepo, eventRepo, logger)
	schedulerModule.SetAdminUsers(adminUsers)

	registry.Register("scheduler", schedulerModule)

	// Регистрируем команды модуля планировщика
	schedulerModule.RegisterCommands(bot)
	schedulerModule.RegisterAdminCommands(bot)

	// Регистрируем middleware
	bot.Use(core.LoggerMiddleware(logger))
	bot.Use(core.PanicRecoveryMiddleware(logger))
	bot.Use(core.RateLimitMiddleware(logger))

	// Регистрируем базовые команды
	registerCommands(bot, registry, chatRepo, moduleRepo, eventRepo, logger, db)

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
	if err := registry.ShutdownAll(); err != nil {
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

// registerCommands регистрирует все команды бота.
// Русский комментарий: Хендлеры для базовых команд: /start, /help, /modules, /enable, /disable.
func registerCommands(
	bot *tele.Bot,
	registry *core.ModuleRegistry,
	chatRepo *repositories.ChatRepository,
	moduleRepo *repositories.ModuleRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	db *sql.DB,
) {
	// /start — приветствие
	bot.Handle("/start", func(c tele.Context) error {
		logger.Info("handling /start command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID),
		)

		// Создаём запись чата в БД
		chatType := string(c.Chat().Type)
		title := c.Chat().Title
		username := c.Chat().Username
		if err := chatRepo.GetOrCreate(c.Chat().ID, chatType, title, username); err != nil {
			logger.Error("failed to create chat", zap.Error(err))
			return c.Send("Произошла ошибка при инициализации чата.")
		}

		// Логируем событие
		_ = eventRepo.Log(c.Chat().ID, c.Sender().ID, "core", "start_command", "User started bot")

		welcomeMsg := `🤖 Привет! Я BMFT — модульный бот для управления Telegram-чатами.

📋 Основные команды:
/help — список всех команд
/modules — показать доступные модули (только админы)
/enable <module> — включить модуль (только админы)
/disable <module> — выключить модуль (только админы)

Добавьте меня в группу и дайте права администратора для полной функциональности!`

		return c.Send(welcomeMsg)
	})

	// /help — помощь
	bot.Handle("/help", func(c tele.Context) error {
		logger.Info("handling /help command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID),
		)

		helpMsg := `📖 Доступные команды:

🔹 Основные:
/start — приветствие и инициализация
/help — эта справка

🔹 Управление модулями (только админы):
/modules — показать все модули
/enable <module> — включить модуль
/disable <module> — выключить модуль

🔹 Модули будут добавлены в Phase 2-6:
- limiter — лимиты на типы контента
- reactions — автоматические реакции
- statistics — статистика чата
- scheduler — задачи по расписанию
- antispam — антиспам фильтры`

		return c.Send(helpMsg)
	})

	// /modules — показать доступные модули
	bot.Handle("/modules", func(c tele.Context) error {
		logger.Info("handling /modules command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID),
		)

		// Проверка прав админа (только для групп)
		if c.Chat().Type == tele.ChatGroup || c.Chat().Type == tele.ChatSuperGroup {
			admins, err := bot.AdminsOf(c.Chat())
			if err != nil {
				logger.Error("failed to get admins", zap.Error(err))
				return c.Send("Не удалось проверить права администратора.")
			}

			isAdmin := false
			for _, admin := range admins {
				if admin.User.ID == c.Sender().ID {
					isAdmin = true
					break
				}
			}

			if !isAdmin {
				return c.Send("❌ Эта команда доступна только администраторам чата.")
			}
		}

		modules := registry.GetModules()
		if len(modules) == 0 {
			return c.Send("📦 Модули пока не зарегистрированы. Будут добавлены в Phase 2-6.")
		}

		msg := "📦 Доступные модули:\n\n"
		for name, commands := range modules {
			// Проверяем включен ли модуль для этого чата
			enabled, _ := moduleRepo.IsEnabled(c.Chat().ID, name)
			status := "❌ Выключен"
			if enabled {
				status = "✅ Включен"
			}

			msg += fmt.Sprintf("🔹 **%s** — %s\n", name, status)
			if len(commands) > 0 {
				msg += "  Команды: "
				for i, cmd := range commands {
					if i > 0 {
						msg += ", "
					}
					msg += cmd.Command
				}
				msg += "\n"
			}
			msg += "\n"
		}

		return c.Send(msg)
	})

	// /enable <module> — включить модуль
	bot.Handle("/enable", func(c tele.Context) error {
		args := c.Args()
		if len(args) == 0 {
			return c.Send("Использование: /enable <module_name>")
		}

		moduleName := args[0]

		logger.Info("handling /enable command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID),
			zap.String("module", moduleName),
		)

		// Проверка прав админа
		if c.Chat().Type == tele.ChatGroup || c.Chat().Type == tele.ChatSuperGroup {
			admins, err := bot.AdminsOf(c.Chat())
			if err != nil {
				logger.Error("failed to get admins", zap.Error(err))
				return c.Send("Не удалось проверить права администратора.")
			}

			isAdmin := false
			for _, admin := range admins {
				if admin.User.ID == c.Sender().ID {
					isAdmin = true
					break
				}
			}

			if !isAdmin {
				return c.Send("❌ Эта команда доступна только администраторам чата.")
			}
		}

		// Проверяем что модуль зарегистрирован
		if _, exists := registry.GetModule(moduleName); !exists {
			return c.Send(fmt.Sprintf("❌ Модуль '%s' не найден. Используйте /modules для просмотра доступных модулей.", moduleName))
		}

		// Включаем модуль
		if err := moduleRepo.Enable(c.Chat().ID, moduleName); err != nil {
			logger.Error("failed to enable module", zap.Error(err))
			return c.Send("Произошла ошибка при включении модуля.")
		}

		// Логируем событие
		_ = eventRepo.Log(c.Chat().ID, c.Sender().ID, "core", "module_enabled", fmt.Sprintf("Module %s enabled", moduleName))

		return c.Send(fmt.Sprintf("✅ Модуль '%s' включен для этого чата.", moduleName))
	})

	// /disable <module> — выключить модуль
	bot.Handle("/disable", func(c tele.Context) error {
		args := c.Args()
		if len(args) == 0 {
			return c.Send("Использование: /disable <module_name>")
		}

		moduleName := args[0]

		logger.Info("handling /disable command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID),
			zap.String("module", moduleName),
		)

		// Проверка прав админа
		if c.Chat().Type == tele.ChatGroup || c.Chat().Type == tele.ChatSuperGroup {
			admins, err := bot.AdminsOf(c.Chat())
			if err != nil {
				logger.Error("failed to get admins", zap.Error(err))
				return c.Send("Не удалось проверить права администратора.")
			}

			isAdmin := false
			for _, admin := range admins {
				if admin.User.ID == c.Sender().ID {
					isAdmin = true
					break
				}
			}

			if !isAdmin {
				return c.Send("❌ Эта команда доступна только администраторам чата.")
			}
		}

		// Выключаем модуль
		if err := moduleRepo.Disable(c.Chat().ID, moduleName); err != nil {
			logger.Error("failed to disable module", zap.Error(err))
			return c.Send("Произошла ошибка при выключении модуля.")
		}

		// Логируем событие
		_ = eventRepo.Log(c.Chat().ID, c.Sender().ID, "core", "module_disabled", fmt.Sprintf("Module %s disabled", moduleName))

		return c.Send(fmt.Sprintf("❌ Модуль '%s' выключен для этого чата.", moduleName))
	})

	// Обработчик всех остальных сообщений — передаём модулям
	bot.Handle(tele.OnText, func(c tele.Context) error {
		// Создаём MessageContext для модулей
		ctx := &core.MessageContext{
			Message: c.Message(),
			Bot:     bot,
			DB:      db,
			Logger:  logger,
			Chat:    c.Chat(),
			Sender:  c.Sender(),
		}

		// Передаём сообщение всем активным модулям
		if err := registry.OnMessage(ctx); err != nil {
			logger.Error("failed to process message in modules", zap.Error(err))
		}

		return nil
	})
}
