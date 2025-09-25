package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config — централизованная структура настроек сервиса.
// Русский комментарий: Все переменные окружения собираются один раз при старте.
// Это упрощает тестирование и делает код чище — далее мы работаем только с этой структурой.
// Логирование всегда на английском для единообразия операционных сообщений.

type Config struct {
	TelegramBotToken string        // Токен Telegram бота
	KafkaBrokers     []string      // Список адресов Kafka брокеров
	PostgresDSN      string        // Строка подключения к PostgreSQL
	LogPretty        bool          // Флаг человекочитаемого (pretty) логирования файлов
	ShutdownTimeout  time.Duration // Таймаут graceful shutdown (общий)
	KafkaGroupCore   string        // Consumer group для core
	KafkaGroupSend   string        // Consumer group для отправки
	KafkaGroupDelete string        // Consumer group для удаления
	KafkaGroupLogger string        // Префикс consumer group для logger
}

// Load загружает и валидирует конфигурацию из окружения.
func Load() (*Config, error) {
	cfg := &Config{}

	cfg.TelegramBotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	cfg.PostgresDSN = strings.TrimSpace(os.Getenv("POSTGRES_DSN"))
	brokersRaw := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))

	if brokersRaw != "" {
		// Разрешаем перечисление через запятую или пробелы
		brokers := strings.FieldsFunc(brokersRaw, func(r rune) bool { return r == ',' || r == ' ' })
		cfg.KafkaBrokers = brokers
	}

	cfg.LogPretty = strings.ToLower(os.Getenv("LOGGER_PRETTY")) == "true"

	shutdownTimeoutStr := strings.TrimSpace(os.Getenv("SHUTDOWN_TIMEOUT"))
	if shutdownTimeoutStr == "" {
		cfg.ShutdownTimeout = 15 * time.Second
	} else {
		dur, err := time.ParseDuration(shutdownTimeoutStr)
		if err != nil {
			return nil, fmt.Errorf("invalid SHUTDOWN_TIMEOUT: %w", err)
		}
		cfg.ShutdownTimeout = dur
	}

	// Имена consumer groups — можно переопределять, чтобы запускать несколько инстансов.
	cfg.KafkaGroupCore = firstNonEmpty(os.Getenv("KAFKA_GROUP_CORE"), "bmft-core")
	cfg.KafkaGroupSend = firstNonEmpty(os.Getenv("KAFKA_GROUP_SEND"), "bmft-telegram-sender")
	cfg.KafkaGroupDelete = firstNonEmpty(os.Getenv("KAFKA_GROUP_DELETE"), "bmft-telegram-deleter")
	cfg.KafkaGroupLogger = firstNonEmpty(os.Getenv("KAFKA_GROUP_LOGGER"), "bmft-logger")

	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) validate() error {
	var missing []string
	if c.TelegramBotToken == "" {
		missing = append(missing, "TELEGRAM_BOT_TOKEN")
	}
	if len(c.KafkaBrokers) == 0 {
		missing = append(missing, "KAFKA_BROKERS")
	}
	if c.PostgresDSN == "" {
		missing = append(missing, "POSTGRES_DSN")
	}
	if len(missing) > 0 {
		return errors.New("missing required env vars: " + strings.Join(missing, ", "))
	}
	return nil
}

// Helper: возвращает первое непустое значение.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// OptionalBool читает переменную окружения и пытается интерпретировать её как bool.
// Возвращает значение и признак было ли оно установлено.
func OptionalBool(name string) (bool, bool) {
	val := strings.TrimSpace(os.Getenv(name))
	if val == "" {
		return false, false
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, false
	}
	return b, true
}
