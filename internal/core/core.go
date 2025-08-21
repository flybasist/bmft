package core

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/flybasist/bmft/internal/kafkabot"
	"github.com/flybasist/bmft/internal/postgresql"
	"github.com/flybasist/bmft/internal/utils"
)

func Run() {
	ctx := context.Background()
	StartKafkaConsumer(ctx)
}

// StartKafkaConsumer слушает Kafka и передаёт сообщения в бизнес-логику
func StartKafkaConsumer(ctx context.Context) {

	reader := kafkabot.NewReader("telegram-listener", "bmft-saver")
	defer reader.Close()

	log.Println("core: Kafka reader started for topic 'telegram-listener', group 'bmft-saver'")

	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("core: reader context cancelled, exiting")
				return
			}
			log.Printf("core: Kafka read error: %v", err)
			time.Sleep(time.Second)
			continue
		}

		log.Printf("core: read msg key=%s partition=%d offset=%d len=%d",
			string(msg.Key), msg.Partition, msg.Offset, len(msg.Value))
		log.Printf("core: raw payload: %s", utils.Truncate(msg.Value, 400))

		var update map[string]any
		if err := json.Unmarshal(msg.Value, &update); err != nil {
			log.Printf("core: Invalid JSON from Kafka: %v — raw: %s", err, utils.Truncate(msg.Value, 400))
			continue
		}

		// 👉 Заглушка: здесь будет твоя бизнес-логика перед записью
		processedUpdate, err := processBusinessLogic(update)
		if err != nil {
			log.Printf("core: business logic error: %v", err)
			continue
		}

		// Сохраняем в БД через CRUD слой
		if err := saveToDatabase(processedUpdate); err != nil {
			log.Printf("core: Failed to save update: %v", err)
		} else {
			log.Printf("core: Processed message key=%s offset=%d", string(msg.Key), msg.Offset)
		}
	}
}

// processBusinessLogic — заглушка для твоей бизнес-логики
func processBusinessLogic(update map[string]any) (map[string]any, error) {
	// Например, фильтрация, валидация, enrich
	return update, nil
}

// saveToDatabase — подготовка данных и вызов CRUD
func saveToDatabase(update map[string]any) error {

	db, err := postgresql.ConnectToBase()
	if err != nil {
		log.Fatalf("DB connect failed: %v", err)
	}
	defer db.Close()

	if err := postgresql.SaveToTable(db, update); err != nil {
		return err
	}

	return nil
}
