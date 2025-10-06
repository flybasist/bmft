package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// StatisticsRepository управляет статистикой пользователей и чатов.
// Использует таблицу content_counters для агрегации данных по типам контента.
// В v0.6.0 убрали дублирующую таблицу statistics_daily - вся статистика через content_counters.
type StatisticsRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewStatisticsRepository создаёт новый экземпляр репозитория статистики.
func NewStatisticsRepository(db *sql.DB, logger *zap.Logger) *StatisticsRepository {
	return &StatisticsRepository{
		db:     db,
		logger: logger,
	}
}

// UserDailyStats представляет статистику пользователя за день.
type UserDailyStats struct {
	ChatID       int64
	UserID       int64
	Username     string
	Date         time.Time
	TextCount    int
	PhotoCount   int
	VideoCount   int
	StickerCount int
	VoiceCount   int
	OtherCount   int
	TotalCount   int
}

// ChatDailyStats представляет статистику чата за день.
type ChatDailyStats struct {
	ChatID       int64
	Date         time.Time
	TextCount    int
	PhotoCount   int
	VideoCount   int
	StickerCount int
	VoiceCount   int
	OtherCount   int
	TotalCount   int
	UserCount    int
}

// TopUser представляет пользователя в топе активности.
type TopUser struct {
	UserID       int64
	Username     string
	FirstName    string
	MessageCount int
	Rank         int
}

// IncrementCounter увеличивает счётчик сообщений для пользователя.
// Использует таблицу content_counters с отдельными полями для каждого типа контента.
// Вызывается при каждом сообщении, использует ON CONFLICT для атомарного инкремента.
func (r *StatisticsRepository) IncrementCounter(chatID, userID int64, contentType string) error {
	// Определяем какое поле инкрементировать
	var column string
	switch contentType {
	case "text":
		column = "count_text"
	case "photo":
		column = "count_photo"
	case "video":
		column = "count_video"
	case "sticker":
		column = "count_sticker"
	case "animation":
		column = "count_animation"
	case "voice":
		column = "count_voice"
	case "video_note":
		column = "count_video_note"
	case "audio":
		column = "count_audio"
	case "document":
		column = "count_document"
	case "location":
		column = "count_location"
	case "contact":
		column = "count_contact"
	default:
		// Неизвестный тип контента - пропускаем
		r.logger.Warn("unknown content type for statistics",
			zap.String("content_type", contentType))
		return nil
	}

	query := fmt.Sprintf(`
		INSERT INTO content_counters (chat_id, user_id, counter_date, %s, updated_at)
		VALUES ($1, $2, CURRENT_DATE, 1, NOW())
		ON CONFLICT (chat_id, user_id, counter_date)
		DO UPDATE SET 
			%s = content_counters.%s + 1,
			updated_at = NOW()
	`, column, column, column)

	_, err := r.db.Exec(query, chatID, userID)
	if err != nil {
		r.logger.Error("failed to increment statistics counter",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Error(err))
		return fmt.Errorf("increment counter: %w", err)
	}

	return nil
}

// GetUserStats возвращает статистику пользователя за указанный день.
// Читает из content_counters, где каждый тип контента хранится в отдельном поле.
func (r *StatisticsRepository) GetUserStats(userID, chatID int64, date time.Time) (*UserDailyStats, error) {
	query := `
		SELECT 
			c.chat_id,
			c.user_id,
			COALESCE(u.username, '') as username,
			c.counter_date,
			c.count_text,
			c.count_photo,
			c.count_video,
			c.count_sticker,
			c.count_voice,
			(c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as other_count,
			(c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			 c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as total_count
		FROM content_counters c
		LEFT JOIN users u ON c.user_id = u.user_id
		WHERE c.user_id = $1 AND c.chat_id = $2 AND c.counter_date = $3
	`

	stats := &UserDailyStats{}
	err := r.db.QueryRow(query, userID, chatID, date).Scan(
		&stats.ChatID,
		&stats.UserID,
		&stats.Username,
		&stats.Date,
		&stats.TextCount,
		&stats.PhotoCount,
		&stats.VideoCount,
		&stats.StickerCount,
		&stats.VoiceCount,
		&stats.OtherCount,
		&stats.TotalCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Нет данных — это ОК
	}
	if err != nil {
		r.logger.Error("failed to get user stats",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID),
			zap.Time("date", date),
			zap.Error(err))
		return nil, fmt.Errorf("get user stats: %w", err)
	}

	return stats, nil
}

