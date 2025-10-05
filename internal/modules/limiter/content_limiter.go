package limiter

import (
	"fmt"
	"time"

	"github.com/flybasist/bmft/internal/core"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// ContentLimiterConfig хранит настройки лимитов на типы контента для чата
type ContentLimiterConfig struct {
	ChatID           int64
	UserID           int64  // 0 = для всех пользователей (allmembers)
	ContentType      string // photo, video, sticker, voice, etc.
	DailyLimit       int    // -1 = полный запрет, 0 = без лимита, N = лимит
	WarningThreshold int    // За сколько сообщений до лимита предупреждать (по умолчанию 2)
	IsVIP            bool   // VIP игнорирует лимиты
}

// detectContentType определяет тип контента сообщения
func detectContentType(msg *tele.Message) string {
	if msg.Photo != nil {
		return "photo"
	}
	if msg.Video != nil {
		return "video"
	}
	if msg.Sticker != nil {
		return "sticker"
	}
	if msg.Voice != nil {
		return "voice"
	}
	if msg.Document != nil {
		return "document"
	}
	if msg.Audio != nil {
		return "audio"
	}
	if msg.Animation != nil {
		return "animation"
	}
	if msg.VideoNote != nil {
		return "video_note"
	}
	if msg.Location != nil {
		return "location"
	}
	if msg.Contact != nil {
		return "contact"
	}
	if msg.Text != "" {
		return "text"
	}
	return "other"
}

// checkContentLimit проверяет лимит на тип контента и удаляет сообщение если превышен
// Возвращает true если сообщение должно быть обработано дальше, false если удалено
func (m *LimiterModule) checkContentLimit(ctx *core.MessageContext) (bool, error) {
	msg := ctx.Message
	if msg == nil {
		return true, nil
	}

	chatID := ctx.Chat.ID
	userID := ctx.Sender.ID
	username := ctx.Sender.Username
	if username == "" {
		username = ctx.Sender.FirstName
	}

	contentType := detectContentType(msg)

	// Игнорируем text и other - они обрабатываются reactions модулем
	if contentType == "text" || contentType == "other" {
		return true, nil
	}

	// 1. Проверяем VIP статус
	isVIP, err := m.limitRepo.IsVIP(userID)
	if err != nil {
		m.logger.Error("failed to check VIP status",
			zap.Int64("user_id", userID),
			zap.Error(err))
		return true, nil // В случае ошибки - пропускаем сообщение
	}

	if isVIP {
		m.logger.Debug("VIP user bypassed content limit",
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType))
		// Всё равно инкрементим счётчик для статистики
		_ = m.limitRepo.IncrementContentCounter(chatID, userID, contentType, time.Now())
		return true, nil
	}

	// 2. Получаем лимит из limiter_config
	// Сначала проверяем персональный лимит (user_id != NULL)
	// Затем общий лимит (user_id = NULL для allmembers)
	limit, err := m.limitRepo.GetContentLimit(chatID, userID, contentType)
	if err != nil {
		m.logger.Error("failed to get content limit",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Error(err))
		return true, nil // В случае ошибки - пропускаем
	}

	// limit == 0 означает "без ограничений"
	if limit == 0 {
		_ = m.limitRepo.IncrementContentCounter(chatID, userID, contentType, time.Now())
		return true, nil
	}

	// limit == -1 означает "полный запрет"
	if limit == -1 {
		// Удаляем сообщение
		if err := ctx.Bot.Delete(msg); err != nil {
			m.logger.Error("failed to delete forbidden content message",
				zap.Int64("chat_id", chatID),
				zap.Int64("user_id", userID),
				zap.String("content_type", contentType),
				zap.Error(err))
		}

		// Отправляем уведомление
		answer := fmt.Sprintf("@%s В этом чате запрещен %s", username, contentType)
		if _, err := ctx.Bot.Send(ctx.Chat, answer); err != nil {
			m.logger.Error("failed to send forbidden content notification",
				zap.Error(err))
		}

		m.logger.Info("content forbidden - message deleted",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType))

		return false, nil // Сообщение удалено
	}

	// limit > 0 - есть дневной лимит
	// 3. Получаем текущий счётчик за сегодня
	today := time.Now()
	count, err := m.limitRepo.GetContentCount(chatID, userID, contentType, today)
	if err != nil {
		m.logger.Error("failed to get content count",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Error(err))
		return true, nil
	}

	// 4. Проверяем превышение лимита
	if count >= limit {
		// Лимит превышен - удаляем сообщение
		if err := ctx.Bot.Delete(msg); err != nil {
			m.logger.Error("failed to delete over-limit message",
				zap.Error(err))
		}

		m.logger.Info("content limit exceeded - message deleted",
			zap.Int64("chat_id", chatID),
			zap.Int64("user_id", userID),
			zap.String("content_type", contentType),
			zap.Int("count", count),
			zap.Int("limit", limit))

		return false, nil // Сообщение удалено
	}

	// 5. Предупреждение за N сообщений до лимита (по умолчанию 2)
	warningThreshold := 2
	if count+warningThreshold >= limit {
		answer := fmt.Sprintf("@%s Ты отправил %d из %d разрешённых %s",
			username, count+1, limit, contentType)
		if _, err := ctx.Bot.Send(ctx.Chat, answer); err != nil {
			m.logger.Error("failed to send warning",
				zap.Error(err))
		}
	}

	// 6. Инкрементируем счётчик
	if err := m.limitRepo.IncrementContentCounter(chatID, userID, contentType, today); err != nil {
		m.logger.Error("failed to increment content counter",
			zap.Error(err))
		// Не блокируем сообщение из-за ошибки счётчика
	}

	return true, nil // Сообщение прошло проверку
}
