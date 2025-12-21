package repositories

import (
	"database/sql"
	"fmt"
)

// ============================================================================
// SettingsRepository - глобальные настройки
// ============================================================================

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
