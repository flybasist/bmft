package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/kafkabot"
	"github.com/flybasist/bmft/internal/logx"
	"github.com/flybasist/bmft/internal/postgresql"
	"github.com/flybasist/bmft/internal/utils"
	"github.com/segmentio/kafka-go"
	"go.uber.org/zap"
)

// Русский комментарий: Пакет core отвечает за приём исходящих обновлений из Kafka (topic telegram-listener),
// применение базовой бизнес-логики и сохранение результата в PostgreSQL.
// Теперь он:
// 1. Работает под управлением контекста.
// 2. Делает явный commit offset только после успешной обработки.
// 3. Логирует структурированно на английском.

// Run запускает основной цикл потребления для core.
func Run(ctx context.Context, cfg *config.Config) {
	log := logx.L().Named("core")
	db, err := postgresql.ConnectToBase(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatal("DB connect failed", zap.Error(err))
	}
	defer db.Close()

	// Русский комментарий: для упрощения dev-среды оставляем создание таблиц.
	// В продакшене рекомендуется использовать миграции отдельно.
	if err := postgresql.CreateTables(ctx, db); err != nil {
		log.Fatal("failed to create tables", zap.Error(err))
	}

	consume(ctx, db, cfg)
}

// consume читает сообщения из Kafka и обрабатывает их.
func consume(ctx context.Context, db *sql.DB, cfg *config.Config) {
	log := logx.L().Named("core.consumer")
	reader := kafkabot.NewReader("telegram-listener", cfg.KafkaBrokers, cfg.KafkaGroupCore)
	defer reader.Close()
	// Writer для DLQ (ленивая инициализация при первом использовании)
	var dlqWriterInit bool
	var dlqWriter = (*kafkaWriterWrapper)(nil)
	log.Info("Kafka reader started", zap.String("topic", "telegram-listener"), zap.String("group", cfg.KafkaGroupCore))

	for {
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				log.Info("context canceled, stopping consumer")
				return
			}
			log.Warn("failed to fetch message", zap.Error(err))
			time.Sleep(time.Second)
			continue
		}

		start := time.Now()
		// Извлекаем счётчик попыток из заголовков (если есть)
		attempt := 0
		for _, h := range msg.Headers {
			if h.Key == "x-attempt" {
				if v, err := strconv.Atoi(string(h.Value)); err == nil {
					attempt = v
				}
			}
		}

		if err := handleMessage(ctx, db, msg.Value); err != nil {
			attempt++
			if attempt > cfg.MaxProcessRetries {
				// Отправляем в DLQ
				if !dlqWriterInit {
					dlqWriter = newKafkaWriterWrapper(cfg.DLQTopic, cfg.KafkaBrokers)
					dlqWriterInit = true
				}
				if dlqWriter != nil && dlqWriter.write(ctx, msg.Key, msg.Value, attempt, err) == nil {
					log.Error("moved to DLQ", zap.Int64("offset", msg.Offset), zap.Int("attempt", attempt))
					// Коммитим оригинал, чтобы не зацикливаться
					_ = reader.CommitMessages(ctx, msg)
				} else {
					log.Error("failed to write to DLQ", zap.Error(err))
				}
			} else {
				log.Warn("processing failed, will retry", zap.Error(err), zap.Int("attempt", attempt))
				// Не коммитим — сообщение будет прочитано снова (крайне простой retry без backoff per message)
				time.Sleep(200 * time.Millisecond)
			}
			continue
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Error("commit failed", zap.Error(err), zap.Int64("offset", msg.Offset))
			continue
		}
		log.Info("message processed", zap.Int64("offset", msg.Offset), zap.Duration("latency", time.Since(start)))
	}
}

// kafkaWriterWrapper — небольшая обёртка для записи в DLQ с добавлением заголовков.
type kafkaWriterWrapper struct {
	w *kafka.Writer
}

func newKafkaWriterWrapper(topic string, brokers []string) *kafkaWriterWrapper {
	return &kafkaWriterWrapper{w: kafkabot.NewWriter(topic, brokers)}
}

func (kw *kafkaWriterWrapper) write(ctx context.Context, key, value []byte, attempt int, origErr error) error {
	if kw == nil || kw.w == nil {
		return errors.New("dlq writer not initialized")
	}
	hdr := kafka.Header{Key: "x-attempt", Value: []byte(strconv.Itoa(attempt))}
	hdr2 := kafka.Header{Key: "x-error", Value: []byte(truncateErr(origErr, 200))}
	return kw.w.WriteMessages(ctx, kafka.Message{Key: key, Value: value, Headers: []kafka.Header{hdr, hdr2}})
}

func truncateErr(err error, n int) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// handleMessage обрабатывает одно сырое сообщение Kafka (JSON апдейт Telegram).
func handleMessage(ctx context.Context, db *sql.DB, data []byte) error {
	// Парсим JSON в карту (гибкость для ранней стадии развития модели данных).
	var update map[string]any
	if err := json.Unmarshal(data, &update); err != nil {
		return err
	}
	// Определяем тип контента
	ctype, err := utils.CheckContentType(update) // переименовано в ASCII
	if err != nil {
		return err
	}
	update["contenttype"] = ctype

	// Бизнес-логика (пока заглушка; можно расширить реакциями)
	processed, err := processBusinessLogic(update)
	if err != nil {
		return err
	}
	return postgresql.SaveToTable(ctx, db, processed, data)
}

// processBusinessLogic — точка расширения; возвращает модифицированную карту.
func processBusinessLogic(update map[string]any) (map[string]any, error) {
	// Русский комментарий: здесь можно реализовать цепочку фильтров, начисление лимитов и пр.
	return update, nil
}
