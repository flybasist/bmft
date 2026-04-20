package main

import (
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
)

// wrapSetProfanityWithWizard возвращает handler /setprofanity, который
// маршрутизирует команду в wizard или в legacy-handler модуля reactions.
//
// Маршрутизация (порядок приоритета):
//
//  1. Если есть аргументы (например, "/setprofanity delete") → legacy
//     (старый синтаксис сохраняется для скриптов и быстрых изменений).
//  2. Если контекст не из группы (личка) → legacy
//     (wizard в личке запрещён security model'ью; legacy сам обработает).
//  3. Если отправитель — anonymous admin → legacy
//     (wizard не различит таких пользователей; см. core.IsAnonymousAdmin).
//  4. Иначе → wizard.
//
// Wrapper нужен потому, что wizard package не знает про reactions module,
// и мы не хотим тащить wizard.Manager внутрь модулей. Вместо этого
// перерегистрируем endpoint в main.go ПОСЛЕ initModules — telebot
// перезаписывает обработчик при повторном bot.Handle с тем же endpoint'ом.
func wrapSetProfanityWithWizard(legacy tele.HandlerFunc, startWizard func(c tele.Context) error) tele.HandlerFunc {
	return func(c tele.Context) error {
		if len(c.Args()) > 0 {
			return legacy(c)
		}
		chat := c.Chat()
		if chat == nil || (chat.Type != tele.ChatGroup && chat.Type != tele.ChatSuperGroup) {
			return legacy(c)
		}
		if core.IsAnonymousAdmin(c.Message()) {
			return legacy(c)
		}
		return startWizard(c)
	}
}

// wrapSetVIPWithWizard — аналогично wrapSetProfanityWithWizard, но с
// дополнительным правилом: /setvip требует ReplyTo (target определяется
// через цитируемое сообщение). Без ReplyTo wizard невозможен — отдаём
// в legacy, тот покажет «❌ Ответьте этой командой на сообщение».
//
// Маршрутизация:
//  1. len(args) > 0    → legacy (старый синтаксис «/setvip <reason>»).
//  2. не группа        → legacy.
//  3. anonymous admin  → legacy.
//  4. ReplyTo == nil   → legacy (он отправит подсказку).
//  5. иначе            → wizard.
func wrapSetVIPWithWizard(legacy tele.HandlerFunc, startWizard func(c tele.Context) error) tele.HandlerFunc {
	return func(c tele.Context) error {
		if len(c.Args()) > 0 {
			return legacy(c)
		}
		chat := c.Chat()
		if chat == nil || (chat.Type != tele.ChatGroup && chat.Type != tele.ChatSuperGroup) {
			return legacy(c)
		}
		msg := c.Message()
		if msg == nil || core.IsAnonymousAdmin(msg) {
			return legacy(c)
		}
		if msg.ReplyTo == nil || msg.ReplyTo.Sender == nil {
			return legacy(c)
		}
		return startWizard(c)
	}
}

// wrapSetLimitWithWizard — двухрежимный handler для /setlimit.
//
// Старый синтаксис «/setlimit <type> <value>» (с опц. ReplyTo для персонального
// лимита) всегда идёт в legacy. Wizard запускается только когда:
//  1. аргументы пусты;
//  2. группа (не личка);
//  3. не анонимный админ.
//
// ReplyTo в wizard НЕ является обязательным (в отличие от /setvip):
// без reply wizard поставит лимит на весь скоуп (чат/топик), с reply —
// персональный для цитируемого пользователя.
func wrapSetLimitWithWizard(legacy tele.HandlerFunc, startWizard func(c tele.Context) error) tele.HandlerFunc {
	return func(c tele.Context) error {
		if len(c.Args()) > 0 {
			return legacy(c)
		}
		chat := c.Chat()
		if chat == nil || (chat.Type != tele.ChatGroup && chat.Type != tele.ChatSuperGroup) {
			return legacy(c)
		}
		if core.IsAnonymousAdmin(c.Message()) {
			return legacy(c)
		}
		return startWizard(c)
	}
}

// wrapAddBanWithWizard — двухрежимный handler для /addban.
//
// Старый синтаксис «/addban <pattern> <action>» — всегда legacy.
// Wizard запускается только когда:
//  1. аргументы пусты;
//  2. группа (не личка);
//  3. не анонимный админ.
//
// ReplyTo не используется (паттерн вводится как текст в ходе wizard'а).
func wrapAddBanWithWizard(legacy tele.HandlerFunc, startWizard func(c tele.Context) error) tele.HandlerFunc {
	return func(c tele.Context) error {
		if len(c.Args()) > 0 {
			return legacy(c)
		}
		chat := c.Chat()
		if chat == nil || (chat.Type != tele.ChatGroup && chat.Type != tele.ChatSuperGroup) {
			return legacy(c)
		}
		if core.IsAnonymousAdmin(c.Message()) {
			return legacy(c)
		}
		return startWizard(c)
	}
}

// wrapAddTaskWithWizard — двухрежимный handler для /addtask.
//
// Старый синтаксис «/addtask <name> "<cron>" <type> <data>» (или с reply
// на медиа и аргументами) — всегда legacy. Wizard запускается только когда:
//  1. аргументы пусты;
//  2. группа (не личка);
//  3. не анонимный админ.
//
// ReplyTo разрешён: если он содержит медиа, wizard извлечёт type/file_id
// из reply на старте и пропустит шаг 3 (ввод текста). Без ReplyTo wizard
// собирает всё (имя/cron/текст) и создаёт текстовую задачу.
func wrapAddTaskWithWizard(legacy tele.HandlerFunc, startWizard func(c tele.Context) error) tele.HandlerFunc {
	return func(c tele.Context) error {
		if len(c.Args()) > 0 {
			return legacy(c)
		}
		chat := c.Chat()
		if chat == nil || (chat.Type != tele.ChatGroup && chat.Type != tele.ChatSuperGroup) {
			return legacy(c)
		}
		if core.IsAnonymousAdmin(c.Message()) {
			return legacy(c)
		}
		return startWizard(c)
	}
}
