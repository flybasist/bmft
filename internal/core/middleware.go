package core

import (
	"fmt"
	"runtime/debug"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// LoggerMiddleware логирует все входящие сообщения.
// Русский комментарий: Middleware для логирования всех сообщений, которые приходят боту.
// Логи на английском для единообразия операционных сообщений.
func LoggerMiddleware(logger *zap.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			msg := c.Message()
			if msg == nil {
				return next(c)
			}

			logger.Info("incoming message",
				zap.Int64("chat_id", msg.Chat.ID),
				zap.String("chat_type", string(msg.Chat.Type)),
				zap.Int("message_id", msg.ID),
				zap.Int64("user_id", msg.Sender.ID),
				zap.String("username", msg.Sender.Username),
				zap.String("text", msg.Text),
			)

			return next(c)
		}
	}
}

// PanicRecoveryMiddleware ловит panic и логирует его вместо падения бота.
// Русский комментарий: Middleware для graceful recovery от паник в хендлерах.
// Если модуль паникует — логируем стек-трейс, но бот продолжает работать.
func PanicRecoveryMiddleware(logger *zap.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) (err error) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("panic recovered in handler",
						zap.Any("panic", r),
						zap.String("stack", string(debug.Stack())),
					)

					// Пытаемся сообщить пользователю об ошибке
					msg := c.Message()
					if msg != nil {
						_ = c.Send("Произошла внутренняя ошибка. Попробуйте позже.")
					}

					err = fmt.Errorf("panic recovered: %v", r)
				}
			}()

			return next(c)
		}
	}
}
