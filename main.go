package main

import (
	"log"

	"github.com/flybasist/bmft/internal/logger"
	"github.com/flybasist/bmft/internal/settings"
	"github.com/flybasist/bmft/internal/tgbot"
)

func main() {
	cfg, err := settings.LoadConfig()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	logger.InitLogger("bmft")

	tgbot.Botstart(cfg.Bot.ID)
}
