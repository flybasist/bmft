package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// StatisticsRepository управляет статистикой пользователей и чатов.
// Русский комментарий: Repository для работы с таблицей statistics_daily.
// Собирает и кэширует статистику по типам контента (text, photo, video, sticker и т.д.)
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
// Русский комментарий: Вызывается при каждом сообщении пользователя.
// Использует ON CONFLICT для атомарного инкремента (UPSERT).
func (r *StatisticsRepository) IncrementCounter(chatID, userID int64, contentType string) error {
	query := `
		INSERT INTO statistics_daily (chat_id, user_id, stat_date, content_type, message_count, updated_at)
		VALUES ($1, $2, CURRENT_DATE, $3, 1, NOW())
		ON CONFLICT (chat_id, user_id, stat_date, content_type)
		DO UPDATE SET 
			message_count = statistics_daily.message_count + 1,
			updated_at = NOW()
	`

	_, err := r.db.Exec(query, chatID, userID, contentType)
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
// Русский комментарий: Получает агрегированную статистику по всем типам контента.
func (r *StatisticsRepository) GetUserStats(userID, chatID int64, date time.Time) (*UserDailyStats, error) {
	query := `
		SELECT 
			s.chat_id,
			s.user_id,
			COALESCE(u.username, '') as username,
			s.stat_date,
			SUM(CASE WHEN s.content_type = 'text' THEN s.message_count ELSE 0 END) as text_count,
			SUM(CASE WHEN s.content_type = 'photo' THEN s.message_count ELSE 0 END) as photo_count,
			SUM(CASE WHEN s.content_type = 'video' THEN s.message_count ELSE 0 END) as video_count,
			SUM(CASE WHEN s.content_type = 'sticker' THEN s.message_count ELSE 0 END) as sticker_count,
			SUM(CASE WHEN s.content_type = 'voice' THEN s.message_count ELSE 0 END) as voice_count,
			SUM(CASE WHEN s.content_type NOT IN ('text', 'photo', 'video', 'sticker', 'voice') THEN s.message_count ELSE 0 END) as other_count,
			SUM(s.message_count) as total_count
		FROM statistics_daily s
		LEFT JOIN users u ON s.user_id = u.user_id
		WHERE s.user_id = $1 AND s.chat_id = $2 AND s.stat_date = $3
		GROUP BY s.chat_id, s.user_id, u.username, s.stat_date
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
// Русский комментарий: Агрегация по всем пользователям чата за день.
func (r *StatisticsRepository) GetChatStats(chatID int64, date time.Time) (*ChatDailyStats, error) {
	query := `
		SELECT 
			chat_id,
			stat_date,
			SUM(CASE WHEN content_type = 'text' THEN message_count ELSE 0 END) as text_count,
			SUM(CASE WHEN content_type = 'photo' THEN message_count ELSE 0 END) as photo_count,
			SUM(CASE WHEN content_type = 'video' THEN message_count ELSE 0 END) as video_count,
			SUM(CASE WHEN content_type = 'sticker' THEN message_count ELSE 0 END) as sticker_count,
			SUM(CASE WHEN content_type = 'voice' THEN message_count ELSE 0 END) as voice_count,
			SUM(CASE WHEN content_type NOT IN ('text', 'photo', 'video', 'sticker', 'voice') THEN message_count ELSE 0 END) as other_count,
			SUM(message_count) as total_count,
			COUNT(DISTINCT user_id) as user_count
		FROM statistics_daily
		WHERE chat_id = $1 AND stat_date = $2
		GROUP BY chat_id, stat_date
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
// Русский комментарий: Сортирует пользователей по количеству сообщений.
func (r *StatisticsRepository) GetTopUsers(chatID int64, date time.Time, limit int) ([]TopUser, error) {
	query := `
		SELECT 
			s.user_id,
			COALESCE(u.username, '') as username,
			COALESCE(u.first_name, 'Unknown') as first_name,
			SUM(s.message_count) as message_count,
			ROW_NUMBER() OVER (ORDER BY SUM(s.message_count) DESC) as rank
		FROM statistics_daily s
		LEFT JOIN users u ON s.user_id = u.user_id
		WHERE s.chat_id = $1 AND s.stat_date = $2
		GROUP BY s.user_id, u.username, u.first_name
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
// Русский комментарий: Агрегация за неделю для отображения тренда.
func (r *StatisticsRepository) GetUserWeeklyStats(userID, chatID int64) (*UserDailyStats, error) {
	weekAgo := time.Now().AddDate(0, 0, -7)

	query := `
		SELECT 
			s.chat_id,
			s.user_id,
			COALESCE(u.username, '') as username,
			NOW()::date as stat_date,
			SUM(CASE WHEN s.content_type = 'text' THEN s.message_count ELSE 0 END) as text_count,
			SUM(CASE WHEN s.content_type = 'photo' THEN s.message_count ELSE 0 END) as photo_count,
			SUM(CASE WHEN s.content_type = 'video' THEN s.message_count ELSE 0 END) as video_count,
			SUM(CASE WHEN s.content_type = 'sticker' THEN s.message_count ELSE 0 END) as sticker_count,
			SUM(CASE WHEN s.content_type = 'voice' THEN s.message_count ELSE 0 END) as voice_count,
			SUM(CASE WHEN s.content_type NOT IN ('text', 'photo', 'video', 'sticker', 'voice') THEN s.message_count ELSE 0 END) as other_count,
			SUM(s.message_count) as total_count
		FROM statistics_daily s
		LEFT JOIN users u ON s.user_id = u.user_id
		WHERE s.user_id = $1 AND s.chat_id = $2 AND s.stat_date >= $3
		GROUP BY s.chat_id, s.user_id, u.username
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
