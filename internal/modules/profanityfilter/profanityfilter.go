package profanityfilter

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

type ProfanityFilterModule struct {
	db                *sql.DB
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	eventRepo         *repositories.EventRepository
	logger            *zap.Logger
	bot               *telebot.Bot
}

type ProfanitySettings struct {
	ChatID   int64
	ThreadID int64
	Action   string
	WarnText string
}

type ProfanityWord struct {
	Pattern  string
	IsRegex  bool
	Severity string
}

func New(
	db *sql.DB,
	vipRepo *repositories.VIPRepository,
	contentLimitsRepo *repositories.ContentLimitsRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	bot *telebot.Bot,
) *ProfanityFilterModule {
	return &ProfanityFilterModule{
		db:                db,
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		eventRepo:         eventRepo,
		logger:            logger,
		bot:               bot,
	}
}

func (m *ProfanityFilterModule) RegisterCommands(bot *telebot.Bot) {
	bot.Handle("/profanity", func(c telebot.Context) error {
		msg := "🚫 <b>Модуль ProfanityFilter</b> — Фильтр матерных слов\n\n"
		msg += "Автоматическое обнаружение и фильтрация ненормативной лексики.\n\n"
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

func (m *ProfanityFilterModule) RegisterAdminCommands(bot *telebot.Bot) {
	bot.Handle("/setprofanity", m.handleSetProfanity)
	bot.Handle("/removeprofanity", m.handleRemoveProfanity)
	bot.Handle("/profanitystatus", m.handleProfanityStatus)
}

func (m *ProfanityFilterModule) OnMessage(ctx *core.MessageContext) error {
	msg := ctx.Message
	if msg.Private() || msg.Text == "" || strings.HasPrefix(msg.Text, "/") {
		return nil
	}

	chatID := msg.Chat.ID
	threadID := core.GetThreadIDFromMessage(m.db, msg)
	userID := msg.Sender.ID

	// VIP-иммунитет
	isVIP, _ := m.vipRepo.IsVIP(chatID, threadID, userID)
	if isVIP {
		return nil
	}

	// Загружаем настройки
	settings, err := m.loadSettings(chatID, int64(threadID))
	if err != nil {
		m.logger.Error("failed to load profanity settings", zap.Error(err))
		return nil
	}

	// Если настройки не найдены - модуль неактивен
	if settings == nil {
		return nil
	}

	// Загружаем словарь
	words, err := m.loadDictionary()
	if err != nil {
		m.logger.Error("failed to load profanity dictionary", zap.Error(err))
		return nil
	}

	// Проверяем текст на мат
	for _, word := range words {
		matched := false
		if word.IsRegex {
			re, err := regexp.Compile(word.Pattern)
			if err != nil {
				continue
			}
			matched = re.MatchString(strings.ToLower(msg.Text))
		} else {
			matched = strings.Contains(strings.ToLower(msg.Text), strings.ToLower(word.Pattern))
		}

		if matched {
			m.logger.Info("profanity detected",
				zap.Int64("chat_id", chatID),
				zap.Int64("user_id", userID),
				zap.String("pattern", word.Pattern),
			)

			// Выполняем действие
			return m.performAction(ctx, settings)
		}
	}

	return nil
}

func (m *ProfanityFilterModule) performAction(ctx *core.MessageContext, settings *ProfanitySettings) error {
	switch settings.Action {
	case "delete":
		return ctx.DeleteMessage()

	case "warn":
		warnText := settings.WarnText
		if warnText == "" {
			warnText = "⚠️ Использование ненормативной лексики запрещено."
		}
		_, err := ctx.Bot.Reply(ctx.Message, warnText)
		return err

	case "delete_warn":
		warnText := settings.WarnText
		if warnText == "" {
			warnText = "⚠️ Сообщение удалено: использование ненормативной лексики запрещено."
		}
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("failed to delete message", zap.Error(err))
		}
		_, err := ctx.Bot.Send(ctx.Message.Chat, warnText)
		return err
	}

	return nil
}

func (m *ProfanityFilterModule) loadSettings(chatID, threadID int64) (*ProfanitySettings, error) {
	m.logger.Debug("loadSettings called", zap.Int64("chat_id", chatID), zap.Int64("thread_id", threadID))

	// Сначала пробуем загрузить для конкретного топика
	settings, err := m.querySettings(chatID, threadID)
	if err != nil {
		return nil, err
	}
	if settings != nil {
		return settings, nil
	}

	// Если не найдено и это топик - пробуем общие настройки чата
	if threadID != 0 {
		return m.querySettings(chatID, 0)
	}

	return nil, nil
}

func (m *ProfanityFilterModule) querySettings(chatID, threadID int64) (*ProfanitySettings, error) {
	var settings ProfanitySettings
	err := m.db.QueryRow(`
		SELECT chat_id, thread_id, action, COALESCE(warn_text, '')
		FROM profanity_settings
		WHERE chat_id = $1 AND thread_id = $2
	`, chatID, threadID).Scan(
		&settings.ChatID,
		&settings.ThreadID,
		&settings.Action,
		&settings.WarnText,
	)

	if err == sql.ErrNoRows {
		m.logger.Debug("querySettings no rows", zap.Int64("chat_id", chatID), zap.Int64("thread_id", threadID))
		return nil, nil
	}
	if err != nil {
		m.logger.Error("querySettings failed", zap.Error(err), zap.Int64("chat_id", chatID))
		return nil, err
	}

	return &settings, nil
}

func (m *ProfanityFilterModule) loadDictionary() ([]ProfanityWord, error) {
	rows, err := m.db.Query(`
		SELECT pattern, is_regex, severity
		FROM profanity_dictionary
		ORDER BY severity DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var words []ProfanityWord
	for rows.Next() {
		var word ProfanityWord
		if err := rows.Scan(&word.Pattern, &word.IsRegex, &word.Severity); err != nil {
			continue
		}
		words = append(words, word)
	}

	return words, nil
}

func (m *ProfanityFilterModule) handleSetProfanity(c telebot.Context) error {
	m.logger.Info("handleSetProfanity called", zap.Int64("chat_id", c.Chat().ID), zap.Int64("user_id", c.Sender().ID))

	// Проверка прав администратора
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	action := c.Message().Payload
	if action == "" {
		action = "delete"
	}

	validActions := map[string]bool{"delete": true, "warn": true, "delete_warn": true}
	if !validActions[action] {
		return c.Reply("❌ Неверное действие. Доступные: delete, warn, delete_warn")
	}

	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	_, err = m.db.Exec(`
		INSERT INTO profanity_settings (chat_id, thread_id, action, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (chat_id, thread_id)
		DO UPDATE SET action = $3, updated_at = NOW()
	`, chatID, threadID, action)

	if err != nil {
		m.logger.Error("failed to set profanity filter", zap.Error(err))
		return c.Reply("❌ Ошибка при настройке фильтра")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "profanityfilter", "set_profanity",
		fmt.Sprintf("Set profanity filter: action=%s (chat=%d, thread=%d)", action, chatID, threadID))

	scope := "этого топика"
	if threadID == 0 {
		scope = "всего чата"
	}

	return c.Reply(fmt.Sprintf("✅ Фильтр мата включен для %s\nДействие: %s", scope, action))
}

func (m *ProfanityFilterModule) handleRemoveProfanity(c telebot.Context) error {
	m.logger.Info("handleRemoveProfanity called", zap.Int64("chat_id", c.Chat().ID), zap.Int64("user_id", c.Sender().ID))

	// Проверка прав администратора
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	result, err := m.db.Exec(`
		DELETE FROM profanity_settings
		WHERE chat_id = $1 AND thread_id = $2
	`, chatID, threadID)

	if err != nil {
		m.logger.Error("failed to remove profanity filter", zap.Error(err))
		return c.Reply("❌ Ошибка при отключении фильтра")
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return c.Reply("ℹ️ Фильтр мата не был настроен")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "profanityfilter", "remove_profanity",
		fmt.Sprintf("Removed profanity filter (chat=%d, thread=%d)", chatID, threadID))

	scope := "этого топика"
	if threadID == 0 {
		scope = "всего чата"
	}

	return c.Reply(fmt.Sprintf("✅ Фильтр мата отключен для %s", scope))
}

func (m *ProfanityFilterModule) handleProfanityStatus(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleProfanityStatus called", zap.Int64("chat_id", chatID), zap.Int64("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	settings, err := m.loadSettings(chatID, threadID)
	if err != nil {
		return c.Reply("❌ Ошибка при загрузке настроек")
	}

	if settings == nil {
		return c.Reply("ℹ️ Фильтр мата не настроен")
	}

	scope := "топика"
	if settings.ThreadID == 0 {
		scope = "чата"
	}

	msg := "📊 **Статус фильтра мата**\n\n"
	msg += fmt.Sprintf("Область: %s\n", scope)
	msg += fmt.Sprintf("Действие: %s\n", settings.Action)

	var wordCount int
	m.db.QueryRow("SELECT COUNT(*) FROM profanity_dictionary").Scan(&wordCount)
	msg += fmt.Sprintf("\nСлов в словаре: %d", wordCount)

	return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}
