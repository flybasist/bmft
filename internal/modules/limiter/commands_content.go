package limiter

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// handleSetContentLimit устанавливает лимит на тип контента
// Usage: /setcontentlimit <content_type> <limit>
// Example: /setcontentlimit photo 5
// Example: /setcontentlimit sticker -1  (полный запрет)
// Example: /setcontentlimit video 0     (без ограничений)
func (m *LimiterModule) handleSetContentLimit(c tele.Context) error {
	// Проверка что команда из группы/супергруппы
	if !c.Message().FromGroup() {
		return c.Reply("❌ Эта команда работает только в группах")
	}

	// Проверка админских прав
	chatID := c.Chat().ID
	adminIDs, err := c.Bot().AdminsOf(c.Chat())
	if err != nil {
		m.logger.Error("failed to get admins", zap.Error(err))
		return c.Reply("❌ Не удалось проверить права администратора")
	}

	isAdmin := false
	senderID := c.Sender().ID
	for _, admin := range adminIDs {
		if admin.User.ID == senderID {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		return c.Reply("❌ Эта команда доступна только администраторам чата")
	}

	// Парсинг аргументов
	args := strings.Fields(c.Message().Text)
	if len(args) != 3 {
		return c.Reply("❌ Использование: /setcontentlimit <content_type> <limit>\n\n" +
			"Типы контента: photo, video, sticker, voice, document, audio, animation, video_note\n" +
			"Лимиты:\n" +
			"  -1 = полный запрет\n" +
			"   0 = без ограничений\n" +
			"   N = лимит на N сообщений/день\n\n" +
			"Пример: /setcontentlimit photo 5")
	}

	contentType := args[1]
	limitStr := args[2]

	// Валидация content_type
	validTypes := []string{"photo", "video", "sticker", "voice", "document", "audio", "animation", "video_note"}
	isValidType := false
	for _, vt := range validTypes {
		if contentType == vt {
			isValidType = true
			break
		}
	}

	if !isValidType {
		return c.Reply(fmt.Sprintf("❌ Неизвестный тип контента: %s\n\nДоступные типы: %s",
			contentType, strings.Join(validTypes, ", ")))
	}

	// Парсинг лимита
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return c.Reply("❌ Лимит должен быть числом")
	}

	if limit < -1 {
		return c.Reply("❌ Лимит не может быть меньше -1")
	}

	// Сохранение в БД (limiter_config)
	if err := m.saveContentLimit(chatID, 0, contentType, limit); err != nil {
		m.logger.Error("failed to save content limit",
			zap.Int64("chat_id", chatID),
			zap.String("content_type", contentType),
			zap.Int("limit", limit),
			zap.Error(err))
		return c.Reply("❌ Не удалось сохранить лимит")
	}

	// Форматирование ответа
	var status string
	switch {
	case limit == -1:
		status = "🚫 Полный запрет"
	case limit == 0:
		status = "✅ Без ограничений"
	default:
		status = fmt.Sprintf("📊 Лимит: %d сообщений/день", limit)
	}

	return c.Reply(fmt.Sprintf("✅ Лимит установлен!\n\n"+
		"Тип контента: %s\n"+
		"Статус: %s", contentType, status))
}

