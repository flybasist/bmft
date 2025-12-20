package repositories

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"go.uber.org/zap"
)

// MessageRepository работает с таблицей messages (единый источник правды).
// Русский комментарий: v0.8.0 - вся информация о сообщениях хранится в messages
// с JSONB metadata вместо отдельных таблиц-счётчиков.
// Модули пишут свои данные в metadata (limiter, statistics, reactions, textfilter).
type MessageRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewMessageRepository создаёт новый репозиторий сообщений.
func NewMessageRepository(db *sql.DB, logger *zap.Logger) *MessageRepository {
	return &MessageRepository{
		db:     db,
		logger: logger,
	}
}

// MessageMetadata содержит модуле-специфичные данные сообщения.
type MessageMetadata struct {
	Limiter    *LimiterMetadata    `json:"limiter,omitempty"`
	Reactions  *ReactionsMetadata  `json:"reactions,omitempty"`
	TextFilter *TextFilterMetadata `json:"textfilter,omitempty"`
	Statistics *StatisticsMetadata `json:"statistics,omitempty"`
}

// LimiterMetadata — метаданные модуля Limiter.
type LimiterMetadata struct {
	Checked    bool   `json:"checked"`
	LimitHit   bool   `json:"limit_hit"`
	DailyCount int    `json:"daily_count"`
	LimitType  string `json:"limit_type"`
}

// ReactionsMetadata — метаданные модуля Reactions.
type ReactionsMetadata struct {
	Triggered     []int64 `json:"triggered,omitempty"`      // ID реакций которые сработали
	CooldownUntil *string `json:"cooldown_until,omitempty"` // ISO8601 timestamp
	DailyCount    int     `json:"daily_count,omitempty"`    // Сколько раз сработало за день
}

// TextFilterMetadata — метаданные модуля TextFilter.
type TextFilterMetadata struct {
	BannedWordsFound []string `json:"banned_words_found,omitempty"`
	Action           string   `json:"action,omitempty"` // delete, warn, delete_warn
}

// StatisticsMetadata — метаданные модуля Statistics.
type StatisticsMetadata struct {
	Processed        bool `json:"processed"`
	ProcessingTimeMs int  `json:"processing_time_ms,omitempty"`
}

