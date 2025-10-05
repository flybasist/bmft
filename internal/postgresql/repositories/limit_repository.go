package repositories

import (
	"database/sql"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// UserLimit — структура лимита пользователя
type UserLimit struct {
	UserID           int64
	Username         string
	DailyLimit       int
	MonthlyLimit     int
	DailyUsed        int
	MonthlyUsed      int
	LastResetDaily   time.Time
	LastResetMonthly time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// LimitInfo — информация о лимитах для отображения пользователю
type LimitInfo struct {
	DailyRemaining   int
	MonthlyRemaining int
	DailyUsed        int
	MonthlyUsed      int
	DailyLimit       int
	MonthlyLimit     int
}

// LimitRepository — репозиторий для работы с лимитами пользователей
type LimitRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewLimitRepository создаёт новый экземпляр репозитория лимитов
func NewLimitRepository(db *sql.DB, logger *zap.Logger) *LimitRepository {
	return &LimitRepository{
		db:     db,
		logger: logger,
	}
}

// GetOrCreate получает или создаёт запись лимита для пользователя
func (r *LimitRepository) GetOrCreate(userID int64, username string) (*UserLimit, error) {
	query := `
		INSERT INTO user_limits (user_id, username)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET username = EXCLUDED.username,
		    updated_at = NOW()
		RETURNING user_id, username, daily_limit, monthly_limit, 
		          daily_used, monthly_used, last_reset_daily, last_reset_monthly,
		          created_at, updated_at
	`

	var limit UserLimit
	err := r.db.QueryRow(query, userID, username).Scan(
		&limit.UserID,
		&limit.Username,
		&limit.DailyLimit,
		&limit.MonthlyLimit,
		&limit.DailyUsed,
		&limit.MonthlyUsed,
		&limit.LastResetDaily,
		&limit.LastResetMonthly,
		&limit.CreatedAt,
		&limit.UpdatedAt,
	)

	if err != nil {
		r.logger.Error("failed to get or create limit",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("get or create limit: %w", err)
	}

	r.logger.Debug("got or created limit",
		zap.Int64("user_id", userID),
		zap.String("username", username),
	)

	return &limit, nil
}

// CheckAndIncrement проверяет лимит и увеличивает счётчик использования
// Возвращает: (разрешено ли, информация о лимитах, ошибка)
func (r *LimitRepository) CheckAndIncrement(userID int64, username string) (bool, *LimitInfo, error) {
	// Сначала сбрасываем лимиты если нужно
	if err := r.ResetDailyIfNeeded(userID); err != nil {
		r.logger.Warn("failed to reset daily limit", zap.Error(err))
	}
	if err := r.ResetMonthlyIfNeeded(userID); err != nil {
		r.logger.Warn("failed to reset monthly limit", zap.Error(err))
	}

	// Получаем или создаём запись
	limit, err := r.GetOrCreate(userID, username)
	if err != nil {
		return false, nil, err
	}

	// Проверяем лимиты
	if limit.DailyUsed >= limit.DailyLimit {
		r.logger.Info("daily limit exceeded",
			zap.Int64("user_id", userID),
			zap.Int("used", limit.DailyUsed),
			zap.Int("limit", limit.DailyLimit),
		)

		info := r.buildLimitInfo(limit)
		return false, info, nil
	}

	if limit.MonthlyUsed >= limit.MonthlyLimit {
		r.logger.Info("monthly limit exceeded",
			zap.Int64("user_id", userID),
			zap.Int("used", limit.MonthlyUsed),
			zap.Int("limit", limit.MonthlyLimit),
		)

		info := r.buildLimitInfo(limit)
		return false, info, nil
	}

	// Инкрементируем счётчики
	query := `
		UPDATE user_limits
		SET daily_used = daily_used + 1,
		    monthly_used = monthly_used + 1,
		    updated_at = NOW()
		WHERE user_id = $1
		RETURNING daily_used, monthly_used
	`

	err = r.db.QueryRow(query, userID).Scan(&limit.DailyUsed, &limit.MonthlyUsed)
	if err != nil {
		r.logger.Error("failed to increment usage",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return false, nil, fmt.Errorf("increment usage: %w", err)
	}

	r.logger.Debug("incremented usage",
		zap.Int64("user_id", userID),
		zap.Int("daily_used", limit.DailyUsed),
		zap.Int("monthly_used", limit.MonthlyUsed),
	)

	info := r.buildLimitInfo(limit)
	return true, info, nil
}

// GetLimitInfo получает информацию о лимитах пользователя
func (r *LimitRepository) GetLimitInfo(userID int64) (*LimitInfo, error) {
	query := `
		SELECT daily_limit, monthly_limit, daily_used, monthly_used
		FROM user_limits
		WHERE user_id = $1
	`

	var limit UserLimit
	err := r.db.QueryRow(query, userID).Scan(
		&limit.DailyLimit,
		&limit.MonthlyLimit,
		&limit.DailyUsed,
		&limit.MonthlyUsed,
	)

	if err == sql.ErrNoRows {
		// Пользователь ещё не использовал бота, возвращаем дефолтные значения
		return &LimitInfo{
			DailyRemaining:   10,
			MonthlyRemaining: 300,
			DailyUsed:        0,
			MonthlyUsed:      0,
			DailyLimit:       10,
			MonthlyLimit:     300,
		}, nil
	}

	if err != nil {
		r.logger.Error("failed to get limit info",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return nil, fmt.Errorf("get limit info: %w", err)
	}

	return r.buildLimitInfo(&limit), nil
}

// SetDailyLimit устанавливает дневной лимит для пользователя
func (r *LimitRepository) SetDailyLimit(userID int64, limit int) error {
	query := `
		UPDATE user_limits
		SET daily_limit = $1,
		    updated_at = NOW()
		WHERE user_id = $2
	`

	result, err := r.db.Exec(query, limit, userID)
	if err != nil {
		r.logger.Error("failed to set daily limit",
			zap.Int64("user_id", userID),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return fmt.Errorf("set daily limit: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Пользователя нет в БД, создаём запись
		_, err := r.GetOrCreate(userID, "")
		if err != nil {
			return err
		}
		// Пробуем ещё раз
		return r.SetDailyLimit(userID, limit)
	}

	r.logger.Info("daily limit updated",
		zap.Int64("user_id", userID),
		zap.Int("new_limit", limit),
	)

	return nil
}

// SetMonthlyLimit устанавливает месячный лимит для пользователя
func (r *LimitRepository) SetMonthlyLimit(userID int64, limit int) error {
	query := `
		UPDATE user_limits
		SET monthly_limit = $1,
		    updated_at = NOW()
		WHERE user_id = $2
	`

	result, err := r.db.Exec(query, limit, userID)
	if err != nil {
		r.logger.Error("failed to set monthly limit",
			zap.Int64("user_id", userID),
			zap.Int("limit", limit),
			zap.Error(err),
		)
		return fmt.Errorf("set monthly limit: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		// Пользователя нет в БД, создаём запись
		_, err := r.GetOrCreate(userID, "")
		if err != nil {
			return err
		}
		// Пробуем ещё раз
		return r.SetMonthlyLimit(userID, limit)
	}

	r.logger.Info("monthly limit updated",
		zap.Int64("user_id", userID),
		zap.Int("new_limit", limit),
	)

	return nil
}

// ResetDailyIfNeeded сбрасывает дневной счётчик если прошло 24 часа
func (r *LimitRepository) ResetDailyIfNeeded(userID int64) error {
	query := `
		UPDATE user_limits
		SET daily_used = 0,
		    last_reset_daily = NOW(),
		    updated_at = NOW()
		WHERE user_id = $1
		  AND last_reset_daily < NOW() - INTERVAL '24 hours'
	`

	result, err := r.db.Exec(query, userID)
	if err != nil {
		r.logger.Error("failed to reset daily limit",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return fmt.Errorf("reset daily limit: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		r.logger.Info("daily limit reset",
			zap.Int64("user_id", userID),
		)
	}

	return nil
}

// ResetMonthlyIfNeeded сбрасывает месячный счётчик если прошло 30 дней
func (r *LimitRepository) ResetMonthlyIfNeeded(userID int64) error {
	query := `
		UPDATE user_limits
		SET monthly_used = 0,
		    last_reset_monthly = NOW(),
		    updated_at = NOW()
		WHERE user_id = $1
		  AND last_reset_monthly < NOW() - INTERVAL '30 days'
	`

	result, err := r.db.Exec(query, userID)
	if err != nil {
		r.logger.Error("failed to reset monthly limit",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return fmt.Errorf("reset monthly limit: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows > 0 {
		r.logger.Info("monthly limit reset",
			zap.Int64("user_id", userID),
		)
	}

	return nil
}

// buildLimitInfo создаёт LimitInfo из UserLimit
func (r *LimitRepository) buildLimitInfo(limit *UserLimit) *LimitInfo {
	dailyRemaining := limit.DailyLimit - limit.DailyUsed
	if dailyRemaining < 0 {
		dailyRemaining = 0
	}

	monthlyRemaining := limit.MonthlyLimit - limit.MonthlyUsed
	if monthlyRemaining < 0 {
		monthlyRemaining = 0
	}

	return &LimitInfo{
		DailyRemaining:   dailyRemaining,
		MonthlyRemaining: monthlyRemaining,
		DailyUsed:        limit.DailyUsed,
		MonthlyUsed:      limit.MonthlyUsed,
		DailyLimit:       limit.DailyLimit,
		MonthlyLimit:     limit.MonthlyLimit,
	}
}

// ============================================================================
// Phase 2.5: Content Type Limiter
// ============================================================================

// GetContentLimit получает лимит на тип контента для пользователя
// Сначала проверяет персональный лимит (user_id != NULL)
// Потом общий лимит (user_id = NULL для allmembers)
// Возвращает: -1 = запрет, 0 = без лимита, N = лимит на N сообщений/день
func (r *LimitRepository) GetContentLimit(chatID, userID int64, contentType string) (int, error) {
	// 1. Проверяем персональный лимит
	query := `
		SELECT daily_limit
		FROM limiter_config
		WHERE chat_id = $1 AND user_id = $2 AND content_type = $3
		LIMIT 1
	`

	var limit int
	err := r.db.QueryRow(query, chatID, userID, contentType).Scan(&limit)
	if err == nil {
		// Нашли персональный лимит
		return limit, nil
	}

	if err != sql.ErrNoRows {
		r.logger.Error("failed to get personal content limit",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Error(err))
		return 0, fmt.Errorf("get personal content limit: %w", err)
	}

	// 2. Проверяем общий лимит (user_id = NULL)
	query = `
		SELECT daily_limit
		FROM limiter_config
		WHERE chat_id = $1 AND user_id IS NULL AND content_type = $2
		LIMIT 1
	`

	err = r.db.QueryRow(query, chatID, contentType).Scan(&limit)
	if err == sql.ErrNoRows {
		// Нет конфигурации - без ограничений
		return 0, nil
	}

	if err != nil {
		r.logger.Error("failed to get allmembers content limit",
			zap.Int64("chat_id", chatID),
			zap.String("content_type", contentType),
			zap.Error(err))
		return 0, fmt.Errorf("get allmembers content limit: %w", err)
	}

	return limit, nil
}

// GetContentCount получает количество сообщений типа contentType за сегодня
func (r *LimitRepository) GetContentCount(chatID, userID int64, contentType string, date time.Time) (int, error) {
	counterDate := date.Format("2006-01-02")

	query := `
		SELECT COALESCE(counter_value, 0)
		FROM limiter_counters
		WHERE chat_id = $1 AND user_id = $2 AND content_type = $3 AND counter_date = $4
	`

	var count int
	err := r.db.QueryRow(query, chatID, userID, contentType, counterDate).Scan(&count)
	if err == sql.ErrNoRows {
		// Нет записи - значит 0
		return 0, nil
	}

	if err != nil {
		r.logger.Error("failed to get content count",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.String("date", counterDate),
			zap.Error(err))
		return 0, fmt.Errorf("get content count: %w", err)
	}

	return count, nil
}

// IncrementContentCounter инкрементирует счётчик контента (UPSERT)
func (r *LimitRepository) IncrementContentCounter(chatID, userID int64, contentType string, date time.Time) error {
	counterDate := date.Format("2006-01-02")

	query := `
		INSERT INTO limiter_counters (chat_id, user_id, content_type, counter_date, counter_value, updated_at)
		VALUES ($1, $2, $3, $4, 1, NOW())
		ON CONFLICT (chat_id, user_id, content_type, counter_date)
		DO UPDATE SET
			counter_value = limiter_counters.counter_value + 1,
			updated_at = NOW()
	`

	_, err := r.db.Exec(query, chatID, userID, contentType, counterDate)
	if err != nil {
		r.logger.Error("failed to increment content counter",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.String("date", counterDate),
			zap.Error(err))
		return fmt.Errorf("increment content counter: %w", err)
	}

	return nil
}

// IsVIP проверяет является ли пользователь VIP (игнорирует лимиты)
// Проверяет флаг is_vip в limiter_config
func (r *LimitRepository) IsVIP(userID int64) (bool, error) {
	query := `
		SELECT COALESCE(is_vip, false)
		FROM limiter_config
		WHERE user_id = $1 AND is_vip = true
		LIMIT 1
	`

	var isVIP bool
	err := r.db.QueryRow(query, userID).Scan(&isVIP)
	if err == sql.ErrNoRows {
		// Нет записи с VIP флагом
		return false, nil
	}

	if err != nil {
		r.logger.Error("failed to check VIP status",
			zap.Int64("user_id", userID),
			zap.Error(err))
		return false, fmt.Errorf("check VIP status: %w", err)
	}

	return isVIP, nil
}

// SaveContentLimit сохраняет лимит в limiter_config (для админских команд)
func (r *LimitRepository) SaveContentLimit(chatID, userID int64, contentType string, limit int) error {
	query := `
		INSERT INTO limiter_config (chat_id, user_id, content_type, daily_limit, warning_threshold, is_vip, updated_at)
		VALUES ($1, NULLIF($2, 0), $3, $4, 2, false, NOW())
		ON CONFLICT (chat_id, COALESCE(user_id, -1), content_type)
		DO UPDATE SET
			daily_limit = EXCLUDED.daily_limit,
			updated_at = NOW()
	`

	_, err := r.db.Exec(query, chatID, userID, contentType, limit)
	if err != nil {
		r.logger.Error("failed to save content limit",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Int("limit", limit),
			zap.Error(err))
		return fmt.Errorf("save content limit: %w", err)
	}

	return nil
}

// GetAllContentLimits получает все лимиты чата из limiter_config
func (r *LimitRepository) GetAllContentLimits(chatID int64) (map[string]int, error) {
	query := `
		SELECT content_type, daily_limit
		FROM limiter_config
		WHERE chat_id = $1 AND user_id IS NULL
		ORDER BY content_type
	`

	rows, err := r.db.Query(query, chatID)
	if err != nil {
		r.logger.Error("failed to get all content limits",
			zap.Int64("chat_id", chatID),
			zap.Error(err))
		return nil, fmt.Errorf("get all content limits: %w", err)
	}
	defer rows.Close()

	limits := make(map[string]int)
	for rows.Next() {
		var contentType string
		var limit int
		if err := rows.Scan(&contentType, &limit); err != nil {
			r.logger.Error("failed to scan content limit",
				zap.Error(err))
			return nil, fmt.Errorf("scan content limit: %w", err)
		}
		limits[contentType] = limit
	}

	return limits, nil
}
