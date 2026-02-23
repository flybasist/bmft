package core

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// commandCooldownKey — ключ для отслеживания кулдауна: один пользователь + одна команда в одном чате.
type commandCooldownKey struct {
	ChatID  int64
	UserID  int64
	Command string
}

// CommandCooldownMiddleware ограничивает частоту вызова ЛЮБЫХ команд.
// Если пользователь отправляет команды чаще чем cooldown — сообщение удаляется,
// бот не отвечает. Это предотвращает спам командами типа /version, /help и т.д.
//
// Параметры:
//   - cooldown: минимальный интервал между командами одного пользователя в одном чате
//   - logger: логгер (warn при throttle)
func CommandCooldownMiddleware(cooldown time.Duration, logger *zap.Logger) tele.MiddlewareFunc {
	var mu sync.Mutex
	lastUsed := make(map[commandCooldownKey]time.Time)

	// Фоновая очистка устаревших записей каждые 5 минут
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mu.Lock()
			now := time.Now()
			for k, t := range lastUsed {
				if now.Sub(t) > 10*time.Minute {
					delete(lastUsed, k)
				}
			}
			mu.Unlock()
		}
	}()

	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			msg := c.Message()
			if msg == nil {
				return next(c)
			}

			// Срабатываем только на команды
			text := msg.Text
			if text == "" || !strings.HasPrefix(text, "/") {
				return next(c)
			}

			// Извлекаем команду (без аргументов и @botname)
			cmd := strings.Fields(text)[0]
			if idx := strings.Index(cmd, "@"); idx != -1 {
				cmd = cmd[:idx]
			}

			key := commandCooldownKey{
				ChatID:  msg.Chat.ID,
				UserID:  msg.Sender.ID,
				Command: strings.ToLower(cmd),
			}

			mu.Lock()
			last, exists := lastUsed[key]
			now := time.Now()

			if exists && now.Sub(last) < cooldown {
				mu.Unlock()

				// Спам — удаляем команду пользователя, бот молчит
				logger.Debug("command throttled",
					zap.Int64("chat_id", msg.Chat.ID),
					zap.Int64("user_id", msg.Sender.ID),
					zap.String("command", text),
				)

				_ = c.Delete()
				return nil
			}

			lastUsed[key] = now
			mu.Unlock()

			return next(c)
		}
	}
}
