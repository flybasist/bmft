package main

import (
	"fmt"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// registerCommands регистрирует все команды бота.
// Хендлеры для базовых команд: /start, /help, /version.
func registerCommands(
	bot *tele.Bot,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	botVersion string,
) {
	// /version — информация о версии бота
	bot.Handle("/version", handleVersion(botVersion))

	// OnUserJoined — приветствие новых пользователей и бота
	bot.Handle(tele.OnUserJoined, handleUserJoined(chatRepo, logger))

	// /start — приветствие
	bot.Handle("/start", handleStart(chatRepo, eventRepo, logger))

	// /help — помощь
	bot.Handle("/help", handleHelp(logger))

	// Универсальный обработчик для всех типов сообщений.
	// Хендлеры нужны для активации middleware (bot.Use).
	// Сами хендлеры ничего не делают — вся логика в middleware pipeline.
	noOpHandler := func(c tele.Context) error { return nil }

	bot.Handle(tele.OnText, noOpHandler)
	bot.Handle(tele.OnVoice, noOpHandler)
	bot.Handle(tele.OnPhoto, noOpHandler)
	bot.Handle(tele.OnVideo, noOpHandler)
	bot.Handle(tele.OnSticker, noOpHandler)
	bot.Handle(tele.OnDocument, noOpHandler)
	bot.Handle(tele.OnAudio, noOpHandler)
	bot.Handle(tele.OnAnimation, noOpHandler)
	bot.Handle(tele.OnVideoNote, noOpHandler)
	bot.Handle(tele.OnLocation, noOpHandler)
	bot.Handle(tele.OnContact, noOpHandler)
	bot.Handle(tele.OnPoll, noOpHandler)
}

// handleVersion возвращает хендлер для команды /version
func handleVersion(botVersion string) func(tele.Context) error {
	return func(c tele.Context) error {
		answer := fmt.Sprintf(
			"🤖 BMFT v%s\n\n"+
				"По всем вопросам писать автору бота — @FlyBasist\n"+
				"Используйте /help для списка всех команд.",
			botVersion,
		)
		return c.Send(answer)
	}
}

// handleUserJoined возвращает хендлер для события OnUserJoined
func handleUserJoined(chatRepo *repositories.ChatRepository, logger *zap.Logger) func(tele.Context) error {
	return func(c tele.Context) error {
		newMember := c.Message().UserJoined

		// Если бот добавлен в чат
		if newMember.ID == c.Bot().Me.ID {
			// Создаём запись чата в БД при добавлении бота.
			// CheckIsForum делает API-запрос getChat, т.к. telebot.v3 v3.3.8
			// не экспортирует IsForum в Chat struct.
			// Критично для работы топиков (GetThreadID проверяет is_forum из БД).
			chatType := string(c.Chat().Type)
			title := c.Chat().Title
			chatUsername := c.Chat().Username
			isForum := core.CheckIsForum(c.Bot(), c.Chat().ID)
			if err := chatRepo.GetOrCreate(c.Chat().ID, chatType, title, chatUsername, isForum); err != nil {
				logger.Error("failed to create chat on bot join", zap.Error(err))
			}

			answer := "👋 Всем привет! Я BMFT (Bot Moderator For Telegram) — ваш новый помощник в управлении чатом!\n\n" +
				"🔹 Автоматическая статистика активности\n" +
				"🔹 Лимиты на контент (фото, видео, стикеры)\n" +
				"🔹 Автоответы, фильтры и модерация контента\n" +
				"🔹 Запланированные задачи по расписанию\n\n" +
				"Используйте /help для списка всех команд.\n" +
				"Администраторы могут настраивать модули самостоятельно.\n\n" +
				"💬 Автор бота: @FlyBasist"
			return c.Send(answer)
		}

		// Приветствие обычного пользователя
		username := newMember.Username
		var answer string

		if username != "" {
			// Есть никнейм - стандартное приветствие
			answer = fmt.Sprintf(
				"👋 Привет, @%s! Добро пожаловать в наш чат!\n\n"+
					"Капча для новых пользователей в разработке, "+
					"поэтому если ты спамер то удались сам пожалуйста 😊",
				username,
			)
		} else {
			// Нет никнейма - альтернативное приветствие
			firstName := newMember.FirstName
			if firstName == "" {
				firstName = "Пользователь"
			}
			answer = fmt.Sprintf(
				"👋 В чат зашёл %s, который предпочёл не использовать никнейм.\n\n"+
					"Но его данные надёжно записаны в базу для истории! 📝",
				firstName,
			)
		}

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

		// Создаём запись чата в БД (is_forum критичен для работы топиков)
		chatType := string(c.Chat().Type)
		title := c.Chat().Title
		username := c.Chat().Username
		isForum := core.CheckIsForum(c.Bot(), c.Chat().ID)
		if err := chatRepo.GetOrCreate(c.Chat().ID, chatType, title, username, isForum); err != nil {
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

🤖 Модули бота (работают автоматически):

🔹 statistics — статистика активности
   Собирает данные о сообщениях пользователей
   📌 /statistics, /myweek, /chatstats, /topchat

🔹 limiter — контроль лимитов контента
   Ограничивает фото, видео, стикеры и т.д.
   📌 /limiter, /setlimit, /mystats, /getlimit
   📌 /setvip, /removevip, /listvips

🔹 reactions — реакции, фильтры и модерация
   Автоответы, фильтрация слов и мата
   📌 /reactions — автоответы на ключевые слова
      /addreaction, /listreactions, /removereaction
   📌 /textfilter — фильтр запрещённых слов
      /addban, /listbans, /removeban
   📌 /profanity — фильтр ненормативной лексики
      /setprofanity, /profanitystatus, /removeprofanity

🔹 scheduler — запланированные задачи
   Выполняет задачи по расписанию (cron)
   📌 /scheduler, /addtask, /listtasks, /deltask, /runtask

💡 Используйте команду модуля (например /reactions) для подробной справки.`

		return c.Send(helpMsg)
	}
}
