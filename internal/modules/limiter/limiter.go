package limiter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// LimiterModule управляет лимитами на контент в чатах
type LimiterModule struct {
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	moduleRepo        *repositories.ModuleRepository
	logger            *zap.Logger
	bot               *tele.Bot
}

// New создаёт новый экземпляр LimiterModule
func New(vipRepo *repositories.VIPRepository, contentLimitsRepo *repositories.ContentLimitsRepository, moduleRepo *repositories.ModuleRepository, logger *zap.Logger, bot *tele.Bot) *LimiterModule {
	return &LimiterModule{
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		moduleRepo:        moduleRepo,
		logger:            logger,
		bot:               bot,
	}
}

// Shutdown завершает работу модуля
func (m *LimiterModule) Shutdown() error {
	m.logger.Info("limiter module shutdown")
	return nil
}

// Init инициализирует модуль
func (m *LimiterModule) Init(deps core.ModuleDependencies) error {
	m.logger.Info("limiter module initialized")
	return nil
}

// Commands возвращает список команд модуля
func (m *LimiterModule) Commands() []core.BotCommand {
	return []core.BotCommand{
		{Command: "/mystats", Description: "Показать статистику использования контента"},
		{Command: "/setlimit", Description: "Установить лимит на тип контента (админы)"},
		{Command: "/setvip", Description: "Установить VIP-статус пользователю (админы)"},
		{Command: "/removevip", Description: "Снять VIP-статус с пользователя (админы)"},
		{Command: "/listvips", Description: "Показать список VIP-пользователей (админы)"},
	}
}

// Enabled проверяет, включен ли модуль для данного чата
func (m *LimiterModule) Enabled(chatID int64) (bool, error) {
	return m.moduleRepo.IsEnabled(chatID, "limiter")
}

