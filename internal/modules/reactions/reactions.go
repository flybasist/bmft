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
	db      *sql.DB
	vipRepo *repositories.VIPRepository
	logger  *zap.Logger
	bot     *telebot.Bot
}

type KeywordReaction struct {
	ID              int64
	ChatID          int64
	Pattern         string
	ResponseType    string // "text", "sticker", "photo", etc.
	ResponseContent string // text content or file_id
	Description     string
	IsRegex         bool
	Cooldown        int
	DailyLimit      int
	DeleteOnLimit   bool
	IsActive        bool
}

func New(
	db *sql.DB,
	vipRepo *repositories.VIPRepository,
	logger *zap.Logger,
	bot *telebot.Bot,
) *ReactionsModule {
	return &ReactionsModule{
		db:      db,
		vipRepo: vipRepo,
		logger:  logger,
		bot:     bot,
	}
}

func (m *ReactionsModule) Name() string {
	return "reactions"
}

func (m *ReactionsModule) Commands() []core.BotCommand {
	return []core.BotCommand{
		{Command: "/addreaction", Description: "добавить реакцию на слово (reply или text) с опциональным дневным лимитом"},
		{Command: "/listreactions", Description: "список реакций"},
		{Command: "/removereaction", Description: "удалить реакцию"},
	}
}

func (m *ReactionsModule) OnMessage(ctx *core.MessageContext) error {
	msg := ctx.Message
	if msg.Private() || msg.Text == "" || strings.HasPrefix(msg.Text, "/") {
		return nil
	}

	chatID := msg.Chat.ID
	userID := msg.Sender.ID

	isVIP, _ := m.vipRepo.IsVIP(chatID, userID)
	if isVIP {
		return nil
	}

	reactions, err := m.loadReactions(chatID)
	if err != nil {
		m.logger.Error("failed to load reactions", zap.Error(err))
		return nil
	}

	for _, reaction := range reactions {
		if !reaction.IsActive {
			continue
		}

		matched := false
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

		if matched {
			if reaction.Cooldown > 0 {
				lastTriggered, err := m.getLastTriggered(chatID, reaction.ID)
				if err == nil && time.Since(lastTriggered) < time.Duration(reaction.Cooldown)*time.Second {
					m.logger.Debug("reaction on cooldown", zap.Int64("reaction_id", reaction.ID))
					continue
				}
			}

			if reaction.DailyLimit > 0 {
				count, err := m.getDailyCount(chatID, reaction.ID)
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
				m.incrementDailyCount(chatID, reaction.ID)
			}
			break
		}
	}

	return nil
}

