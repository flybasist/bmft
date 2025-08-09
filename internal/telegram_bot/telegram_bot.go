package telegram_bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/segmentio/kafka-go"
)

// SendMessage — структура для сообщений, которые мы получаем из Kafka
// и затем отправляем в Telegram. Содержит chat_id и текст сообщения.
type SendMessage struct {
	ChatID  int64  `json:"chat_id"`
	Text    string `json:"text,omitempty"`
	Sticker string `json:"sticker,omitempty"`
	TypeMsg string `json:"type_msg"`
}

type DeleteMessage struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}

func Run() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	kafkaAddr := os.Getenv("KAFKA_BROKERS")
	if kafkaAddr == "" {
		log.Fatal("KAFKA_BROKERS not set")
	}

	// listenerTopic — для сообщений из Telegram
	// senderTopic — для сообщений, которые надо отправить в Telegram
	// deleteTopic - для сообщений, которые мы удаляем в telegram
	listenerTopic := "telegram-listener"
	senderTopic := "telegram-send"
	deleteTopic := "telegram-delete"

	// Создаём экземпляр Telegram-бота
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Создаём Kafka writer — объект, через который будем отправлять сообщения в Kafka
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaAddr}, // Список брокеров Kafka
		Topic:    listenerTopic,       // Топик, в который будем писать
		Balancer: &kafka.LeastBytes{}, // Балансировщик — сообщения будут направляться на партицию с наименьшим объёмом данных
	})
	defer writer.Close()

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{kafkaAddr},
		Topic:   senderTopic,
		GroupID: "telegram-sender-group",
	})
	defer reader.Close()

	deleter := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{kafkaAddr},
		Topic:   deleteTopic,
		GroupID: "telegram-deleter-group",
	})
	defer deleter.Close()

	ctx := context.Background()

	go listenerFromTelegram(bot, writer, ctx)
	go senderToTelegram(bot, reader, ctx)
	go deleteFromTelegram(bot, deleter, ctx)

	select {}
}

// listenerFromTelegram — функция, которая постоянно слушает входящие сообщения от Telegram,
// и отправляет в Kafka
func listenerFromTelegram(bot *tgbotapi.BotAPI, writer *kafka.Writer, ctx context.Context) {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	updates := bot.GetUpdatesChan(updateConfig)

	for update := range updates {
		// Сохраняем весь update, даже если это не сообщение
		msgData, err := json.Marshal(update)
		if err != nil {
			log.Printf("Failed to marshal full update: %v", err)
			continue
		}

		err = writer.WriteMessages(ctx, kafka.Message{
			Key:   []byte(fmt.Sprint(update.UpdateID)),
			Value: msgData,
		})
		if err != nil {
			log.Printf("Failed to write to Kafka: %v", err)
		} else {
			log.Printf("Saved full update ID %d to Kafka", update.UpdateID)
		}
	}
}

// senderToTelegram — функция, которая постоянно читает сообщения из Kafka
// и отправляет их в соответствующие чаты Telegram
func senderToTelegram(bot *tgbotapi.BotAPI, reader *kafka.Reader, ctx context.Context) {
	for {
		// Читаем следующее сообщение из Kafka
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Failed to read from Kafka: %v", err)
			time.Sleep(time.Second) // Если ошибка — небольшая пауза и пробуем снова
			continue
		}

		// Десериализуем сообщение в структуру SendMessage
		var outMsg SendMessage
		if err := json.Unmarshal(msg.Value, &outMsg); err != nil {
			log.Printf("Failed to parse outgoing message: %v", err)
			continue // Если данные некорректны — пропускаем
		}

		// Формируем и отправляем сообщение в Telegram
		switch outMsg.TypeMsg {
		case "text":
			tgMsg := tgbotapi.NewMessage(outMsg.ChatID, outMsg.Text)
			if _, err := bot.Send(tgMsg); err != nil {
				log.Printf("Failed to send message to chat %d: %v", outMsg.ChatID, err)
			} else {
				log.Printf("Sent message to chat %d", outMsg.ChatID)
			}
		case "sticker":
			tgSticker := tgbotapi.NewSticker(outMsg.ChatID, tgbotapi.FileID(outMsg.Sticker))
			if _, err := bot.Send(tgSticker); err != nil {
				log.Printf("Failed to send message to chat %d: %v", outMsg.ChatID, err)
			} else {
				log.Printf("Sent message to chat %d", outMsg.ChatID)
			}
		}
	}
}

func deleteFromTelegram(bot *tgbotapi.BotAPI, deleter *kafka.Reader, ctx context.Context) {
	for {
		msg, err := deleter.ReadMessage(ctx)
		if err != nil {
			log.Printf("Failed to read from Kafka: %v", err)
			time.Sleep(time.Second)
			continue
		}
		var delMsg DeleteMessage
		if err := json.Unmarshal(msg.Value, &delMsg); err != nil {
			log.Printf("Failed to parse outgoing message: %v", err)
			continue
		}
		tgMsg := tgbotapi.NewDeleteMessage(delMsg.ChatID, delMsg.MessageID)
		if _, err := bot.Request(tgMsg); err != nil {
			log.Printf("Failed to send message to chat %d: %v", delMsg.ChatID, err)
		} else {
			log.Printf("Sent message to chat %d", delMsg.ChatID)
		}
	}
}
