package repositories

import (
	"database/sql"
	"fmt"
)

// ModuleRepository управляет операциями с таблицей chat_modules.
// Русский комментарий: Репозиторий для управления модулями в чатах.
// Проверяет включен ли модуль, включает/выключает, читает/пишет JSONB конфиг.
type ModuleRepository struct {
	db *sql.DB
}

// NewModuleRepository создаёт новый инстанс репозитория модулей.
func NewModuleRepository(db *sql.DB) *ModuleRepository {
	return &ModuleRepository{db: db}
}

// IsEnabled проверяет включен ли модуль для данного чата.
func (r *ModuleRepository) IsEnabled(chatID int64, moduleName string) (bool, error) {
	var isEnabled bool
	query := `SELECT is_enabled FROM chat_modules WHERE chat_id = $1 AND module_name = $2`
	err := r.db.QueryRow(query, chatID, moduleName).Scan(&isEnabled)
	if err == sql.ErrNoRows {
		return false, nil // Модуль не зарегистрирован для чата = выключен
	}
	if err != nil {
		return false, fmt.Errorf("failed to check module enabled: %w", err)
	}
	return isEnabled, nil
}

// Enable включает модуль для чата (создаёт запись или обновляет is_enabled = true).
func (r *ModuleRepository) Enable(chatID int64, moduleName string) error {
	query := `
		INSERT INTO chat_modules (chat_id, module_name, is_enabled)
		VALUES ($1, $2, true)
		ON CONFLICT (chat_id, module_name) DO UPDATE
		SET is_enabled = true, updated_at = NOW()
	`
	_, err := r.db.Exec(query, chatID, moduleName)
	if err != nil {
		return fmt.Errorf("failed to enable module: %w", err)
	}
	return nil
}

// Disable выключает модуль для чата (is_enabled = false).
func (r *ModuleRepository) Disable(chatID int64, moduleName string) error {
	query := `UPDATE chat_modules SET is_enabled = false, updated_at = NOW() WHERE chat_id = $1 AND module_name = $2`
	_, err := r.db.Exec(query, chatID, moduleName)
	if err != nil {
		return fmt.Errorf("failed to disable module: %w", err)
	}
	return nil
}
