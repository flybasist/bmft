package wizard

import (
	tele "gopkg.in/telebot.v3"
)

// Конвенция Unique для wizard-кнопок: `wiz_<wizard>_<action>`.
//
//   - Префикс `wiz_` — позволяет фильтровать wizard-кнопки от обычных кнопок
//     других модулей (когда такие появятся).
//   - `<wizard>` — имя wizard'а (welcome, addtask, ...).
//   - `<action>` — действие шага (on, off, ttl, back, cancel, apply, ...).
//
// Data (опциональный payload, до 64 байт): хранит значение, выбранное
// пользователем (например, секунды TTL: "60", "300", "manual").
//
// CancelUnique вынесен как общая константа, чтобы все wizard'ы использовали
// одну и ту же кнопку «Отмена» (handler регистрируется один раз).
const (
	UniqueCancel = "wiz_cancel"
)

// CancelButton — стандартная кнопка отмены wizard'а.
// Используется во всех wizard'ах для единообразия UX.
func CancelButton() tele.Btn {
	return tele.Btn{Unique: UniqueCancel, Text: "❌ Отмена"}
}

// RegisterCancelHandler регистрирует обработчик глобальной кнопки «Отмена».
// Должен вызываться один раз при инициализации Manager'а.
//
// Wizard может определить, какой именно wizard отменяется, через Manager.Guard
// — но для отмены нам не нужна привязка к конкретному wizard'у: мы просто
// удаляем state по ключу (chat, thread, user) если он есть.
func (m *Manager) RegisterCancelHandler(bot *tele.Bot) {
	btn := CancelButton()
	bot.Handle(&btn, func(c tele.Context) error {
		// Подтверждаем callback, чтобы у пользователя не висел спиннер.
		_ = c.Respond()

		cb := c.Callback()
		if cb == nil || cb.Message == nil || c.Sender() == nil {
			return nil
		}
		key := StateKey{
			ChatID: cb.Message.Chat.ID,
			UserID: c.Sender().ID,
		}
		m.Cancel(c, key, "🚫 Wizard отменён.")
		return nil
	})
}
