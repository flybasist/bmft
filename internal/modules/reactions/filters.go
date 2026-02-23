package reactions

// Этот файл содержит логику фильтрации сообщений,
// объединённую из бывших модулей TextFilter и ProfanityFilter.
// Фильтрация — часть модуля Reactions (единый pipeline текстового анализа).

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

// ProfanitySettings — настройки фильтра мата для конкретного чата/топика.
type ProfanitySettings struct {
	ChatID   int64
	ThreadID int64
	Action   string
	WarnText string
}

// ProfanityWord — слово из глобального словаря мата.
type ProfanityWord struct {
	Pattern  string
	IsRegex  bool
	Severity string
}

// checkProfanity проверяет сообщение на мат.
// textToCheck — уже подготовленный текст (caption или text).
// При ctx.MessageDeleted=true (сообщение удалено Limiter-ом): мат считается, metadata
// обновляется, banned_words проверяется (бан при превышении),
// но performProfanityAction НЕ вызывается — сообщение и так удалено.
// Возвращает true если мат обнаружен.
func (m *ReactionsModule) checkProfanity(ctx *core.MessageContext, chatID int64, threadID int, userID int64, textToCheck string) bool {
	// Загружаем настройки фильтра мата для чата/топика
	settings, err := m.loadProfanitySettings(chatID, threadID)
	if err != nil {
		m.logger.Error("failed to load profanity settings", zap.Error(err))
		return false
	}

	// Если настройки не найдены — фильтр мата не активен для этого чата
	if settings == nil {
		return false
	}

	// Загружаем глобальный словарь мата
	words, err := m.loadProfanityDictionary()
	if err != nil {
		m.logger.Error("failed to load profanity dictionary", zap.Error(err))
		return false
	}

	// Проверяем текст на совпадение со словарём
	textLower := strings.ToLower(textToCheck)
	for _, word := range words {
		matched := false
		if word.IsRegex {
			re, err := regexp.Compile(word.Pattern)
			if err != nil {
				continue
			}
			matched = re.MatchString(textLower)
		} else {
			matched = strings.Contains(textLower, strings.ToLower(word.Pattern))
		}

		if matched {
			m.logger.Info("profanity detected",
				zap.Int64("chat_id", chatID),
				zap.Int64("user_id", userID),
				zap.String("pattern", word.Pattern),
			)

			// Проверяем лимит banned_words (автобан при превышении)
			if m.checkProfanityLimit(ctx, chatID, threadID, userID) {
				return true // Пользователь забанен, pipeline останавливается
			}

			// Обновляем metadata сообщения (для подсчёта нарушений)
			profanityMeta := repositories.ProfanityMetadata{
				Detected: true,
				Action:   settings.Action,
			}
			if err := m.messageRepo.UpdateMessageMetadata(
				ctx.Chat.ID, ctx.Message.ID, "profanity", profanityMeta,
			); err != nil {
				m.logger.Error("failed to update profanity metadata", zap.Error(err))
			}

			// Выполняем действие только если сообщение ещё не удалено.
			// При ctx.MessageDeleted=true (Limiter удалил) мат посчитан,
			// metadata обновлена, banned_words проверен — но delete/warn не нужны.
			if !ctx.MessageDeleted {
				m.performProfanityAction(ctx, settings)
			}
			return true
		}
	}

	return false
}

// checkProfanityLimit проверяет лимит banned_words и банит пользователя при превышении.
// Возвращает true если пользователь забанен.
func (m *ReactionsModule) checkProfanityLimit(ctx *core.MessageContext, chatID int64, threadID int, userID int64) bool {
	limits, err := m.contentLimitsRepo.GetLimits(chatID, threadID, nil)
	if err != nil || limits == nil || limits.LimitBannedWords <= 0 {
		return false
	}

	// Считаем маты за сегодня через metadata сообщений
	count, err := m.messageRepo.GetTodayCountByMetadata(
		chatID, threadID, userID, "profanity", true,
	)
	if err != nil {
		m.logger.Error("failed to get today profanity count", zap.Error(err))
		return false
	}

	// +1 потому что текущее сообщение ещё не обновлено
	actualCount := count + 1

	// Предупреждение перед баном — как в rts_bot.
	// Если до бана осталось warning_threshold нарушений — предупреждаем.
	if actualCount < limits.LimitBannedWords {
		if limits.WarningThreshold > 0 && actualCount+limits.WarningThreshold >= limits.LimitBannedWords {
			warnMsg := fmt.Sprintf("⚠️ %s, у вас %d из %d нарушений за мат. При достижении лимита — бан.",
				core.DisplayName(ctx.Message.Sender), actualCount, limits.LimitBannedWords)
			if err := ctx.Send(warnMsg); err != nil {
				m.logger.Error("failed to send profanity warning", zap.Error(err))
			}
		}
		return false
	}

	m.logger.Warn("banned_words limit exceeded, banning user",
		zap.Int64("chat_id", chatID),
		zap.Int64("user_id", userID),
		zap.Int("count", actualCount),
		zap.Int("limit", limits.LimitBannedWords),
	)

	// Удаляем сообщение (если ещё не удалено Limiter-ом)
	if !ctx.MessageDeleted {
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("failed to delete message before ban", zap.Error(err))
		}
	}

	// Баним пользователя
	if err := ctx.Bot.Ban(ctx.Message.Chat, &telebot.ChatMember{
		User: ctx.Message.Sender,
	}); err != nil {
		m.logger.Error("failed to ban user", zap.Error(err))
	} else {
		banMsg := fmt.Sprintf("⛔ Пользователь %s забанен за превышение лимита ненормативной лексики (%d/%d)",
			core.DisplayName(ctx.Message.Sender), actualCount, limits.LimitBannedWords)
		ctx.Send(banMsg)
	}

	return true
}

