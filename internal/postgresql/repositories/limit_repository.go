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
