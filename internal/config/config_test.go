package config

import (
	"os"
	"testing"
	"time"
)

// TestLoadConfig проверяет загрузку конфигурации из env
func TestLoadConfig(t *testing.T) {
	// Устанавливаем тестовые переменные окружения
	os.Setenv("TELEGRAM_BOT_TOKEN", "test_token_12345")
	os.Setenv("POSTGRES_DSN", "postgres://test:test@localhost/test")
	os.Setenv("LOG_LEVEL", "debug")
	os.Setenv("LOGGER_PRETTY", "true")
	os.Setenv("SHUTDOWN_TIMEOUT", "45s")
	os.Setenv("METRICS_ADDR", ":9090")
	os.Setenv("POLLING_TIMEOUT", "30")
	defer func() {
		// Очищаем после теста
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("POSTGRES_DSN")
		os.Unsetenv("LOG_LEVEL")
		os.Unsetenv("LOGGER_PRETTY")
		os.Unsetenv("SHUTDOWN_TIMEOUT")
		os.Unsetenv("METRICS_ADDR")
		os.Unsetenv("POLLING_TIMEOUT")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Проверяем все поля
	if cfg.TelegramBotToken != "test_token_12345" {
		t.Errorf("Expected TelegramBotToken='test_token_12345', got '%s'", cfg.TelegramBotToken)
	}
	if cfg.PostgresDSN != "postgres://test:test@localhost/test" {
		t.Errorf("Expected PostgresDSN='postgres://test:test@localhost/test', got '%s'", cfg.PostgresDSN)
	}
	if cfg.LogLevel != "debug" {
		t.Errorf("Expected LogLevel='debug', got '%s'", cfg.LogLevel)
	}
	if !cfg.LogPretty {
		t.Error("Expected LogPretty=true, got false")
	}
	if cfg.ShutdownTimeout != 45*time.Second {
		t.Errorf("Expected ShutdownTimeout=45s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.MetricsAddr != ":9090" {
		t.Errorf("Expected MetricsAddr=':9090', got '%s'", cfg.MetricsAddr)
	}
	if cfg.PollingTimeout != 30 {
		t.Errorf("Expected PollingTimeout=30, got %d", cfg.PollingTimeout)
	}
}

// TestLoadConfigDefaults проверяет дефолтные значения
func TestLoadConfigDefaults(t *testing.T) {
	// Устанавливаем только обязательные поля
	os.Setenv("TELEGRAM_BOT_TOKEN", "test_token")
	os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
	defer func() {
		os.Unsetenv("TELEGRAM_BOT_TOKEN")
		os.Unsetenv("POSTGRES_DSN")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Проверяем дефолтные значения
	if cfg.LogLevel != "info" {
		t.Errorf("Expected default LogLevel='info', got '%s'", cfg.LogLevel)
	}
	if cfg.LogPretty != false {
		t.Error("Expected default LogPretty=false, got true")
	}
	if cfg.ShutdownTimeout != 15*time.Second {
		t.Errorf("Expected default ShutdownTimeout=15s, got %v", cfg.ShutdownTimeout)
	}
	if cfg.MetricsAddr != ":9090" {
		t.Errorf("Expected default MetricsAddr=':9090', got '%s'", cfg.MetricsAddr)
	}
	if cfg.PollingTimeout != 60 {
		t.Errorf("Expected default PollingTimeout=60, got %d", cfg.PollingTimeout)
	}
}

// TestValidateConfig проверяет валидацию конфигурации
func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name          string
		token         string
		dsn           string
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid config",
			token:       "valid_token",
			dsn:         "postgres://localhost/test",
			expectError: false,
		},
		{
			name:          "Missing token",
			token:         "",
			dsn:           "postgres://localhost/test",
			expectError:   true,
			errorContains: "TELEGRAM_BOT_TOKEN",
		},
		{
			name:          "Missing DSN",
			token:         "valid_token",
			dsn:           "",
			expectError:   true,
			errorContains: "POSTGRES_DSN",
		},
		{
			name:          "Both missing",
			token:         "",
			dsn:           "",
			expectError:   true,
			errorContains: "TELEGRAM_BOT_TOKEN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Устанавливаем переменные окружения
			if tt.token != "" {
				os.Setenv("TELEGRAM_BOT_TOKEN", tt.token)
			} else {
				os.Unsetenv("TELEGRAM_BOT_TOKEN")
			}
			if tt.dsn != "" {
				os.Setenv("POSTGRES_DSN", tt.dsn)
			} else {
				os.Unsetenv("POSTGRES_DSN")
			}

			cfg, err := Load()

			if tt.expectError {
				if err == nil {
					t.Error("Expected error, got nil")
				} else if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got '%v'", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
				if cfg == nil {
					t.Error("Expected non-nil config, got nil")
				}
			}

			// Очистка
			os.Unsetenv("TELEGRAM_BOT_TOKEN")
			os.Unsetenv("POSTGRES_DSN")
		})
	}
}

// TestPollingTimeoutParsing проверяет парсинг POLLING_TIMEOUT
func TestPollingTimeoutParsing(t *testing.T) {
	tests := []struct {
		value    string
		expected int
	}{
		{"30", 30},
		{"120", 120},
		{"invalid", 60}, // должен вернуться к дефолту
		{"", 60},        // пустое значение = дефолт
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv("TELEGRAM_BOT_TOKEN", "test")
			os.Setenv("POSTGRES_DSN", "postgres://localhost/test")
			if tt.value != "" {
				os.Setenv("POLLING_TIMEOUT", tt.value)
			} else {
				os.Unsetenv("POLLING_TIMEOUT")
			}

			cfg, err := Load()
			if err != nil {
				t.Fatalf("Load() failed: %v", err)
			}

			if cfg.PollingTimeout != tt.expected {
				t.Errorf("Expected PollingTimeout=%d, got %d", tt.expected, cfg.PollingTimeout)
			}

			os.Unsetenv("TELEGRAM_BOT_TOKEN")
			os.Unsetenv("POSTGRES_DSN")
			os.Unsetenv("POLLING_TIMEOUT")
		})
	}
}

// contains проверяет, содержит ли строка подстроку
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
