package textfilter

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

type TextFilterModule struct {
	db                *sql.DB
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	eventRepo         *repositories.EventRepository
	logger            *zap.Logger
	bot               *telebot.Bot
}

type BannedWord struct {
	ID       int64
	ChatID   int64
	ThreadID int64
	Pattern  string
	Action   string
	IsRegex  bool
	IsActive bool
}

func New(
	db *sql.DB,
	vipRepo *repositories.VIPRepository,
	contentLimitsRepo *repositories.ContentLimitsRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	bot *telebot.Bot,
) *TextFilterModule {
	return &TextFilterModule{
		db:                db,
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		eventRepo:         eventRepo,
		logger:            logger,
		bot:               bot,
	}
}

func (m *TextFilterModule) Name() string {
	return "textfilter"
}

// RegisterCommands регистрирует команды модуля в боте.
func (m *TextFilterModule) RegisterCommands(bot *telebot.Bot) {
	// /textfilter — справка по модулю
	bot.Handle("/textfilter", func(c telebot.Context) error {
		msg := "🚫 **Модуль TextFilter** — Фильтр запрещённых слов\n\n"
		msg += "Автоматическое удаление сообщений с запрещёнными словами и фразами.\n\n"
		msg += "**Доступные команды:**\n\n"

		msg += "🔹 `/addban <паттерн> [действие]` — Добавить бан-слово (только админы)\n"
		msg += "   Действия: delete (удалить), warn (предупредить), delete_warn (удалить и предупредить)\n"
		msg += "   📌 Примеры:\n"
		msg += "   • `/addban спам delete` — удалять сообщения со словом \"спам\"\n"
		msg += "   • `/addban (мат|ругательство) delete_warn` — удалять и предупреждать\n"
		msg += "   • `/addban реклама warn` — только предупреждение, без удаления\n"
		msg += "   • `/addban (?i)bad_word delete` — без учёта регистра\n\n"

		msg += "🔹 `/listbans` — Список всех запрещённых слов\n"
		msg += "   Показывает все активные фильтры с их ID и действиями\n"
		msg += "   📌 Пример: `/listbans`\n\n"

		msg += "🔹 `/removeban <ID>` — Удалить бан-слово (только админы)\n"
		msg += "   ID можно узнать из команды /listbans\n"
		msg += "   📌 Пример: `/removeban 7`\n\n"

		msg += "⚙️ **Работа с топиками:**\n"
		msg += "• Команда в **топике** настраивает фильтры только для этого топика\n"
		msg += "• Команда в **основном чате** настраивает фильтры для всего чата\n"
		msg += "• Если фильтр для топика не установлен, используется общий фильтр чата\n\n"

		msg += "⚠️ **Виды действий:**\n"
		msg += "• `delete` — просто удалить сообщение\n"
		msg += "• `warn` — предупредить пользователя (сообщение остаётся)\n"
		msg += "• `delete_warn` — удалить сообщение И отправить предупреждение\n\n"

		msg += "🛡️ *VIP-защита:* VIP-пользователи не попадают под фильтр запрещённых слов."

		return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	})
}

// RegisterAdminCommands регистрирует админские команды.
func (m *TextFilterModule) RegisterAdminCommands(bot *telebot.Bot) {
	bot.Handle("/addban", m.handleAddBan)
	bot.Handle("/listbans", m.handleListBans)
	bot.Handle("/removeban", m.handleRemoveBan)
}

