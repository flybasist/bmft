package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	// Импорт библиотеки для работы с Telegram Bot API
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	// Импорт библиотеки Kafka для работы с брокером сообщений
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

// Run — основная точка запуска бота.
// Здесь создаётся бот, настраиваются подключения к Kafka и запускаются два потока:
// один отправляет сообщения из Telegram в Kafka,
// второй читает из Kafka и отправляет сообщения в Telegram.
func Run() {
	// Считываем токен Telegram из переменных окружения
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set") // Если токена нет — аварийное завершение
	}

	// Адрес брокера Kafka
	kafkaAddr := os.Getenv("KAFKA_BROKERS")
	if kafkaAddr == "" {
		log.Fatal("KAFKA_BROKERS not set")
	}

	// Названия топиков в Kafka:
	// listenerTopic — для сообщений из Telegram
	// senderTopic — для сообщений, которые надо отправить в Telegram
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
	defer writer.Close() // Гарантируем закрытие writer при выходе из функции

	// Создаём Kafka reader — объект, через который будем читать сообщения из Kafka
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{kafkaAddr},     // Список брокеров Kafka
		Topic:   senderTopic,             // Топик, из которого будем читать
		GroupID: "telegram-sender-group", // Идентификатор группы — для балансировки нагрузки при нескольких экземплярах бота
	})
	defer reader.Close() // Закрываем reader при завершении

	deleter := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{kafkaAddr},      // Список брокеров Kafka
		Topic:   deleteTopic,              // Топик, из которого будем читать
		GroupID: "telegram-deleter-group", // Идентификатор группы — для балансировки нагрузки при нескольких экземплярах бота
	})
	defer deleter.Close() // Закрываем reader при завершении

	// Создаём контекст для управления жизненным циклом потоков
	ctx := context.Background()

	// Запускаем первый поток: читает обновления из Telegram и отправляет их в Kafka
	go listenerFromTelegram(bot, writer, ctx)

	// Запускаем второй поток: читает из Kafka и отправляет сообщения в Telegram
	go senderToTelegram(bot, reader, ctx)

	go deleteFromTelegram(bot, deleter, ctx)

	// Блокируем главный поток, чтобы программа не завершалась
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
			Key:   []byte(fmt.Sprint(update.UpdateID)), // Ключ можно взять по UpdateID
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
			time.Sleep(time.Second) // Если ошибка — небольшая пауза и пробуем снова
			continue
		}
		var delMsg DeleteMessage
		if err := json.Unmarshal(msg.Value, &delMsg); err != nil {
			log.Printf("Failed to parse outgoing message: %v", err)
			continue // Если данные некорректны — пропускаем
		}
		tgMsg := tgbotapi.NewDeleteMessage(delMsg.ChatID, delMsg.MessageID)
		if _, err := bot.Request(tgMsg); err != nil {
			log.Printf("Failed to send message to chat %d: %v", delMsg.ChatID, err)
		} else {
			log.Printf("Sent message to chat %d", delMsg.ChatID)
		}
	}
}
