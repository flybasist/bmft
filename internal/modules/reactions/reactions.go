package reactions

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

type ReactionsModule struct {
	db        *sql.DB
	vipRepo   *repositories.VIPRepository
	eventRepo *repositories.EventRepository
	logger    *zap.Logger
	bot       *telebot.Bot
}

type KeywordReaction struct {
	ID                 int64
	ChatID             int64
	ThreadID           int64
	UserID             int64 // 0 или NULL = для всех, >0 = только для конкретного пользователя (персональная реакция)
	Pattern            string
	ResponseType       string // "text", "sticker", "photo", etc.
	ResponseContent    string // text content or file_id
	Description        string
	TriggerContentType string // "" или NULL = любой контент, "photo" = только фото, "video" = только видео, etc.
	IsRegex            bool
	Cooldown           int
	DailyLimit         int
	DeleteOnLimit      bool
	IsActive           bool
}

func New(
	db *sql.DB,
	vipRepo *repositories.VIPRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	bot *telebot.Bot,
) *ReactionsModule {
	return &ReactionsModule{
		db:        db,
		vipRepo:   vipRepo,
		eventRepo: eventRepo,
		logger:    logger,
		bot:       bot,
	}
}

// RegisterCommands регистрирует команды модуля в боте.
func (m *ReactionsModule) RegisterCommands(bot *telebot.Bot) {
	// /reactions — справка по модулю
	bot.Handle("/reactions", func(c telebot.Context) error {
		msg := "🤖 <b>Модуль Reactions</b> — Автоматические реакции\n\n"
		msg += "Бот автоматически отвечает на ключевые слова в чате.\n\n"
		msg += "<b>Команды:</b>\n\n"

		msg += "🔹 <code>/addreaction</code> — Добавить реакцию (только админы)\n"
		msg += "🔹 <code>/listreactions</code> — Показать все реакции\n"
		msg += "🔹 <code>/removereaction &lt;ID&gt;</code> — Удалить реакцию\n\n"

		msg += "<b>КАК ДОБАВИТЬ РЕАКЦИЮ:</b>\n\n"

		msg += "<b>1️⃣ Текстовая реакция:</b>\n"
		msg += "🔸 Когда кто-то пишет <u>слово</u>, бот отвечает <u>текстом</u>\n\n"
		msg += "📝 <b>Формат:</b>\n"
		msg += "<code>/addreaction слово \"<u>текст ответа</u>\" \"<u>описание</u>\"</code>\n\n"
		msg += "📌 <b>Примеры:</b>\n"
		msg += "• <code>/addreaction привет \"Привет всем!\" \"Приветствие\"</code>\n"
		msg += "  → Кто-то пишет 'привет' → бот ответит 'Привет всем!'\n"
		msg += "• <code>/addreaction пельмени \"🥟 Ммм!\" \"Реакция\"</code>\n\n"

		msg += "<b>2️⃣ Реакция стикером/фото:</b>\n"
		msg += "🔸 Когда кто-то пишет <u>слово</u>, бот отвечает <u>стикером/фото</u>\n\n"
		msg += "📝 <b>Как добавить:</b>\n"
		msg += "1. Найдите стикер/фото в чате\n"
		msg += "2. Нажмите 'Ответить' на это сообщение\n"
		msg += "3. Напишите: <code>/addreaction слово описание</code>\n\n"
		msg += "📌 <b>Пример:</b>\n"
		msg += "• Ответьте на стикер и напишите:\n"
		msg += "  <code>/addreaction котики Котики_стикер</code>\n"
		msg += "  → Кто-то пишет 'котики' → бот отправит этот стикер\n\n"

		msg += "<b>⚙️ Дополнительные настройки:</b>\n"
		msg += "• Добавьте <code>photo</code> или <code>sticker</code> - реагировать только на этот тип\n"
		msg += "• Добавьте число - задержка в секундах (3600 = 1 час)\n\n"

		msg += "<b>👤 Персональная реакция для одного человека:</b>\n"
		msg += "<code>/addreaction user:123456 привет \"Здравствуй!\" \"Личное\"</code>\n"
		msg += "ℹ️ Узнать ID: перешлите сообщение боту @userinfobot\n\n"

		msg += "<b>📋 Чтобы посмотреть все реакции:</b>\n"
		msg += "<code>/listreactions</code> - покажет ID каждой реакции\n\n"

		msg += "⚠️ <b>Топики:</b> Команда в топике = реакция только в нём"

		return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
	})
}

