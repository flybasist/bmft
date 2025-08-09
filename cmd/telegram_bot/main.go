package main

import (
	"github.com/flybasist/bmft/internal/logger"
	"github.com/flybasist/bmft/internal/postgresql"
	"github.com/flybasist/bmft/internal/telegram_bot"
)

func main() {
	go telegram_bot.Run()
	go logger.Run()
	go postgresql.Run()

	select {}
}
