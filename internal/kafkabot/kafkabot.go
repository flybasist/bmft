package kafkabot

import (
	"context"
	"time"

	"github.com/segmentio/kafka-go"
)

// Русский комментарий: Пакет предоставляет фабричные функции создания reader/writer без глобального состояния.
// Это упрощает тестирование и позволяет гибко настраивать брокеров и группы.

// NewReader создаёт новый kafka.Reader с ручным контролем commit (через FetchMessage / CommitMessages).
func NewReader(topic string, brokers []string, groupID string) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		StartOffset:    kafka.FirstOffset,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: 0, // 0 отключает авто-commit, делаем явный commit после успешной обработки
	})
}

// NewWriter создаёт новый kafka.Writer для указанного топика.
func NewWriter(topic string, brokers []string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		Balancer:     &kafka.LeastBytes{},
		BatchSize:    1,           // Русский комментарий: пока по одному сообщению; можно повысить для throughput.
		BatchTimeout: time.Second, // Ограничение времени на флеш батча.
	}
}

// WriteMessage — helper для записи одного сообщения с ключом.
func WriteMessage(ctx context.Context, w *kafka.Writer, key string, value []byte) error {
	return w.WriteMessages(ctx, kafka.Message{Key: []byte(key), Value: value})
}
