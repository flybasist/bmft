package reactions

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/flybasist/bmft/internal/core"
	"go.uber.org/zap"
)

// Phase 3.5: Text Violations Counter
// Аналог Python: checkmessage.py::regextext() с violation=21

// TextViolationConfig хранит настройки лимита на текстовые нарушения
type TextViolationConfig struct {
	ChatID           int64
	UserID           int64 // 0 = для всех пользователей
	DailyLimit       int   // Лимит текстовых нарушений в день (по умолчанию из limitviolation)
	WarningThreshold int   // За сколько нарушений до лимита предупреждать (по умолчанию 2)
	IsVIP            bool  // VIP игнорирует лимиты
}

// checkTextViolation проверяет счётчик текстовых нарушений и удаляет сообщение если превышен
// Вызывается когда reaction.ViolationCode == 21
// Возвращает: (shouldDelete bool, err error)
func (m *ReactionsModule) checkTextViolation(ctx *core.MessageContext, reaction ReactionConfig) (bool, error) {
	chatID := ctx.Chat.ID
	userID := ctx.Sender.ID
	username := ctx.Sender.Username
	if username == "" {
		username = ctx.Sender.FirstName
	}

	// 1. Проверяем VIP статус
	isVIP, err := m.isVIPUser(chatID, userID)
	if err != nil {
		m.logger.Error("failed to check VIP status for text violation",
			zap.Int64("user_id", userID),
			zap.Error(err))
		return false, nil // В случае ошибки не блокируем
	}

	if isVIP {
		m.logger.Debug("VIP user bypassed text violation limit",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID))
		return false, nil
	}

	// 2. Получаем лимит на текстовые нарушения
	limit, err := m.getTextViolationLimit(chatID, userID)
	if err != nil {
		m.logger.Error("failed to get text violation limit",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Error(err))
		return false, nil
	}

	// limit == 0 означает "без ограничений"
	if limit == 0 {
		// Всё равно инкрементим для статистики
		_ = m.incrementTextViolationCounter(chatID, userID, time.Now())
		return false, nil
	}

	// 3. Получаем текущий счётчик нарушений за сегодня
	today := time.Now()
	count, err := m.getTextViolationCount(chatID, userID, today)
	if err != nil {
		m.logger.Error("failed to get text violation count",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Error(err))
		return false, nil
	}

	// 4. Проверяем превышение лимита
	if count >= limit {
		// Лимит превышен - нужно удалить сообщение
		m.logger.Info("text violation limit exceeded - will delete message",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.Int("count", count),
			zap.Int("limit", limit))

		// Инкрементим счётчик
		_ = m.incrementTextViolationCounter(chatID, userID, today)

		return true, nil // Удалить сообщение
	}

	// 5. Предупреждение за N нарушений до лимита (по умолчанию 2)
	warningThreshold := 2
	if count+warningThreshold >= limit {
		answer := fmt.Sprintf("@%s Ты набрал %d из %d текстовых нарушений за сегодня",
			username, count+1, limit)
		if _, err := ctx.Bot.Send(ctx.Chat, answer); err != nil {
			m.logger.Error("failed to send text violation warning",
				zap.Error(err))
		}
	}

	// 6. Инкрементируем счётчик
	if err := m.incrementTextViolationCounter(chatID, userID, today); err != nil {
		m.logger.Error("failed to increment text violation counter",
			zap.Error(err))
	}

	return false, nil // Не удаляем сообщение
}

// getTextViolationLimit получает лимит на текстовые нарушения
// Сначала проверяет персональный лимит, потом общий (user_id = NULL)
// Возвращает: 0 = без лимита, N = лимит нарушений/день
func (m *ReactionsModule) getTextViolationLimit(chatID, userID int64) (int, error) {
	// 1. Проверяем персональный лимит в reactions_config
	// Ищем любую реакцию с violation_code=21 и персональным user_id
	query := `
		SELECT COALESCE(MAX(daily_limit), 0)
		FROM (
			SELECT 10 as daily_limit
			FROM reactions_config
			WHERE chat_id = $1 AND user_id = $2 AND violation_code = 21
			LIMIT 1
		) AS personal_limit
	`

	var limit int
	err := m.db.QueryRow(query, chatID, userID).Scan(&limit)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("get personal text violation limit: %w", err)
	}

	if limit > 0 {
		return limit, nil
	}

	// 2. Проверяем общий лимит (user_id = NULL или не указан)
	// По умолчанию используем значение из Python бота: limitviolation (обычно 10-20)
	// Для простоты используем фиксированное значение 10 (можно вынести в конфиг чата)
	defaultLimit := 10

	query = `
		SELECT COALESCE(COUNT(*), 0) as has_config
		FROM reactions_config
		WHERE chat_id = $1 AND violation_code = 21
		LIMIT 1
	`

	var hasConfig int
	err = m.db.QueryRow(query, chatID).Scan(&hasConfig)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("check text violation config: %w", err)
	}

	if hasConfig > 0 {
		return defaultLimit, nil
	}

	// Нет конфигурации - без ограничений
	return 0, nil
}

// getTextViolationCount получает количество текстовых нарушений за день
// Использует reactions_log с violation_code=21
func (m *ReactionsModule) getTextViolationCount(chatID, userID int64, date time.Time) (int, error) {
	// Начало дня
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())

	query := `
		SELECT COUNT(*)
		FROM reactions_log
		WHERE chat_id = $1 AND user_id = $2 
		  AND violation_code = 21
		  AND created_at >= $3
	`

	var count int
	err := m.db.QueryRow(query, chatID, userID, startOfDay).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("get text violation count: %w", err)
	}

	return count, nil
}

// incrementTextViolationCounter инкрементирует счётчик текстовых нарушений
// Записывает в reactions_log с violation_code=21
func (m *ReactionsModule) incrementTextViolationCounter(chatID, userID int64, date time.Time) error {
	query := `
		INSERT INTO reactions_log (chat_id, user_id, message_id, keyword, emojis_added, violation_code, created_at)
		VALUES ($1, $2, 0, 'text_violation', '', 21, $3)
	`

	_, err := m.db.Exec(query, chatID, userID, date)
	if err != nil {
		return fmt.Errorf("increment text violation counter: %w", err)
	}

	return nil
}

// isVIPUser проверяет является ли пользователь VIP
// Проверяет флаг is_vip в reactions_config или limiter_config
func (m *ReactionsModule) isVIPUser(chatID, userID int64) (bool, error) {
	// Проверяем в reactions_config
	query := `
		SELECT COALESCE(is_vip, false)
		FROM reactions_config
		WHERE chat_id = $1 AND user_id = $2 AND is_vip = true
		LIMIT 1
	`

	var isVIP bool
	err := m.db.QueryRow(query, chatID, userID).Scan(&isVIP)
	if err == nil && isVIP {
		return true, nil
	}

	// Проверяем в limiter_config
	query = `
		SELECT COALESCE(is_vip, false)
		FROM limiter_config
		WHERE user_id = $1 AND is_vip = true
		LIMIT 1
	`

	err = m.db.QueryRow(query, userID).Scan(&isVIP)
	if err == sql.ErrNoRows {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("check VIP status: %w", err)
	}

	return isVIP, nil
}
