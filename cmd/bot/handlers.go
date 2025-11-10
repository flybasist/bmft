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

		// Список модулей с описаниями (без команд)
		type moduleInfo struct {
			name        string
			description string
		}

		modulesList := []moduleInfo{
			{"statistics", "сбор и анализ статистики активности пользователей"},
			{"limiter", "контроль лимитов на различные типы контента (фото, видео, стикеры)"},
			{"reactions", "автоматические ответы на ключевые слова и триггеры"},
			{"scheduler", "запланированные задачи по расписанию (cron)"},
			{"textfilter", "фильтрация запрещённых слов и фраз"},
		}

		msg := "� **Доступные модули:**\n\n"
		msg += "Используйте /enable <имя_модуля> для включения.\n"
		msg += "Для просмотра команд модуля используйте /<имя_модуля>\n\n"

		for _, module := range modulesList {
			// Проверяем включен ли модуль для этого чата
			// Для команды /modules используем thread_id = 0 (настройки на уровне чата)
			enabled, _ := moduleRepo.IsEnabled(c.Chat().ID, 0, module.name)
			status := "❌"
			if enabled {
				status = "✅"
			}

			msg += fmt.Sprintf("%s **%s**\n   %s\n\n", status, module.name, module.description)
		}

		msg += "💡 *Подсказка:* После включения модуля используйте команду `/<имя_модуля>` для просмотра всех доступных команд с примерами."

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

		// Включаем модуль для всего чата (thread_id = 0)
		// Если нужно включить для конкретного топика, используйте команду в топике
		threadID := c.Message().ThreadID
		if err := moduleRepo.Enable(c.Chat().ID, threadID, moduleName); err != nil {
			logger.Error("failed to enable module", zap.Error(err))
			return c.Send("Произошла ошибка при включении модуля.")
		}

		// Логируем событие
		_ = eventRepo.Log(c.Chat().ID, c.Sender().ID, "core", "module_enabled", fmt.Sprintf("Module %s enabled", moduleName))

		location := "чата"
		if threadID != 0 {
			location = "топика"
		}
		return c.Send(fmt.Sprintf("✅ Модуль '%s' включен для этого %s.", moduleName, location))
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

		// Выключаем модуль (учитываем топик)
		threadID := c.Message().ThreadID
		if err := moduleRepo.Disable(c.Chat().ID, threadID, moduleName); err != nil {
			logger.Error("failed to disable module", zap.Error(err))
			return c.Send("Произошла ошибка при выключении модуля.")
		}

		// Логируем событие
		_ = eventRepo.Log(c.Chat().ID, c.Sender().ID, "core", "module_disabled", fmt.Sprintf("Module %s disabled", moduleName))

		location := "чата"
		if threadID != 0 {
			location = "топика"
		}
		return c.Send(fmt.Sprintf("❌ Модуль '%s' выключен для этого %s.", moduleName, location))
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
