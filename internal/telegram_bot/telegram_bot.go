package telegram_bot

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/flybasist/bmft/internal/kafkabot"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

	defer kafkabot.CloseKafka() // закрываем Kafka при завершении

	go listenerFromTelegram(bot)
	go senderToTelegram(bot)
	go deleteFromTelegram(bot)

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
func senderToTelegram(bot *tgbotapi.BotAPI) {
	for {
		msg, err := kafkabot.Reader.ReadMessage(kafkabot.Ctx)
		if err != nil {
			log.Printf("Failed to read from Kafka: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var outMsg SendMessage
		if err := json.Unmarshal(msg.Value, &outMsg); err != nil {
			log.Printf("Failed to parse outgoing message: %v", err)
			continue
		}

		switch outMsg.TypeMsg {
		case "text":
			tgMsg := tgbotapi.NewMessage(outMsg.ChatID, outMsg.Text)
			if _, err := bot.Send(tgMsg); err != nil {
				log.Printf("Failed to send text to chat %d: %v", outMsg.ChatID, err)
			}
		case "sticker":
			tgSticker := tgbotapi.NewSticker(outMsg.ChatID, tgbotapi.FileID(outMsg.Sticker))
			if _, err := bot.Send(tgSticker); err != nil {
				log.Printf("Failed to send sticker to chat %d: %v", outMsg.ChatID, err)
			}
		}
	}
}

// deleteFromTelegram — читает команды удаления из Kafka и удаляет сообщения в Telegram
func deleteFromTelegram(bot *tgbotapi.BotAPI) {
	for {
		msg, err := kafkabot.Deleter.ReadMessage(kafkabot.Ctx)
		if err != nil {
			log.Printf("Failed to read from Kafka: %v", err)
			time.Sleep(time.Second)
			continue
		}

		var delMsg DeleteMessage
		if err := json.Unmarshal(msg.Value, &delMsg); err != nil {
			log.Printf("Failed to parse delete message: %v", err)
			continue
		}

		tgMsg := tgbotapi.NewDeleteMessage(delMsg.ChatID, delMsg.MessageID)
		if _, err := bot.Request(tgMsg); err != nil {
			log.Printf("Failed to delete message in chat %d: %v", delMsg.ChatID, err)
		}
	}
}
