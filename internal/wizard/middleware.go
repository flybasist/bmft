package wizard

import (
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// TextInterceptMiddleware регистрируется в pipeline ПЕРЕД статистикой/limiter/reactions.
//
// Логика:
//   - Если для (chat, thread, sender) есть активный wizard в режиме AwaitText,
//     вызываем wizard.consumeText: тот сохраняет текст в State.Data, переходит
//     к следующему шагу (или завершает) и возвращает — middleware удаляет
//     сообщение пользователя (чтобы не засоряло чат) и НЕ вызывает next(c)
//     (последующие модули игнорируют сообщение).
//   - Команды (/foo) на текстовом шаге трактуем как escape: отменяем wizard
//     с понятным сообщением и НЕ поглощаем команду — она пройдёт дальше
//     (например, /cancel или другая команда сработает нормально).
//   - В остальных случаях — next(c) без изменений.
//
// onText — обработчики текста, зарегистрированные через RegisterTextHandler
// для каждого wizard'а отдельно (ключ — имя wizard'а из state.Wizard).
func (m *Manager) TextInterceptMiddleware() tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			msg := c.Message()
			if msg == nil || c.Sender() == nil {
				return next(c)
			}
			// Wizard работает только в группах.
			if msg.Chat == nil || (msg.Chat.Type != tele.ChatGroup && msg.Chat.Type != tele.ChatSuperGroup) {
				return next(c)
			}

			key := StateKey{
				ChatID: msg.Chat.ID,
				UserID: c.Sender().ID,
			}
			state := m.store.get(key)
			if state == nil || !state.AwaitText {
				return next(c)
			}

			// Команда (например /cancel) → отменяем wizard, пропускаем команду дальше.
			if len(msg.Text) > 0 && msg.Text[0] == '/' {
				m.cancelWithText(c, key, "🚫 Wizard отменён командой.")
				return next(c)
			}

			// Передаём текст в обработчик wizard'а по имени.
			handler, ok := m.textHandlers[state.Wizard]
			if !ok {
				m.logger.Warn("wizard text intercepted but no handler registered",
					zap.String("wizard", state.Wizard),
					zap.String("step", state.Step))
				return next(c)
			}

			// Сбрасываем флаг ДО вызова handler'а: handler сам решит,
			// поставить ли AwaitText снова (если шаг повторяется при ошибке валидации).
			state.AwaitText = false

			if err := handler(c, state, msg.Text); err != nil {
				m.logger.Warn("wizard text handler failed",
					zap.String("wizard", state.Wizard),
					zap.String("step", state.Step),
					zap.Error(err))
				// Не возвращаем ошибку наверх — wizard либо отменён внутри handler'а,
				// либо остался в прежнем шаге для повторного ввода.
			}

			// Удаляем сообщение пользователя, чтобы не засоряло чат
			// (особенно когда это «300» в ответ на «введите TTL в секундах»).
			if err := c.Delete(); err != nil {
				m.logger.Debug("failed to delete user text input for wizard",
					zap.Int("message_id", msg.ID),
					zap.Error(err))
			}

			// НЕ вызываем next(c) — статистика/limiter/reactions это сообщение не должны обрабатывать.
			return nil
		}
	}
}

// TextHandler — обработчик текстовога ввода одного wizard'а.
// Отвечает за валидацию ввода, переход к следующему шагу или завершение.
type TextHandler func(c tele.Context, state *State, text string) error

// RegisterTextHandler регистрирует текстовый обработчик для wizard'а с именем name.
// Вызывается wizard'ом при инициализации (в RegisterXxx).
// Два wizard'а с одним и тем же именем — ошибка конфигурации; логируем warning.
func (m *Manager) RegisterTextHandler(name string, handler TextHandler) {
	if _, exists := m.textHandlers[name]; exists {
		m.logger.Warn("wizard text handler overwritten", zap.String("name", name))
	}
	m.textHandlers[name] = handler
}
