package postgresql

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/flybasist/bmft/internal/utils"

	_ "github.com/lib/pq"
)

// ConnectToBase — подключение к базе
func ConnectToBase() (*sql.DB, error) {
	pgURL := os.Getenv("POSTGRES_DSN")
	if pgURL == "" {
		return nil, fmt.Errorf("POSTGRES_DSN not set")
	}

	db, err := sql.Open("postgres", pgURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Postgres: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping Postgres: %w", err)
	}

	return db, nil
}

// CreateIfNotExists — создаёт таблицу под чат
func CreateIfNotExists(db *sql.DB, table string) error {
	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (
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
		date_message TIMESTAMP
	);`, table)

	_, err := db.Exec(query)
	return err
}

// SaveToTable — сохраняет извлечённые поля в таблицу конкретного чата
func SaveToTable(db *sql.DB, table string, update map[string]any) error {
	msg, ok1 := update["message"].(map[string]any)
	chat, ok2 := msg["chat"].(map[string]any)
	from, ok3 := msg["from"].(map[string]any)
	if !ok1 || !ok2 || !ok3 {
		return fmt.Errorf("failed to extract fields for table save")
	}

	var dateTime time.Time
	switch v := msg["date"].(type) {
	case float64:
		dateTime = time.Unix(int64(v), 0).UTC()
	case json.Number:
		sec, _ := v.Int64()
		dateTime = time.Unix(sec, 0).UTC()
	default:
		dateTime = time.Now().UTC()
	}

	query := fmt.Sprintf(`INSERT INTO "%s" (
		chat_id, user_id, chatname, chattitle, username, message_id,
		contenttype, text, caption, date_message
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, table)

	_, err := db.Exec(query,
		utils.IntToStr(chat["id"]),
		utils.IntToStr(from["id"]),
		chat["username"],
		chat["title"],
		from["username"],
		utils.IntToStr(msg["message_id"]),
		msg["type"],
		msg["text"],
		msg["caption"],
		dateTime,
	)
	return err
}

// SaveJSON — сохраняет необработанный update в отдельную таблицу
func SaveJSON(db *sql.DB, chatID string, raw []byte) error {
	query := `CREATE TABLE IF NOT EXISTS raw_updates (
		id SERIAL PRIMARY KEY,
		chat_id TEXT,
		payload JSONB,
		created_at TIMESTAMP DEFAULT now()
	)`
	if _, err := db.Exec(query); err != nil {
		return err
	}

	_, err := db.Exec(`INSERT INTO raw_updates (chat_id, payload) VALUES ($1, $2)`, chatID, raw)
	return err
}
