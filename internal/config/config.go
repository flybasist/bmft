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
	LogLevel         string        // Уровень логирования: debug|info|warn|error
	ShutdownTimeout  time.Duration // Таймаут graceful shutdown (общий)
	KafkaGroupCore   string        // Consumer group для core
	KafkaGroupSend   string        // Consumer group для отправки
	KafkaGroupDelete string        // Consumer group для удаления
	KafkaGroupLogger string        // Префикс consumer group для logger

	LogTopics           []string      // Список топиков Kafka для файлового логгера
	MetricsAddr         string        // Адрес HTTP сервера метрик /healthz /readyz
	DLQTopic            string        // Топик для безнадёжных сообщений (dead-letter queue)
	MaxProcessRetries   int           // Сколько попыток обработки прежде чем отправить в DLQ
	BatchInsertSize     int           // Размер батча для вставки в БД (>1 включает batching)
	BatchInsertInterval time.Duration // Максимальный интервал ожидания добора батча
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
	cfg.LogLevel = normalizeLevel(strings.TrimSpace(os.Getenv("LOG_LEVEL")))
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}

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

	// Топики для файлового логгера
	logTopicsRaw := strings.TrimSpace(os.Getenv("LOG_TOPICS"))
	if logTopicsRaw == "" {
		cfg.LogTopics = []string{"telegram-listener", "telegram-send", "telegram-delete"}
	} else {
		cfg.LogTopics = splitAndClean(logTopicsRaw)
	}

	// Метрики / health endpoints
	cfg.MetricsAddr = firstNonEmpty(os.Getenv("METRICS_ADDR"), ":9090")

	// DLQ настройки
	cfg.DLQTopic = firstNonEmpty(os.Getenv("DLQ_TOPIC"), "telegram-dlq")
	if v := strings.TrimSpace(os.Getenv("MAX_PROCESS_RETRIES")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil && iv >= 0 {
			cfg.MaxProcessRetries = iv
		}
	}
	if cfg.MaxProcessRetries == 0 { // по умолчанию 3
		cfg.MaxProcessRetries = 3
	}

	// Batching настроечные параметры
	if v := strings.TrimSpace(os.Getenv("BATCH_INSERT_SIZE")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil && iv > 1 {
			cfg.BatchInsertSize = iv
		}
	}
	if v := strings.TrimSpace(os.Getenv("BATCH_INSERT_INTERVAL")); v != "" {
		if dur, err := time.ParseDuration(v); err == nil {
			cfg.BatchInsertInterval = dur
		}
	}
	if cfg.BatchInsertSize > 1 && cfg.BatchInsertInterval == 0 { // разумный дефолт
		cfg.BatchInsertInterval = 2 * time.Second
	}

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

// normalizeLevel приводит уровень логирования к одному из допустимых значений.
func normalizeLevel(l string) string {
	switch strings.ToLower(l) {
	case "debug", "info", "warn", "error":
		return strings.ToLower(l)
	default:
		return ""
	}
}

// splitAndClean разбивает строку по запятым/пробелам и удаляет пустые элементы.
func splitAndClean(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
	res := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		res = append(res, p)
	}
	return res
}
