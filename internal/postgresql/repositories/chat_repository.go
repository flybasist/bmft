package repositories

import (
	"database/sql"
	"fmt"
)

// ============================================================================
// ChatRepository - управление чатами
// ============================================================================

// ChatRepository управляет операциями с таблицей chats.
// Репозиторий для работы с чатами.
// Автоматически создаёт запись при первом сообщении, деактивирует удалённые чаты.
type ChatRepository struct {
	db *sql.DB
}

// NewChatRepository создаёт новый инстанс репозитория чатов.
func NewChatRepository(db *sql.DB) *ChatRepository {
	return &ChatRepository{db: db}
}

// GetOrCreate получает существующий чат или создаёт новую запись.
// Вызывается при добавлении бота в чат и при /start.
// isForum = true для супергрупп с включёнными топиками (Telegram Forums).
// Критично: без записи is_forum функции GetThreadID/GetThreadIDFromMessage
// всегда возвращают 0, и все топик-зависимые функции (лимиты, VIP, реакции) ломаются.
func (r *ChatRepository) GetOrCreate(chatID int64, chatType, title, username string, isForum bool) error {
	query := `
		INSERT INTO chats (chat_id, chat_type, title, username, is_forum, is_active)
		VALUES ($1, $2, $3, $4, $5, true)
		ON CONFLICT (chat_id) DO UPDATE
		SET
			chat_type = EXCLUDED.chat_type,
			title = EXCLUDED.title,
			username = EXCLUDED.username,
			is_forum = EXCLUDED.is_forum,
			is_active = true,
			updated_at = NOW()
	`
	_, err := r.db.Exec(query, chatID, chatType, title, username, isForum)
	if err != nil {
		return fmt.Errorf("failed to get or create chat: %w", err)
	}
	return nil
}

// EnsureExists гарантирует наличие записи о чате в таблице chats.
// Используется перед операциями, требующими FK на chats (content_limits,
// profanity_settings, scheduled_tasks, keyword_reactions). Если чат уже
// существует, ничего не делает; полные метаданные заполняются позже через
// GetOrCreate при добавлении бота в чат или при /start.
func (r *ChatRepository) EnsureExists(chatID int64) error {
	_, err := r.db.Exec(`
		INSERT INTO chats (chat_id, chat_type, title)
		VALUES ($1, 'unknown', 'unknown')
		ON CONFLICT (chat_id) DO NOTHING
	`, chatID)
	if err != nil {
		return fmt.Errorf("ensure chat exists: %w", err)
	}
	return nil
}
