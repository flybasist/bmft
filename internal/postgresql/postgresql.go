package postgresql

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// Русский комментарий: этот пакет инкапсулирует работу с PostgreSQL. Добавлен контекст для отмены и поле raw_update.

// PingWithRetry пингует базу с ретраями.
// Русский комментарий: Полезная функция для проверки подключения с повторными попытками.
// Используется при старте бота для гарантии что PostgreSQL доступен.
func PingWithRetry(db *sql.DB, maxRetries int, delay time.Duration, logger interface{}) error {
	type zapLogger interface {
		Info(msg string, fields ...interface{})
		Warn(msg string, fields ...interface{})
	}

	var log zapLogger
	if logger != nil {
		if zl, ok := logger.(zapLogger); ok {
			log = zl
		}
	}

	for i := 0; i < maxRetries; i++ {
		err := db.Ping()
		if err == nil {
			if log != nil {
				log.Info("postgres connection established")
			}
			return nil
		}

		if log != nil {
			log.Warn("failed to ping postgres, retrying...")
		}

		if i < maxRetries-1 {
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("failed to ping postgres after %d retries", maxRetries)
}