// InsertMessage сохраняет сообщение в БД с метаданными.
// Русский комментарий: Главная функция для записи сообщений.
// Модули передают свои метаданные через MessageMetadata структуру.
// threadID = 0 означает основной чат, >0 - сообщение в топике.
func (r *MessageRepository) InsertMessage(
	chatID int64,
	threadID int,
	userID int64,
	messageID int,
	contentType string,
	text string,
	caption string,
	fileID string,
	chatName string,
	metadata MessageMetadata,
) (int64, error) {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
		INSERT INTO messages (chat_id, thread_id, user_id, message_id, content_type, text, caption, file_id, chat_name, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id
	`

	var id int64
	err = r.db.QueryRow(query, chatID, threadID, userID, messageID, contentType, text, caption, fileID, chatName, metadataJSON).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("failed to insert message: %w", err)
	}

	r.logger.Debug("message inserted",
		zap.Int64("id", id),
		zap.Int64("chat_id", chatID),
		zap.Int("thread_id", threadID),
		zap.Int64("user_id", userID),
		zap.Int("message_id", messageID),
		zap.String("content_type", contentType),
	)

	return id, nil
}

// UpdateMetadata обновляет metadata существующего сообщения.
// Используется если модуль обрабатывает сообщение асинхронно.
// TODO: В текущей версии не используется ни одним модулем. Может понадобиться для будущих функций.
func (r *MessageRepository) UpdateMetadata(messageID int64, metadata MessageMetadata) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `UPDATE messages SET metadata = $1 WHERE id = $2`
	_, err = r.db.Exec(query, metadataJSON, messageID)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	return nil
}

// MarkDeleted помечает сообщение как удалённое.
// TODO: В текущей версии не используется ни одним модулем. Может понадобиться для будущих функций.
func (r *MessageRepository) MarkDeleted(chatID int64, threadID int, messageID int, reason string) error {
	query := `
		UPDATE messages 
		SET was_deleted = TRUE, deletion_reason = $1 
		WHERE chat_id = $2 AND thread_id = $3 AND message_id = $4
	`

	_, err := r.db.Exec(query, reason, chatID, threadID, messageID)
	if err != nil {
		return fmt.Errorf("failed to mark message as deleted: %w", err)
	}

	return nil
}

// GetTodayCountByType возвращает количество сообщений определённого типа за сегодня.
// Используется Limiter для проверки лимитов.
// threadID = 0 означает подсчёт для всего чата, >0 - только для конкретного топика.
func (r *MessageRepository) GetTodayCountByType(chatID int64, threadID int, userID int64, contentType string) (int, error) {
	query := `
		SELECT COUNT(*) 
		FROM messages 
		WHERE chat_id = $1 
		  AND thread_id = $2
		  AND user_id = $3 
		  AND content_type = $4 
		  AND DATE(created_at) = CURRENT_DATE
		  AND was_deleted = FALSE
	`

	var count int
	err := r.db.QueryRow(query, chatID, threadID, userID, contentType).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get today count: %w", err)
	}

	return count, nil
}

// GetUserStats возвращает статистику пользователя за период.
// Используется модулем Statistics для команды /mystats.
// threadID = 0 означает статистику по всему чату, >0 - только для конкретного топика.
func (r *MessageRepository) GetUserStats(chatID int64, threadID int, userID int64, days int) (map[string]int, error) {
	query := `
		SELECT content_type, COUNT(*) as count
		FROM messages
		WHERE chat_id = $1
		  AND thread_id = $2
		  AND user_id = $3
		  AND created_at >= NOW() - INTERVAL '%d days'
		  AND was_deleted = FALSE
		GROUP BY content_type
	`

	rows, err := r.db.Query(fmt.Sprintf(query, days), chatID, threadID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var contentType string
		var count int
		if err := rows.Scan(&contentType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan stats row: %w", err)
		}
		stats[contentType] = count
	}

	return stats, rows.Err()
}

// GetChatTopUsers возвращает топ активных пользователей чата за период.
// threadID = 0 означает статистику по всему чату, >0 - только для конкретного топика.
func (r *MessageRepository) GetChatTopUsers(chatID int64, threadID int, days int, limit int) ([]UserStat, error) {
	query := `
		SELECT user_id, COUNT(*) as message_count
		FROM messages
		WHERE chat_id = $1
		  AND thread_id = $2
		  AND created_at >= NOW() - INTERVAL '%d days'
		  AND was_deleted = FALSE
		GROUP BY user_id
		ORDER BY message_count DESC
		LIMIT $3
	`

	rows, err := r.db.Query(fmt.Sprintf(query, days), chatID, threadID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat top users: %w", err)
	}
	defer rows.Close()

	var topUsers []UserStat
	for rows.Next() {
		var userID int64
		var count int
		if err := rows.Scan(&userID, &count); err != nil {
			return nil, fmt.Errorf("failed to scan top users row: %w", err)
		}
		topUsers = append(topUsers, UserStat{UserID: userID, MessageCount: count})
	}

	return topUsers, rows.Err()
}

// GetChatStats возвращает статистику чата по типам контента за период.
// threadID = 0 означает статистику по всему чату, >0 - только для конкретного топика.
func (r *MessageRepository) GetChatStats(chatID int64, threadID int, days int) (map[string]int, error) {
	query := `
		SELECT content_type, COUNT(*) as count
		FROM messages
		WHERE chat_id = $1
		  AND thread_id = $2
		  AND created_at >= NOW() - INTERVAL '%d days'
		  AND was_deleted = FALSE
		GROUP BY content_type
		ORDER BY count DESC
	`

	rows, err := r.db.Query(fmt.Sprintf(query, days), chatID, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var contentType string
		var count int
		if err := rows.Scan(&contentType, &count); err != nil {
			return nil, fmt.Errorf("failed to scan chat stats row: %w", err)
		}
		stats[contentType] = count
	}

	return stats, rows.Err()
}

// UserStat представляет статистику пользователя.
type UserStat struct {
	UserID       int64
	MessageCount int
}
