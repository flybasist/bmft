package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// welcomeMessageTTL — дефолтный fallback, если в БД нет записи про чат. Реальный TTL
// берётся из chats.welcome_ttl_seconds (per-chat, настраивается /welcome ttl <сек>).
// Сообщение авто-удаляется, чтобы после массового захода спам-ботов
// чат не оказался забит десятками сообщений-приветствий
// (сами спамеры будут удалены сторонними анти-спам ботами раньше).
// История захода всё равно видна в админке Telegram.
const welcomeMessageTTL = 5 * time.Minute

// registerCommands регистрирует все команды бота.
// Хендлеры для базовых команд: /start, /help, /version.
//
// startWelcomeWizard — функция, возвращённая wizard.RegisterWelcome(...).
// /welcome без аргументов в группе будет запускать wizard — все проверки
// (anonymous-админ, личка) сделает сам Manager.Start.
// Старый синтаксис (/welcome on|off|ttl) продолжает работать как раньше.
func registerCommands(
	bot *tele.Bot,
	chatRepo *repositories.ChatRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	botVersion string,
	startWelcomeWizard func(c tele.Context) error,
) {
	// /version — информация о версии бота
	bot.Handle("/version", handleVersion(botVersion))

	// OnUserJoined — приветствие новых пользователей и бота
	bot.Handle(tele.OnUserJoined, handleUserJoined(chatRepo, logger))

	// /start — приветствие
	bot.Handle("/start", handleStart(chatRepo, eventRepo, logger))

	// /welcome — управление приветствием (admin-only через AdminOnlyMiddleware).
	bot.Handle("/welcome", handleWelcome(chatRepo, logger, startWelcomeWizard))

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

			answer := "👋 Привет! Я <b>BMFT</b> — модульный бот для модерации и автоматизации чата.\n\n" +
				"Что умею: статистика, лимиты на контент, автоответы, фильтр мата, задачи по расписанию.\n\n" +
				"Дайте мне права администратора и нажмите /help — там кнопками сгруппированы все команды.\n\n" +
				"💬 Автор: @FlyBasist"
			return c.Send(answer, &tele.SendOptions{ParseMode: tele.ModeHTML})
		}

		// Приветствие обычного пользователя.
		// Настройки (enabled + ttl) берутся из chats; админы управляют ими через /welcome.
		settings, err := chatRepo.GetWelcomeSettings(c.Chat().ID)
		if err != nil {
			logger.Warn("failed to load welcome settings, using defaults",
				zap.Error(err), zap.Int64("chat_id", c.Chat().ID))
			settings = repositories.WelcomeSettings{Enabled: true, TTLSeconds: int(welcomeMessageTTL.Seconds())}
		}
		if !settings.Enabled {
			return nil
		}

		username := newMember.Username
		var answer string

		if username != "" {
			answer = fmt.Sprintf("👋 Привет, @%s! Добро пожаловать в наш чат.", username)
		} else {
			firstName := newMember.FirstName
			if firstName == "" {
				firstName = "Пользователь"
			}
			answer = fmt.Sprintf("👋 Привет, %s! Добро пожаловать в наш чат.", firstName)
		}

		// Используем bot.Send (а не c.Send), чтобы получить *Message и
		// запланировать его удаление по таймеру.
		sentMsg, sendErr := c.Bot().Send(c.Chat(), answer)
		if sendErr != nil {
			return sendErr
		}
		if settings.TTLSeconds > 0 {
			core.ScheduleDelete(c.Bot(), sentMsg, time.Duration(settings.TTLSeconds)*time.Second, logger)
		}
		return nil
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

