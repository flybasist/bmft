package main

import (
	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/logger"
	"github.com/flybasist/bmft/internal/telegram_bot"
)

func main() {
	logger.InitServiceLogDuplication()

	go telegram_bot.Run()
	go logger.Run()
	go core.Run()

	select {}
}
