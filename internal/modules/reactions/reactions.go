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
	db                *sql.DB
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	messageRepo       *repositories.MessageRepository
	eventRepo         *repositories.EventRepository
	logger            *zap.Logger
	bot               *telebot.Bot
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
	Action             string // пустая строка = реакция (ответ), 'delete'/'warn'/'delete_warn' = фильтр
	IsActive           bool
}

// getTextForMatching возвращает текст сообщения для проверки на совпадение.
// Caption приоритетнее Text — у медиа-сообщений (фото, видео, документ)
// текст лежит в Caption, а Text всегда пустой. Без этого фото с матом в подписи
// проходит все фильтры.
func getTextForMatching(msg *telebot.Message) string {
	if msg.Caption != "" {
		return msg.Caption
	}
	return msg.Text
}

func New(
	db *sql.DB,
	vipRepo *repositories.VIPRepository,
	contentLimitsRepo *repositories.ContentLimitsRepository,
	messageRepo *repositories.MessageRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	bot *telebot.Bot,
) *ReactionsModule {
	return &ReactionsModule{
		db:                db,
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		messageRepo:       messageRepo,
		eventRepo:         eventRepo,
		logger:            logger,
		bot:               bot,
	}
}

// RegisterCommands регистрирует команды модуля в боте.
func (m *ReactionsModule) RegisterCommands(bot *telebot.Bot) {
	// /reactions — справка по модулю реакций
	bot.Handle("/reactions", func(c telebot.Context) error {
		msg := "🤖 <b>Модуль Reactions</b> — Реакции, фильтры и модерация\n\n"
		msg += "Единый модуль для автоответов, фильтрации и контроля контента.\n\n"

		msg += "<b>📋 Разделы:</b>\n"
		msg += "• /reactions — автоответы на ключевые слова (эта справка)\n"
		msg += "• /textfilter — фильтр запрещённых слов\n"
		msg += "• /profanity — фильтр ненормативной лексики\n\n"

		msg += "<b>🔹 Команды автоответов:</b>\n\n"
		msg += "🔸 <code>/addreaction</code> — Добавить реакцию (только админы)\n"
		msg += "🔸 <code>/listreactions</code> — Показать все реакции (только админы)\n"
		msg += "🔸 <code>/removereaction &lt;ID&gt;</code> — Удалить реакцию (только админы)\n\n"

		msg += "<b>КАК ДОБАВИТЬ РЕАКЦИЮ:</b>\n\n"

		msg += "<b>1️⃣ Текстовая реакция:</b>\n"
		msg += "<code>/addreaction слово \"<u>текст ответа</u>\" \"<u>описание</u>\"</code>\n\n"
		msg += "📌 <b>Пример:</b>\n"
		msg += "• <code>/addreaction привет \"Привет всем!\" \"Приветствие\"</code>\n\n"

		msg += "<b>2️⃣ Реакция стикером/фото:</b>\n"
		msg += "📝 Ответьте на стикер/фото и напишите:\n"
		msg += "<code>/addreaction слово описание</code>\n\n"

		msg += "<b>⚙️ Опции:</b> тип контента, кулдаун (секунды), дневной лимит\n"
		msg += "<b>👤 Персональная:</b> <code>/addreaction user:123456 слово ...</code>\n\n"

		msg += "⚠️ <b>Топики:</b> Команда в топике = реакция только в нём\n\n"
		msg += "📌 <b>Приоритет обработки сообщений:</b>\n"
		msg += "1. Фильтр мата (/profanity) — высший приоритет\n"
		msg += "2. Фильтр запрещённых слов (/textfilter)\n"
		msg += "3. Автоответы на ключевые слова\n"
		msg += "ℹ️ VIP-пользователи игнорируют все фильтры и автоответы"

		return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
	})

	// /textfilter — справка по фильтру запрещённых слов
	bot.Handle("/textfilter", func(c telebot.Context) error {
		msg := "🚫 <b>Фильтр запрещённых слов</b> (часть модуля Reactions)\n\n"
		msg += "Автоматическое удаление сообщений с запрещёнными словами и фразами.\n\n"
		msg += "<b>Доступные команды:</b>\n\n"

		msg += "🔹 <code>/addban &lt;слово&gt; &lt;действие&gt;</code> — Забанить слово (только админы)\n\n"

		msg += "<b>ℹ️ ПРОСТЫЕ ПРИМЕРЫ:</b>\n"
		msg += "• <code>/addban спам delete</code> - удалять сообщения со словом 'спам'\n"
		msg += "• <code>/addban реклама warn</code> - предупреждать за 'реклама'\n"
		msg += "• <code>/addban @username delete</code> - удалять упоминания пользователя\n\n"

		msg += "<b>🔄 НЕСКОЛЬКО СЛОВ СРАЗУ (regex):</b>\n"
		msg += "• <code>/addban спам|реклама|продам delete</code>\n\n"

		msg += "🔹 <code>/listbans</code> — Список всех запрещённых слов (только админы)\n\n"

		msg += "🔹 <code>/removeban &lt;ID&gt;</code> — Удалить бан-слово (только админы)\n\n"

		msg += "⚠️ <b>Действия:</b>\n"
		msg += "• <code>delete</code> — удалить сообщение молча\n"
		msg += "• <code>warn</code> — предупредить (сообщение остаётся)\n"
		msg += "• <code>delete_warn</code> — удалить И предупредить\n\n"

		msg += "🛡️ <i>VIP-защита:</i> VIP-пользователи игнорируют фильтры."

		return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
	})

	// /profanity — справка по фильтру мата
	bot.Handle("/profanity", func(c telebot.Context) error {
		msg := "🚫 <b>Фильтр ненормативной лексики</b> (часть модуля Reactions)\n\n"
		msg += "Автоматическое обнаружение и фильтрация мата по встроенному словарю.\n\n"
		msg += "<b>Доступные команды:</b>\n\n"

		msg += "🔹 <code>/setprofanity &lt;действие&gt;</code> — Включить фильтр (только админы)\n"
		msg += "   📌 Пример: <code>/setprofanity delete_warn</code>\n\n"

		msg += "🔹 <code>/profanitystatus</code> — Проверить статус фильтра\n\n"

		msg += "🔹 <code>/removeprofanity</code> — Отключить фильтр (только админы)\n\n"

		msg += "⚠️ <b>Действия:</b>\n"
		msg += "• <code>delete</code> — удалить сообщение молча\n"
		msg += "• <code>warn</code> — предупредить (сообщение остаётся)\n"
		msg += "• <code>delete_warn</code> — удалить И предупредить\n\n"

		msg += "🛡️ <i>VIP-защита:</i> VIP игнорируют фильтр."
		return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
	})
}

