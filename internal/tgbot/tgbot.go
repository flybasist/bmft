package tgbot

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/flybasist/bmft/internal/mbrabbit"
	"gopkg.in/telebot.v3"
)

func Botstart(idbotfromyaml string) {
	pref := telebot.Settings{
		Token:  idbotfromyaml,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		log.Fatalf("error starting bot: %v", err)
	}

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		msg := c.Message()

		raw, _ := json.MarshalIndent(msg, "", "  ")
		log.Printf("New Message: %s", string(raw))

		err := mbrabbit.Publish(raw)
		if err != nil {
			log.Printf("Error publish to RabbitMQ: %v", err)
		}

		return c.Send("Принято!")
	})

	log.Println("Bot started, waiting message...")
	fmt.Println("Bot started, waiting message...")
	bot.Start()
}
