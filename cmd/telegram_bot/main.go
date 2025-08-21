package main

import (
	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/logger"
	"github.com/flybasist/bmft/internal/postgresql"
	"github.com/flybasist/bmft/internal/telegram_bot"
)

func main() {
	logger.InitServiceLogDuplication()
	db := postgresql.CreateTables()
	defer db.Close() // Закрыть пул соединений при завершении

	go telegram_bot.Run() // передавай дальше в сервисы
	go logger.Run()
	go core.Run()

	select {}
}
