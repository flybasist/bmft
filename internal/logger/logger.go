package logger

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "time"

    "github.com/flybasist/bmft/internal/config"
    "github.com/flybasist/bmft/internal/kafkabot"
    "github.com/flybasist/bmft/internal/logx"
    "go.uber.org/zap"
)

// Русский комментарий: Обновлённый пакет logger — минималистичный потребитель Kafka, который
// транслирует сообщения из указанных топиков в файлы (по дням) + структурированный stdout уже обеспечен zap.

const (
    logDir       = "./logs"
    retention    = 7 * 24 * time.Hour
)

// Run запускает файлоориентированное логирование Kafka топиков.
func Run(ctx context.Context, cfg *config.Config) {
    log := logx.L().Named("filelogger")
    if err := os.MkdirAll(logDir, 0o755); err != nil {
        log.Error("create log dir failed", zap.Error(err))
        return
    }

    topics := []string{"telegram-listener", "telegram-send", "telegram-delete"}
    for _, t := range topics {
        go runTopicLogger(ctx, t, cfg)
    }
    go cleaner(ctx)
}

// runTopicLogger пишет полученные сообщения Kafka в файл.
func runTopicLogger(ctx context.Context, topic string, cfg *config.Config) {
    log := logx.L().Named("filelogger.topic").With(zap.String("topic", topic))
    reader := kafkabot.NewReader(topic, cfg.KafkaBrokers, cfg.KafkaGroupLogger+"-"+topic)
    defer reader.Close()
    log.Info("topic logger started")
    for {
        msg, err := reader.FetchMessage(ctx)
        if err != nil {
            if ctx.Err() != nil {
                log.Info("context canceled, exit")
                return
            }
            log.Warn("fetch failed", zap.Error(err))
            time.Sleep(time.Second)
            continue
        }
        // Формируем путь: отдельный файл в день.
        path := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
        if err := appendLine(path, cfg.LogPretty, msg.Value); err != nil {
            log.Error("write failed", zap.Error(err))
        }
        if err := reader.CommitMessages(ctx, msg); err != nil {
            log.Warn("commit failed", zap.Error(err))
        }
    }
}

// appendLine добавляет строку в файл, делая pretty JSON при необходимости.
func appendLine(path string, pretty bool, raw []byte) error {
    f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
    if err != nil { return err }
    defer f.Close()
    if pretty {
        var js map[string]any
        if json.Unmarshal(raw, &js) == nil {
            b, _ := json.MarshalIndent(js, "", "  ")
            raw = b
        }
    }
    if _, err := f.Write(append(raw, '\n')); err != nil { return err }
    return nil
}

// cleaner удаляет устаревшие файлы логов.
func cleaner(ctx context.Context) {
    log := logx.L().Named("filelogger.cleaner")
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            entries, err := os.ReadDir(logDir)
            if err != nil { log.Warn("read dir failed", zap.Error(err)); continue }
            cutoff := time.Now().Add(-retention)
            for _, e := range entries {
                info, err := e.Info(); if err != nil { continue }
                if info.ModTime().Before(cutoff) {
                    _ = os.Remove(filepath.Join(logDir, e.Name()))
                    log.Info("log file removed", zap.String("file", e.Name()))
                }
            }
        }
    }
}
