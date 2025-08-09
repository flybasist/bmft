package logger

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/segmentio/kafka-go"
)

const (
	logDir       = "./logs"
	logRetention = 7 * 24 * time.Hour
)

func Run() {
	kafkaAddr := os.Getenv("KAFKA_BROKERS")
	if kafkaAddr == "" {
		log.Fatal("KAFKA_BROKERS not set")
	}

	ctx := context.Background()

	go RunKafkaLogger(ctx, kafkaAddr, "telegram-listener")
	go RunKafkaLogger(ctx, kafkaAddr, "telegram-send")
	go RunKafkaLogger(ctx, kafkaAddr, "telegram-delete")

	// Блокируем основной поток, чтобы программа не завершалась
	select {}
}

// Читает сообщения из Kafka и пишет в ежедневные лог-файлы
func RunKafkaLogger(ctx context.Context, kafkaAddr, topic string) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("Failed to create log dir: %v", err)
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{kafkaAddr},
		Topic:   topic,
		GroupID: "kafka-json-logger",
	})
	defer reader.Close()

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Kafka read error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		go cleanOldLogs()

		logPath := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
		file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Failed to open log file: %v", err)
			continue
		}
		writer := bufio.NewWriter(file)

		var pretty map[string]any
		if json.Unmarshal(msg.Value, &pretty) == nil {
			data, _ := json.MarshalIndent(pretty, "", "  ")
			writer.Write(data)
			writer.WriteString("\n")
		} else {
			writer.Write(msg.Value)
			writer.WriteString("\n")
		}

		writer.Flush()
		file.Close()
	}
}

func cleanOldLogs() {
	files, err := os.ReadDir(logDir)
	if err != nil {
		log.Printf("Failed to read log directory: %v", err)
		return
	}

	now := time.Now()
	for _, file := range files {
		if info, err := file.Info(); err == nil {
			if now.Sub(info.ModTime()) > logRetention {
				os.Remove(filepath.Join(logDir, file.Name()))
			}
		}
	}
}
