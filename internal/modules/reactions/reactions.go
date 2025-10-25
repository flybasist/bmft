package reactions

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

type ReactionsModule struct {
	db       *sql.DB
	vipRepo  *repositories.VIPRepository
	logger   *zap.Logger
	bot      *telebot.Bot
	cache    map[int64][]KeywordReaction
	lastLoad time.Time
}

type KeywordReaction struct {
	ID          int64
	ChatID      int64
	Pattern     string
	Response    string
	Description string
	IsRegex     bool
	Cooldown    int
	IsActive    bool
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
		cache:   make(map[int64][]KeywordReaction),
	}
}

func (m *ReactionsModule) Name() string {
	return "reactions"
}

func (m *ReactionsModule) Init(deps core.ModuleDependencies) error {
	m.logger.Info("reactions module initialized")
	return nil
}

func (m *ReactionsModule) Commands() []core.BotCommand {
	return []core.BotCommand{}
}

func (m *ReactionsModule) Enabled(chatID int64) (bool, error) {
	return true, nil
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

			if err := ctx.SendReply(reaction.Response); err != nil {
				m.logger.Error("failed to send reaction", zap.Error(err))
			}

			m.recordTrigger(chatID, reaction.ID, userID)
			break
		}
	}

	return nil
}

func (m *ReactionsModule) Shutdown() error {
	m.logger.Info("reactions module shutdown")
	return nil
}

func (m *ReactionsModule) loadReactions(chatID int64) ([]KeywordReaction, error) {
	if reactions, ok := m.cache[chatID]; ok && time.Since(m.lastLoad) < 5*time.Minute {
		return reactions, nil
	}

	rows, err := m.db.Query(`
		SELECT id, chat_id, pattern, response, description, is_regex, cooldown, is_active
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
		if err := rows.Scan(&r.ID, &r.ChatID, &r.Pattern, &r.Response, &r.Description, &r.IsRegex, &r.Cooldown, &r.IsActive); err != nil {
			m.logger.Error("failed to scan reaction", zap.Error(err))
			continue
		}
		reactions = append(reactions, r)
	}

	m.cache[chatID] = reactions
	m.lastLoad = time.Now()
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

	args := strings.SplitN(c.Text(), " ", 4)
	if len(args) < 4 {
		return c.Send("Использование: /addreaction <pattern> <response> <description>\nПример: /addreaction привет Привет! Приветствие")
	}

	pattern := args[1]
	response := args[2]
	description := args[3]

	chatID := c.Chat().ID

	_, err = m.db.Exec(`
		INSERT INTO keyword_reactions (chat_id, pattern, response, description, is_regex, cooldown, is_active)
		VALUES ($1, $2, $3, $4, false, 30, true)
	`, chatID, pattern, response, description)

	if err != nil {
		m.logger.Error("failed to add reaction", zap.Error(err))
		return c.Send("❌ Не удалось добавить реакцию")
	}

	delete(m.cache, chatID)
	return c.Send(fmt.Sprintf("✅ Реакция добавлена\n\nПаттерн: %s\nОтвет: %s\nОписание: %s", pattern, response, description))
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
		text += fmt.Sprintf("%d. %s ID: %d\n   Паттерн: `%s`\n   Ответ: %s\n   Описание: %s\n\n", i+1, status, r.ID, r.Pattern, r.Response, r.Description)
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

	delete(m.cache, chatID)
	return c.Send(fmt.Sprintf("✅ Реакция #%s удалена", reactionID))
}
