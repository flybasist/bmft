package reactions

import (
	"fmt"
	"html"
	"strconv"

	"go.uber.org/zap"
	telebot "gopkg.in/telebot.v3"
)

// UniqueMenuDelReaction — Unique для inline-кнопки 🗑 реакции в меню.
const UniqueMenuDelReaction = "m_rm_react"

// UniqueMenuDelBan — Unique для inline-кнопки 🗑 фильтра в меню.
const UniqueMenuDelBan = "m_rm_ban"

// RenderListReactions возвращает HTML-текст со списком реакций (action IS NULL).
// Второй return — slice ID для кнопок 🗑 (не более maxButtons).
// Пустая строка + nil = нет реакций.
func (m *ReactionsModule) RenderListReactions(chatID int64, threadID int, maxButtons int) (string, []int64, error) {
	rows, err := m.db.Query(`
		SELECT id, thread_id, COALESCE(user_id, 0), pattern, response_type, response_content, COALESCE(description, ''),
			COALESCE(trigger_content_type, ''), cooldown, daily_limit, delete_on_limit, is_active
		FROM keyword_reactions
		WHERE chat_id = $1 AND (thread_id = $2 OR thread_id = 0)
		  AND action IS NULL
		ORDER BY thread_id DESC, id
	`, chatID, threadID)
	if err != nil {
		return "", nil, fmt.Errorf("не удалось получить реакции")
	}
	defer rows.Close()

	type reaction struct {
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

	var reactions []reaction
	for rows.Next() {
		var r reaction
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.UserID, &r.Pattern, &r.ResponseType,
			&r.ResponseContent, &r.Description, &r.TriggerContentType,
			&r.Cooldown, &r.DailyLimit, &r.DeleteOnLimit, &r.IsActive); err != nil {
			m.logger.Error("RenderListReactions scan", zap.Error(err))
			continue
		}
		reactions = append(reactions, r)
	}

	if len(reactions) == 0 {
		return "", nil, nil
	}

	location := "чата"
	if threadID != 0 {
		location = "топика"
	}

	text := fmt.Sprintf("📋 <b>Реакции %s (%d):</b>\n\n", location, len(reactions))

	var ids []int64
	// Ограничиваем длину: inline-сообщение макс ~4000 символов, оставляем запас для кнопок.
	const maxLen = 3200
	truncated := false
	for i, r := range reactions {
		status := "✅"
		if !r.IsActive {
			status = "❌"
		}
		scope := "чат"
		if r.ThreadID != 0 {
			scope = "топик"
		}

		preview := r.ResponseContent
		switch r.ResponseType {
		case "text":
			if len(preview) > 30 {
				preview = preview[:30] + "…"
			}
			preview = fmt.Sprintf("<code>%s</code>", html.EscapeString(preview))
		default:
			preview = fmt.Sprintf("<i>%s</i>", r.ResponseType)
		}

		line := fmt.Sprintf("%d. %s #%d [%s] <code>%s</code> → %s\n",
			i+1, status, r.ID, scope, html.EscapeString(r.Pattern), preview)

		if len(text)+len(line) > maxLen {
			truncated = true
			break
		}
		text += line

		if len(ids) < maxButtons {
			ids = append(ids, r.ID)
		}
	}
	if truncated {
		text += fmt.Sprintf("\n<i>…и ещё %d реакций</i>", len(reactions)-len(ids))
	}

	text += "\n\n💡 Добавить: /addreaction\n🗑 Удалить: кнопки ниже"

	return text, ids, nil
}

