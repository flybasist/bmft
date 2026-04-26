// Package statistics — сбор статистики активности и отчёты по чату/пользователям.
package statistics

import (
	"database/sql"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// StatisticsModule реализует модуль статистики.
// Записывает все сообщения в messages с metadata вместо отдельной таблицы content_counters.
// Предоставляет команды: /mystats (личная статистика), /chatstats (статистика чата), /topchat (топ активных).
type StatisticsModule struct {
	db          *sql.DB
	bot         *tele.Bot
	logger      *zap.Logger
	messageRepo *repositories.MessageRepository
	eventRepo   *repositories.EventRepository
}

// New создаёт новый экземпляр модуля статистики.
// messageRepo — общий экземпляр из initModules (не создаём дубликат).
func New(
	db *sql.DB,
	eventRepo *repositories.EventRepository,
	messageRepo *repositories.MessageRepository,
	logger *zap.Logger,
	bot *tele.Bot,
) *StatisticsModule {
	return &StatisticsModule{
		db:          db,
		logger:      logger,
		messageRepo: messageRepo,
		eventRepo:   eventRepo,
		bot:         bot,
	}
}

// OnMessage обрабатывает входящее сообщение.
// При каждом сообщении инкрементим счётчик в БД.
func (m *StatisticsModule) OnMessage(ctx *core.MessageContext) error {
	if ctx.Message == nil || ctx.Sender == nil {
		m.logger.Warn("statistics: empty message or sender", zap.Any("ctx", ctx))
		return nil
	}

	// ThreadID уже вычислен в middleware и закеширован — без лишнего SQL-запроса.
	threadID := ctx.ThreadID

	m.logger.Debug("statistics: received message",
		zap.Int64("chat_id", ctx.Chat.ID),
		zap.Int("thread_id", threadID),
		zap.Int64("user_id", ctx.Sender.ID),
		zap.String("username", ctx.Sender.Username),
		zap.String("text", ctx.Message.Text),
	)

	contentType := core.DetectContentType(ctx.Message)
	m.logger.Debug("statistics: detected content type", zap.String("content_type", contentType))

	// Формируем chat_name для удобства статистики
	// Для ЛС: username пользователя
	// Для групп: название чата
	// Если нет - используем пустую строку (не падаем)
	chatName := ""
	if ctx.Chat.Type == "private" {
		// Личные сообщения - используем username отправителя
		if ctx.Sender.Username != "" {
			chatName = "@" + ctx.Sender.Username
		} else if ctx.Sender.FirstName != "" {
			chatName = ctx.Sender.FirstName
		}
	} else {
		// Группы/супергруппы/каналы - используем название чата
		if ctx.Chat.Title != "" {
			chatName = ctx.Chat.Title
		} else if ctx.Chat.Username != "" {
			chatName = "@" + ctx.Chat.Username
		}
	}

	// Сохраняем сообщение с metadata
	metadata := repositories.MessageMetadata{
		Statistics: &repositories.StatisticsMetadata{
			Processed:        true,
			ProcessingTimeMs: 0, // TODO: замерять реальное время обработки
		},
	}

	_, err := m.messageRepo.InsertMessage(
		ctx.Chat.ID,
		threadID,
		ctx.Sender.ID,
		ctx.Message.ID,
		contentType,
		ctx.Message.Text,
		ctx.Message.Caption,
		m.getFileID(ctx.Message),
		chatName,
		metadata,
	)
	if err != nil {
		m.logger.Error("statistics: failed to insert message",
			zap.Int64("chat_id", ctx.Chat.ID),
			zap.Int("thread_id", threadID),
			zap.Int64("user_id", ctx.Sender.ID),
			zap.Int("message_id", ctx.Message.ID),
			zap.Error(err))
		return err
	}

	m.logger.Debug("statistics: message saved with metadata",
		zap.Int64("chat_id", ctx.Chat.ID),
		zap.Int("thread_id", threadID),
		zap.Int64("user_id", ctx.Sender.ID),
		zap.String("content_type", contentType),
	)

	return nil
}

// getFileID извлекает file_id из сообщения если есть медиа.
func (m *StatisticsModule) getFileID(msg *tele.Message) string {
	if msg.Photo != nil {
		return msg.Photo.FileID
	}
	if msg.Video != nil {
		return msg.Video.FileID
	}
	if msg.Sticker != nil {
		return msg.Sticker.FileID
	}
	if msg.Animation != nil {
		return msg.Animation.FileID
	}
	if msg.Voice != nil {
		return msg.Voice.FileID
	}
	if msg.VideoNote != nil {
		return msg.VideoNote.FileID
	}
	if msg.Audio != nil {
		return msg.Audio.FileID
	}
	if msg.Document != nil {
		return msg.Document.FileID
	}
	return ""
}

// RegisterCommands регистрирует команды модуля в боте.
func (m *StatisticsModule) RegisterCommands(bot *tele.Bot) {
	// Все команды обслуживаются inline-меню (cmd/bot/menus.go).
}

// RegisterAdminCommands регистрирует админские команды.
func (m *StatisticsModule) RegisterAdminCommands(bot *tele.Bot) {
	// Все команды обслуживаются inline-меню (cmd/bot/menus.go).
}
