package reactions

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// handleMyTextViolations показывает счётчик текстовых нарушений за сегодня
// Usage: /mytextviolations
func (m *ReactionsModule) handleMyTextViolations(c tele.Context) error {
	// Проверка что команда из группы/супергруппы
	if c.Chat().Type != tele.ChatGroup && c.Chat().Type != tele.ChatSuperGroup {
		return c.Reply("❌ Эта команда работает только в группах")
	}

	chatID := c.Chat().ID
	userID := c.Sender().ID
	username := c.Sender().Username
	if username == "" {
		username = c.Sender().FirstName
	}

	today := time.Now()

	// Получаем лимит
	limit, err := m.getTextViolationLimit(chatID, userID)
	if err != nil {
		m.logger.Error("failed to get text violation limit",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Error(err))
		return c.Reply("❌ Не удалось получить лимит")
	}

	// Получаем текущий счётчик
	count, err := m.getTextViolationCount(chatID, userID, today)
	if err != nil {
		m.logger.Error("failed to get text violation count",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Error(err))
		return c.Reply("❌ Не удалось получить счётчик")
	}

	// Формируем ответ
	var response string

	if limit == 0 {
		response = fmt.Sprintf("📊 @%s\n\n"+
			"Текстовые нарушения за сегодня: %d\n"+
			"Лимит: ✅ Без ограничений", username, count)
	} else {
		var statusIcon string
		if count >= limit {
			statusIcon = "❌"
		} else if count+2 >= limit {
			statusIcon = "⚠️"
		} else {
			statusIcon = "✅"
		}

		response = fmt.Sprintf("📊 @%s\n\n"+
			"%s Текстовые нарушения: %d/%d\n"+
			"Осталось: %d", username, statusIcon, count, limit, limit-count)
	}

	return c.Reply(response)
}

// handleSetTextViolationLimit устанавливает лимит на текстовые нарушения
// Usage: /settextlimit <limit>
// Example: /settextlimit 10
// Example: /settextlimit 0  (без ограничений)
func (m *ReactionsModule) handleSetTextViolationLimit(c tele.Context) error {
	// Проверка прав админа
	adminIDs, err := c.Bot().AdminsOf(c.Chat())
	if err != nil {
		m.logger.Error("failed to get admins", zap.Error(err))
		return c.Reply("❌ Не удалось проверить права администратора")
	}

	isAdmin := false
	senderID := c.Sender().ID
	for _, admin := range adminIDs {
		if admin.User.ID == senderID {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		return c.Reply("❌ Эта команда доступна только администраторам чата")
	}

	// Проверка что команда из группы/супергруппы
	if c.Chat().Type != tele.ChatGroup && c.Chat().Type != tele.ChatSuperGroup {
		return c.Reply("❌ Эта команда работает только в группах")
	}

	// Парсинг аргументов
	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Reply("❌ Использование: /settextlimit <limit>\n\n" +
			"Лимит:\n" +
			"   0 = без ограничений\n" +
			"   N = лимит на N нарушений/день\n\n" +
			"Пример: /settextlimit 10")
	}

	limitStr := args[1]

	// Парсинг лимита
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return c.Reply("❌ Лимит должен быть числом")
	}

	if limit < 0 {
		return c.Reply("❌ Лимит не может быть отрицательным")
	}

	// Сохранение лимита (через reactions_config с violation_code=21)
	// Создаём специальную реакцию с violation_code=21
	chatID := c.Chat().ID

	query := `
		INSERT INTO reactions_config 
			(chat_id, user_id, content_type, trigger_type, trigger_pattern, 
			 reaction_type, reaction_data, violation_code, cooldown_minutes, 
			 is_enabled, is_vip, updated_at)
		VALUES ($1, NULL, 'text', 'regex', '.*', 'delete', '', 21, 0, true, false, NOW())
		ON CONFLICT (chat_id, COALESCE(user_id, -1), content_type, trigger_pattern)
		DO UPDATE SET
			violation_code = 21,
			updated_at = NOW()
	`

	_, err = m.db.Exec(query, chatID)
	if err != nil {
		m.logger.Error("failed to save text violation limit",
			zap.Int64("chat_id", chatID),
			zap.Int("limit", limit),
			zap.Error(err))
		return c.Reply("❌ Не удалось сохранить лимит")
	}

	// Форматирование ответа
	var status string
	if limit == 0 {
		status = "✅ Без ограничений"
	} else {
		status = fmt.Sprintf("📊 Лимит: %d нарушений/день", limit)
	}

	return c.Reply(fmt.Sprintf("✅ Лимит на текстовые нарушения установлен!\n\n"+
		"Статус: %s\n\n"+
		"ℹ️ Пользователи будут получать предупреждения за 2 нарушения до лимита.\n"+
		"При превышении лимита сообщения с нарушениями будут удаляться автоматически.", status))
}

// handleChatTextViolations показывает статистику текстовых нарушений чата (админы)
// Usage: /chattextviolations
func (m *ReactionsModule) handleChatTextViolations(c tele.Context) error {
	// Проверка прав админа
	adminIDs, err := c.Bot().AdminsOf(c.Chat())
	if err != nil {
		m.logger.Error("failed to get admins", zap.Error(err))
		return c.Reply("❌ Не удалось проверить права администратора")
	}

	isAdmin := false
	senderID := c.Sender().ID
	for _, admin := range adminIDs {
		if admin.User.ID == senderID {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		return c.Reply("❌ Эта команда доступна только администраторам чата")
	}

	// Проверка что команда из группы/супергруппы
	if c.Chat().Type != tele.ChatGroup && c.Chat().Type != tele.ChatSuperGroup {
		return c.Reply("❌ Эта команда работает только в группах")
	}

	chatID := c.Chat().ID
	today := time.Now()
	startOfDay := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	// Получаем топ нарушителей за сегодня
	query := `
		SELECT user_id, COUNT(*) as violation_count
		FROM reactions_log
		WHERE chat_id = $1 
		  AND violation_code = 21
		  AND created_at >= $2
		GROUP BY user_id
		ORDER BY violation_count DESC
		LIMIT 10
	`

	rows, err := m.db.Query(query, chatID, startOfDay)
	if err != nil {
		m.logger.Error("failed to get chat text violations",
			zap.Int64("chat_id", chatID),
			zap.Error(err))
		return c.Reply("❌ Не удалось получить статистику")
	}
	defer rows.Close()

	var response strings.Builder
	response.WriteString("📊 Текстовые нарушения за сегодня\n\n")

	hasViolations := false
	rank := 1

	for rows.Next() {
		var userID int64
		var count int

		if err := rows.Scan(&userID, &count); err != nil {
			m.logger.Error("failed to scan violation stats", zap.Error(err))
			continue
		}

		hasViolations = true

		// Пытаемся получить username (если бот видел этого пользователя)
		// Для простоты просто показываем user_id
		response.WriteString(fmt.Sprintf("%d. User ID %d: %d нарушений\n", rank, userID, count))
		rank++
	}

	if !hasViolations {
		response.WriteString("✅ Нарушений за сегодня не зафиксировано")
	}

	return c.Reply(response.String())
}
