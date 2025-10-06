package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ContentLimitsRepository управляет лимитами на контент
type ContentLimitsRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewContentLimitsRepository создаёт новый репозиторий лимитов
func NewContentLimitsRepository(db *sql.DB, logger *zap.Logger) *ContentLimitsRepository {
	return &ContentLimitsRepository{
		db:     db,
		logger: logger,
	}
}

// ContentLimits представляет лимиты для чата/пользователя
type ContentLimits struct {
	ChatID           int64
	UserID           *int64 // nil = настройки для всех (allmembers)
	LimitText        int
	LimitPhoto       int
	LimitVideo       int
	LimitSticker     int
	LimitAnimation   int
	LimitVoice       int
	LimitVideoNote   int
	LimitAudio       int
	LimitDocument    int
	LimitLocation    int
	LimitContact     int
	LimitBannedWords int
	WarningThreshold int
}

// GetLimits получает лимиты для пользователя (или allmembers если не указан)
func (r *ContentLimitsRepository) GetLimits(chatID int64, userID *int64) (*ContentLimits, error) {
	var limits ContentLimits
	
	query := `
		SELECT 
			chat_id, user_id,
			limit_text, limit_photo, limit_video, limit_sticker,
			limit_animation, limit_voice, limit_video_note, limit_audio,
			limit_document, limit_location, limit_contact, limit_banned_words,
			warning_threshold
		FROM content_limits
		WHERE chat_id = $1 AND (user_id = $2 OR (user_id IS NULL AND $2 IS NULL))
		LIMIT 1
	`
	
	err := r.db.QueryRow(query, chatID, userID).Scan(
		&limits.ChatID, &limits.UserID,
		&limits.LimitText, &limits.LimitPhoto, &limits.LimitVideo, &limits.LimitSticker,
		&limits.LimitAnimation, &limits.LimitVoice, &limits.LimitVideoNote, &limits.LimitAudio,
		&limits.LimitDocument, &limits.LimitLocation, &limits.LimitContact, &limits.LimitBannedWords,
		&limits.WarningThreshold,
	)
	
	if err == sql.ErrNoRows {
		// Нет лимитов - возвращаем дефолтные (всё разрешено)
		return &ContentLimits{
			ChatID:           chatID,
			UserID:           userID,
			WarningThreshold: 2,
		}, nil
	}
	
	if err != nil {
		r.logger.Error("failed to get limits",
			zap.Int64("chat_id", chatID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("get limits: %w", err)
	}
	
	return &limits, nil
}

// GetLimitForContentType получает лимит для конкретного типа контента
func (r *ContentLimitsRepository) GetLimitForContentType(chatID int64, userID *int64, contentType string) (int, error) {
	limits, err := r.GetLimits(chatID, userID)
	if err != nil {
		return 0, err
	}
	
	// Мапим тип контента на поле
	switch contentType {
	case "text":
		return limits.LimitText, nil
	case "photo":
		return limits.LimitPhoto, nil
	case "video":
		return limits.LimitVideo, nil
	case "sticker":
		return limits.LimitSticker, nil
	case "animation":
		return limits.LimitAnimation, nil
	case "voice":
		return limits.LimitVoice, nil
	case "video_note":
		return limits.LimitVideoNote, nil
	case "audio":
		return limits.LimitAudio, nil
	case "document":
		return limits.LimitDocument, nil
	case "location":
		return limits.LimitLocation, nil
	case "contact":
		return limits.LimitContact, nil
	default:
		return 0, nil // нет лимита
	}
}

// SetLimit устанавливает лимит для типа контента
func (r *ContentLimitsRepository) SetLimit(chatID int64, userID *int64, contentType string, limit int) error {
	// Определяем какое поле обновлять
	var columnName string
	switch contentType {
	case "text":
		columnName = "limit_text"
	case "photo":
		columnName = "limit_photo"
	case "video":
		columnName = "limit_video"
	case "sticker":
		columnName = "limit_sticker"
	case "animation":
		columnName = "limit_animation"
	case "voice":
		columnName = "limit_voice"
	case "video_note":
		columnName = "limit_video_note"
	case "audio":
		columnName = "limit_audio"
	case "document":
		columnName = "limit_document"
	case "location":
		columnName = "limit_location"
	case "contact":
		columnName = "limit_contact"
	case "banned_words":
		columnName = "limit_banned_words"
	default:
		return fmt.Errorf("unknown content type: %s", contentType)
	}
	
	// Upsert лимита
	query := fmt.Sprintf(`
		INSERT INTO content_limits (chat_id, user_id, %s)
		VALUES ($1, $2, $3)
		ON CONFLICT (chat_id, COALESCE(user_id, -1))
		DO UPDATE SET %s = EXCLUDED.%s, updated_at = NOW()
	`, columnName, columnName, columnName)
	
	_, err := r.db.Exec(query, chatID, userID, limit)
	if err != nil {
		r.logger.Error("failed to set limit",
			zap.Int64("chat_id", chatID),
			zap.String("content_type", contentType),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return fmt.Errorf("set limit: %w", err)
	}
	
	r.logger.Info("limit set",
		zap.Int64("chat_id", chatID),
		zap.String("content_type", contentType),
		zap.Int("limit", limit),
	)
	
	return nil
}

// GetCounter получает счётчик контента за сегодня
func (r *ContentLimitsRepository) GetCounter(chatID, userID int64, contentType string) (int, error) {
	today := time.Now().Format("2006-01-02")
	
	var columnName string
	switch contentType {
	case "text":
		columnName = "count_text"
	case "photo":
		columnName = "count_photo"
	case "video":
		columnName = "count_video"
	case "sticker":
		columnName = "count_sticker"
	case "animation":
		columnName = "count_animation"
	case "voice":
		columnName = "count_voice"
	case "video_note":
		columnName = "count_video_note"
	case "audio":
		columnName = "count_audio"
	case "document":
		columnName = "count_document"
	case "location":
		columnName = "count_location"
	case "contact":
		columnName = "count_contact"
	case "banned_words":
		columnName = "count_banned_words"
	default:
		return 0, nil
	}
	
	query := fmt.Sprintf(`
		SELECT %s FROM content_counters
		WHERE chat_id = $1 AND user_id = $2 AND counter_date = $3
	`, columnName)
	
	var count int
	err := r.db.QueryRow(query, chatID, userID, today).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		r.logger.Error("failed to get counter",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Error(err),
		)
		return 0, fmt.Errorf("get counter: %w", err)
	}
	
	return count, nil
}

// IncrementCounter увеличивает счётчик контента
func (r *ContentLimitsRepository) IncrementCounter(chatID, userID int64, contentType string) error {
	today := time.Now().Format("2006-01-02")
	
	var columnName string
	switch contentType {
	case "text":
		columnName = "count_text"
	case "photo":
		columnName = "count_photo"
	case "video":
		columnName = "count_video"
	case "sticker":
		columnName = "count_sticker"
	case "animation":
		columnName = "count_animation"
	case "voice":
		columnName = "count_voice"
	case "video_note":
		columnName = "count_video_note"
	case "audio":
		columnName = "count_audio"
	case "document":
		columnName = "count_document"
	case "location":
		columnName = "count_location"
	case "contact":
		columnName = "count_contact"
	case "banned_words":
		columnName = "count_banned_words"
	default:
		return nil
	}
	
	query := fmt.Sprintf(`
		INSERT INTO content_counters (chat_id, user_id, counter_date, %s)
		VALUES ($1, $2, $3, 1)
		ON CONFLICT (chat_id, user_id, counter_date)
		DO UPDATE SET %s = content_counters.%s + 1, updated_at = NOW()
	`, columnName, columnName, columnName)
	
	_, err := r.db.Exec(query, chatID, userID, today)
	if err != nil {
		r.logger.Error("failed to increment counter",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Error(err),
		)
		return fmt.Errorf("increment counter: %w", err)
	}
	
	return nil
}