// RegisterAdminCommands регистрирует админские команды.
func (m *ReactionsModule) RegisterAdminCommands(bot *telebot.Bot) {
	// Реакции (автоответы)
	bot.Handle("/addreaction", m.handleAddReaction)
	bot.Handle("/listreactions", m.handleListReactions)
	bot.Handle("/removereaction", m.handleRemoveReaction)

	// Фильтр запрещённых слов (бывший TextFilter)
	bot.Handle("/addban", m.handleAddBan)
	bot.Handle("/listbans", m.handleListBans)
	bot.Handle("/removeban", m.handleRemoveBan)

	// Фильтр мата (бывший ProfanityFilter)
	bot.Handle("/setprofanity", m.handleSetProfanity)
	bot.Handle("/removeprofanity", m.handleRemoveProfanity)
	bot.Handle("/profanitystatus", m.handleProfanityStatus)
}

func (m *ReactionsModule) OnMessage(ctx *core.MessageContext) error {
	msg := ctx.Message

	// Пропускаем приватные сообщения и команды
	if msg.Private() || (msg.Text != "" && strings.HasPrefix(msg.Text, "/")) {
		return nil
	}

	chatID := msg.Chat.ID
	// ThreadID уже вычислен в middleware и закеширован — без лишнего SQL-запроса.
	threadID := ctx.ThreadID
	userID := msg.Sender.ID

	m.logger.Debug("reactions OnMessage", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", userID), zap.String("text", msg.Text))

	isVIP, _ := m.vipRepo.IsVIP(chatID, threadID, userID)
	if isVIP {
		return nil
	}

	// Получаем текст для проверки (caption приоритетнее text).
	// У медиа-сообщений текст в Caption, а Text пустой.
	textToCheck := getTextForMatching(msg)

	// ─── Этап 1: Фильтр мата (глобальный словарь profanity_dictionary) ───
	// Проверяем ВСЕГДА, даже если сообщение удалено Limiter-ом.
	// При ctx.MessageDeleted=true: мат считается, banned_words проверяется, но delete/warn не выполняются.
	if textToCheck != "" {
		if m.checkProfanity(ctx, chatID, threadID, userID, textToCheck) {
			return nil // Мат обнаружен, действие выполнено (или только подсчёт при MessageDeleted)
		}
	}

	// Если сообщение удалено (Limiter или profanity) — фильтры и автоответы бессмысленны
	if ctx.MessageDeleted {
		return nil
	}

	// ─── Этап 2: Загружаем keyword_reactions (и фильтры, и автоответы) ───
	reactions, err := m.loadReactions(chatID, threadID, userID)
	if err != nil {
		m.logger.Error("failed to load reactions", zap.Error(err))
		return nil
	}

	m.logger.Debug("loaded reactions", zap.Int("count", len(reactions)))

	// ─── Этап 3: Проверяем фильтры (action IS NOT NULL) ───
	for _, reaction := range reactions {
		if !reaction.IsActive || reaction.Action == "" {
			continue // Пропускаем неактивные и обычные реакции
		}

		matched := false
		if textToCheck != "" {
			if reaction.IsRegex {
				re, err := regexp.Compile(reaction.Pattern)
				if err != nil {
					m.logger.Warn("invalid regex pattern", zap.String("pattern", reaction.Pattern))
					continue
				}
				matched = re.MatchString(textToCheck)
			} else {
				matched = strings.Contains(strings.ToLower(textToCheck), strings.ToLower(reaction.Pattern))
			}
		}

		if matched {
			m.logger.Info("filter word detected",
				zap.Int64("chat_id", chatID),
				zap.Int64("user_id", userID),
				zap.String("pattern", reaction.Pattern),
				zap.String("action", reaction.Action),
			)
			m.performFilterAction(ctx, reaction)
			return nil // Фильтр сработал, автоответы не нужны
		}
	}

	// ─── Этап 4: Проверяем автоответы (action IS NULL) ───
	for _, reaction := range reactions {
		if !reaction.IsActive || reaction.Action != "" {
			continue // Пропускаем неактивные и фильтры
		}

		// Проверяем фильтр по типу контента.
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

		// Проверяем соответствие паттерна.
		// Если pattern пустой и user_id совпадает - срабатывает (без проверки текста).
		matched := false

		// Персональная реакция на любой контент (pattern пустой)
		if reaction.Pattern == "" && reaction.UserID > 0 && reaction.UserID == userID {
			matched = true
		} else if textToCheck != "" {
			// Обычная текстовая/caption реакция
			if reaction.IsRegex {
				re, err := regexp.Compile(reaction.Pattern)
				if err != nil {
					m.logger.Warn("invalid regex pattern", zap.String("pattern", reaction.Pattern))
					continue
				}
				matched = re.MatchString(textToCheck)
			} else {
				matched = strings.Contains(strings.ToLower(textToCheck), strings.ToLower(reaction.Pattern))
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
				count, err := m.getDailyCount(chatID, reaction.ID, reaction.UserID)
				if err != nil {
					m.logger.Error("failed to get daily count", zap.Error(err))
					continue
				}
				if count >= reaction.DailyLimit {
					if reaction.DeleteOnLimit {
						// Удаляем сообщение и отправляем предупреждение
						if err := ctx.DeleteMessage(); err != nil {
							m.logger.Error("failed to delete message", zap.Error(err))
						}
						// Отправляем warning только при ПЕРВОМ превышении
						if count == reaction.DailyLimit {
							warning := fmt.Sprintf("⚠️ Достигнут дневной лимит для реакции на '%s'", reaction.Pattern)
							err = ctx.Send(warning)
							if err != nil {
								m.logger.Error("failed to send warning", zap.Error(err))
							}
						}
						// Инкрементируем счётчик — иначе count == DailyLimit всегда
						// и предупреждение будет повторяться при каждом сообщении
						m.incrementDailyCount(chatID, reaction.ID, reaction.UserID)
					}
					continue
				}
			}

			var err error
			switch reaction.ResponseType {
			case "text":
				err = ctx.SendReply(reaction.ResponseContent)
			case "sticker":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Sticker{File: telebot.File{FileID: reaction.ResponseContent}}, ctx.SendOptions())
			case "photo":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Photo{File: telebot.File{FileID: reaction.ResponseContent}}, ctx.SendOptions())
			case "animation":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Animation{File: telebot.File{FileID: reaction.ResponseContent}}, ctx.SendOptions())
			case "video":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Video{File: telebot.File{FileID: reaction.ResponseContent}}, ctx.SendOptions())
			case "voice":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Voice{File: telebot.File{FileID: reaction.ResponseContent}}, ctx.SendOptions())
			case "document":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Document{File: telebot.File{FileID: reaction.ResponseContent}}, ctx.SendOptions())
			case "audio":
				_, err = ctx.Bot.Send(ctx.Chat, &telebot.Audio{File: telebot.File{FileID: reaction.ResponseContent}}, ctx.SendOptions())
			default:
				err = ctx.SendReply(reaction.ResponseContent)
			}
			if err != nil {
				m.logger.Error("failed to send reaction", zap.Error(err))
			}

			m.recordTrigger(chatID, reaction.ID, userID)
			if reaction.DailyLimit > 0 {
				// Инкрементируем счётчик для того же user_id, что проверяли выше
				m.incrementDailyCount(chatID, reaction.ID, reaction.UserID)
			}
			break
		}
	}

	return nil
}

