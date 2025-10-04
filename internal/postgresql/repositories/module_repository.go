package repositories

import (
	"database/sql"
	"encoding/json"
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

// GetConfig читает JSONB конфигурацию модуля для чата.
// Русский комментарий: Каждый модуль может хранить свою конфигурацию в JSONB колонке.
func (r *ModuleRepository) GetConfig(chatID int64, moduleName string) (map[string]interface{}, error) {
	var configJSON []byte
	query := `SELECT config FROM chat_modules WHERE chat_id = $1 AND module_name = $2`
	err := r.db.QueryRow(query, chatID, moduleName).Scan(&configJSON)
	if err == sql.ErrNoRows {
		return make(map[string]interface{}), nil // Нет записи = пустой конфиг
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get module config: %w", err)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal module config: %w", err)
	}
	return config, nil
}

// UpdateConfig обновляет JSONB конфигурацию модуля.
func (r *ModuleRepository) UpdateConfig(chatID int64, moduleName string, config map[string]interface{}) error {
	configJSON, err := json.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal module config: %w", err)
	}

	query := `
		INSERT INTO chat_modules (chat_id, module_name, config)
		VALUES ($1, $2, $3)
		ON CONFLICT (chat_id, module_name) DO UPDATE
		SET config = EXCLUDED.config, updated_at = NOW()
	`
	_, err = r.db.Exec(query, chatID, moduleName, configJSON)
	if err != nil {
		return fmt.Errorf("failed to update module config: %w", err)
	}
	return nil
}

// GetEnabledModules возвращает список включенных модулей для чата.
func (r *ModuleRepository) GetEnabledModules(chatID int64) ([]string, error) {
	query := `SELECT module_name FROM chat_modules WHERE chat_id = $1 AND is_enabled = true`
	rows, err := r.db.Query(query, chatID)
	if err != nil {
		return nil, fmt.Errorf("failed to get enabled modules: %w", err)
	}
	defer rows.Close()

	var modules []string
	for rows.Next() {
		var moduleName string
		if err := rows.Scan(&moduleName); err != nil {
			return nil, fmt.Errorf("failed to scan module name: %w", err)
		}
		modules = append(modules, moduleName)
	}
	return modules, rows.Err()
}
