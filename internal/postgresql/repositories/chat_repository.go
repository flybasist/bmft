// Package repositories — доступ к таблицам PostgreSQL: chats, messages, content_limits, scheduled_tasks, vip_users, event_log, bot_settings.
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

// WelcomeSettings — настройки приветствия для конкретного чата.
// TTLSeconds = 0 означает "не удалять автоматически".
type WelcomeSettings struct {
	Enabled    bool
	TTLSeconds int
}

// GetWelcomeSettings возвращает настройки приветствия чата.
// Если записи о чате ещё нет — возвращает дефолты (включено, 300 сек),
// чтобы хендлер OnUserJoined работал даже до первого /start.
func (r *ChatRepository) GetWelcomeSettings(chatID int64) (WelcomeSettings, error) {
	var s WelcomeSettings
	err := r.db.QueryRow(`
		SELECT welcome_enabled, welcome_ttl_seconds
		FROM chats WHERE chat_id = $1
	`, chatID).Scan(&s.Enabled, &s.TTLSeconds)
	if err == sql.ErrNoRows {
		return WelcomeSettings{Enabled: true, TTLSeconds: 300}, nil
	}
	if err != nil {
		return WelcomeSettings{}, fmt.Errorf("get welcome settings: %w", err)
	}
	return s, nil
}

// SetWelcomeEnabled включает/выключает приветствие для чата.
func (r *ChatRepository) SetWelcomeEnabled(chatID int64, enabled bool) error {
	if err := r.EnsureExists(chatID); err != nil {
		return err
	}
	_, err := r.db.Exec(`
		UPDATE chats SET welcome_enabled = $1, updated_at = NOW()
		WHERE chat_id = $2
	`, enabled, chatID)
	if err != nil {
		return fmt.Errorf("set welcome enabled: %w", err)
	}
	return nil
}

// SetWelcomeTTL устанавливает TTL приветствия в секундах. ttl=0 — не удалять.
func (r *ChatRepository) SetWelcomeTTL(chatID int64, ttlSeconds int) error {
	if ttlSeconds < 0 {
		return fmt.Errorf("ttl must be >= 0, got %d", ttlSeconds)
	}
	if err := r.EnsureExists(chatID); err != nil {
		return err
	}
	_, err := r.db.Exec(`
		UPDATE chats SET welcome_ttl_seconds = $1, updated_at = NOW()
		WHERE chat_id = $2
	`, ttlSeconds, chatID)
	if err != nil {
		return fmt.Errorf("set welcome ttl: %w", err)
	}
	return nil
}
