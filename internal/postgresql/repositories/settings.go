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
		SELECT value FROM bot_settings WHERE key = 'bot_version'
	`).Scan(&version)

	if err == sql.ErrNoRows {
		return "unknown", nil
	}
	if err != nil {
		return "", fmt.Errorf("get version: %w", err)
	}

	return version, nil
}

// GetSetting получает любую настройку по ключу
func (r *SettingsRepository) GetSetting(key string) (string, error) {
	var value string
	err := r.db.QueryRow(`
		SELECT value FROM bot_settings WHERE key = $1
	`, key).Scan(&value)

	if err == sql.ErrNoRows {
		return "", fmt.Errorf("setting not found: %s", key)
	}
	if err != nil {
		return "", fmt.Errorf("get setting: %w", err)
	}

	return value, nil
}

// SetSetting устанавливает значение настройки
func (r *SettingsRepository) SetSetting(key, value, description string) error {
	_, err := r.db.Exec(`
		INSERT INTO bot_settings (key, value, description)
		VALUES ($1, $2, $3)
		ON CONFLICT (key) DO UPDATE 
		SET value = EXCLUDED.value, 
		    description = EXCLUDED.description,
		    updated_at = NOW()
	`, key, value, description)

	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}

	return nil
}