// RegisterAdminCommands регистрирует админские команды.
func (m *ReactionsModule) RegisterAdminCommands(bot *telebot.Bot) {
	bot.Handle("/addreaction", m.handleAddReaction)
	bot.Handle("/listreactions", m.handleListReactions)
	bot.Handle("/removereaction", m.handleRemoveReaction)
}

func (m *ReactionsModule) OnMessage(ctx *core.MessageContext) error {
	msg := ctx.Message

	// Пропускаем приватные сообщения и команды
	if msg.Private() || (msg.Text != "" && strings.HasPrefix(msg.Text, "/")) {
		return nil
	}

	chatID := msg.Chat.ID
	threadID := core.GetThreadIDFromMessage(m.db, msg)
	userID := msg.Sender.ID

	m.logger.Debug("reactions OnMessage", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", userID), zap.String("text", msg.Text))

	isVIP, _ := m.vipRepo.IsVIP(chatID, threadID, userID)
	if isVIP {
		return nil
	}

	reactions, err := m.loadReactions(chatID, int64(threadID), userID)
	if err != nil {
		m.logger.Error("failed to load reactions", zap.Error(err))
		return nil
	}

	m.logger.Debug("loaded reactions", zap.Int("count", len(reactions)))

	for _, reaction := range reactions {
		if !reaction.IsActive {
			continue
		}

		// Русский комментарий: Проверяем фильтр по типу контента.
		// Если trigger_content_type задан, проверяем соответствие типа сообщения.
		if reaction.TriggerContentType != "" {
			contentMatched := false
			switch reaction.TriggerContentType {
			case "photo":
				contentMatched = msg.Photo != nil
			case "video":
				contentMatched = msg.Video != nil
			case "sticker":
				contentMatched = msg.Sticker != nil
			case "animation":
				contentMatched = msg.Animation != nil
			case "voice":
				contentMatched = msg.Voice != nil
			case "video_note":
				contentMatched = msg.VideoNote != nil
			case "audio":
				contentMatched = msg.Audio != nil
			case "document":
				contentMatched = msg.Document != nil
			case "text":
				contentMatched = msg.Text != ""
			}

			if !contentMatched {
				continue // Тип контента не совпадает, пропускаем эту реакцию
			}
		}

		// Русский комментарий: Проверяем соответствие паттерна.
		// Если pattern пустой и user_id совпадает - срабатывает (без проверки текста).
		matched := false

		// Персональная реакция на любой контент (pattern пустой)
		if reaction.Pattern == "" && reaction.UserID > 0 && reaction.UserID == userID {
			matched = true
		} else if msg.Text != "" {
			// Обычная текстовая реакция
			if reaction.IsRegex {
				re, err := regexp.Compile(reaction.Pattern)
				if err != nil {
					m.logger.Warn("invalid regex pattern", zap.String("pattern", reaction.Pattern))
					continue
				}
				matched = re.MatchString(msg.Text)
			} else {
				matched = strings.Contains(strings.ToLower(msg.Text), strings.ToLower(reaction.Pattern))
			}
		}

		if matched {
			if reaction.Cooldown > 0 {
				lastTriggered, err := m.getLastTriggered(chatID, reaction.ID)
				if err == nil && time.Since(lastTriggered) < time.Duration(reaction.Cooldown)*time.Second {
					m.logger.Debug("reaction on cooldown", zap.Int64("reaction_id", reaction.ID))
					continue
				}
			}

			if reaction.DailyLimit > 0 {
				// Для персональной реакции (user_id>0) проверяем индивидуальный лимит
				// Для общей реакции (user_id=0) проверяем общий лимит чата
				countUserID := reaction.UserID
				if countUserID == 0 {
					// Общая реакция - считаем для всего чата (user_id=0)
					countUserID = 0
				}
				count, err := m.getDailyCount(chatID, reaction.ID, countUserID)
				if err != nil {
					m.logger.Error("failed to get daily count", zap.Error(err))
					continue
				}
				if count >= reaction.DailyLimit {
					if reaction.DeleteOnLimit {
						// Delete the message and send warning
						err := ctx.Bot.Delete(ctx.Message)
						if err != nil {
							m.logger.Error("failed to delete message", zap.Error(err))
						}
						warning := fmt.Sprintf("Достигнут дневной лимит для реакции на '%s'", reaction.Pattern)
						err = ctx.Send(warning)
						if err != nil {
							m.logger.Error("failed to send warning", zap.Error(err))
						}
					}
					continue
				}
			}

			var err error
			switch reaction.ResponseType {
			case "text":
				err = ctx.SendReply(reaction.ResponseContent)
			case "sticker":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Sticker{File: telebot.File{FileID: reaction.ResponseContent}}, &telebot.SendOptions{ReplyTo: ctx.Message})
			case "photo":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Photo{File: telebot.File{FileID: reaction.ResponseContent}}, &telebot.SendOptions{ReplyTo: ctx.Message})
			case "animation":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Animation{File: telebot.File{FileID: reaction.ResponseContent}}, &telebot.SendOptions{ReplyTo: ctx.Message})
			case "video":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Video{File: telebot.File{FileID: reaction.ResponseContent}}, &telebot.SendOptions{ReplyTo: ctx.Message})
			case "voice":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Voice{File: telebot.File{FileID: reaction.ResponseContent}}, &telebot.SendOptions{ReplyTo: ctx.Message})
			case "document":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Document{File: telebot.File{FileID: reaction.ResponseContent}}, &telebot.SendOptions{ReplyTo: ctx.Message})
			case "audio":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Audio{File: telebot.File{FileID: reaction.ResponseContent}}, &telebot.SendOptions{ReplyTo: ctx.Message})
			default:
				err = ctx.SendReply(reaction.ResponseContent)
			}
			if err != nil {
				m.logger.Error("failed to send reaction", zap.Error(err))
			}

			m.recordTrigger(chatID, reaction.ID, userID)
			if reaction.DailyLimit > 0 {
				// Инкрементируем счётчик для того же user_id, что проверяли выше
				countUserID := reaction.UserID
				if countUserID == 0 {
					countUserID = 0
				}
				m.incrementDailyCount(chatID, reaction.ID, countUserID)
			}
			break
		}
	}

	return nil
}

