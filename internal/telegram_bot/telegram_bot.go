package telegram_bot

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/flybasist/bmft/internal/kafkabot"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/segmentio/kafka-go"
)

// SendMessage — структура для сообщений, которые мы получаем из Kafka
// и затем отправляем в Telegram.
type SendMessage struct {
	ChatID  int64  `json:"chat_id"`
	Text    string `json:"text,omitempty"`
	Sticker string `json:"sticker,omitempty"`
	TypeMsg string `json:"type_msg"`
}

// DeleteMessage — структура для удаления сообщений в Telegram
type DeleteMessage struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}

func Run() {
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	if botToken == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	// Создаём экземпляр Telegram-бота
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}
	log.Printf("Authorized on account %s", bot.Self.UserName)

	// Создаём dedicated reader'ы для отправки и удаления сообщений.
	// Это гарантирует, что telegram_bot читает все сообщения telegram-send и telegram-delete.
	senderReader := kafkabot.NewReader("telegram-send", "bmft-telegram-sender")
	deleterReader := kafkabot.NewReader("telegram-delete", "bmft-telegram-deleter")

	// Закрываем их при завершении функции (функция блокируется select{}, поэтому выполнятся при exit).
	defer senderReader.Close()
	defer deleterReader.Close()
	// Также закрываем глобальные ресурсы Kafka при завершении (не ломает текущую логику).
	defer kafkabot.CloseKafka()

	go listenerFromTelegram(bot)              // пишет в kafka topic telegram-listener
	go senderToTelegram(bot, senderReader)    // читает telegram-send и отправляет в Telegram
	go deleteFromTelegram(bot, deleterReader) // читает telegram-delete и удаляет сообщения

	select {} // блокируемся навсегда
}

// listenerFromTelegram — слушает входящие сообщения от Telegram и отправляет в Kafka
func listenerFromTelegram(bot *tgbotapi.BotAPI) {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 60

	updates := bot.GetUpdatesChan(updateConfig)

	for update := range updates {
		msgData, err := json.Marshal(update)
		if err != nil {
			log.Printf("Failed to marshal full update: %v", err)
			continue
		}
		// Используем update.UpdateID как key для Kafka
		kafkabot.WriteKafka(fmt.Sprint(update.UpdateID), msgData)
	}
}

// senderToTelegram — читает сообщения из Kafka и отправляет в Telegram
func senderToTelegram(bot *tgbotapi.BotAPI, reader *kafka.Reader) {
	for {
		msg, err := reader.ReadMessage(kafkabot.Ctx)
		if err != nil {
			log.Printf("telegram_bot: Failed to read from Kafka (send): %v", err)
			time.Sleep(time.Second)
			continue
		}

		// Диагностический лог — показывает, что бот прочитал сообщение из Kafka
		log.Printf("telegram_bot: read send msg key=%s partition=%d offset=%d len=%d",
			string(msg.Key), msg.Partition, msg.Offset, len(msg.Value))
		log.Printf("telegram_bot: raw send payload: %s", truncate(msg.Value, 400))

		var outMsg SendMessage
		if err := json.Unmarshal(msg.Value, &outMsg); err != nil {
			log.Printf("telegram_bot: Failed to parse outgoing message: %v — raw: %s", err, truncate(msg.Value, 400))
			continue
		}

		log.Printf("telegram_bot: parsed send message: %+v", outMsg)

		switch outMsg.TypeMsg {
		case "text":
			tgMsg := tgbotapi.NewMessage(outMsg.ChatID, outMsg.Text)
			if _, err := bot.Send(tgMsg); err != nil {
				log.Printf("telegram_bot: Failed to send text to chat %d: %v", outMsg.ChatID, err)
			} else {
				log.Printf("telegram_bot: Sent text to chat %d", outMsg.ChatID)
			}
		case "sticker":
			tgSticker := tgbotapi.NewSticker(outMsg.ChatID, tgbotapi.FileID(outMsg.Sticker))
			if _, err := bot.Send(tgSticker); err != nil {
				log.Printf("telegram_bot: Failed to send sticker to chat %d: %v", outMsg.ChatID, err)
			} else {
				log.Printf("telegram_bot: Sent sticker to chat %d", outMsg.ChatID)
			}
		default:
			log.Printf("telegram_bot: unknown TypeMsg: %s — raw: %s", outMsg.TypeMsg, truncate(msg.Value, 200))
		}
	}
}

// deleteFromTelegram — читает команды удаления из Kafka и удаляет сообщения в Telegram
func deleteFromTelegram(bot *tgbotapi.BotAPI, reader *kafka.Reader) {
	for {
		msg, err := reader.ReadMessage(kafkabot.Ctx)
		if err != nil {
			log.Printf("telegram_bot: Failed to read from Kafka (delete): %v", err)
			time.Sleep(time.Second)
			continue
		}

		// Диагностический лог
		log.Printf("telegram_bot: read delete msg key=%s partition=%d offset=%d len=%d",
			string(msg.Key), msg.Partition, msg.Offset, len(msg.Value))
		log.Printf("telegram_bot: raw delete payload: %s", truncate(msg.Value, 400))

		var delMsg DeleteMessage
		if err := json.Unmarshal(msg.Value, &delMsg); err != nil {
			log.Printf("telegram_bot: Failed to parse delete message: %v — raw: %s", err, truncate(msg.Value, 400))
			continue
		}

		log.Printf("telegram_bot: parsed delete message: %+v", delMsg)

		tgMsg := tgbotapi.NewDeleteMessage(delMsg.ChatID, delMsg.MessageID)
		if _, err := bot.Request(tgMsg); err != nil {
			log.Printf("telegram_bot: Failed to delete message in chat %d: %v", delMsg.ChatID, err)
		} else {
			log.Printf("telegram_bot: Deleted message %d in chat %d", delMsg.MessageID, delMsg.ChatID)
		}
	}
}
