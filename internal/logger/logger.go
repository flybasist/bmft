package logger

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/flybasist/bmft/internal/kafkabot"
)

const (
	logDir       = "./logs"            // Папка для логов
	logRetention = 30 * 24 * time.Hour // Храним 30 дней
)

var prettyPrint bool

func Run() {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Fatalf("Failed to create log dir: %v", err)
	}

	prettyPrint = strings.ToLower(os.Getenv("LOGGER_PRETTY")) == "true"

	ctx, cancel := context.WithCancel(context.Background())

	// Запускаем чистку старых логов раз в сутки
	go func() {
		for {
			cleanOldLogs()
			time.Sleep(24 * time.Hour)
		}
	}()

	// Список топиков, которые пишем в общий лог
	topics := []string{"telegram-listener", "telegram-send", "telegram-delete"}
	for _, topic := range topics {
		go RunKafkaLogger(ctx, topic)
	}

	// Ловим сигналы для корректного завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
	log.Println("Logger shutting down...")
	cancel()
	time.Sleep(time.Second)
}

// Читает сообщения из Kafka и пишет в общий лог-файл
func RunKafkaLogger(ctx context.Context, topic string) {
	reader := kafkabot.NewReader(topic, "logger-"+topic)
	defer reader.Close()

	log.Printf("Logger started for topic: %s", topic)

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // Завершаем при остановке
			}
			log.Printf("Kafka read error [%s]: %v", topic, err)
			time.Sleep(time.Second)
			continue
		}

		logPath := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
		file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Printf("Failed to open log file: %v", err)
			continue
		}
		writer := bufio.NewWriter(file)

		if prettyPrint {
			var pretty map[string]any
			if json.Unmarshal(msg.Value, &pretty) == nil {
				data, _ := json.MarshalIndent(pretty, "", "  ")
				writer.Write(data)
				writer.WriteString("\n")
			} else {
				writer.Write(msg.Value)
				writer.WriteString("\n")
			}
		} else {
			writer.Write(msg.Value)
			writer.WriteString("\n")
		}

		writer.Flush()
		file.Close()
	}
}

// Удаление старых логов
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
				log.Printf("Old log deleted: %s", file.Name())
			}
		}
	}
}