func (m *ReactionsModule) loadReactions(chatID int64, threadID int64, userID int64) ([]KeywordReaction, error) {
	m.logger.Debug("loadReactions called", zap.Int64("chat_id", chatID), zap.Int64("thread_id", threadID), zap.Int64("user_id", userID))

	// Русский комментарий: Читаем реакции напрямую из БД (без кеша).
	// Чтение ~1-2ms, не критично для производительности.
	// Fallback логика (приоритет сверху вниз):
	// 1. Персональная реакция для user_id в конкретном топике (thread_id + user_id)
	// 2. Персональная реакция для user_id во всём чате (thread_id=0 + user_id)
	// 3. Общая реакция для топика (thread_id, user_id IS NULL)
	// 4. Общая реакция для чата (thread_id=0, user_id IS NULL)
	rows, err := m.db.Query(`
		SELECT id, chat_id, thread_id, COALESCE(user_id, 0), pattern, response_type, response_content, description, COALESCE(trigger_content_type, ''), is_regex, cooldown, daily_limit, delete_on_limit, is_active
		FROM keyword_reactions
		WHERE chat_id = $1 
		  AND (thread_id = $2 OR thread_id = 0) 
		  AND (user_id = $3 OR user_id IS NULL)
		  AND is_active = true
		ORDER BY 
		  CASE WHEN user_id IS NOT NULL THEN 0 ELSE 1 END,  -- Персональные реакции в приоритете
		  thread_id DESC,  -- Топик приоритетнее чата
		  id
	`, chatID, threadID, userID)
	if err != nil {
		m.logger.Error("loadReactions query failed", zap.Error(err), zap.Int64("chat_id", chatID), zap.Int64("thread_id", threadID))
		return nil, err
	}
	defer rows.Close()

	var reactions []KeywordReaction
	for rows.Next() {
		var r KeywordReaction
		if err := rows.Scan(&r.ID, &r.ChatID, &r.ThreadID, &r.UserID, &r.Pattern, &r.ResponseType, &r.ResponseContent, &r.Description, &r.TriggerContentType, &r.IsRegex, &r.Cooldown, &r.DailyLimit, &r.DeleteOnLimit, &r.IsActive); err != nil {
			m.logger.Error("failed to scan reaction", zap.Error(err))
			continue
		}
		reactions = append(reactions, r)
	}

	m.logger.Debug("loadReactions completed", zap.Int("count", len(reactions)))

	return reactions, nil
}

