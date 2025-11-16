package main

import (
	"github.com/flybasist/bmft/internal/core"
	"go.uber.org/zap"
)

// processMessage обрабатывает входящее сообщение через pipeline модулей.
// Русский комментарий: Явный pipeline обработки сообщений.
// ВАЖНО: Порядок модулей критичен!
// 1. Statistics — считает все сообщения (единый источник правды для таблицы messages)
// 2. Limiter — проверяет лимиты (читает из messages), может удалить сообщение
// 3. ProfanityFilter — проверяет матерные слова, может удалить сообщение
// 4. TextFilter — проверяет запрещённые слова, может удалить сообщение
// 5. Reactions — отвечает на ключевые слова
// 6. Scheduler — не участвует в обработке сообщений (только cron tasks)
//
// Все модули работают по принципу "есть конфиг в БД = работают, нет конфига = молчат"
func processMessage(
	ctx *core.MessageContext,
	modules *Modules,
	logger *zap.Logger,
) error {
	chatID := ctx.Chat.ID

	// Pipeline обработки (порядок важен!)
	// Statistics ПЕРВОЙ — записывает в messages (единый источник правды)
	// Limiter читает из messages, поэтому идёт после Statistics
	// ProfanityFilter перед TextFilter (глобальные правила перед локальными)
	pipeline := []struct {
		name      string
		onMessage func(*core.MessageContext) error
	}{
		{"statistics", modules.Statistics.OnMessage},
		{"limiter", modules.Limiter.OnMessage},
		{"profanityfilter", modules.ProfanityFilter.OnMessage},
		{"textfilter", modules.TextFilter.OnMessage},
		{"reactions", modules.Reactions.OnMessage},
	}

	for _, p := range pipeline {
		// Передаём сообщение модулю (модули сами решают работать или нет)
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
