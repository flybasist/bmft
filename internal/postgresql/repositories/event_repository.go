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

// GetRecentEvents получает последние N событий для чата.
// FUTURE(Phase4): Statistics Module будет использовать для команды /events и анализа
func (r *EventRepository) GetRecentEvents(chatID int64, limit int) ([]Event, error) {
	query := `
		SELECT id, chat_id, user_id, module_name, event_type, details, created_at
		FROM event_log
		WHERE chat_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := r.db.Query(query, chatID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent events: %w", err)
	}
	defer rows.Close()

	var events []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.ChatID, &e.UserID, &e.ModuleName, &e.EventType, &e.Details, &e.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// Event представляет событие из event_log.
type Event struct {
	ID         int64
	ChatID     int64
	UserID     int64
	ModuleName string
	EventType  string
	Details    string
	CreatedAt  string // PostgreSQL timestamp
}
