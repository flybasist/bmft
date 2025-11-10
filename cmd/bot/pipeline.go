package main

import (
	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
)

// processMessage обрабатывает входящее сообщение через pipeline модулей.
// Русский комментарий: Явный pipeline обработки сообщений.
// ВАЖНО: Порядок модулей критичен!
// 1. Limiter — проверяет лимиты, может удалить сообщение
// 2. TextFilter — проверяет запрещённые слова, может удалить сообщение
// 3. Statistics — считает все сообщения (даже удалённые)
// 4. Reactions — отвечает на ключевые слова
// 5. Scheduler — не участвует в обработке сообщений (только cron tasks)
func processMessage(
	ctx *core.MessageContext,
	modules *Modules,
	moduleRepo *repositories.ModuleRepository,
	logger *zap.Logger,
) error {
	chatID := ctx.Chat.ID

	// Pipeline обработки (порядок важен!)
	pipeline := []struct {
		name      string
		onMessage func(*core.MessageContext) error
	}{
		{"limiter", modules.Limiter.OnMessage},
		{"textfilter", modules.TextFilter.OnMessage},
		{"statistics", modules.Statistics.OnMessage},
		{"reactions", modules.Reactions.OnMessage},
	}

	for _, p := range pipeline {
		// Проверяем включен ли модуль для этого чата/топика (с fallback)
		threadID := ctx.Message.ThreadID
		enabled, err := moduleRepo.IsEnabled(chatID, threadID, p.name)
		if err != nil {
			logger.Error("failed to check if module enabled",
				zap.String("module", p.name),
				zap.Int64("chat_id", chatID),
				zap.Int("thread_id", threadID),
				zap.Error(err))
			continue
		}

		if !enabled {
			continue // Модуль отключен для этого чата/топика
		}

		// Передаём сообщение модулю
		if err := p.onMessage(ctx); err != nil {
			logger.Error("module failed to process message",
				zap.String("module", p.name),
				zap.Int64("chat_id", chatID),
				zap.Int("message_id", ctx.Message.ID),
				zap.Error(err))
			// Не прерываем обработку, даём другим модулям шанс
		}
	}

	return nil
}