// handleMyContentUsage показывает использование лимитов контента за сегодня
// Usage: /mycontentusage
func (m *LimiterModule) handleMyContentUsage(c tele.Context) error {
	// Проверка что команда из группы/супергруппы
	if !c.Message().FromGroup() {
		return c.Reply("❌ Эта команда работает только в группах")
	}

	chatID := c.Chat().ID
	userID := c.Sender().ID
	username := c.Sender().Username
	if username == "" {
		username = c.Sender().FirstName
	}

	today := time.Now()

	// Типы контента для проверки
	contentTypes := []string{"photo", "video", "sticker", "voice", "document", "audio", "animation", "video_note"}

	var response strings.Builder
	response.WriteString(fmt.Sprintf("📊 Использование лимитов @%s за сегодня:\n\n", username))

	hasAnyUsage := false

	for _, contentType := range contentTypes {
		// Получаем лимит
		limit, err := m.limitRepo.GetContentLimit(chatID, userID, contentType)
		if err != nil {
			m.logger.Error("failed to get content limit",
				zap.String("content_type", contentType),
				zap.Error(err))
			continue
		}

		// Если лимита нет (0 = без ограничений), пропускаем
		if limit == 0 {
			continue
		}

		// Получаем счётчик
		count, err := m.limitRepo.GetContentCount(chatID, userID, contentType, today)
		if err != nil {
			m.logger.Error("failed to get content count",
				zap.String("content_type", contentType),
				zap.Error(err))
			continue
		}

		// Показываем только если есть лимит или использование
		if limit != 0 || count > 0 {
			hasAnyUsage = true

			var statusIcon string
			var statusText string

			switch {
			case limit == -1:
				statusIcon = "🚫"
				statusText = "запрещено"
			case count >= limit && limit > 0:
				statusIcon = "❌"
				statusText = fmt.Sprintf("%d/%d (превышен!)", count, limit)
			case limit > 0:
				statusIcon = "📈"
				statusText = fmt.Sprintf("%d/%d", count, limit)
			default:
				continue // skip если 0 и не использовался
			}

			response.WriteString(fmt.Sprintf("%s %s: %s\n", statusIcon, contentType, statusText))
		}
	}

	if !hasAnyUsage {
		response.WriteString("✅ Нет активных лимитов или использования за сегодня")
	}

	return c.Reply(response.String())
}

// handleListContentLimits показывает все лимиты чата (только для админов)
// Usage: /listcontentlimits
func (m *LimiterModule) handleListContentLimits(c tele.Context) error {
	// Проверка что команда из группы/супергруппы
	if !c.Message().FromGroup() {
		return c.Reply("❌ Эта команда работает только в группах")
	}

	// Проверка админских прав
	chatID := c.Chat().ID
	adminIDs, err := c.Bot().AdminsOf(c.Chat())
	if err != nil {
		m.logger.Error("failed to get admins", zap.Error(err))
		return c.Reply("❌ Не удалось проверить права администратора")
	}

	isAdmin := false
	senderID := c.Sender().ID
	for _, admin := range adminIDs {
		if admin.User.ID == senderID {
			isAdmin = true
			break
		}
	}

	if !isAdmin {
		return c.Reply("❌ Эта команда доступна только администраторам чата")
	}

	// Получаем все лимиты из БД
	limits, err := m.getAllContentLimits(chatID)
	if err != nil {
		m.logger.Error("failed to get all content limits",
			zap.Int64("chat_id", chatID),
			zap.Error(err))
		return c.Reply("❌ Не удалось получить список лимитов")
	}

	if len(limits) == 0 {
		return c.Reply("📋 Лимиты на типы контента не установлены\n\n" +
			"Используйте /setcontentlimit для настройки")
	}

	var response strings.Builder
	response.WriteString("📋 Активные лимиты на типы контента:\n\n")

	for contentType, limit := range limits {
		var status string
		switch {
		case limit == -1:
			status = "🚫 Полный запрет"
		case limit == 0:
			status = "✅ Без ограничений"
		default:
			status = fmt.Sprintf("📊 %d сообщений/день", limit)
		}

		response.WriteString(fmt.Sprintf("%s: %s\n", contentType, status))
	}

	return c.Reply(response.String())
}

// saveContentLimit сохраняет лимит в limiter_config
func (m *LimiterModule) saveContentLimit(chatID, userID int64, contentType string, limit int) error {
	return m.limitRepo.SaveContentLimit(chatID, userID, contentType, limit)
}

// getAllContentLimits получает все лимиты чата из limiter_config
func (m *LimiterModule) getAllContentLimits(chatID int64) (map[string]int, error) {
	return m.limitRepo.GetAllContentLimits(chatID)
}