// performProfanityAction выполняет действие при обнаружении мата.
func (m *ReactionsModule) performProfanityAction(ctx *core.MessageContext, settings *ProfanitySettings) {
	switch settings.Action {
	case "delete":
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("failed to delete message", zap.Error(err))
		}
	case "warn":
		warnText := settings.WarnText
		if warnText == "" {
			warnText = "⚠️ Использование ненормативной лексики запрещено."
		}
		ctx.SendReply(warnText)
	case "delete_warn":
		warnText := settings.WarnText
		if warnText == "" {
			warnText = "⚠️ Сообщение удалено: использование ненормативной лексики запрещено."
		}
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("failed to delete message", zap.Error(err))
		}
		ctx.Send(warnText)
	}
}

// performFilterAction выполняет действие для фильтра запрещённых слов (keyword_reactions с action).
func (m *ReactionsModule) performFilterAction(ctx *core.MessageContext, reaction KeywordReaction) {
	switch reaction.Action {
	case "delete":
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("failed to delete message", zap.Error(err))
		}
	case "warn":
		_ = ctx.SendReply(fmt.Sprintf("⚠️ %s, пожалуйста, следите за своими словами", core.DisplayName(ctx.Message.Sender)))
	case "delete_warn":
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("failed to delete message", zap.Error(err))
		}
		// Отправляем в чат без ReplyTo — сообщение уже удалено,
		// reply на удалённое вызывал ошибку Telegram API (message not found).
		// ctx.Send автоматически добавляет ThreadID для форумов.
		ctx.Send(fmt.Sprintf("🚫 %s, сообщение удалено за нарушение правил", core.DisplayName(ctx.Message.Sender)))
	}
}

// ============================================================================
// Загрузка данных из БД
// ============================================================================

// loadProfanitySettings загружает настройки фильтра мата для чата/топика.
// Логика fallback: сначала для конкретного топика, потом для всего чата.
func (m *ReactionsModule) loadProfanitySettings(chatID int64, threadID int) (*ProfanitySettings, error) {
	m.logger.Debug("loadProfanitySettings called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID))

	// Сначала пробуем загрузить для конкретного топика
	settings, err := m.queryProfanitySettings(chatID, threadID)
	if err != nil {
		return nil, err
	}
	if settings != nil {
		return settings, nil
	}

	// Если не найдено и это топик — пробуем общие настройки чата
	if threadID != 0 {
		return m.queryProfanitySettings(chatID, 0)
	}

	return nil, nil
}

// queryProfanitySettings загружает настройки для конкретного chat_id + thread_id.
func (m *ReactionsModule) queryProfanitySettings(chatID int64, threadID int) (*ProfanitySettings, error) {
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
		return nil, nil
	}
	if err != nil {
		m.logger.Error("queryProfanitySettings failed", zap.Error(err), zap.Int64("chat_id", chatID))
		return nil, err
	}

	return &settings, nil
}

