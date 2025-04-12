package main

import (
	"fmt"
	"log"

	"github.com/flybasist/bmft/internal/logger"
	"github.com/flybasist/bmft/internal/settings"
)

func main() {
	cfg, err := settings.LoadConfig()
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	logger.InitLogger("bot")
	log.Printf("ID бота из YAML: %s", cfg.Bot.ID)

	fmt.Println(cfg.Bot.ID)
}
