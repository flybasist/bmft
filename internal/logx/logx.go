package logx

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Русский комментарий: Этот пакет инкапсулирует настройку структурированного логирования.
// Вся операционная информация выводится только на английском, но комментарии в коде максимально подробны.
// Мы используем zap для высокой производительности и единообразия формата.
// lumberjack обеспечивает автоматическую ротацию файлов логов.

// LogRotationConfig содержит параметры ротации логов.
type LogRotationConfig struct {
	MaxSizeMB  int // максимальный размер файла лога в MB
	MaxBackups int // количество старых файлов для хранения
	MaxAgeDays int // максимальный возраст файла лога в днях
}

// NewLogger создаёт новый логгер с заданным уровнем и режимом.
// Русский комментарий: Удобная функция для создания нового логгера без глобального состояния.
// Используется в cmd/bot/main.go для инициализации логгера приложения.
// Использует lumberjack для автоматической ротации файлов логов.
func NewLogger(level string, pretty bool, rotationCfg LogRotationConfig) (*zap.Logger, error) {
	// Парсим уровень логирования
	zapLevel, err := zapcore.ParseLevel(level)
	if err != nil {
		zapLevel = zapcore.InfoLevel // fallback to info
	}

	// Настраиваем encoder
	var encoderCfg zapcore.EncoderConfig
	if pretty {
		encoderCfg = zap.NewDevelopmentEncoderConfig()
	} else {
		encoderCfg = zap.NewProductionEncoderConfig()
	}
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	// Создаём encoder
	var encoder zapcore.Encoder
	if pretty {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	// Настраиваем ротацию логов через lumberjack
	logFile := &lumberjack.Logger{
		Filename:   "logs/bot.log",
		MaxSize:    rotationCfg.MaxSizeMB,
		MaxBackups: rotationCfg.MaxBackups,
		MaxAge:     rotationCfg.MaxAgeDays,
		Compress:   true, // сжимаем старые файлы
	}

	// Создаём multi-writer: stdout + файл с ротацией
	fileWriter := zapcore.AddSync(logFile)
	consoleWriter := zapcore.AddSync(os.Stdout)

	core := zapcore.NewTee(
		zapcore.NewCore(encoder, consoleWriter, zapLevel),
		zapcore.NewCore(encoder, fileWriter, zapLevel),
	)

	return zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)), nil
}
