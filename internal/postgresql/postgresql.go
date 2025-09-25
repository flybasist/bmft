package postgresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/flybasist/bmft/internal/utils"
	_ "github.com/lib/pq"
)

// Русский комментарий: этот пакет инкапсулирует работу с PostgreSQL. Добавлен контекст для отмены и поле raw_update.

// ConnectToBase — подключение к базе по DSN.
func ConnectToBase(ctx context.Context, dsn string) (*sql.DB, error) {
	if dsn == "" {
		return nil, errors.New("empty postgres dsn")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	// Проверяем соединение с учётом контекста.
	pingCh := make(chan error, 1)
	go func() { pingCh <- db.Ping() }()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-pingCh:
		if err != nil {
			return nil, fmt.Errorf("ping: %w", err)
		}
	}
	return db, nil
}

// SaveToTable сохраняет распарсенный апдейт + сырой JSON.
func SaveToTable(ctx context.Context, db *sql.DB, update map[string]any, raw []byte) error {
	msg, ok1 := update["message"].(map[string]any)
	chat, ok2 := msg["chat"].(map[string]any)
	from, ok3 := msg["from"].(map[string]any)
	typemessage, ok4 := update["contenttype"].(string)
	if !ok1 || !ok2 || !ok3 || !ok4 {
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

	query := `INSERT INTO messages (
		chat_id, user_id, chatname, chattitle, username, message_id,
		contenttype, text, caption, date_message, raw_update
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	ON CONFLICT (chat_id, message_id) DO NOTHING /* идемпотентность сохранения */`

	_, err := db.ExecContext(ctx, query,
		utils.IntToStr(chat["id"]),
		utils.IntToStr(from["id"]),
		chat["username"],
		chat["title"],
		from["username"],
		utils.IntToStr(msg["message_id"]),
		typemessage,
		msg["text"],
		msg["caption"],
		dateTime,
		string(raw),
	)
	return err
}
