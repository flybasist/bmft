package main

import (
	"database/sql"
	"fmt"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// registerCommands регистрирует все команды бота.
// Русский комментарий: Хендлеры для базовых команд: /start, /help, /modules, /enable, /disable, /version.
func registerCommands(
	bot *tele.Bot,
	modules *Modules,
	chatRepo *repositories.ChatRepository,
	moduleRepo *repositories.ModuleRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	db *sql.DB,
	botVersion string,
) {
	// /version — информация о версии бота
	bot.Handle("/version", handleVersion(botVersion))

	// OnUserJoined — приветствие новых пользователей и бота
	bot.Handle(tele.OnUserJoined, handleUserJoined())

	// /start — приветствие
	bot.Handle("/start", handleStart(chatRepo, eventRepo, logger))

	// /help — помощь
	bot.Handle("/help", handleHelp(logger))

	// /modules — показать доступные модули
	bot.Handle("/modules", handleModules(bot, modules, moduleRepo, logger))

	// /enable <module> — включить модуль
	bot.Handle("/enable", handleEnable(bot, moduleRepo, eventRepo, logger))

	// /disable <module> — выключить модуль
	bot.Handle("/disable", handleDisable(bot, moduleRepo, eventRepo, logger))

	// Универсальный обработчик для всех типов сообщений
	handleAll := handleAllMessages(bot, db, modules, moduleRepo, logger)

	bot.Handle(tele.OnText, handleAll)
	bot.Handle(tele.OnVoice, handleAll)
	bot.Handle(tele.OnPhoto, handleAll)
	bot.Handle(tele.OnVideo, handleAll)
	bot.Handle(tele.OnSticker, handleAll)
	bot.Handle(tele.OnDocument, handleAll)
	bot.Handle(tele.OnAudio, handleAll)
	bot.Handle(tele.OnAnimation, handleAll)
	bot.Handle(tele.OnVideoNote, handleAll)
	bot.Handle(tele.OnLocation, handleAll)
	bot.Handle(tele.OnContact, handleAll)
	bot.Handle(tele.OnPoll, handleAll)

	// Обработчик отредактированных сообщений
	// Русский комментарий: Аналог Python @bot.edited_message_handler()
	// Python: telegrambot.py::handle_edited_message() — обрабатывает точно так же как новое сообщение
	bot.Handle(tele.OnEdited, handleEdited(bot, db, modules, moduleRepo, logger))
}

// handleVersion возвращает хендлер для команды /version
func handleVersion(botVersion string) func(tele.Context) error {
	return func(c tele.Context) error {
		answer := fmt.Sprintf(
			"Текущая версия - %s\n"+
				"По всем вопросам писать автору бота - @FlyBasist\n"+
				"Индивидуальная реакция стикером не чаще одного раза в десять минут\n"+
				"Разработка бота требует ресурсов, поддержи разработку донатом!",
			botVersion,
		)
		return c.Send(answer)
	}
}

// handleUserJoined возвращает хендлер для события OnUserJoined
func handleUserJoined() func(tele.Context) error {
	return func(c tele.Context) error {
		newMember := c.Message().UserJoined

		// Если бот добавлен в чат
		if newMember.ID == c.Bot().Me.ID {
			answer := "Всем привет! Я ваш новый бот! " +
				"Пока все индивидуальные настройки под чат задаются через @FlyBasist " +
				"но потом меня можно будет настраивать владельцу чата самостоятельно"
			return c.Send(answer)
		}

		// Приветствие обычного пользователя
		username := newMember.Username
		if username == "" {
			username = newMember.FirstName
		}
		answer := fmt.Sprintf(
			"Привет, @%s! Добро пожаловать в наш чат! "+
				"Капча для новых пользователей в разработке, "+
				"поэтому если ты спамер то удались сам пожалуйста",
			username,
		)
		return c.Send(answer)
	}
}

// handleStart возвращает хендлер для команды /start
func handleStart(
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(tele.Context) error {
	return func(c tele.Context) error {
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

/help — список всех команд

Добавьте меня в группу и дайте права администратора для полной функциональности!`

		return c.Send(welcomeMsg)
	}
}

// handleHelp возвращает хендлер для команды /help
func handleHelp(logger *zap.Logger) func(tele.Context) error {
	return func(c tele.Context) error {
		logger.Info("handling /help command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID),
		)

		helpMsg := `📖 Доступные команды:

🔹 Основные:
/start — приветствие и инициализация
/help — эта справка
/version — информация о версии бота

🔹 Управление модулями (только админы):
/modules — показать все модули
/enable <module> — включить модуль
/disable <module> — выключить модуль

🔹 Доступные модули: используйте /modules для подробностей`

		return c.Send(helpMsg)
	}
}

// handleModules возвращает хендлер для команды /modules
func handleModules(
	bot *tele.Bot,
	modules *Modules,
	moduleRepo *repositories.ModuleRepository,
	logger *zap.Logger,
) func(tele.Context) error {
	return func(c tele.Context) error {
		logger.Info("handling /modules command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID),
		)

		// Проверка прав админа (только для групп)
		if c.Chat().Type == tele.ChatGroup || c.Chat().Type == tele.ChatSuperGroup {
			if !isAdmin(bot, c, logger) {
				logger.Warn("user is not admin",
					zap.Int64("chat_id", c.Chat().ID),
					zap.Int64("user_id", c.Sender().ID),
				)
				return c.Send("❌ Эта команда доступна только администраторам чата.")
			}
		}

		// Явный список модулей и их команд
		modulesList := map[string][]core.BotCommand{
			"limiter":    modules.Limiter.Commands(),
			"statistics": modules.Statistics.Commands(),
			"reactions":  modules.Reactions.Commands(),
			"scheduler":  modules.Scheduler.Commands(),
			"textfilter": modules.TextFilter.Commands(),
		}

		msg := "📦 Доступные модули:\n\n"
		for name, commands := range modulesList {
			// Проверяем включен ли модуль для этого чата
			enabled, _ := moduleRepo.IsEnabled(c.Chat().ID, name)
			status := "❌ Выключен"
			if enabled {
				status = "✅ Включен"
			}

			// Описание модуля
			var description string
			switch name {
			case "statistics":
				description = "статистика активности пользователей"
			case "limiter":
				description = "лимиты на контент с предупреждениями"
			case "scheduler":
				description = "задачи по расписанию (автоматическая отправка сообщений по cron)"
			case "reactions":
				description = "автоматические реакции на ключевые слова"
			case "textfilter":
				description = "фильтр запрещённых слов"
			default:
				description = "модуль"
			}

			msg += fmt.Sprintf("🔹 **%s** — %s\n  %s\n", name, status, description)
			if len(commands) > 0 {
				msg += "  Команды:\n"
				for _, cmd := range commands {
					var help string
					switch cmd.Command {
					case "/mystats":
						help = "показать вашу статистику"
					case "/myweek":
						help = "статистика за неделю"
					case "/mymonth":
						help = "статистика за месяц"
					case "/topweek":
						help = "топ пользователей за неделю"
					case "/topmonth":
						help = "топ пользователей за месяц"
					case "/resetstats":
						help = "сбросить статистику (админ)"
					case "/setlimit":
						help = "установить лимит (type: text/photo/video/sticker/animation/voice/document/audio/location/contact)"
					case "/mylimits":
						help = "показать ваши лимиты"
					case "/resetlimits":
						help = "сбросить лимиты (админ)"
					case "/addtask":
						help = "добавить задачу (cron) - /addtask <cron> <текст> или ответьте на сообщение с /addtask <cron>"
					case "/listtasks":
						help = "список задач"
					case "/removetask":
						help = "удалить задачу"
					case "/addreaction":
						help = "добавить реакцию на слово"
					case "/listreactions":
						help = "список реакций"
					case "/removereaction":
						help = "удалить реакцию"
					case "/addban":
						help = "добавить запрещённое слово"
					case "/listbans":
						help = "список запрещённых слов"
					case "/removeban":
						help = "удалить запрещённое слово"
					default:
						help = ""
					}
					if help != "" {
						msg += fmt.Sprintf("    %s - %s\n", cmd.Command, help)
					} else {
						msg += fmt.Sprintf("    %s\n", cmd.Command)
					}
				}
				// Дополнительная подсказка для scheduler
				if name == "scheduler" {
					msg += "  Подсказка: /addtask <cron> <текст> или reply на сообщение с /addtask <cron>\n"
					msg += "  Примеры cron: '0 9 * * *' (каждый день в 9:00), '*/30 * * * *' (каждые 30 мин)\n"
				}
			}
			msg += "\n"
		}

		return c.Send(msg)
	}
}

// handleEnable возвращает хендлер для команды /enable
func handleEnable(
	bot *tele.Bot,
	moduleRepo *repositories.ModuleRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(tele.Context) error {
	return func(c tele.Context) error {
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
			if !isAdmin(bot, c, logger) {
				return c.Send("❌ Эта команда доступна только администраторам чата.")
			}
		}

		// Проверяем что модуль существует
		validModules := map[string]bool{
			"limiter":    true,
			"statistics": true,
			"reactions":  true,
			"scheduler":  true,
			"textfilter": true,
		}
		if !validModules[moduleName] {
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
	}
}

// handleDisable возвращает хендлер для команды /disable
func handleDisable(
	bot *tele.Bot,
	moduleRepo *repositories.ModuleRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
) func(tele.Context) error {
	return func(c tele.Context) error {
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
			if !isAdmin(bot, c, logger) {
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
	}
}

// handleAllMessages возвращает универсальный хендлер для всех типов сообщений
func handleAllMessages(
	bot *tele.Bot,
	db *sql.DB,
	modules *Modules,
	moduleRepo *repositories.ModuleRepository,
	logger *zap.Logger,
) func(tele.Context) error {
	return func(c tele.Context) error {
		ctx := &core.MessageContext{
			Message: c.Message(),
			Bot:     bot,
			DB:      db,
			Logger:  logger,
			Chat:    c.Chat(),
			Sender:  c.Sender(),
		}
		if err := processMessage(ctx, modules, moduleRepo, logger); err != nil {
			logger.Error("failed to process message in modules", zap.Error(err))
		}
		return nil
	}
}

// handleEdited возвращает хендлер для отредактированных сообщений
func handleEdited(
	bot *tele.Bot,
	db *sql.DB,
	modules *Modules,
	moduleRepo *repositories.ModuleRepository,
	logger *zap.Logger,
) func(tele.Context) error {
	return func(c tele.Context) error {
		// Создаём MessageContext для модулей
		ctx := &core.MessageContext{
			Message: c.Message(),
			Bot:     bot,
			DB:      db,
			Logger:  logger,
			Chat:    c.Chat(),
			Sender:  c.Sender(),
		}

		// Передаём отредактированное сообщение всем активным модулям
		// Python бот обрабатывает edited_message идентично новому сообщению
		if err := processMessage(ctx, modules, moduleRepo, logger); err != nil {
			logger.Error("failed to process edited message in modules", zap.Error(err))
		}

		return nil
	}
}

// isAdmin проверяет, является ли отправитель администратором чата
func isAdmin(bot *tele.Bot, c tele.Context, logger *zap.Logger) bool {
	admins, err := bot.AdminsOf(c.Chat())
	if err != nil {
		logger.Error("failed to get admins", zap.Error(err))
		return false
	}

	logger.Info("admin check",
		zap.Int64("chat_id", c.Chat().ID),
		zap.Int64("user_id", c.Sender().ID),
		zap.Int("admins_count", len(admins)),
	)

	for _, admin := range admins {
		logger.Info("checking admin",
			zap.Int64("admin_id", admin.User.ID),
			zap.String("admin_username", admin.User.Username),
		)
		if admin.User.ID == c.Sender().ID {
			return true
		}
	}

	return false
}
