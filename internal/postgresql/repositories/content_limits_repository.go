package repositories

import (
	"database/sql"
	"fmt"
)

// ============================================================================
// ContentLimitsRepository - лимиты на контент
// ============================================================================

// ContentLimitsRepository управляет лимитами на контент
type ContentLimitsRepository struct {
	db *sql.DB
}

// NewContentLimitsRepository создаёт новый репозиторий лимитов
func NewContentLimitsRepository(db *sql.DB) *ContentLimitsRepository {
	return &ContentLimitsRepository{
		db: db,
	}
}

// ContentLimits представляет лимиты для чата/топика/пользователя
type ContentLimits struct {
	ChatID           int64
	ThreadID         int    // 0 = лимит для всего чата, >0 = лимит только для топика
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

// GetLimits получает лимиты для пользователя в чате/топике (или allmembers если не указан).
// Логика fallback: сначала ищем лимит для (chat_id, thread_id, user_id),
// затем для (chat_id, thread_id, NULL), затем для (chat_id, 0, user_id), затем для (chat_id, 0, NULL).
func (r *ContentLimitsRepository) GetLimits(chatID int64, threadID int, userID *int64) (*ContentLimits, error) {
	var limits ContentLimits

	// 1. Сначала ищем лимит для конкретного пользователя в конкретном топике
	queryUser := `
		SELECT 
			chat_id, thread_id, user_id,
			limit_text, limit_photo, limit_video, limit_sticker,
			limit_animation, limit_voice, limit_video_note, limit_audio,
			limit_document, limit_location, limit_contact, limit_banned_words,
			warning_threshold
		FROM content_limits
		WHERE chat_id = $1 AND thread_id = $2 AND user_id = $3
		LIMIT 1
	`

	err := r.db.QueryRow(queryUser, chatID, threadID, userID).Scan(
		&limits.ChatID, &limits.ThreadID, &limits.UserID,
		&limits.LimitText, &limits.LimitPhoto, &limits.LimitVideo, &limits.LimitSticker,
		&limits.LimitAnimation, &limits.LimitVoice, &limits.LimitVideoNote, &limits.LimitAudio,
		&limits.LimitDocument, &limits.LimitLocation, &limits.LimitContact, &limits.LimitBannedWords,
		&limits.WarningThreshold,
	)

	if err != sql.ErrNoRows {
		if err != nil {
			return nil, fmt.Errorf("get limits (user+thread): %w", err)
		}
		return &limits, nil
	}

	// 2. Если нет лимита для пользователя в топике, ищем лимит для всех в топике
	queryAllInThread := `
		SELECT 
			chat_id, thread_id, user_id,
			limit_text, limit_photo, limit_video, limit_sticker,
			limit_animation, limit_voice, limit_video_note, limit_audio,
			limit_document, limit_location, limit_contact, limit_banned_words,
			warning_threshold
		FROM content_limits
		WHERE chat_id = $1 AND thread_id = $2 AND user_id IS NULL
		LIMIT 1
	`
	err = r.db.QueryRow(queryAllInThread, chatID, threadID).Scan(
		&limits.ChatID, &limits.ThreadID, &limits.UserID,
		&limits.LimitText, &limits.LimitPhoto, &limits.LimitVideo, &limits.LimitSticker,
		&limits.LimitAnimation, &limits.LimitVoice, &limits.LimitVideoNote, &limits.LimitAudio,
		&limits.LimitDocument, &limits.LimitLocation, &limits.LimitContact, &limits.LimitBannedWords,
		&limits.WarningThreshold,
	)

	if err != sql.ErrNoRows {
		if err != nil {
			return nil, fmt.Errorf("get limits (all+thread): %w", err)
		}
		return &limits, nil
	}

	// 3. Если нет лимита для топика, fallback на лимит для конкретного пользователя во всём чате
	if threadID != 0 && userID != nil {
		err = r.db.QueryRow(queryUser, chatID, 0, userID).Scan(
			&limits.ChatID, &limits.ThreadID, &limits.UserID,
			&limits.LimitText, &limits.LimitPhoto, &limits.LimitVideo, &limits.LimitSticker,
			&limits.LimitAnimation, &limits.LimitVoice, &limits.LimitVideoNote, &limits.LimitAudio,
			&limits.LimitDocument, &limits.LimitLocation, &limits.LimitContact, &limits.LimitBannedWords,
			&limits.WarningThreshold,
		)

		if err != sql.ErrNoRows {
			if err != nil {
				return nil, fmt.Errorf("get limits (user+chat): %w", err)
			}
			return &limits, nil
		}
	}

	// 4. Если нет, ищем лимит для всех во всём чате (thread_id = 0, user_id = NULL)
	if threadID != 0 {
		err = r.db.QueryRow(queryAllInThread, chatID, 0).Scan(
			&limits.ChatID, &limits.ThreadID, &limits.UserID,
			&limits.LimitText, &limits.LimitPhoto, &limits.LimitVideo, &limits.LimitSticker,
			&limits.LimitAnimation, &limits.LimitVoice, &limits.LimitVideoNote, &limits.LimitAudio,
			&limits.LimitDocument, &limits.LimitLocation, &limits.LimitContact, &limits.LimitBannedWords,
			&limits.WarningThreshold,
		)

		if err != sql.ErrNoRows {
			if err != nil {
				return nil, fmt.Errorf("get limits (all+chat): %w", err)
			}
			return &limits, nil
		}
	}

	// 5. Нет лимитов вообще — возвращаем дефолтные (всё разрешено)
	return &ContentLimits{
		ChatID:           chatID,
		ThreadID:         threadID,
		UserID:           userID,
		WarningThreshold: 2,
	}, nil
}

// GetLimitForContentType получает лимит для конкретного типа контента
func (r *ContentLimitsRepository) GetLimitForContentType(chatID int64, threadID int, userID *int64, contentType string) (int, error) {
	limits, err := r.GetLimits(chatID, threadID, userID)
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

// SetLimit устанавливает лимит для типа контента в чате/топике
func (r *ContentLimitsRepository) SetLimit(chatID int64, threadID int, userID *int64, contentType string, limit int) error {
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
		INSERT INTO content_limits (chat_id, thread_id, user_id, %s)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (chat_id, thread_id, COALESCE(user_id, -1))
		DO UPDATE SET %s = EXCLUDED.%s, updated_at = NOW()
	`, columnName, columnName, columnName)

	_, err := r.db.Exec(query, chatID, threadID, userID, limit)
	if err != nil {
		return fmt.Errorf("set limit: %w", err)
	}

	return nil
}
