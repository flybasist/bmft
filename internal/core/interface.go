package core

import (
	tele "gopkg.in/telebot.v3"
)

// MessageContext — контекст входящего сообщения для модулей pipeline.
// Передаётся модулям в явном pipeline:
//
//	statistics → limiter → reactions
//
// Reactions включает: фильтр мата, фильтр запрещённых слов, автоответы.
// ThreadID вычисляется один раз в middleware и кешируется для всех модулей (−2 SQL-запроса).
// MessageDeleted пропагируется между модулями: если Limiter удалил сообщение,
// Reactions увидит MessageDeleted=true и скорректирует поведение (подсчёт мата без delete/warn).
type MessageContext struct {
	Message        *tele.Message // Оригинальное сообщение от telebot.v3
	Bot            *tele.Bot     // Инстанс бота
	Chat           *tele.Chat    // Чат из которого пришло сообщение
	Sender         *tele.User    // Пользователь, отправивший сообщение
	ThreadID       int           // ID топика (0 = основной чат, вычислен в middleware)
	MessageDeleted bool          // Сообщение удалено (пропагируется через pipeline)
}

// SendReply отправляет ответ на сообщение с автоматическим ThreadID для форумов.
func (ctx *MessageContext) SendReply(text string) error {
	opts := &tele.SendOptions{
		ReplyTo: ctx.Message,
	}
	if ctx.ThreadID != 0 {
		opts.ThreadID = ctx.ThreadID
	}
	_, err := ctx.Bot.Send(ctx.Chat, text, opts)
	return err
}

// Send отправляет сообщение в чат без reply с автоматическим ThreadID для форумов.
func (ctx *MessageContext) Send(text string) error {
	opts := &tele.SendOptions{}
	if ctx.ThreadID != 0 {
		opts.ThreadID = ctx.ThreadID
	}
	_, err := ctx.Bot.Send(ctx.Chat, text, opts)
	return err
}

// SendOptions возвращает SendOptions с ThreadID для форумных чатов.
// Используется для отправки медиа (стикеры, фото) через Bot.Send с правильным топиком.
func (ctx *MessageContext) SendOptions() *tele.SendOptions {
	opts := &tele.SendOptions{
		ReplyTo: ctx.Message,
	}
	if ctx.ThreadID != 0 {
		opts.ThreadID = ctx.ThreadID
	}
	return opts
}

// DeleteMessage удаляет текущее сообщение и помечает контекст.
// MessageDeleted = true всегда: даже при ошибке удаления (message not found, no permission)
// не рискуем обрабатывать потенциально удалённое сообщение в следующих модулях.
func (ctx *MessageContext) DeleteMessage() error {
	err := ctx.Bot.Delete(ctx.Message)
	ctx.MessageDeleted = true
	return err
}
