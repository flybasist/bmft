package core

import (
	tele "gopkg.in/telebot.v3"
)

// MessageContext — контекст входящего сообщения для модулей pipeline.
// Русский комментарий: Передаётся модулям в явном pipeline:
//
//	limiter → textfilter → statistics → reactions
//
// Содержит всё необходимое: Message, Bot, Chat, Sender.
// Модули могут отправлять ответы, удалять сообщения.
type MessageContext struct {
	Message         *tele.Message // Оригинальное сообщение от telebot.v3
	Bot             *tele.Bot     // Инстанс бота
	Chat            *tele.Chat    // Чат из которого пришло сообщение
	Sender          *tele.User    // Пользователь, отправивший сообщение
	StopPropagation bool          // Флаг остановки pipeline (если true, следующие модули не вызываются)
}

// SendReply отправляет ответ на сообщение.
func (ctx *MessageContext) SendReply(text string) error {
	_, err := ctx.Bot.Send(ctx.Chat, text, &tele.SendOptions{
		ReplyTo: ctx.Message,
	})
	return err
}

// Send отправляет сообщение в чат без reply.
func (ctx *MessageContext) Send(text string) error {
	_, err := ctx.Bot.Send(ctx.Chat, text)
	return err
}

// DeleteMessage удаляет текущее сообщение.
func (ctx *MessageContext) DeleteMessage() error {
	return ctx.Bot.Delete(ctx.Message)
}
