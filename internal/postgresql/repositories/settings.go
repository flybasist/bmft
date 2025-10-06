package repositories

import (
	"database/sql"
	"fmt"
)

// SettingsRepository управляет глобальными настройками бота
type SettingsRepository struct {
	db *sql.DB
}

// NewSettingsRepository создаёт новый репозиторий настроек
func NewSettingsRepository(db *sql.DB) *SettingsRepository {
	return &SettingsRepository{db: db}
}

// GetVersion получает версию бота из БД
func (r *SettingsRepository) GetVersion() (string, error) {
	var version string
	err := r.db.QueryRow(`
		SELECT bot_version FROM bot_settings WHERE id = 1
	`).Scan(&version)

	if err == sql.ErrNoRows {
		return "unknown", nil
	}
	if err != nil {
		return "", fmt.Errorf("get version: %w", err)
	}

	return version, nil
}

// GetTimezone получает часовой пояс из БД
func (r *SettingsRepository) GetTimezone() (string, error) {
	var timezone string
	err := r.db.QueryRow(`
		SELECT timezone FROM bot_settings WHERE id = 1
	`).Scan(&timezone)

	if err == sql.ErrNoRows {
		return "UTC", nil
	}
	if err != nil {
		return "", fmt.Errorf("get timezone: %w", err)
	}

	return timezone, nil
}

// SetVersion устанавливает версию бота
func (r *SettingsRepository) SetVersion(version string) error {
	_, err := r.db.Exec(`
		UPDATE bot_settings SET bot_version = $1 WHERE id = 1
	`, version)

	if err != nil {
		return fmt.Errorf("set version: %w", err)
	}

	return nil
}

// SetTimezone устанавливает часовой пояс
func (r *SettingsRepository) SetTimezone(timezone string) error {
	_, err := r.db.Exec(`
		UPDATE bot_settings SET timezone = $1 WHERE id = 1
	`, timezone)

	if err != nil {
		return fmt.Errorf("set timezone: %w", err)
	}

	return nil
}