func (m *TextFilterModule) OnMessage(ctx *core.MessageContext) error {
	msg := ctx.Message
	if msg.Private() || msg.Text == "" || strings.HasPrefix(msg.Text, "/") {
		return nil
	}

	chatID := msg.Chat.ID
	threadID := msg.ThreadID
	userID := msg.Sender.ID

	isVIP, _ := m.vipRepo.IsVIP(chatID, threadID, userID)
	if isVIP {
		return nil
	}

	words, err := m.loadBannedWords(chatID, int64(threadID))
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

func (m *TextFilterModule) loadBannedWords(chatID int64, threadID int64) ([]BannedWord, error) {
	// Русский комментарий: Читаем запрещённые слова напрямую из БД (без кеша).
	// Чтение ~1-2ms, не критично для производительности.
	// Fallback: сначала для топика, потом для всего чата
	rows, err := m.db.Query(`
		SELECT id, chat_id, thread_id, pattern, action, is_regex, is_active
		FROM banned_words
		WHERE chat_id = $1 AND (thread_id = $2 OR thread_id = 0) AND is_active = true
		ORDER BY thread_id DESC, id
	`, chatID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []BannedWord
	for rows.Next() {
		var w BannedWord
		if err := rows.Scan(&w.ID, &w.ChatID, &w.ThreadID, &w.Pattern, &w.Action, &w.IsRegex, &w.IsActive); err != nil {
			m.logger.Error("failed to scan banned word", zap.Error(err))
			continue
		}
		words = append(words, w)
	}

	return words, nil
}

func (m *TextFilterModule) handleAddBan(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := int64(0)
	if c.Message().ThreadID != 0 {
		threadID = int64(c.Message().ThreadID)
	}

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
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

	_, err = m.db.Exec(`
		INSERT INTO banned_words (chat_id, thread_id, pattern, action, is_regex, is_active)
		VALUES ($1, $2, $3, $4, false, true)
	`, chatID, threadID, pattern, action)

	if err != nil {
		m.logger.Error("failed to add banned word", zap.Error(err))
		return c.Send("❌ Не удалось добавить запрещённое слово")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "textfilter", "add_filter",
		fmt.Sprintf("Added filter: pattern='%s', action=%s (chat=%d, thread=%d)", pattern, action, chatID, threadID))

	var scopeMsg string
	if threadID != 0 {
		scopeMsg = fmt.Sprintf("✅ Запрещённое слово добавлено **для этого топика**\n\n💡 Для настройки всего чата используйте команду в основном чате\n\nПаттерн: %s\nДействие: %s", pattern, action)
	} else {
		scopeMsg = fmt.Sprintf("✅ Запрещённое слово добавлено **для всего чата**\n\n💡 Для настройки топика используйте команду внутри топика\n\nПаттерн: %s\nДействие: %s", pattern, action)
	}

	return c.Send(scopeMsg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

func (m *TextFilterModule) handleListBans(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := int64(0)
	if c.Message().ThreadID != 0 {
		threadID = int64(c.Message().ThreadID)
	}

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	words, err := m.loadBannedWords(chatID, threadID)
	if err != nil {
		return c.Send("❌ Не удалось получить список")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "textfilter", "list_filters",
		fmt.Sprintf("Admin viewed filters list (chat=%d, thread=%d)", chatID, threadID))

	if len(words) == 0 {
		if threadID != 0 {
			return c.Send("ℹ️ В этом топике нет запрещённых слов")
		}
		return c.Send("ℹ️ В этом чате нет запрещённых слов")
	}

	var scopeHeader string
	if threadID != 0 {
		scopeHeader = "🚫 *Запрещённые слова (для этого топика):*\n\n"
	} else {
		scopeHeader = "🚫 *Запрещённые слова (для всего чата):*\n\n"
	}

	text := scopeHeader
	for i, w := range words {
		status := "✅"
		if !w.IsActive {
			status = "❌"
		}
		scope := "чат"
		if w.ThreadID != 0 {
			scope = "топик"
		}
		text += fmt.Sprintf("%d. %s ID: %d [%s]\n   Паттерн: `%s`\n   Действие: %s\n\n", i+1, status, w.ID, scope, w.Pattern, w.Action)
	}

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

func (m *TextFilterModule) handleRemoveBan(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := int64(0)
	if c.Message().ThreadID != 0 {
		threadID = int64(c.Message().ThreadID)
	}

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("Использование: /removeban <id>\nПример: /removeban 3")
	}

	banID := args[1]

	result, err := m.db.Exec(`
		DELETE FROM banned_words WHERE chat_id = $1 AND thread_id = $2 AND id = $3
	`, chatID, threadID, banID)

	if err != nil {
		return c.Send("❌ Не удалось удалить")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Send("ℹ️ Запись не найдена")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "textfilter", "remove_filter",
		fmt.Sprintf("Removed filter ID=%s (chat=%d, thread=%d)", banID, chatID, threadID))

	var scopeMsg string
	if threadID != 0 {
		scopeMsg = fmt.Sprintf("✅ Запрет #%s удалён **для этого топика**\n\n💡 Для удаления запрета всего чата используйте команду в основном чате", banID)
	} else {
		scopeMsg = fmt.Sprintf("✅ Запрет #%s удалён **для всего чата**\n\n💡 Для удаления запрета топика используйте команду внутри топика", banID)
	}

	return c.Send(scopeMsg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}
