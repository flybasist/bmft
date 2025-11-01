package repositories

import (
	"database/sql"
	"fmt"
)

// ChatRepository управляет операциями с таблицей chats.
// Русский комментарий: Репозиторий для работы с чатами.
// Автоматически создаёт запись при первом сообщении, деактивирует удалённые чаты.
type ChatRepository struct {
	db *sql.DB
}

// NewChatRepository создаёт новый инстанс репозитория чатов.
func NewChatRepository(db *sql.DB) *ChatRepository {
	return &ChatRepository{db: db}
}

// GetOrCreate получает существующий чат или создаёт новую запись.
// Русский комментарий: Вызывается при каждом сообщении для гарантии, что чат есть в БД.
func (r *ChatRepository) GetOrCreate(chatID int64, chatType, title, username string) error {
	query := `
		INSERT INTO chats (chat_id, chat_type, title, username, is_active)
		VALUES ($1, $2, $3, $4, true)
		ON CONFLICT (chat_id) DO UPDATE
		SET
			chat_type = EXCLUDED.chat_type,
			title = EXCLUDED.title,
			username = EXCLUDED.username,
			is_active = true,
			updated_at = NOW()
	`
	_, err := r.db.Exec(query, chatID, chatType, title, username)
	if err != nil {
		return fmt.Errorf("failed to get or create chat: %w", err)
	}
	return nil
}