// ──────────────────────────────────────────────────────────────────────────────
// handleWelcome — управление приветствием новых пользователей (admin-only).
//
// Использование:
//
//	/welcome           — в группе: запуск wizard'а с inline-кнопками
//	                     (anonymous админ / личка → fallback на показ настроек)
//	/welcome on|off    — включить/выключить приветствие (старый синтаксис)
//	/welcome ttl <сек> — задать TTL авто-удаления (старый синтаксис)
//
// Старый синтаксис сохраняется для скриптов, анонимных админов и быстрых изменений.
func handleWelcome(chatRepo *repositories.ChatRepository, logger *zap.Logger, startWizard func(c tele.Context) error) func(tele.Context) error {
	return func(c tele.Context) error {
		args := c.Args()
		chatID := c.Chat().ID

		if len(args) == 0 {
			// /welcome без аргументов в группе и не от anonymous админа → wizard.
			// В личке или для anonymous админа startWizard сам отправит explanation
			// и не создаст state — это обрабатывает wizard.Manager.Start.
			chat := c.Chat()
			if startWizard != nil && chat != nil && (chat.Type == tele.ChatGroup || chat.Type == tele.ChatSuperGroup) && !core.IsAnonymousAdmin(c.Message()) {
				return startWizard(c)
			}

			// Fallback: показ настроек в текстовом виде (личка / anonymous админ).
			s, err := chatRepo.GetWelcomeSettings(chatID)
			if err != nil {
				logger.Error("welcome: get settings", zap.Error(err), zap.Int64("chat_id", chatID))
				return c.Send("Не удалось прочитать настройки.")
			}
			state := "выключено"
			if s.Enabled {
				state = "включено"
			}
			ttlNote := fmt.Sprintf("%d сек", s.TTLSeconds)
			if s.TTLSeconds == 0 {
				ttlNote = "без авто-удаления"
			}
			return c.Send(fmt.Sprintf(
				"Приветствие: %s\nTTL: %s\n\n"+
					"Команды:\n"+
					"/welcome on — включить\n"+
					"/welcome off — выключить\n"+
					"/welcome ttl <секунды> — задать TTL (0 = не удалять)",
				state, ttlNote,
			))
		}

		switch strings.ToLower(args[0]) {
		case "on":
			if err := chatRepo.SetWelcomeEnabled(chatID, true); err != nil {
				logger.Error("welcome: set enabled", zap.Error(err), zap.Int64("chat_id", chatID))
				return c.Send("Не удалось сохранить настройку.")
			}
			return c.Send("✅ Приветствие включено.")
		case "off":
			if err := chatRepo.SetWelcomeEnabled(chatID, false); err != nil {
				logger.Error("welcome: set disabled", zap.Error(err), zap.Int64("chat_id", chatID))
				return c.Send("Не удалось сохранить настройку.")
			}
			return c.Send("✅ Приветствие выключено.")
		case "ttl":
			if len(args) < 2 {
				return c.Send("Укажите TTL в секундах: /welcome ttl 300 (0 — не удалять).")
			}
			ttl, err := strconv.Atoi(args[1])
			if err != nil || ttl < 0 {
				return c.Send("TTL должен быть целым числом ≥ 0.")
			}
			// Допустимый диапазон: 0 (отключено), либо 10..86400 секунд (24 часа).
			// 0..9 фактически означает «удалить мгновенно» — приветствие не успевает прочитать никто.
			// >24ч — у Telegram нет смысла, всё равно сообщение можно удалить ботом не дольше 48ч.
			if ttl != 0 && (ttl < 10 || ttl > 86400) {
				return c.Send("TTL должен быть 0 (без авто-удаления) либо в диапазоне 10..86400 секунд (24 часа).")
			}
			if err := chatRepo.SetWelcomeTTL(chatID, ttl); err != nil {
				logger.Error("welcome: set ttl", zap.Error(err), zap.Int64("chat_id", chatID))
				return c.Send("Не удалось сохранить настройку.")
			}
			if ttl == 0 {
				return c.Send("✅ Приветствие больше не удаляется автоматически.")
			}
			return c.Send(fmt.Sprintf("✅ TTL приветствия: %d сек.", ttl))
		default:
			return c.Send("Неизвестная подкоманда. Доступно: on, off, ttl <сек>.")
		}
	}
}
