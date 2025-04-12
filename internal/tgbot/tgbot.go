package tgbot

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gopkg.in/telebot.v3"
)

func Botstart(idbotfromyaml string) {
	log.Printf("ID бота из YAML: %s", idbotfromyaml)

	fmt.Println(idbotfromyaml)

	pref := telebot.Settings{
		Token:  idbotfromyaml,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}

	bot, err := telebot.NewBot(pref)
	if err != nil {
		log.Fatalf("Ошибка запуска бота: %v", err)
	}

	bot.Handle(telebot.OnText, func(c telebot.Context) error {
		msg := c.Message()

		raw, _ := json.MarshalIndent(msg, "", "  ")
		log.Printf("Новое сообщение: %s", string(raw))

		return c.Send("Принято!")
	})

	log.Println("Бот запущен, ожидание сообщений...")
	bot.Start()
}
