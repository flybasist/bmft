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

// IsActive проверяет активен ли чат.
// FUTURE(Phase3): Будет использоваться в Reactions Module для проверки заблокированных чатов
func (r *ChatRepository) IsActive(chatID int64) (bool, error) {
	var isActive bool
	query := `SELECT is_active FROM chats WHERE chat_id = $1`
	err := r.db.QueryRow(query, chatID).Scan(&isActive)
	if err == sql.ErrNoRows {
		return false, nil // Чат не найден = не активен
	}
	if err != nil {
		return false, fmt.Errorf("failed to check chat active status: %w", err)
	}
	return isActive, nil
}

// Deactivate деактивирует чат (помечает is_active = false).
// Русский комментарий: Используется когда бота удаляют из группы или блокируют.
// FUTURE(Phase4): Будет использоваться в Statistics Module для обработки удаления бота
func (r *ChatRepository) Deactivate(chatID int64) error {
	query := `UPDATE chats SET is_active = false, updated_at = NOW() WHERE chat_id = $1`
	_, err := r.db.Exec(query, chatID)
	if err != nil {
		return fmt.Errorf("failed to deactivate chat: %w", err)
	}
	return nil
}

// GetChatInfo получает информацию о чате.
// FUTURE(Phase4): Будет использоваться в админ-командах для получения информации о чате
func (r *ChatRepository) GetChatInfo(chatID int64) (chatType, title, username string, isActive bool, err error) {
	query := `SELECT chat_type, title, username, is_active FROM chats WHERE chat_id = $1`
	err = r.db.QueryRow(query, chatID).Scan(&chatType, &title, &username, &isActive)
	if err != nil {
		return "", "", "", false, fmt.Errorf("failed to get chat info: %w", err)
	}
	return
}