func (m *ReactionsModule) loadReactions(chatID int64) ([]KeywordReaction, error) {
	// Русский комментарий: Читаем реакции напрямую из БД (без кеша).
	// Чтение ~1-2ms, не критично для производительности.
	rows, err := m.db.Query(`
		SELECT id, chat_id, pattern, response_type, response_content, description, is_regex, cooldown, daily_limit, delete_on_limit, is_active
		FROM keyword_reactions
		WHERE chat_id = $1 AND is_active = true
		ORDER BY id
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reactions []KeywordReaction
	for rows.Next() {
		var r KeywordReaction
		if err := rows.Scan(&r.ID, &r.ChatID, &r.Pattern, &r.ResponseType, &r.ResponseContent, &r.Description, &r.IsRegex, &r.Cooldown, &r.DailyLimit, &r.DeleteOnLimit, &r.IsActive); err != nil {
			m.logger.Error("failed to scan reaction", zap.Error(err))
			continue
		}
		reactions = append(reactions, r)
	}

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

func (m *ReactionsModule) getDailyCount(chatID, reactionID int64) (int, error) {
	var count int
	err := m.db.QueryRow(`
		SELECT count FROM reaction_daily_counters
		WHERE chat_id = $1 AND reaction_id = $2 AND counter_date = CURRENT_DATE
	`, chatID, reactionID).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return count, nil
}

func (m *ReactionsModule) incrementDailyCount(chatID, reactionID int64) {
	_, err := m.db.Exec(`
		INSERT INTO reaction_daily_counters (chat_id, reaction_id, counter_date, count)
		VALUES ($1, $2, CURRENT_DATE, 1)
		ON CONFLICT (chat_id, reaction_id, counter_date) DO UPDATE
		SET count = reaction_daily_counters.count + 1
	`, chatID, reactionID)
	if err != nil {
		m.logger.Error("failed to increment daily count", zap.Error(err))
	}
}

func (m *ReactionsModule) RegisterCommands(bot *telebot.Bot) {}

func (m *ReactionsModule) RegisterAdminCommands(bot *telebot.Bot) {
	bot.Handle("/addreaction", m.handleAddReaction)
	bot.Handle("/listreactions", m.handleListReactions)
	bot.Handle("/removereaction", m.handleRemoveReaction)
}

func (m *ReactionsModule) handleAddReaction(c telebot.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := c.Args()

	var responseType, responseContent, description string
	var pattern string
	var dailyLimit int
	var deleteOnLimit bool

	if c.Message().ReplyTo != nil {
		// Reply mode: get response from replied message
		if len(args) < 1 {
			return c.Send("Использование: /addreaction <pattern> [limit] [delete] (reply на сообщение со стикером/фото/etc.)\nПример: /addreaction привет 5 delete")
		}
		pattern = args[0]
		dailyLimit = 0
		deleteOnLimit = false
		remainingArgs := args[1:]
		if len(remainingArgs) > 0 && remainingArgs[len(remainingArgs)-1] == "delete" {
			deleteOnLimit = true
			remainingArgs = remainingArgs[:len(remainingArgs)-1]
		}
		if len(remainingArgs) > 0 {
			if l, err := strconv.Atoi(remainingArgs[0]); err == nil {
				dailyLimit = l
			}
		}
		description = strings.Join(remainingArgs, " ")

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
			return c.Send("Использование: /addreaction <pattern> <response> <description> [limit] [delete]\nИли reply на сообщение со стикером/фото/etc.\nПример: /addreaction привет Привет! Приветствие 10 delete")
		}
		pattern = args[0]
		responseType = "text"
		responseContent = args[1]
		description = args[2]
		dailyLimit = 0
		deleteOnLimit = false
		remainingArgs := args[3:]
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

	chatID := c.Chat().ID

	_, err = m.db.Exec(`
		INSERT INTO keyword_reactions (chat_id, pattern, response_type, response_content, description, is_regex, cooldown, daily_limit, delete_on_limit, is_active)
		VALUES ($1, $2, $3, $4, $5, false, 30, $6, $7, true)
	`, chatID, pattern, responseType, responseContent, description, dailyLimit, deleteOnLimit)

	if err != nil {
		m.logger.Error("failed to add reaction", zap.Error(err))
		return c.Send("❌ Не удалось добавить реакцию")
	}

	deleteMsg := ""
	if deleteOnLimit {
		deleteMsg = "\nУдалять при превышении лимита: да"
	}
	return c.Send(fmt.Sprintf("✅ Реакция добавлена\n\nПаттерн: %s\nТип ответа: %s\nСодержимое: %s\nОписание: %s\nДневной лимит: %d%s", pattern, responseType, responseContent, description, dailyLimit, deleteMsg))
}

func (m *ReactionsModule) handleListReactions(c telebot.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	chatID := c.Chat().ID
	reactions, err := m.loadReactions(chatID)
	if err != nil {
		return c.Send("❌ Не удалось получить список реакций")
	}

	if len(reactions) == 0 {
		return c.Send("ℹ️ В этом чате нет настроенных реакций")
	}

	text := "📋 *Список реакций:*\n\n"
	for i, r := range reactions {
		status := "✅"
		if !r.IsActive {
			status = "❌"
		}
		deleteMsg := "нет"
		if r.DeleteOnLimit {
			deleteMsg = "да"
		}
		text += fmt.Sprintf("%d. %s ID: %d\n   Паттерн: `%s`\n   Тип ответа: %s\n   Содержимое: %s\n   Описание: %s\n   Дневной лимит: %d\n   Удалять при превышении лимита: %s\n\n", i+1, status, r.ID, r.Pattern, r.ResponseType, r.ResponseContent, r.Description, r.DailyLimit, deleteMsg)
	}

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

func (m *ReactionsModule) handleRemoveReaction(c telebot.Context) error {
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
	chatID := c.Chat().ID

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

	return c.Send(fmt.Sprintf("✅ Реакция #%s удалена", reactionID))
}
