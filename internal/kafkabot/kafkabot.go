package kafkabot

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/segmentio/kafka-go"
)

// Глобальные переменные пакета
var (
	KafkaAddr string
	Writer    *kafka.Writer
	Reader    *kafka.Reader
	Deleter   *kafka.Reader
	Ctx       = context.Background()
)

// init — инициализация подключения к Kafka
func init() {
	// Читаем адрес Kafka из переменной окружения
	KafkaAddr = os.Getenv("KAFKA_BROKERS")
	if KafkaAddr == "" {
		log.Fatal("KAFKA_BROKERS not set")
	}

	// Настройки топиков
	listenerTopic := "telegram-listener"
	senderTopic := "telegram-send"
	deleteTopic := "telegram-delete"

	// Создаём writer — для сообщений из Telegram в Kafka
	Writer = NewWriter(listenerTopic)

	// Удобные глобальные reader'ы (оставляем — это вспомогательные переменные).
	// Но теперь telegram_bot создаёт свои dedicated reader'ы в рантайме.
	Reader = NewReader(senderTopic, "telegram-sender-group")
	Deleter = NewReader(deleteTopic, "telegram-deleter-group")

	log.Println("Kafka connections initialized")
}

// CloseKafka — закрываем соединения с Kafka
func CloseKafka() {
	if Writer != nil {
		Writer.Close()
	}
	if Reader != nil {
		Reader.Close()
	}
	if Deleter != nil {
		Deleter.Close()
	}
	log.Println("Kafka connections closed")
}

// WriteKafka — записывает сообщение в Kafka
func WriteKafka(key string, msgData []byte) {
	err := Writer.WriteMessages(Ctx, kafka.Message{
		Key:   []byte(key),
		Value: msgData,
	})
	if err != nil {
		log.Printf("Failed to write to Kafka: %v", err)
	} else {
		log.Printf("Saved message %s to Kafka", key)
	}
}

// NewReader — фабрика для создания ридера Kafka
func NewReader(topic, groupID string) *kafka.Reader {
	// StartOffset: kafka.FirstOffset — полезно при ПЕРВОМ старте новой consumer-group.
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{KafkaAddr},
		Topic:          topic,
		GroupID:        groupID,
		StartOffset:    kafka.FirstOffset,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second, // используем явный time.Duration
	})
}

// NewWriter — фабрика для создания врайтера Kafka
func NewWriter(topic string) *kafka.Writer {
	return kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{KafkaAddr},
		Topic:    topic,
		Balancer: &kafka.LeastBytes{},
	})
}
