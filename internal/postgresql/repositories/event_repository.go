package repositories

import (
	"database/sql"
	"fmt"
)

// EventRepository управляет записью событий в таблицу event_log.
// Русский комментарий: Репозиторий для audit trail — все действия модулей логируются здесь.
type EventRepository struct {
	db *sql.DB
}

// NewEventRepository создаёт новый инстанс репозитория событий.
func NewEventRepository(db *sql.DB) *EventRepository {
	return &EventRepository{db: db}
}

// Log записывает событие в event_log.
// Русский комментарий: Каждое действие модуля (лимит превышен, реакция сработала, etc.)
// логируется для последующего анализа и отладки.
func (r *EventRepository) Log(chatID, userID int64, moduleName, eventType, details string) error {
	query := `
		INSERT INTO event_log (chat_id, user_id, module_name, event_type, details)
		VALUES ($1, $2, $3, $4, $5)
	`
	_, err := r.db.Exec(query, chatID, userID, moduleName, eventType, details)
	if err != nil {
		return fmt.Errorf("failed to log event: %w", err)
	}
	return nil
}