func (m *ReactionsModule) loadReactions(chatID int64, threadID int, userID int64) ([]KeywordReaction, error) {
	m.logger.Debug("loadReactions called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", userID))

	// Читаем реакции напрямую из БД (без кеша).
	// Чтение ~1-2ms, не критично для производительности.
	// Fallback логика (приоритет сверху вниз):
	// 1. Персональная реакция для user_id в конкретном топике (thread_id + user_id)
	// 2. Персональная реакция для user_id во всём чате (thread_id=0 + user_id)
	// 3. Общая реакция для топика (thread_id, user_id IS NULL)
	// 4. Общая реакция для чата (thread_id=0, user_id IS NULL)
	rows, err := m.db.Query(`
		SELECT id, chat_id, thread_id, COALESCE(user_id, 0), pattern, response_type, response_content, description, COALESCE(trigger_content_type, ''), is_regex, cooldown, daily_limit, delete_on_limit, COALESCE(action, ''), is_active
		FROM keyword_reactions
		WHERE chat_id = $1 
		  AND (thread_id = $2 OR thread_id = 0) 
		  AND (user_id = $3 OR user_id IS NULL)
		  AND is_active = true
		ORDER BY 
		  CASE WHEN action IS NOT NULL THEN 0 ELSE 1 END,  -- Фильтры в приоритете
		  CASE WHEN user_id IS NOT NULL THEN 0 ELSE 1 END,  -- Персональные реакции в приоритете
		  thread_id DESC,  -- Топик приоритетнее чата
		  id
	`, chatID, threadID, userID)
	if err != nil {
		m.logger.Error("loadReactions query failed", zap.Error(err), zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID))
		return nil, err
	}
	defer rows.Close()

	var reactions []KeywordReaction
	for rows.Next() {
		var r KeywordReaction
		if err := rows.Scan(&r.ID, &r.ChatID, &r.ThreadID, &r.UserID, &r.Pattern, &r.ResponseType, &r.ResponseContent, &r.Description, &r.TriggerContentType, &r.IsRegex, &r.Cooldown, &r.DailyLimit, &r.DeleteOnLimit, &r.Action, &r.IsActive); err != nil {
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
		SET last_triggered_at = NOW(), trigger_count = reaction_triggers.trigger_count + 1, user_id = EXCLUDED.user_id
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
		zap.Int("thread_id", threadID),
		zap.Int64("user_id", c.Sender().ID),
		zap.String("message_text", c.Text()),
		zap.Bool("has_reply", c.Message().ReplyTo != nil))

	// Парсим аргументы с учётом кавычек
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

	// Проверяем префикс user:<user_id> для персональной реакции
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

	// Если user_id указан, сохраняем его в БД. NULL для общих реакций.
	var userIDParam interface{}
	if userID > 0 {
		userIDParam = userID
	} else {
		userIDParam = nil
	}

	// Если trigger_content_type указан, сохраняем его в БД. NULL для любого контента.
	var triggerContentTypeParam interface{}
	if triggerContentType != "" {
		triggerContentTypeParam = triggerContentType
	} else {
		triggerContentTypeParam = nil
	}

	m.logger.Info("inserting reaction into DB",
		zap.Int64("chat_id", chatID),
		zap.Int("thread_id", threadID),
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

	// Убеждаемся что chat_id существует в таблице chats (для foreign key)
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
		zap.Int("thread_id", threadID),
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
	if userID > 0 {
		scopeMsg = fmt.Sprintf("✅ Реакция добавлена <b>для пользователя</b> (user_id: %d)\n\n💡 Реакция сработает только для этого пользователя\n\n", userID)
	} else if threadID != 0 {
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

// splitIntoMessages разбивает список строк на несколько частей по maxLen символов
func splitIntoMessages(lines []string, maxLen int) []string {
	var messages []string
	var currentMessage strings.Builder

	for _, line := range lines {
		// Если добавление строки превысит лимит → начинаем новое сообщение
		if currentMessage.Len()+len(line)+1 > maxLen {
			if currentMessage.Len() > 0 {
				messages = append(messages, currentMessage.String())
				currentMessage.Reset()
			}
		}

		if currentMessage.Len() > 0 {
			currentMessage.WriteString("\n")
		}
		currentMessage.WriteString(line)
	}

	// Добавляем последнее сообщение
	if currentMessage.Len() > 0 {
		messages = append(messages, currentMessage.String())
	}

	return messages
}
func (m *ReactionsModule) handleListReactions(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleListReactions called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID))

	// Получаем реакции с учетом fallback: сначала для топика, потом для чата
	// Показываем ТОЛЬКО обычные реакции (action IS NULL), фильтры через /listbans
	rows, err := m.db.Query(`
		SELECT id, thread_id, COALESCE(user_id, 0), pattern, response_type, response_content, description, COALESCE(trigger_content_type, ''), cooldown, daily_limit, delete_on_limit, is_active
		FROM keyword_reactions
		WHERE chat_id = $1 AND (thread_id = $2 OR thread_id = 0)
		  AND action IS NULL
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

	// Формируем строки для каждой реакции
	var lines []string
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

		// Показываем user_id если реакция персональная
		userInfo := ""
		if r.UserID > 0 {
			userInfo = fmt.Sprintf("\n   🎯 <b>Персональная для user_id:</b> %d", r.UserID)
		}

		// Показываем trigger_content_type если задан
		contentTypeInfo := ""
		if r.TriggerContentType != "" {
			contentTypeInfo = fmt.Sprintf("\n   📎 <b>Только для:</b> %s", r.TriggerContentType)
		}

		// Показываем cooldown если не стандартный
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

		line := fmt.Sprintf("%d. %s ID: %d [%s]\n   Паттерн: <code>%s</code>\n   Тип ответа: %s\n   Содержимое: <code>%s</code>\n   Описание: %s\n   Дневной лимит: %d\n   Удалять при превышении: %s%s%s%s", i+1, status, r.ID, scope, r.Pattern, r.ResponseType, displayContent, r.Description, r.DailyLimit, deleteMsg, userInfo, contentTypeInfo, cooldownInfo)
		lines = append(lines, line)
	}

	// Разбиваем на части по 3500 символов (оставляем запас до 4096)
	const maxMessageLength = 3500
	messages := splitIntoMessages(lines, maxMessageLength)

	m.logger.Debug("handleListReactions formatted response", zap.Int("total_reactions", len(reactions)), zap.Int("pages", len(messages)))

	// Отправляем каждую часть
	for i, msg := range messages {
		var header string
		if len(messages) > 1 {
			header = fmt.Sprintf("📋 <b>Список реакций (страница %d/%d):</b>\n\n", i+1, len(messages))
		} else {
			header = scopeHeader
		}
		text := header + msg

		if err := c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeHTML}); err != nil {
			m.logger.Error("handleListReactions send failed", zap.Error(err), zap.Int("page", i+1), zap.Int("text_length", len(text)))
			return c.Send("❌ Не удалось отправить список реакций (ошибка API)")
		}

		// Небольшая задержка между сообщениями (защита от rate limit)
		if i < len(messages)-1 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	m.logger.Info("handleListReactions completed successfully", zap.Int("reactions_count", len(reactions)), zap.Int("pages_sent", len(messages)))
	return nil
}

func (m *ReactionsModule) handleRemoveReaction(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleRemoveReaction called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("Использование: /removereaction <id>\nПример: /removereaction 5")
	}

	reactionID := args[1]

	result, err := m.db.Exec(`
		DELETE FROM keyword_reactions WHERE chat_id = $1 AND id = $2
	`, chatID, reactionID)

	if err != nil {
		return c.Send("❌ Не удалось удалить реакцию")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Send("ℹ️ Реакция не найдена")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "remove_reaction",
		fmt.Sprintf("Removed reaction ID=%s (chat=%d)", reactionID, chatID))

	return c.Send(fmt.Sprintf("✅ Реакция #%s удалена", reactionID), &telebot.SendOptions{ParseMode: telebot.ModeHTML})
}
