package mbrabbit

import (
	"fmt"
	"os"

	amqp "github.com/rabbitmq/amqp091-go"
)

func Publish(raw []byte) error {
	rmqURL := os.Getenv("RABBIT_URL")
	if rmqURL == "" {
		rmqURL = "amqp://telegram:secret123@rabbitmq:5672/"
	}

	conn, err := amqp.Dial(rmqURL)
	if err != nil {
		return fmt.Errorf("error connect to RabbitMQ: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("error open channel: %w", err)
	}
	defer ch.Close()

	// Очередь
	_, err = ch.QueueDeclare(
		"telegram_updates", // имя
		true,               // durable
		false,              // autoDelete
		false,              // exclusive
		false,              // noWait
		nil,
	)
	if err != nil {
		return fmt.Errorf("fail created queue: %w", err)
	}

	// Публикация
	err = ch.Publish(
		"",                 // exchange
		"telegram_updates", // routing key
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        raw,
		},
	)
	if err != nil {
		return fmt.Errorf("publish error: %w", err)
	}

	return nil
}
