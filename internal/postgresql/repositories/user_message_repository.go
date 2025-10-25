package repositories

import (
	"database/sql"
	"go.uber.org/zap"
)

// UserMessageRepository реализует сохранение пользователей и сообщений
// в таблицы users и messages.
type UserMessageRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewUserMessageRepository(db *sql.DB, logger *zap.Logger) *UserMessageRepository {
	return &UserMessageRepository{db: db, logger: logger}
}

// UpsertUser сохраняет или обновляет пользователя в таблице users.
func (r *UserMessageRepository) UpsertUser(userID int64, username, firstName string) error {
	query := `
		INSERT INTO users (user_id, username, first_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			username = EXCLUDED.username,
			first_name = EXCLUDED.first_name
	`
	_, err := r.db.Exec(query, userID, username, firstName)
	if err != nil {
		r.logger.Error("failed to upsert user", zap.Int64("user_id", userID), zap.Error(err))
		return err
	}
	return nil
}

// InsertMessage сохраняет сообщение в таблице messages.
func (r *UserMessageRepository) InsertMessage(chatID, userID int64, messageID int, contentType string) error {
	query := `
		INSERT INTO messages (chat_id, user_id, message_id, content_type)
		VALUES ($1, $2, $3, $4)
	`
	_, err := r.db.Exec(query, chatID, userID, messageID, contentType)
	if err != nil {
		r.logger.Error("failed to insert message", zap.Int64("user_id", userID), zap.Error(err))
		return err
	}
	return nil
}
