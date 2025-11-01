package core

import (
	"database/sql"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// BotCommand описывает команду бота для регистрации в Telegram.
// Русский комментарий: Используется модулями для объявления своих команд.
type BotCommand struct {
	Command     string // Команда (например: "/start", "/help")
	Description string // Описание команды (на русском)
}

// MessageContext — контекст входящего сообщения для модулей pipeline.
// Русский комментарий: Передаётся модулям в явном pipeline:
//
//	limiter → textfilter → statistics → reactions
//
// Содержит всё необходимое: Message, Bot, DB, Logger, Chat, Sender.
// Модули могут отправлять ответы, удалять сообщения, логировать события.
type MessageContext struct {
	Message *tele.Message // Оригинальное сообщение от telebot.v3
	Bot     *tele.Bot     // Инстанс бота
	DB      *sql.DB       // Подключение к БД
	Logger  *zap.Logger   // Логгер
	Chat    *tele.Chat    // Чат из которого пришло сообщение
	Sender  *tele.User    // Пользователь, отправивший сообщение
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

// LogEvent записывает событие в таблицу event_log для аудита.
// Русский комментарий: Все действия модулей (лимит превышен, реакция сработала и т.д.)
// записываются в event_log для последующего анализа.
func (ctx *MessageContext) LogEvent(moduleName, eventType, details string) error {
	query := `
		INSERT INTO event_log (chat_id, user_id, module_name, event_type, details)
		VALUES ($1, $2, $3, $4, $5)
	`
	userID := int64(0)
	if ctx.Sender != nil {
		userID = ctx.Sender.ID
	}
	_, err := ctx.DB.Exec(query, ctx.Chat.ID, userID, moduleName, eventType, details)
	return err
}
