package postgresql

import (
	"context"
	"database/sql"
)

// CreateTables создает необходимые таблицы (dev helper).
// Русский комментарий: В продакшене рекомендуется система миграций.
func CreateTables(ctx context.Context, db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS messages (
			id SERIAL PRIMARY KEY,
			chat_id TEXT,
			user_id TEXT,
			chatname TEXT,
			chattitle TEXT,
			username TEXT,
			message_id TEXT,
			contenttype TEXT,
			text TEXT,
			caption TEXT,
			vip INTEGER DEFAULT 0,
			violation INTEGER DEFAULT 0,
			date_message TIMESTAMP,
			raw_update JSONB
		);`,
		// Уникальный индекс для идемпотентности вставки
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_messages_chat_msg ON messages (chat_id, message_id);`,
		`CREATE TABLE IF NOT EXISTS limits (
			id SERIAL PRIMARY KEY,
			user_id TEXT UNIQUE,
			audio INTEGER DEFAULT 0,
			photo INTEGER DEFAULT 0,
			voice INTEGER DEFAULT 0,
			video INTEGER DEFAULT 0,
			document INTEGER DEFAULT 0,
			text INTEGER DEFAULT 0,
			location INTEGER DEFAULT 0,
			contact INTEGER DEFAULT 0,
			sticker INTEGER DEFAULT 0,
			animation INTEGER DEFAULT 0,
			video_note INTEGER DEFAULT 0,
			violation INTEGER DEFAULT 0,
			vip INTEGER DEFAULT 0
		);`,
		`CREATE TABLE IF NOT EXISTS reaction (
			id SERIAL PRIMARY KEY,
			user_id TEXT UNIQUE,
			contenttype TEXT,
			answertype TEXT,
			regex TEXT,
			answer TEXT,
			violation INTEGER DEFAULT 0,
			vip INTEGER DEFAULT 0
		);`,
		`INSERT INTO limits (user_id) VALUES ('allmembers') ON CONFLICT (user_id) DO NOTHING;`,
		`INSERT INTO reaction (user_id) VALUES ('allmembers') ON CONFLICT (user_id) DO NOTHING;`,
	}
	for _, q := range queries {
		if _, err := db.ExecContext(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