// detectContentType определяет тип контента сообщения
func (m *LimiterModule) detectContentType(msg *tele.Message) string {
	if msg.Photo != nil {
		return "photo"
	}
	if msg.Video != nil {
		return "video"
	}
	if msg.Sticker != nil {
		return "sticker"
	}
	if msg.Animation != nil {
		return "animation"
	}
	if msg.Voice != nil {
		return "voice"
	}
	if msg.VideoNote != nil {
		return "video_note"
	}
	if msg.Audio != nil {
		return "audio"
	}
	if msg.Document != nil {
		// Специальная проверка для гифок, отправленных как файлы
		if msg.Document.MIME == "image/gif" {
			return "animation"
		}
		return "document"
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
	return "unknown"
}

// RegisterCommands регистрирует пользовательские команды
func (m *LimiterModule) RegisterCommands(bot *tele.Bot) {
	bot.Handle("/mystats", m.handleMyStats)
	// bot.Handle("/myweek", m.handleMyWeek) // TODO: реализовать статистику за неделю
}

// RegisterAdminCommands регистрирует административные команды
func (m *LimiterModule) RegisterAdminCommands(bot *tele.Bot) {
	bot.Handle("/setlimit", m.handleSetLimit)
	bot.Handle("/setvip", m.handleSetVIP)
	bot.Handle("/removevip", m.handleRemoveVIP)
	bot.Handle("/listvips", m.handleListVIPs)
}

// OnMessage обрабатывает входящие сообщения
func (m *LimiterModule) OnMessage(ctx *core.MessageContext) error {
	chatID := ctx.Chat.ID
	userID := ctx.Sender.ID

	// Проверяем VIP-статус
	isVIP, err := m.vipRepo.IsVIP(chatID, userID)
	if err != nil {
		m.logger.Error("failed to check VIP status", zap.Error(err))
		return nil // Не блокируем сообщение из-за ошибки
	}
	if isVIP {
		return nil // VIP не имеет лимитов
	}

	// Определяем тип контента
	contentType := m.detectContentType(ctx.Message)
	if contentType == "unknown" {
		return nil
	}

	// Получаем лимиты
	limits, err := m.contentLimitsRepo.GetLimits(chatID, nil)
	if err != nil {
		m.logger.Error("failed to get limits", zap.Error(err))
		return nil
	}

	// Получаем текущий счётчик
	counter, err := m.contentLimitsRepo.GetCounter(chatID, userID, contentType)
	if err != nil {
		m.logger.Error("failed to get counter", zap.Error(err))
		return nil
	}

	// Проверяем лимит
	var limitValue int
	switch contentType {
	case "text":
		limitValue = limits.LimitText
	case "photo":
		limitValue = limits.LimitPhoto
	case "video":
		limitValue = limits.LimitVideo
	case "sticker":
		limitValue = limits.LimitSticker
	case "animation":
		limitValue = limits.LimitAnimation
	case "voice":
		limitValue = limits.LimitVoice
	case "document":
		limitValue = limits.LimitDocument
	case "audio":
		limitValue = limits.LimitAudio
	case "location":
		limitValue = limits.LimitLocation
	case "contact":
		limitValue = limits.LimitContact
	case "video_note":
		limitValue = limits.LimitVideoNote
	default:
		return nil
	}

	// Отправляем предупреждения в чате, если близко к лимиту
	if limitValue > 0 {
		if counter == limitValue {
			// Лимит достигнут, но сообщение остается
			warning := fmt.Sprintf("⚠️ @%s, лимит на %s достигнут (%d/%d)", ctx.Sender.Username, contentType, counter, limitValue)
			if _, err := ctx.Bot.Send(ctx.Chat, warning); err != nil {
				m.logger.Error("failed to send warning", zap.Error(err))
			}
		} else if counter == limitValue-1 {
			// Остался 1 до лимита
			warning := fmt.Sprintf("⚠️ @%s, остался 1 %s до лимита", ctx.Sender.Username, contentType)
			if _, err := ctx.Bot.Send(ctx.Chat, warning); err != nil {
				m.logger.Error("failed to send warning", zap.Error(err))
			}
		}
	}

	// Если лимит -1 (запрещено) или достигнут
	if limitValue == -1 || (limitValue > 0 && counter > limitValue) {
		// Удаляем сообщение
		if err := ctx.Bot.Delete(ctx.Message); err != nil {
			m.logger.Error("failed to delete message", zap.Error(err))
		}

		// Отправляем предупреждение
		warning := fmt.Sprintf("❌ @%s, лимит на %s достигнут (%d/%d)", ctx.Sender.Username, contentType, counter, limitValue)
		if limitValue == -1 {
			warning = fmt.Sprintf("❌ @%s, %s запрещено в этом чате", ctx.Sender.Username, contentType)
		}
		if _, err := ctx.Bot.Send(ctx.Chat, warning); err != nil {
			m.logger.Error("failed to send warning", zap.Error(err))
		}
		return nil
	}

	// Счётчик уже увеличен модулем statistics, не увеличиваем повторно
	// if err := m.contentLimitsRepo.IncrementCounter(chatID, userID, contentType); err != nil {
	//     m.logger.Error("failed to increment counter", zap.Error(err))
	// }

	return nil
}

// handleMyStats показывает статистику пользователя
func (m *LimiterModule) handleMyStats(c tele.Context) error {
	chatID := c.Chat().ID
	userID := c.Sender().ID

	isVIP, err := m.vipRepo.IsVIP(chatID, userID)
	if err != nil {
		return c.Send("❌ Ошибка получения статуса")
	}

	if isVIP {
		return c.Send("👑 *VIP-статус активен*\n\nВсе лимиты для вас отключены!", &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	limits, err := m.contentLimitsRepo.GetLimits(chatID, &userID)
	if err != nil {
		return c.Send("❌ Не удалось получить лимиты")
	}

	// Все типы контента для вывода
	types := []struct {
		emoji string
		name  string
		field string
		value int
	}{
		{"📝", "Текст", "text", limits.LimitText},
		{"📷", "Фото", "photo", limits.LimitPhoto},
		{"🎬", "Видео", "video", limits.LimitVideo},
		{"😀", "Стикеры", "sticker", limits.LimitSticker},
		{"🎞️", "Гифки", "animation", limits.LimitAnimation},
		{"🎤", "Голосовые", "voice", limits.LimitVoice},
		{"📎", "Документы", "document", limits.LimitDocument},
		{"🎵", "Аудио", "audio", limits.LimitAudio},
		{"📍", "Геолокация", "location", limits.LimitLocation},
		{"👤", "Контакты", "contact", limits.LimitContact},
		{"🔞", "Мат", "banned_words", limits.LimitBannedWords},
		{"🎥", "Кружочки", "video_note", limits.LimitVideoNote},
	}
	text := "📊 Ваша статистика за сегодня:\n\n"
	for _, t := range types {
		counter, _ := m.contentLimitsRepo.GetCounter(chatID, userID, t.field)
		switch {
		case t.value == -1:
			text += fmt.Sprintf("%s %s: %d из 0 (запрещено)\n", t.emoji, t.name, counter)
		case t.value == 0:
			text += fmt.Sprintf("%s %s: %d из 0 (без лимита)\n", t.emoji, t.name, counter)
		default:
			warn := ""
			if counter >= t.value {
				warn = "⛔️"
			} else if counter >= t.value-2 {
				warn = "⚠️"
			}
			text += fmt.Sprintf("%s %s: %d из %d%s\n", t.emoji, t.name, counter, t.value, warn)
		}
	}
	return c.Send(text, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleSetLimit устанавливает лимит
func (m *LimiterModule) handleSetLimit(c tele.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := c.Args()
	if len(args) < 2 || len(args) > 3 {
		return c.Send("Использование: /setlimit <тип> <значение> [@username]")
	}

	contentType := args[0]
	limitValue, err := strconv.Atoi(args[1])
	if err != nil || limitValue < -1 {
		return c.Send("❌ Неверное значение лимита")
	}

	chatID := c.Chat().ID
	var userID *int64

	// Если указан @username, найти пользователя
	if len(args) == 3 {
		return c.Send("❌ Для индивидуальных лимитов ответьте командой на сообщение пользователя")
	}

	// Для индивидуального лимита используем reply
	if c.Message().ReplyTo != nil {
		id := c.Message().ReplyTo.Sender.ID
		userID = &id
	}

	if err := m.contentLimitsRepo.SetLimit(chatID, userID, contentType, limitValue); err != nil {
		return c.Send("❌ Не удалось установить лимит")
	}

	if userID == nil {
		return c.Send(fmt.Sprintf("✅ Лимит для всех установлен: %s = %d", contentType, limitValue))
	}
	return c.Send(fmt.Sprintf("✅ Лимит установлен для пользователя: %s = %d", contentType, limitValue))
}

// handleSetVIP устанавливает VIP-статус
func (m *LimiterModule) handleSetVIP(c tele.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	if c.Message().ReplyTo == nil {
		return c.Send("❌ Ответьте этой командой на сообщение пользователя")
	}

	chatID := c.Chat().ID
	userID := c.Message().ReplyTo.Sender.ID

	args := c.Args()
	reason := "Установлено администратором"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	if err := m.vipRepo.GrantVIP(chatID, userID, c.Sender().ID, reason); err != nil {
		return c.Send("❌ Не удалось установить VIP-статус")
	}

	return c.Send(fmt.Sprintf("✅ VIP-статус установлен для пользователя %d", userID))
}

// handleRemoveVIP снимает VIP-статус
func (m *LimiterModule) handleRemoveVIP(c tele.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	if c.Message().ReplyTo == nil {
		return c.Send("❌ Ответьте этой командой на сообщение пользователя")
	}

	chatID := c.Chat().ID
	userID := c.Message().ReplyTo.Sender.ID

	if err := m.vipRepo.RevokeVIP(chatID, userID); err != nil {
		return c.Send("❌ Не удалось снять VIP-статус")
	}

	return c.Send(fmt.Sprintf("✅ VIP-статус снят с пользователя %d", userID))
}

// handleListVIPs показывает список VIP-пользователей
func (m *LimiterModule) handleListVIPs(c tele.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	chatID := c.Chat().ID
	vips, err := m.vipRepo.ListVIPs(chatID)
	if err != nil {
		return c.Send("❌ Не удалось получить список VIP")
	}

	if len(vips) == 0 {
		return c.Send("ℹ️ В этом чате нет VIP-пользователей")
	}

	text := "👑 *VIP-пользователи:*\n\n"
	for i, vip := range vips {
		text += fmt.Sprintf("%d. User ID: `%d`\n   Причина: %s\n\n", i+1, vip.UserID, vip.Reason)
	}

	return c.Send(text, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
