package logx

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Русский комментарий: Этот пакет инкапсулирует настройку структурированного логирования.
// Вся операционная информация выводится только на английском, но комментарии в коде максимально подробны.
// Мы используем zap для высокой производительности и единообразия формата.

var (
	logger     *zap.Logger
	loggerOnce sync.Once
)

// Init инициализирует глобальный логгер.
// pretty=true включает человекочитаемый (console) вывод, иначе JSON для агрегаторов логов.
func Init(pretty bool, service string) error {
	var initErr error
	loggerOnce.Do(func() {
		cfg := zap.NewProductionConfig()
		if pretty {
			cfg = zap.NewDevelopmentConfig()
			// Уберём лишний stacktrace по умолчанию
			cfg.Development = false
		}
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stdout"}

		l, err := cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
		if err != nil {
			initErr = fmt.Errorf("failed to build logger: %w", err)
			return
		}
		logger = l.With(zap.String("service", service))
		logger.Info("logger initialized")
	})
	return initErr
}

// L возвращает активный логгер.
func L() *zap.Logger { return logger }

// Sync безопасно синхронизирует буферы.
func Sync() { _ = logger.Sync() }
