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
	PostgresDSN      string        // Строка подключения к PostgreSQL
	LogPretty        bool          // Флаг человекочитаемого (pretty) логирования
	LogLevel         string        // Уровень логирования: debug|info|warn|error
	ShutdownTimeout  time.Duration // Таймаут graceful shutdown
	MetricsAddr      string        // Адрес HTTP сервера метрик /healthz /metrics
	PollingTimeout   int           // Таймаут Long Polling в секундах (default: 60)
}

// Load загружает и валидирует конфигурацию из окружения.
func Load() (*Config, error) {
	cfg := &Config{}

	cfg.TelegramBotToken = strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN"))
	cfg.PostgresDSN = strings.TrimSpace(os.Getenv("POSTGRES_DSN"))

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

	// Метрики / health endpoints
	cfg.MetricsAddr = firstNonEmpty(os.Getenv("METRICS_ADDR"), ":9090")

	// Polling timeout для Long Polling
	if v := strings.TrimSpace(os.Getenv("POLLING_TIMEOUT")); v != "" {
		if iv, err := strconv.Atoi(v); err == nil && iv > 0 {
			cfg.PollingTimeout = iv
		}
	}
	if cfg.PollingTimeout == 0 {
		cfg.PollingTimeout = 60 // default 60 секунд
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
