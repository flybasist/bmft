package postgresql

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"github.com/segmentio/kafka-go"
)

func EnsureDatabaseExists(dsn string) {
	adminDB, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer adminDB.Close()

	_, err = adminDB.Exec("CREATE DATABASE bmft")
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		log.Fatalf("Failed to create database: %v", err)
	}
	log.Println("Database bmft is ready")
}

func StartKafkaToPostgres(ctx context.Context, kafkaAddr string, db *sql.DB) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{kafkaAddr},
		Topic:   "telegram-listener",
		GroupID: "bmft-saver",
	})
	defer reader.Close()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Kafka read error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var update map[string]any
		if err := json.Unmarshal(msg.Value, &update); err != nil {
			log.Printf("Invalid JSON from Kafka: %v", err)
			continue
		}

		chatID := extractChatID(update)
		if chatID == "" {
			log.Printf("Could not extract chat_id")
			continue
		}

		tableName := fmt.Sprintf("chat_%s", chatID)
		createIfNotExists(db, tableName)
		saveToTable(db, tableName, update)
		saveJSON(db, chatID, msg.Value)
	}
}

func extractChatID(update map[string]any) string {
	if msg, ok := update["message"].(map[string]any); ok {
		if chat, ok := msg["chat"].(map[string]any); ok {
			return intToStr(chat["id"]) // исправлено здесь
		}
	}
	return ""
}

func createIfNotExists(db *sql.DB, table string) {
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

	if _, err := db.Exec(query); err != nil {
		log.Printf("Failed to create table %s: %v", table, err)
	}
}

func saveToTable(db *sql.DB, table string, update map[string]any) {
	msg, ok1 := update["message"].(map[string]any)
	chat, ok2 := msg["chat"].(map[string]any)
	from, ok3 := msg["from"].(map[string]any)
	if !ok1 || !ok2 || !ok3 {
		log.Printf("Failed to extract fields for table save")
		return
	}

	// Конвертация даты из UNIX в time.Time
	var dateTime time.Time
	switch v := msg["date"].(type) {
	case float64:
		dateTime = time.Unix(int64(v), 0).UTC()
	case json.Number:
		sec, _ := v.Int64()
		dateTime = time.Unix(sec, 0).UTC()
	default:
		log.Printf("Invalid date field in message")
		dateTime = time.Now().UTC()
	}

	query := fmt.Sprintf(`INSERT INTO "%s" (
		chat_id, user_id, chatname, chattitle, username, message_id,
		contenttype, text, caption, date_message
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, table)

	_, err := db.Exec(query,
		intToStr(chat["id"]),
		intToStr(from["id"]),
		chat["username"],
		chat["title"],
		from["username"],
		intToStr(msg["message_id"]),
		msg["type"],
		msg["text"],
		msg["caption"],
		dateTime, // ← правильный TIMESTAMP
	)
	if err != nil {
		log.Printf("Failed to insert message into %s: %v", table, err)
	}
}

// intToStr безопасно конвертирует числовое значение в строку без экспоненты
func intToStr(v any) string {
	switch val := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", val) // Без дробной части, без e+07
	case int:
		return fmt.Sprint(val)
	case int64:
		return fmt.Sprint(val)
	case json.Number:
		return val.String()
	default:
		return fmt.Sprint(v)
	}
}

func saveJSON(db *sql.DB, chatID string, raw []byte) {
	query := `CREATE TABLE IF NOT EXISTS raw_updates (
		id SERIAL PRIMARY KEY,
		chat_id TEXT,
		payload JSONB,
		created_at TIMESTAMP DEFAULT now()
	)`
	if _, err := db.Exec(query); err != nil {
		log.Printf("Failed to ensure raw_updates table: %v", err)
		return
	}

	_, err := db.Exec(`INSERT INTO raw_updates (chat_id, payload) VALUES ($1, $2)`, chatID, raw)
	if err != nil {
		log.Printf("Failed to save raw update: %v", err)
	}
}