// RenderListBans возвращает HTML-текст со списком фильтров (action IS NOT NULL).
// Второй return — slice ID для кнопок 🗑.
func (m *ReactionsModule) RenderListBans(chatID int64, threadID int, maxButtons int) (string, []int64, error) {
	rows, err := m.db.Query(`
		SELECT id, thread_id, pattern, action, is_active
		FROM keyword_reactions
		WHERE chat_id = $1 AND (thread_id = $2 OR thread_id = 0)
		  AND action IS NOT NULL
		ORDER BY thread_id DESC, id
	`, chatID, threadID)
	if err != nil {
		return "", nil, fmt.Errorf("не удалось получить фильтры")
	}
	defer rows.Close()

	type ban struct {
		ID       int64
		ThreadID int64
		Pattern  string
		Action   string
		IsActive bool
	}

	var bans []ban
	for rows.Next() {
		var b ban
		if err := rows.Scan(&b.ID, &b.ThreadID, &b.Pattern, &b.Action, &b.IsActive); err != nil {
			m.logger.Error("RenderListBans scan", zap.Error(err))
			continue
		}
		bans = append(bans, b)
	}

	if len(bans) == 0 {
		return "", nil, nil
	}

	location := "чата"
	if threadID != 0 {
		location = "топика"
	}

	text := fmt.Sprintf("🚫 <b>Фильтры %s (%d):</b>\n\n", location, len(bans))

	var ids []int64
	const maxLen = 3200
	truncated := false
	for i, b := range bans {
		status := "✅"
		if !b.IsActive {
			status = "❌"
		}
		scope := "чат"
		if b.ThreadID != 0 {
			scope = "топик"
		}

		line := fmt.Sprintf("%d. %s #%d [%s] <code>%s</code> → %s\n",
			i+1, status, b.ID, scope, html.EscapeString(b.Pattern), b.Action)

		if len(text)+len(line) > maxLen {
			truncated = true
			break
		}
		text += line

		if len(ids) < maxButtons {
			ids = append(ids, b.ID)
		}
	}
	if truncated {
		text += fmt.Sprintf("\n<i>…и ещё %d фильтров</i>", len(bans)-len(ids))
	}

	text += "\n\n💡 Запретить слово: /addban\n💡 Запретить инлайн-бота: /addviaban\n🗑 Удалить: кнопки ниже"

	return text, ids, nil
}

// RenderProfanityStatus возвращает HTML-текст статуса фильтра мата.
// Пустая строка = фильтр не настроен.
func (m *ReactionsModule) RenderProfanityStatus(chatID int64, threadID int) (string, error) {
	settings, err := m.loadProfanitySettings(chatID, threadID)
	if err != nil {
		return "", fmt.Errorf("ошибка при загрузке настроек")
	}
	if settings == nil {
		return "", nil
	}

	scope := "топика"
	if settings.ThreadID == 0 {
		scope = "чата"
	}

	text := "🔞 <b>Фильтр мата</b>\n\n"
	text += fmt.Sprintf("Область: %s\n", scope)
	text += fmt.Sprintf("Действие: %s\n", settings.Action)

	var wordCount int
	_ = m.db.QueryRow("SELECT COUNT(*) FROM profanity_dictionary").Scan(&wordCount)
	text += fmt.Sprintf("\nСлов в словаре: %d", wordCount)

	text += "\n\n💡 Выключить: /removeprofanity"

	return text, nil
}

// DeleteReactionByID удаляет реакцию по ID. Используется inline-кнопкой 🗑 в меню.
func (m *ReactionsModule) DeleteReactionByID(chatID int64, reactionID int64, adminID int64) error {
	result, err := m.db.Exec(`DELETE FROM keyword_reactions WHERE chat_id = $1 AND id = $2 AND action IS NULL`, chatID, reactionID)
	if err != nil {
		m.logger.Error("DeleteReactionByID failed", zap.Error(err), zap.Int64("id", reactionID))
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("not found")
	}
	_ = m.eventRepo.Log(chatID, adminID, "reactions", "remove_reaction",
		fmt.Sprintf("Removed reaction ID=%d via menu (chat=%d)", reactionID, chatID))
	return nil
}

// DeleteBanByID удаляет фильтр по ID. Используется inline-кнопкой 🗑 в меню.
func (m *ReactionsModule) DeleteBanByID(chatID int64, banID int64, adminID int64) error {
	result, err := m.db.Exec(`DELETE FROM keyword_reactions WHERE chat_id = $1 AND id = $2 AND action IS NOT NULL`, chatID, banID)
	if err != nil {
		m.logger.Error("DeleteBanByID failed", zap.Error(err), zap.Int64("id", banID))
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("not found")
	}
	_ = m.eventRepo.Log(chatID, adminID, "reactions", "remove_filter",
		fmt.Sprintf("Removed filter ID=%d via menu (chat=%d)", banID, chatID))
	return nil
}

// BuildReactionButtons создаёт inline-кнопки 🗑 для списка реакций.
func BuildReactionButtons(ids []int64) []telebot.Btn {
	btns := make([]telebot.Btn, 0, len(ids))
	for _, id := range ids {
		btns = append(btns, telebot.Btn{
			Unique: UniqueMenuDelReaction,
			Text:   fmt.Sprintf("🗑 #%d", id),
			Data:   strconv.FormatInt(id, 10),
		})
	}
	return btns
}

// BuildBanButtons создаёт inline-кнопки 🗑 для списка фильтров.
func BuildBanButtons(ids []int64) []telebot.Btn {
	btns := make([]telebot.Btn, 0, len(ids))
	for _, id := range ids {
		btns = append(btns, telebot.Btn{
			Unique: UniqueMenuDelBan,
			Text:   fmt.Sprintf("🗑 #%d", id),
			Data:   strconv.FormatInt(id, 10),
		})
	}
	return btns
}