func (m *ReactionsModule) getLastTriggered(chatID, reactionID int64) (time.Time, error) {
	var lastTriggered time.Time
	err := m.db.QueryRow(`
		SELECT last_triggered_at FROM reaction_triggers
		WHERE chat_id = $1 AND reaction_id = $2
	`, chatID, reactionID).Scan(&lastTriggered)
	return lastTriggered, err
}

func (m *ReactionsModule) recordTrigger(chatID, reactionID, userID int64) {
	_, err := m.db.Exec(`
		INSERT INTO reaction_triggers (chat_id, reaction_id, user_id, last_triggered_at, trigger_count)
		VALUES ($1, $2, $3, NOW(), 1)
		ON CONFLICT (chat_id, reaction_id) DO UPDATE
		SET last_triggered_at = NOW(), trigger_count = reaction_triggers.trigger_count + 1
	`, chatID, reactionID, userID)
	if err != nil {
		m.logger.Error("failed to record trigger", zap.Error(err))
	}
}

func (m *ReactionsModule) getDailyCount(chatID, reactionID, userID int64) (int, error) {
	var count int
	err := m.db.QueryRow(`
		SELECT count FROM reaction_daily_counters
		WHERE chat_id = $1 AND reaction_id = $2 AND user_id = $3 AND counter_date = CURRENT_DATE
	`, chatID, reactionID, userID).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	m.logger.Debug("getDailyCount",
		zap.Int64("chat_id", chatID),
		zap.Int64("reaction_id", reactionID),
		zap.Int64("user_id", userID),
		zap.Int("count", count))
	return count, nil
}

func (m *ReactionsModule) incrementDailyCount(chatID, reactionID, userID int64) {
	_, err := m.db.Exec(`
		INSERT INTO reaction_daily_counters (chat_id, reaction_id, user_id, counter_date, count)
		VALUES ($1, $2, $3, CURRENT_DATE, 1)
		ON CONFLICT (chat_id, reaction_id, user_id, counter_date) DO UPDATE
		SET count = reaction_daily_counters.count + 1
	`, chatID, reactionID, userID)
	if err != nil {
		m.logger.Error("failed to increment daily count", zap.Error(err))
	}
	m.logger.Debug("incrementDailyCount",
		zap.Int64("chat_id", chatID),
		zap.Int64("reaction_id", reactionID),
		zap.Int64("user_id", userID))
}

// parseQuotedArgs парсит строку команды с учётом кавычек
// Пример: `/addreaction "text with spaces" sticker` → ["text with spaces", "sticker"]
func parseQuotedArgs(text string) []string {
	// Убираем команду в начале
	text = strings.TrimPrefix(text, "/addreaction")
	text = strings.TrimSpace(text)

	var args []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(text); i++ {
		ch := text[i]

		switch ch {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				current.WriteByte(ch)
			} else if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		args = append(args, current.String())
	}

	return args
}