// GetChatStats возвращает статистику всего чата за указанный день.
// Агрегирует данные по всем пользователям чата из content_counters.
func (r *StatisticsRepository) GetChatStats(chatID int64, date time.Time) (*ChatDailyStats, error) {
	query := `
		SELECT 
			chat_id,
			counter_date,
			SUM(count_text) as text_count,
			SUM(count_photo) as photo_count,
			SUM(count_video) as video_count,
			SUM(count_sticker) as sticker_count,
			SUM(count_voice) as voice_count,
			SUM(count_animation + count_video_note + count_audio + count_document + count_location + count_contact) as other_count,
			SUM(count_text + count_photo + count_video + count_sticker + count_voice + 
			    count_animation + count_video_note + count_audio + count_document + count_location + count_contact) as total_count,
			COUNT(DISTINCT user_id) as user_count
		FROM content_counters
		WHERE chat_id = $1 AND counter_date = $2
		GROUP BY chat_id, counter_date
	`

	stats := &ChatDailyStats{}
	err := r.db.QueryRow(query, chatID, date).Scan(
		&stats.ChatID,
		&stats.Date,
		&stats.TextCount,
		&stats.PhotoCount,
		&stats.VideoCount,
		&stats.StickerCount,
		&stats.VoiceCount,
		&stats.OtherCount,
		&stats.TotalCount,
		&stats.UserCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Нет данных — это ОК
	}
	if err != nil {
		r.logger.Error("failed to get chat stats",
			zap.Int64("chat_id", chatID),
			zap.Time("date", date),
			zap.Error(err))
		return nil, fmt.Errorf("get chat stats: %w", err)
	}

	return stats, nil
}

// GetTopUsers возвращает топ активных пользователей чата за указанный день.
// Сортирует пользователей по общему количеству сообщений из content_counters.
func (r *StatisticsRepository) GetTopUsers(chatID int64, date time.Time, limit int) ([]TopUser, error) {
	query := `
		SELECT 
			c.user_id,
			COALESCE(u.username, '') as username,
			COALESCE(u.first_name, 'Unknown') as first_name,
			(c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			 c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as message_count,
			ROW_NUMBER() OVER (ORDER BY (c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			                              c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) DESC) as rank
		FROM content_counters c
		LEFT JOIN users u ON c.user_id = u.user_id
		WHERE c.chat_id = $1 AND c.counter_date = $2
		ORDER BY message_count DESC
		LIMIT $3
	`

	rows, err := r.db.Query(query, chatID, date, limit)
	if err != nil {
		r.logger.Error("failed to get top users",
			zap.Int64("chat_id", chatID),
			zap.Time("date", date),
			zap.Int("limit", limit),
			zap.Error(err))
		return nil, fmt.Errorf("get top users: %w", err)
	}
	defer rows.Close()

	var topUsers []TopUser
	for rows.Next() {
		var user TopUser
		err := rows.Scan(
			&user.UserID,
			&user.Username,
			&user.FirstName,
			&user.MessageCount,
			&user.Rank,
		)
		if err != nil {
			r.logger.Error("failed to scan top user row", zap.Error(err))
			continue
		}
		topUsers = append(topUsers, user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate top users: %w", err)
	}

	return topUsers, nil
}

// GetUserWeeklyStats возвращает статистику пользователя за последние 7 дней.
// Агрегирует данные за неделю для отображения тренда активности.
func (r *StatisticsRepository) GetUserWeeklyStats(userID, chatID int64) (*UserDailyStats, error) {
	weekAgo := time.Now().AddDate(0, 0, -7)

	query := `
		SELECT 
			c.chat_id,
			c.user_id,
			COALESCE(u.username, '') as username,
			NOW()::date as stat_date,
			SUM(c.count_text) as text_count,
			SUM(c.count_photo) as photo_count,
			SUM(c.count_video) as video_count,
			SUM(c.count_sticker) as sticker_count,
			SUM(c.count_voice) as voice_count,
			SUM(c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as other_count,
			SUM(c.count_text + c.count_photo + c.count_video + c.count_sticker + c.count_voice + 
			    c.count_animation + c.count_video_note + c.count_audio + c.count_document + c.count_location + c.count_contact) as total_count
		FROM content_counters c
		LEFT JOIN users u ON c.user_id = u.user_id
		WHERE c.user_id = $1 AND c.chat_id = $2 AND c.counter_date >= $3
		GROUP BY c.chat_id, c.user_id, u.username
	`

	stats := &UserDailyStats{}
	err := r.db.QueryRow(query, userID, chatID, weekAgo).Scan(
		&stats.ChatID,
		&stats.UserID,
		&stats.Username,
		&stats.Date,
		&stats.TextCount,
		&stats.PhotoCount,
		&stats.VideoCount,
		&stats.StickerCount,
		&stats.VoiceCount,
		&stats.OtherCount,
		&stats.TotalCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // Нет данных — это ОК
	}
	if err != nil {
		r.logger.Error("failed to get user weekly stats",
			zap.Int64("user_id", userID),
			zap.Int64("chat_id", chatID),
			zap.Error(err))
		return nil, fmt.Errorf("get user weekly stats: %w", err)
	}

	return stats, nil
}
