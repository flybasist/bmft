package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/logger"
	"github.com/flybasist/bmft/internal/logx"
	"github.com/flybasist/bmft/internal/telegram_bot"
	"go.uber.org/zap"
)

// Русский комментарий: main теперь отвечает за полный жизненный цикл приложения:
// 1. Загружает конфигурацию.
// 2. Инициализирует структурированный логгер.
// 3. Запускает подсистемы (telegram_bot, logger, core) с общим контекстом.
// 4. Корректно завершает работу при получении сигнала (graceful shutdown).
// Все runtime-логи строго на английском для операционной предсказуемости.

func main() {
	// Загружаем конфиг
	cfg, err := config.Load()
	if err != nil {
		// Поскольку логгер ещё не инициализирован — используем стандартный stderr.
		// Сообщение на английском.
		os.Stderr.WriteString("failed to load config: " + err.Error() + "\n")
		os.Exit(1)
	}

	// Инициализация структурированного логгера (pretty управляется переменной окружения)
	if err := logx.Init(cfg.LogPretty, "bmft"); err != nil {
		os.Stderr.WriteString("failed to init logger: " + err.Error() + "\n")
		os.Exit(1)
	}
	log := logx.L()
	log.Info("service starting")

	// Создаём корневой контекст отмены
	ctx, cancel := context.WithCancel(context.Background())

	// Канал сигналов ОС
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Русский комментарий: запускаем подсистемы в отдельных горутинах.
	// Они сами будут подписываться на ctx.Done() по мере рефакторинга (частично уже сделано в logger).
	go telegram_bot.Run(ctx, cfg)
	go logger.Run(ctx, cfg)
	go core.Run(ctx, cfg)

	// Ожидание сигнала
	sig := <-sigCh
	log.Info("shutdown signal received", zap.String("signal", sig.String()))

	// Инициация остановки
	cancel()

	// Ждём таймаут прежде чем форсировать выход
	timeout := time.NewTimer(cfg.ShutdownTimeout)
	<-timeout.C
	log.Info("service exited")
	logx.Sync()
}
