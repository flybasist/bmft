package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/flybasist/bmft/internal/config"
	"github.com/flybasist/bmft/internal/kafkabot"
	"github.com/flybasist/bmft/internal/postgresql"
	"github.com/flybasist/bmft/internal/utils"
	"github.com/flybasist/bmft/internal/logx"
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
	log.Info("Kafka reader started", zap.String("topic", "telegram-listener"), zap.String("group", cfg.KafkaGroupCore))

	for {
		// FetchMessage — без автокоммита, чтобы контролировать идемпотентность.
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
		if err := handleMessage(ctx, db, msg.Value); err != nil {
			log.Error("failed to handle message", zap.Error(err), zap.Int64("offset", msg.Offset))
			// Русский комментарий: решение — не коммитить offset, чтобы сообщение было перечитано (простой retry).
			// В будущем: добавить DLQ после N неудачных попыток.
			continue
		}

		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Error("commit failed", zap.Error(err), zap.Int64("offset", msg.Offset))
			continue
		}
		log.Info("message processed", zap.Int64("offset", msg.Offset), zap.Duration("latency", time.Since(start)))
	}
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
