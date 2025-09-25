package main

import (
	"context"
	"net/http"
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
	"go.uber.org/zap/zapcore"
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
	setLogLevel(cfg.LogLevel)
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
	go startMetricsServer(ctx, cfg)

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

// setLogLevel — конфигурация уровня логирования (debug|info|warn|error).
// Русский комментарий: базовый zap logger из logx не поддерживает горячее изменение уровня без AtomicLevel,
// здесь просто информируем об уровне; для продвинутой динамики можно доработать logx позже.
func setLogLevel(level string) {
	l := logx.L()
	var zl zapcore.Level
	if err := zl.UnmarshalText([]byte(level)); err != nil {
		zl = zapcore.InfoLevel
	}
	l.Info("log level configured", zap.String("requested", level), zap.String("effective", zl.String()))
}

// startMetricsServer — простой HTTP сервер для health/ready/metrics (заглушка для будущего Prometheus).
func startMetricsServer(ctx context.Context, cfg *config.Config) {
	log := logx.L().Named("metrics")
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ok")) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("ready")) })
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) { w.Write([]byte("# metrics placeholder\n")) })
	srv := &http.Server{Addr: cfg.MetricsAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctxShutdown)
	}()
	log.Info("metrics server starting", zap.String("addr", cfg.MetricsAddr))
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Error("metrics server failed", zap.Error(err))
	}
}
