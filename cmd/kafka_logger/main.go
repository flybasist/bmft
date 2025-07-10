package main

import (
	"context"
	"log"
	"os"

	"github.com/flybasist/bmft/internal/logger"
)

func main() {
	kafkaAddr := os.Getenv("KAFKA_BROKERS")
	if kafkaAddr == "" {
		log.Fatal("KAFKA_BROKERS not set")
	}

	ctx := context.Background()

	go logger.RunKafkaLogger(ctx, kafkaAddr, "telegram-listener")
	go logger.RunKafkaLogger(ctx, kafkaAddr, "telegram-send")
	go logger.RunKafkaLogger(ctx, kafkaAddr, "telegram-delete")

	// Блокируем основной поток, чтобы программа не завершалась
	select {}
}