// loadProfanityDictionary загружает глобальный словарь мата из БД.
func (m *ReactionsModule) loadProfanityDictionary() ([]ProfanityWord, error) {
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

// ============================================================================
// Обработчики команд фильтра запрещённых слов (бывший TextFilter)
// ============================================================================

// handleAddBan обрабатывает команду /addban — добавление запрещённого слова.
func (m *ReactionsModule) handleAddBan(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleAddBan called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	args := c.Args()
	if len(args) < 2 {
		return c.Send("Использование: /addban <pattern> <action>\nAction: delete, warn, delete_warn\nПример: /addban мат delete_warn")
	}

	action := args[len(args)-1]                      // Последний аргумент — действие
	pattern := strings.Join(args[:len(args)-1], " ") // Всё остальное — паттерн

	if action != "delete" && action != "warn" && action != "delete_warn" {
		return c.Send("❌ Action должен быть: delete, warn или delete_warn")
	}

	// Валидация длины pattern
	if len(pattern) == 0 {
		return c.Send("❌ Паттерн не может быть пустым")
	}
	if len(pattern) > 500 {
		return c.Send("❌ Паттерн слишком длинный (макс. 500 символов)")
	}

	// Автоопределение regex по наличию спецсимволов
	isRegex := false
	regexChars := []string{"|", "(", ")", "[", "]", ".", "*", "+", "?", "^", "$"}
	for _, char := range regexChars {
		if strings.Contains(pattern, char) {
			isRegex = true
			break
		}
	}

	// Если это regex, проверяем что он валидный
	if isRegex {
		if _, err := regexp.Compile(pattern); err != nil {
			return c.Send(fmt.Sprintf("❌ Некорректное regex-выражение: %v", err))
		}
	}

	// Убеждаемся что chat_id существует в таблице chats (для foreign key)
	_, err := m.db.Exec(`
		INSERT INTO chats (chat_id, chat_type, title)
		VALUES ($1, 'unknown', 'unknown')
		ON CONFLICT (chat_id) DO NOTHING
	`, chatID)
	if err != nil {
		m.logger.Error("failed to ensure chat exists", zap.Error(err))
		return c.Send("❌ Ошибка при проверке чата")
	}

	// Вставляем в keyword_reactions с полем action (фильтр, не реакция)
	_, err = m.db.Exec(`
		INSERT INTO keyword_reactions (chat_id, thread_id, pattern, is_regex, response_type, response_content, description, action, is_active)
		VALUES ($1, $2, $3, $4, 'none', '', '', $5, true)
	`, chatID, threadID, pattern, isRegex, action)

	if err != nil {
		m.logger.Error("failed to add banned word", zap.Error(err))
		return c.Send("❌ Не удалось добавить запрещённое слово")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "add_filter",
		fmt.Sprintf("Added filter: pattern='%s', action=%s (chat=%d, thread=%d)", pattern, action, chatID, threadID))

	var scopeMsg string
	if threadID != 0 {
		scopeMsg = fmt.Sprintf("✅ Запрещённое слово добавлено <b>для этого топика</b>\n\n💡 Для настройки всего чата используйте команду в основном чате\n\nПаттерн: <code>%s</code>\nДействие: %s", pattern, action)
	} else {
		scopeMsg = fmt.Sprintf("✅ Запрещённое слово добавлено <b>для всего чата</b>\n\n💡 Для настройки топика используйте команду внутри топика\n\nПаттерн: <code>%s</code>\nДействие: %s", pattern, action)
	}

	return c.Send(scopeMsg, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
}

// handleListBans обрабатывает команду /listbans — список запрещённых слов.
func (m *ReactionsModule) handleListBans(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleListBans called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	// Читаем только фильтры (action IS NOT NULL) из keyword_reactions
	rows, err := m.db.Query(`
		SELECT id, chat_id, thread_id, pattern, action, is_regex, is_active
		FROM keyword_reactions
		WHERE chat_id = $1 AND (thread_id = $2 OR thread_id = 0)
		  AND action IS NOT NULL
		ORDER BY thread_id DESC, id
	`, chatID, threadID)
	if err != nil {
		m.logger.Error("handleListBans query failed", zap.Error(err))
		return c.Send("❌ Не удалось получить список")
	}
	defer rows.Close()

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "list_filters",
		fmt.Sprintf("Admin viewed filters list (chat=%d, thread=%d)", chatID, threadID))

	type BanEntry struct {
		ID       int64
		ChatID   int64
		ThreadID int64
		Pattern  string
		Action   string
		IsRegex  bool
		IsActive bool
	}

	var bans []BanEntry
	for rows.Next() {
		var b BanEntry
		if err := rows.Scan(&b.ID, &b.ChatID, &b.ThreadID, &b.Pattern, &b.Action, &b.IsRegex, &b.IsActive); err != nil {
			m.logger.Error("failed to scan ban entry", zap.Error(err))
			continue
		}
		bans = append(bans, b)
	}

	if len(bans) == 0 {
		if threadID != 0 {
			return c.Send("ℹ️ В этом топике нет запрещённых слов")
		}
		return c.Send("ℹ️ В этом чате нет запрещённых слов")
	}

	var scopeHeader string
	if threadID != 0 {
		scopeHeader = "🚫 <b>Запрещённые слова (для этого топика):</b>\n\n"
	} else {
		scopeHeader = "🚫 <b>Запрещённые слова (для всего чата):</b>\n\n"
	}

	text := scopeHeader
	for i, b := range bans {
		status := "✅"
		if !b.IsActive {
			status = "❌"
		}
		scope := "чат"
		if b.ThreadID != 0 {
			scope = "топик"
		}
		text += fmt.Sprintf("%d. %s ID: %d [%s]\n   Паттерн: <code>%s</code>\n   Действие: %s\n\n", i+1, status, b.ID, scope, b.Pattern, b.Action)
	}

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
}

// handleRemoveBan обрабатывает команду /removeban — удаление запрещённого слова.
func (m *ReactionsModule) handleRemoveBan(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleRemoveBan called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("Использование: /removeban <id>\nПример: /removeban 3")
	}

	banID := args[1]

	// Удаляем только записи с action IS NOT NULL (фильтры, не реакции)
	result, err := m.db.Exec(`
		DELETE FROM keyword_reactions
		WHERE chat_id = $1 AND id = $2 AND action IS NOT NULL
	`, chatID, banID)

	if err != nil {
		return c.Send("❌ Не удалось удалить")
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return c.Send("ℹ️ Запись не найдена")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "remove_filter",
		fmt.Sprintf("Removed filter ID=%s (chat=%d)", banID, chatID))

	return c.Send(fmt.Sprintf("✅ Запрет #%s удалён", banID), &telebot.SendOptions{ParseMode: telebot.ModeHTML})
}