func (m *ReactionsModule) handleAddReaction(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleAddReaction called",
		zap.Int64("chat_id", chatID),
		zap.Int64("thread_id", threadID),
		zap.Int64("user_id", c.Sender().ID),
		zap.String("message_text", c.Text()),
		zap.Bool("has_reply", c.Message().ReplyTo != nil))

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		m.logger.Error("failed to check admin status", zap.Error(err))
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		m.logger.Warn("non-admin tried to add reaction", zap.Int64("user_id", c.Sender().ID))
		return c.Send("❌ Команда доступна только администраторам")
	}

	// Русский комментарий: Парсим аргументы с учётом кавычек
	// Проблема: telebot.v3 Args() разбивает текст по пробелам, игнорируя кавычки
	// Решение: парсим вручную, учитывая кавычки как границы одного аргумента
	args := parseQuotedArgs(c.Text())
	m.logger.Info("parsed args",
		zap.Strings("args", args),
		zap.Int("args_count", len(args)))

	var responseType, responseContent, description string
	var pattern string
	var dailyLimit int
	var deleteOnLimit bool
	var userID int64 = 0               // 0 = для всех пользователей
	var triggerContentType string = "" // пустая строка = любой контент
	var cooldown int = 30              // по умолчанию 30 секунд

	// Русский комментарий: Проверяем префикс user:<user_id> для персональной реакции
	// Пример: /addreaction user:123456 "" "Привет, рад тебя видеть!" "Персональное приветствие" photo 86400
	if len(args) > 0 && strings.HasPrefix(args[0], "user:") {
		userIDStr := strings.TrimPrefix(args[0], "user:")
		parsedUserID, err := strconv.ParseInt(userIDStr, 10, 64)
		if err != nil {
			return c.Send("❌ Неверный формат user_id. Используйте: user:303724504")
		}
		userID = parsedUserID
		args = args[1:] // Убираем префикс из аргументов
	}

	if c.Message().ReplyTo != nil {
		// Reply mode: get response from replied message
		if len(args) < 1 {
			return c.Send("Использование: /addreaction [user:<user_id>] <pattern> [<content_type>] [<cooldown>] [<daily_limit>] [delete] (reply на сообщение)\n\nПримеры:\n• /addreaction привет (ответьте на стикер) - простая реакция\n• /addreaction user:123456 \"\" photo 86400 (ответьте на фото) - персональная реакция на фото раз в сутки")
		}

		m.logger.Info("reply mode addreaction",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Strings("args", args),
			zap.Int("args_count", len(args)))

		pattern = args[0]
		dailyLimit = 0
		deleteOnLimit = false
		remainingArgs := args[1:]

		// Проверяем тип контента (photo/video/sticker/etc)
		if len(remainingArgs) > 0 {
			validContentTypes := map[string]bool{
				"photo": true, "video": true, "sticker": true, "animation": true,
				"voice": true, "video_note": true, "audio": true, "document": true, "text": true,
			}
			if validContentTypes[remainingArgs[0]] {
				triggerContentType = remainingArgs[0]
				remainingArgs = remainingArgs[1:]
			}
		}

		// Проверяем cooldown
		if len(remainingArgs) > 0 {
			if cd, err := strconv.Atoi(remainingArgs[0]); err == nil && cd > 0 {
				cooldown = cd
				remainingArgs = remainingArgs[1:]
			}
		}

		// Проверяем delete flag
		if len(remainingArgs) > 0 && remainingArgs[len(remainingArgs)-1] == "delete" {
			deleteOnLimit = true
			remainingArgs = remainingArgs[:len(remainingArgs)-1]
		}

		// Проверяем daily limit (должно быть числом)
		if len(remainingArgs) > 0 {
			if l, err := strconv.Atoi(remainingArgs[0]); err == nil && l > 0 {
				dailyLimit = l
				remainingArgs = remainingArgs[1:]
			}
		}

		description = strings.Join(remainingArgs, " ")

		m.logger.Info("reply mode parsed",
			zap.String("pattern", pattern),
			zap.String("trigger_content_type", triggerContentType),
			zap.Int("cooldown", cooldown),
			zap.Int("daily_limit", dailyLimit),
			zap.Bool("delete_on_limit", deleteOnLimit),
			zap.String("description", description))

		replyMsg := c.Message().ReplyTo
		if replyMsg.Sticker != nil {
			responseType = "sticker"
			responseContent = replyMsg.Sticker.FileID
		} else if replyMsg.Photo != nil {
			responseType = "photo"
			responseContent = replyMsg.Photo.FileID
		} else if replyMsg.Animation != nil {
			responseType = "animation"
			responseContent = replyMsg.Animation.FileID
		} else if replyMsg.Video != nil {
			responseType = "video"
			responseContent = replyMsg.Video.FileID
		} else if replyMsg.Voice != nil {
			responseType = "voice"
			responseContent = replyMsg.Voice.FileID
		} else if replyMsg.Document != nil {
			responseType = "document"
			responseContent = replyMsg.Document.FileID
		} else if replyMsg.Audio != nil {
			responseType = "audio"
			responseContent = replyMsg.Audio.FileID
		} else {
			responseType = "text"
			responseContent = replyMsg.Text
		}
	} else {
		// Text mode
		if len(args) < 3 {
			return c.Send("Использование: /addreaction [user:<user_id>] <pattern> <response> <description> [<content_type>] [<cooldown>] [limit] [delete]\nИли reply на сообщение со стикером/фото/etc.\nПример: /addreaction user:123456 \"\" \"Привет, рад тебя видеть!\" \"Персональное приветствие\" text 86400")
		}
		pattern = args[0]
		responseType = "text"
		responseContent = args[1]
		description = args[2]
		dailyLimit = 0
		deleteOnLimit = false
		remainingArgs := args[3:]

		// Проверяем тип контента (photo/video/sticker/etc)
		if len(remainingArgs) > 0 {
			validContentTypes := map[string]bool{
				"photo": true, "video": true, "sticker": true, "animation": true,
				"voice": true, "video_note": true, "audio": true, "document": true, "text": true,
			}
			if validContentTypes[remainingArgs[0]] {
				triggerContentType = remainingArgs[0]
				remainingArgs = remainingArgs[1:]
			}
		}

		// Проверяем cooldown
		if len(remainingArgs) > 0 {
			if cd, err := strconv.Atoi(remainingArgs[0]); err == nil && cd > 0 {
				cooldown = cd
				remainingArgs = remainingArgs[1:]
			}
		}

		if len(remainingArgs) > 0 && remainingArgs[len(remainingArgs)-1] == "delete" {
			deleteOnLimit = true
			remainingArgs = remainingArgs[:len(remainingArgs)-1]
		}
		if len(remainingArgs) > 0 {
			if l, err := strconv.Atoi(remainingArgs[0]); err == nil {
				dailyLimit = l
			}
		}
	}

	// Русский комментарий: Если user_id указан, сохраняем его в БД. NULL для общих реакций.
	var userIDParam interface{}
	if userID > 0 {
		userIDParam = userID
	} else {
		userIDParam = nil
	}

	// Русский комментарий: Если trigger_content_type указан, сохраняем его в БД. NULL для любого контента.
	var triggerContentTypeParam interface{}
	if triggerContentType != "" {
		triggerContentTypeParam = triggerContentType
	} else {
		triggerContentTypeParam = nil
	}

	m.logger.Info("inserting reaction into DB",
		zap.Int64("chat_id", chatID),
		zap.Int64("thread_id", threadID),
		zap.Any("user_id_param", userIDParam),
		zap.String("pattern", pattern),
		zap.String("response_type", responseType),
		zap.String("response_content", responseContent),
		zap.String("description", description),
		zap.Any("trigger_content_type", triggerContentTypeParam),
		zap.Int("cooldown", cooldown),
		zap.Int("daily_limit", dailyLimit),
		zap.Bool("delete_on_limit", deleteOnLimit))

	// Валидация входных данных
	if len(pattern) > 1000 {
		return c.Send("❌ Паттерн слишком длинный (макс. 1000 символов)")
	}
	if len(description) > 500 {
		return c.Send("❌ Описание слишком длинное (макс. 500 символов)")
	}
	if len(responseContent) > 5000 {
		return c.Send("❌ Содержимое ответа слишком длинное (макс. 5000 символов)")
	}
	if cooldown < 0 || cooldown > 2592000 { // 30 дней
		return c.Send("❌ Кулдаун должен быть от 0 до 2592000 секунд (30 дней)")
	}
	if dailyLimit < 0 || dailyLimit > 10000 {
		return c.Send("❌ Дневной лимит должен быть от 0 до 10000")
	}

	// Русский комментарий: Убеждаемся что chat_id существует в таблице chats (для foreign key)
	// Используем ON CONFLICT DO NOTHING чтобы не перезаписывать существующие данные
	_, err = m.db.Exec(`
		INSERT INTO chats (chat_id, chat_type, title)
		VALUES ($1, 'unknown', 'unknown')
		ON CONFLICT (chat_id) DO NOTHING
	`, chatID)
	if err != nil {
		m.logger.Error("failed to ensure chat exists", zap.Error(err))
		return c.Send("❌ Ошибка при проверке чата")
	}

	_, err = m.db.Exec(`
		INSERT INTO keyword_reactions (chat_id, thread_id, user_id, pattern, response_type, response_content, description, is_regex, trigger_content_type, cooldown, daily_limit, delete_on_limit, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, false, $8, $9, $10, $11, true)
	`, chatID, threadID, userIDParam, pattern, responseType, responseContent, description, triggerContentTypeParam, cooldown, dailyLimit, deleteOnLimit)

	if err != nil {
		m.logger.Error("failed to add reaction", zap.Error(err))
		return c.Send("❌ Не удалось добавить реакцию")
	}

	m.logger.Info("reaction added successfully",
		zap.Int64("chat_id", chatID),
		zap.Int64("thread_id", threadID),
		zap.String("pattern", pattern))

	// Логируем событие
	details := fmt.Sprintf("Added reaction: pattern='%s', type=%s, thread=%d", pattern, responseType, threadID)
	if userID > 0 {
		details = fmt.Sprintf("Added personal reaction: pattern='%s', type=%s, user=%d, thread=%d", pattern, responseType, userID, threadID)
	}
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "add_reaction", details)

	deleteMsg := ""
	if deleteOnLimit {
		deleteMsg = "\nУдалять при превышении лимита: да"
	}

	contentTypeMsg := ""
	if triggerContentType != "" {
		contentTypeMsg = fmt.Sprintf("\n🎯 Только для: %s", triggerContentType)
	}

	cooldownMsg := ""
	if cooldown != 30 {
		if cooldown >= 86400 {
			days := cooldown / 86400
			cooldownMsg = fmt.Sprintf("\n⏰ Кулдаун: %d сек (%d дн.)", cooldown, days)
		} else if cooldown >= 3600 {
			hours := cooldown / 3600
			cooldownMsg = fmt.Sprintf("\n⏰ Кулдаун: %d сек (%d ч.)", cooldown, hours)
		} else {
			cooldownMsg = fmt.Sprintf("\n⏰ Кулдаун: %d сек", cooldown)
		}
	}

	var scopeMsg string
	if threadID != 0 {
		scopeMsg = "✅ Реакция добавлена <b>для этого топика</b>\n\n💡 Для настройки всего чата используйте команду в основном чате\n\n"
	} else {
		scopeMsg = "✅ Реакция добавлена <b>для всего чата</b>\n\n💡 Для настройки топика используйте команду внутри топика\n\n"
	}

	// Обрезаем длинные FileID
	displayContent := responseContent
	if len(displayContent) > 50 {
		displayContent = displayContent[:50] + "..."
	}

	return c.Send(fmt.Sprintf("%sПаттерн: <code>%s</code>\nТип ответа: %s\nСодержимое: <code>%s</code>\nОписание: %s\nДневной лимит: %d%s%s%s", scopeMsg, pattern, responseType, displayContent, description, dailyLimit, deleteMsg, contentTypeMsg, cooldownMsg), &telebot.SendOptions{ParseMode: telebot.ModeHTML})
}

