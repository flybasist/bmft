package logx

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Русский комментарий: Этот пакет инкапсулирует настройку структурированного логирования.
// Вся операционная информация выводится только на английском, но комментарии в коде максимально подробны.
// Мы используем zap для высокой производительности и единообразия формата.

// NewLogger создаёт новый логгер с заданным уровнем и режимом.
// Русский комментарий: Удобная функция для создания нового логгера без глобального состояния.
// Используется в cmd/bot/main.go для инициализации логгера приложения.
func NewLogger(level string, pretty bool) (*zap.Logger, error) {
	var cfg zap.Config
	if pretty {
		cfg = zap.NewDevelopmentConfig()
		cfg.Development = false
	} else {
		cfg = zap.NewProductionConfig()
	}

	// Парсим уровень логирования
	zapLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		zapLevel = zapcore.InfoLevel // fallback to info
	}
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)

	cfg.EncoderConfig.TimeKey = "ts"
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// Пишем логи в stdout и в файл
	cfg.OutputPaths = []string{"stdout", "logs/bot.log"}
	cfg.ErrorOutputPaths = []string{"stderr", "logs/bot.log"}

	return cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
}