// ============================================================================
// Обработчики команд фильтра мата (бывший ProfanityFilter)
// ============================================================================

// handleSetProfanity обрабатывает команду /setprofanity — включение фильтра мата.
func (m *ReactionsModule) handleSetProfanity(c telebot.Context) error {
	m.logger.Info("handleSetProfanity called", zap.Int64("chat_id", c.Chat().ID), zap.Int64("user_id", c.Sender().ID))

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

	// Убеждаемся что chat_id существует в таблице chats (для foreign key).
	// profanity_settings имеет REFERENCES chats(chat_id) — без записи в chats INSERT упадёт.
	_, _ = m.db.Exec(`
		INSERT INTO chats (chat_id, chat_type, title)
		VALUES ($1, 'unknown', 'unknown')
		ON CONFLICT (chat_id) DO NOTHING
	`, chatID)

	_, err := m.db.Exec(`
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
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "set_profanity",
		fmt.Sprintf("Set profanity filter: action=%s (chat=%d, thread=%d)", action, chatID, threadID))

	scope := "этого топика"
	if threadID == 0 {
		scope = "всего чата"
	}

	return c.Reply(fmt.Sprintf("✅ Фильтр мата включен для %s\nДействие: %s", scope, action))
}

// handleRemoveProfanity обрабатывает команду /removeprofanity — выключение фильтра мата.
func (m *ReactionsModule) handleRemoveProfanity(c telebot.Context) error {
	m.logger.Info("handleRemoveProfanity called", zap.Int64("chat_id", c.Chat().ID), zap.Int64("user_id", c.Sender().ID))

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

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return c.Reply("ℹ️ Фильтр мата не был настроен")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "reactions", "remove_profanity",
		fmt.Sprintf("Removed profanity filter (chat=%d, thread=%d)", chatID, threadID))

	scope := "этого топика"
	if threadID == 0 {
		scope = "всего чата"
	}

	return c.Reply(fmt.Sprintf("✅ Фильтр мата отключен для %s", scope))
}

// handleProfanityStatus обрабатывает команду /profanitystatus — статус фильтра мата.
func (m *ReactionsModule) handleProfanityStatus(c telebot.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleProfanityStatus called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	settings, err := m.loadProfanitySettings(chatID, threadID)
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

	msg := "📊 <b>Статус фильтра мата</b>\n\n"
	msg += fmt.Sprintf("Область: %s\n", scope)
	msg += fmt.Sprintf("Действие: %s\n", settings.Action)

	var wordCount int
	m.db.QueryRow("SELECT COUNT(*) FROM profanity_dictionary").Scan(&wordCount)
	msg += fmt.Sprintf("\nСлов в словаре: %d", wordCount)

	return c.Send(msg, &telebot.SendOptions{ParseMode: telebot.ModeHTML})
}
