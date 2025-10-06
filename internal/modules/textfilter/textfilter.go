package textfilter

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

type TextFilterModule struct {
	db                *sql.DB
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	logger            *zap.Logger
	adminUsers        []int64
	cache             map[int64][]BannedWord
	lastLoad          time.Time
}

type BannedWord struct {
	ID       int64
	ChatID   int64
	Pattern  string
	Action   string
	IsRegex  bool
	IsActive bool
}

func New(
	db *sql.DB,
	vipRepo *repositories.VIPRepository,
	contentLimitsRepo *repositories.ContentLimitsRepository,
	logger *zap.Logger,
) *TextFilterModule {
	return &TextFilterModule{
		db:                db,
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		logger:            logger,
		adminUsers:        []int64{},
		cache:             make(map[int64][]BannedWord),
	}
}

func (m *TextFilterModule) Name() string {
	return "textfilter"
}

func (m *TextFilterModule) Init(deps core.ModuleDependencies) error {
	m.logger.Info("textfilter module initialized")
	return nil
}

func (m *TextFilterModule) Commands() []core.BotCommand {
	return []core.BotCommand{}
}

func (m *TextFilterModule) Enabled(chatID int64) (bool, error) {
	return true, nil
}

func (m *TextFilterModule) OnMessage(ctx *core.MessageContext) error {
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

	words, err := m.loadBannedWords(chatID)
	if err != nil {
		m.logger.Error("failed to load banned words", zap.Error(err))
		return nil
	}

	for _, word := range words {
		if !word.IsActive {
			continue
		}

		matched := false
		if word.IsRegex {
			re, err := regexp.Compile(word.Pattern)
			if err != nil {
				m.logger.Warn("invalid regex pattern", zap.String("pattern", word.Pattern))
				continue
			}
			matched = re.MatchString(msg.Text)
		} else {
			matched = strings.Contains(strings.ToLower(msg.Text), strings.ToLower(word.Pattern))
		}

		if matched {
			m.logger.Info("banned word detected",
				zap.Int64("chat_id", chatID),
				zap.Int64("user_id", userID),
				zap.String("pattern", word.Pattern),
			)

			if err := m.contentLimitsRepo.IncrementCounter(chatID, userID, "banned_words"); err != nil {
				m.logger.Error("failed to increment banned words counter", zap.Error(err))
			}

			switch word.Action {
			case "delete":
				if err := ctx.DeleteMessage(); err != nil {
					m.logger.Error("failed to delete message", zap.Error(err))
				}
			case "warn":
				_ = ctx.SendReply(fmt.Sprintf("⚠️ @%s, пожалуйста, следите за своими словами", msg.Sender.Username))
			case "delete_warn":
				if err := ctx.DeleteMessage(); err != nil {
					m.logger.Error("failed to delete message", zap.Error(err))
				}
				_ = ctx.SendReply(fmt.Sprintf("🚫 @%s, сообщение удалено за нарушение правил", msg.Sender.Username))
			}

			return nil
		}
	}

	return nil
}

func (m *TextFilterModule) Shutdown() error {
	m.logger.Info("textfilter module shutdown")
	return nil
}

func (m *TextFilterModule) loadBannedWords(chatID int64) ([]BannedWord, error) {
	if words, ok := m.cache[chatID]; ok && time.Since(m.lastLoad) < 5*time.Minute {
		return words, nil
	}

	rows, err := m.db.Query(`
		SELECT id, chat_id, pattern, action, is_regex, is_active
		FROM banned_words
		WHERE chat_id = $1 AND is_active = true
		ORDER BY id
	`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []BannedWord
	for rows.Next() {
		var w BannedWord
		if err := rows.Scan(&w.ID, &w.ChatID, &w.Pattern, &w.Action, &w.IsRegex, &w.IsActive); err != nil {
			m.logger.Error("failed to scan banned word", zap.Error(err))
			continue
		}
		words = append(words, w)
	}

	m.cache[chatID] = words
	m.lastLoad = time.Now()
	return words, nil
}

func (m *TextFilterModule) RegisterCommands(bot *telebot.Bot) {}

func (m *TextFilterModule) RegisterAdminCommands(bot *telebot.Bot) {
	bot.Handle("/addban", m.handleAddBan)
	bot.Handle("/listbans", m.handleListBans)
	bot.Handle("/removeban", m.handleRemoveBan)
}

func (m *TextFilterModule) handleAddBan(c telebot.Context) error {
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := strings.SplitN(c.Text(), " ", 3)
	if len(args) < 3 {
		return c.Send("Использование: /addban <pattern> <action>\nAction: delete, warn, delete_warn\nПример: /addban мат delete_warn")
	}

	pattern := args[1]
	action := args[2]

	if action != "delete" && action != "warn" && action != "delete_warn" {
		return c.Send("❌ Action должен быть: delete, warn или delete_warn")
	}

	chatID := c.Chat().ID

	_, err := m.db.Exec(`
		INSERT INTO banned_words (chat_id, pattern, action, is_regex, is_active)
		VALUES ($1, $2, $3, false, true)
	`, chatID, pattern, action)

	if err != nil {
		m.logger.Error("failed to add banned word", zap.Error(err))
		return c.Send("❌ Не удалось добавить запрещённое слово")
	}

	delete(m.cache, chatID)
	return c.Send(fmt.Sprintf("✅ Запрещённое слово добавлено\n\nПаттерн: %s\nДействие: %s", pattern, action))
}

func (m *TextFilterModule) handleListBans(c telebot.Context) error {
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Команда доступна только администраторам")
	}

	chatID := c.Chat().ID
	words, err := m.loadBannedWords(chatID)
	if err != nil {
		return c.Send("❌ Не удалось получить список")
	}

	if len(words) == 0 {
		return c.Send("ℹ️ В этом чате нет запрещённых слов")
	}

	text := "🚫 *Запрещённые слова:*\n\n"
	for i, w := range words {
		status := "✅"
		if !w.IsActive {
			status = "❌"
		}
		text += fmt.Sprintf("%d. %s ID: %d\n   Паттерн: `%s`\n   Действие: %s\n\n", i+1, status, w.ID, w.Pattern, w.Action)
	}

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

func (m *TextFilterModule) handleRemoveBan(c telebot.Context) error {
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("Использование: /removeban <id>\nПример: /removeban 3")
	}

	banID := args[1]
	chatID := c.Chat().ID

	result, err := m.db.Exec(`
		DELETE FROM banned_words WHERE chat_id = $1 AND id = $2
	`, chatID, banID)

	if err != nil {
		return c.Send("❌ Не удалось удалить")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Send("ℹ️ Запись не найдена")
	}

	delete(m.cache, chatID)
	return c.Send(fmt.Sprintf("✅ Запрет #%s удалён", banID))
}

func (m *TextFilterModule) isAdmin(userID int64) bool {
	for _, id := range m.adminUsers {
		if id == userID {
			return true
		}
	}
	return false
}

func (m *TextFilterModule) SetAdminUsers(adminUsers []int64) {
	m.adminUsers = adminUsers
	m.logger.Info("admin users updated", zap.Int("count", len(adminUsers)))
}
