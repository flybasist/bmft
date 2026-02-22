// Пакет postgresql содержит утилиты для работы с PostgreSQL (подключение, ретраи).
package postgresql

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

// PingWithRetry пингует базу с ретраями.
// Используется при старте бота для гарантии что PostgreSQL доступен.
func PingWithRetry(db *sql.DB, maxRetries int, delay time.Duration, logger *zap.Logger) error {
	for i := 0; i < maxRetries; i++ {
		err := db.Ping()
		if err == nil {
			logger.Info("postgres connection established")
			return nil
		}

		logger.Warn("failed to ping postgres, retrying...",
			zap.Int("attempt", i+1),
			zap.Int("max_retries", maxRetries),
			zap.Error(err))

		if i < maxRetries-1 {
			time.Sleep(delay)
		}
	}

	return fmt.Errorf("failed to ping postgres after %d retries", maxRetries)
}