func (m *ReactionsModule) handleListReactions(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleListReactions called", zap.Int64("chat_id", chatID), zap.Int64("thread_id", threadID))

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	// Получаем реакции с учетом fallback: сначала для топика, потом для чата
	rows, err := m.db.Query(`
		SELECT id, thread_id, COALESCE(user_id, 0), pattern, response_type, response_content, description, COALESCE(trigger_content_type, ''), cooldown, daily_limit, delete_on_limit, is_active
		FROM keyword_reactions
		WHERE chat_id = $1 AND (thread_id = $2 OR thread_id = 0)
		ORDER BY thread_id DESC, id
	`, chatID, threadID)

	if err != nil {
		m.logger.Error("handleListReactions query failed", zap.Error(err))
		return c.Send("❌ Не удалось получить список реакций")
	}
	defer rows.Close()

	m.logger.Debug("handleListReactions query executed")

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "list_reactions",
		fmt.Sprintf("Admin viewed reactions list (chat=%d, thread=%d)", chatID, threadID))

	var reactions []struct {
		ID                 int64
		ThreadID           int64
		UserID             int64
		Pattern            string
		ResponseType       string
		ResponseContent    string
		Description        string
		TriggerContentType string
		Cooldown           int
		DailyLimit         int
		DeleteOnLimit      bool
		IsActive           bool
	}

	for rows.Next() {
		var r struct {
			ID                 int64
			ThreadID           int64
			UserID             int64
			Pattern            string
			ResponseType       string
			ResponseContent    string
			Description        string
			TriggerContentType string
			Cooldown           int
			DailyLimit         int
			DeleteOnLimit      bool
			IsActive           bool
		}
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.UserID, &r.Pattern, &r.ResponseType, &r.ResponseContent, &r.Description, &r.TriggerContentType, &r.Cooldown, &r.DailyLimit, &r.DeleteOnLimit, &r.IsActive); err != nil {
			m.logger.Error("failed to scan reaction", zap.Error(err))
			continue
		}
		reactions = append(reactions, r)
	}

	m.logger.Debug("handleListReactions scanned reactions", zap.Int("count", len(reactions)))

	if len(reactions) == 0 {
		if threadID != 0 {
			return c.Send("ℹ️ В этом топике нет настроенных реакций")
		}
		return c.Send("ℹ️ В этом чате нет настроенных реакций")
	}

	var scopeHeader string
	if threadID != 0 {
		scopeHeader = "📋 <b>Список реакций (для этого топика):</b>\n\n"
	} else {
		scopeHeader = "📋 <b>Список реакций (для всего чата):</b>\n\n"
	}

	text := scopeHeader
	for i, r := range reactions {
		status := "✅"
		if !r.IsActive {
			status = "❌"
		}
		deleteMsg := "нет"
		if r.DeleteOnLimit {
			deleteMsg = "да"
		}
		scope := "чат"
		if r.ThreadID != 0 {
			scope = "топик"
		}

		// Русский комментарий: Показываем user_id если реакция персональная
		userInfo := ""
		if r.UserID > 0 {
			userInfo = fmt.Sprintf("\n   🎯 <b>Персональная для user_id:</b> %d", r.UserID)
		}

		// Русский комментарий: Показываем trigger_content_type если задан
		contentTypeInfo := ""
		if r.TriggerContentType != "" {
			contentTypeInfo = fmt.Sprintf("\n   📎 <b>Только для:</b> %s", r.TriggerContentType)
		}

		// Русский комментарий: Показываем cooldown если не стандартный
		cooldownInfo := ""
		if r.Cooldown != 30 {
			if r.Cooldown >= 86400 {
				days := r.Cooldown / 86400
				cooldownInfo = fmt.Sprintf("\n   ⏰ <b>Кулдаун:</b> %d сек (%d дн.)", r.Cooldown, days)
			} else if r.Cooldown >= 3600 {
				hours := r.Cooldown / 3600
				cooldownInfo = fmt.Sprintf("\n   ⏰ <b>Кулдаун:</b> %d сек (%d ч.)", r.Cooldown, hours)
			} else {
				cooldownInfo = fmt.Sprintf("\n   ⏰ <b>Кулдаун:</b> %d сек", r.Cooldown)
			}
		}

		// Обрезаем длинные FileID для стикеров/фото
		displayContent := r.ResponseContent
		if len(displayContent) > 50 {
			displayContent = displayContent[:50] + "..."
		}

		text += fmt.Sprintf("%d. %s ID: %d [%s]\n   Паттерн: <code>%s</code>\n   Тип ответа: %s\n   Содержимое: <code>%s</code>\n   Описание: %s\n   Дневной лимит: %d\n   Удалять при превышении: %s%s%s%s\n\n", i+1, status, r.ID, scope, r.Pattern, r.ResponseType, displayContent, r.Description, r.DailyLimit, deleteMsg, userInfo, contentTypeInfo, cooldownInfo)
	}

	m.logger.Debug("handleListReactions formatted response", zap.Int("text_length", len(text)))

	if err := c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeHTML}); err != nil {
		m.logger.Error("handleListReactions send failed", zap.Error(err), zap.Int("text_length", len(text)))
		return c.Send("❌ Не удалось отправить список реакций (сообщение слишком длинное или ошибка API)")
	}

	m.logger.Info("handleListReactions completed successfully", zap.Int("reactions_count", len(reactions)))
	return nil
}

func (m *ReactionsModule) handleRemoveReaction(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleRemoveReaction called", zap.Int64("chat_id", chatID), zap.Int64("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("Использование: /removereaction <id>\nПример: /removereaction 5")
	}

	reactionID := args[1]

	result, err := m.db.Exec(`
		DELETE FROM keyword_reactions WHERE chat_id = $1 AND thread_id = $2 AND id = $3
	`, chatID, threadID, reactionID)

	if err != nil {
		return c.Send("❌ Не удалось удалить реакцию")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Send("ℹ️ Реакция не найдена")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "remove_reaction",
		fmt.Sprintf("Removed reaction ID=%s (chat=%d, thread=%d)", reactionID, chatID, threadID))

	var scopeMsg string
	if threadID != 0 {
		scopeMsg = fmt.Sprintf("✅ Реакция #%s удалена <b>для этого топика</b>\n\n💡 Для удаления реакции всего чата используйте команду в основном чате", reactionID)
	} else {
		scopeMsg = fmt.Sprintf("✅ Реакция #%s удалена <b>для всего чата</b>\n\n💡 Для удаления реакции топика используйте команду внутри топика", reactionID)
	}

	return c.Send(scopeMsg, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
}
