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

// OutgoingMessage — структура для сообщений, которые мы получаем из Kafka
// и затем отправляем в Telegram. Содержит chat_id и текст сообщения.
type OutgoingMessage struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
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
	// incomingTopic — для сообщений из Telegram
	// outgoingTopic — для сообщений, которые надо отправить в Telegram
	incomingTopic := "telegram-updates"
	outgoingTopic := "telegram-send"

	// Создаём экземпляр Telegram-бота
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Создаём Kafka writer — объект, через который будем отправлять сообщения в Kafka
	writer := kafka.NewWriter(kafka.WriterConfig{
		Brokers:  []string{kafkaAddr}, // Список брокеров Kafka
		Topic:    incomingTopic,       // Топик, в который будем писать
		Balancer: &kafka.LeastBytes{}, // Балансировщик — сообщения будут направляться на партицию с наименьшим объёмом данных
	})
	defer writer.Close() // Гарантируем закрытие writer при выходе из функции

	// Создаём Kafka reader — объект, через который будем читать сообщения из Kafka
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{kafkaAddr},  // Список брокеров Kafka
		Topic:   outgoingTopic,        // Топик, из которого будем читать
		GroupID: "telegram-bot-group", // Идентификатор группы — для балансировки нагрузки при нескольких экземплярах бота
	})
	defer reader.Close() // Закрываем reader при завершении

	// Создаём контекст для управления жизненным циклом потоков
	ctx := context.Background()

	// Запускаем первый поток: читает обновления из Telegram и отправляет их в Kafka
	go handleIncomingUpdates(bot, writer, ctx)

	// Запускаем второй поток: читает из Kafka и отправляет сообщения в Telegram
	go handleOutgoingMessages(bot, reader, ctx)

	// Блокируем главный поток, чтобы программа не завершалась
	select {}
}

// handleIncomingUpdates — функция, которая постоянно слушает входящие сообщения от Telegram,
// сериализует их и отправляет в Kafka
func handleIncomingUpdates(bot *tgbotapi.BotAPI, writer *kafka.Writer, ctx context.Context) {
	// Конфигурация получения обновлений от Telegram
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60 // Долгий запрос (long polling)

	// Получаем канал, по которому приходят обновления
	updates := bot.GetUpdatesChan(updateConfig)

	// Обрабатываем каждое обновление
	for update := range updates {
		// Нас интересуют только сообщения
		if update.Message != nil {
			// Создаём упрощённую структуру для отправки в Kafka
			msgData, err := json.Marshal(struct {
				ChatID int64  `json:"chat_id"`
				Text   string `json:"text"`
			}{
				ChatID: update.Message.Chat.ID,
				Text:   update.Message.Text,
			})
			if err != nil {
				log.Printf("Failed to marshal message: %v", err)
				continue // Пропускаем сообщение, если возникла ошибка сериализации
			}

			// Пытаемся отправить сообщение в Kafka
			err = writer.WriteMessages(ctx, kafka.Message{
				Key:   []byte(fmt.Sprint(update.Message.Chat.ID)), // Ключ сообщения — ID чата (необязательно, но может помочь для балансировки)
				Value: msgData,                                    // Само сообщение
			})
			if err != nil {
				log.Printf("Failed to write to Kafka: %v", err)
			} else {
				log.Printf("Sent message from chat %d to Kafka", update.Message.Chat.ID)
			}
		}
	}
}

// handleOutgoingMessages — функция, которая постоянно читает сообщения из Kafka
// и отправляет их в соответствующие чаты Telegram
func handleOutgoingMessages(bot *tgbotapi.BotAPI, reader *kafka.Reader, ctx context.Context) {
	for {
		// Читаем следующее сообщение из Kafka
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			log.Printf("Failed to read from Kafka: %v", err)
			time.Sleep(time.Second) // Если ошибка — небольшая пауза и пробуем снова
			continue
		}

		// Десериализуем сообщение в структуру OutgoingMessage
		var outMsg OutgoingMessage
		if err := json.Unmarshal(msg.Value, &outMsg); err != nil {
			log.Printf("Failed to parse outgoing message: %v", err)
			continue // Если данные некорректны — пропускаем
		}

		// Формируем и отправляем сообщение в Telegram
		tgMsg := tgbotapi.NewMessage(outMsg.ChatID, outMsg.Text)
		if _, err := bot.Send(tgMsg); err != nil {
			log.Printf("Failed to send message to chat %d: %v", outMsg.ChatID, err)
		} else {
			log.Printf("Sent message to chat %d", outMsg.ChatID)
		}
	}
}
