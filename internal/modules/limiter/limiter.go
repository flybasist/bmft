package limiter

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

type LimiterModule struct {
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	logger            *zap.Logger
	bot               *telebot.Bot
}

func New(
	vipRepo *repositories.VIPRepository,
	contentLimitsRepo *repositories.ContentLimitsRepository,
	logger *zap.Logger,
	bot *telebot.Bot,
) *LimiterModule {
	return &LimiterModule{
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		logger:            logger,
		bot:               bot,
	}
}

func (m *LimiterModule) Name() string {
	return "limiter"
}

func (m *LimiterModule) Init(deps core.ModuleDependencies) error {
	m.logger.Info("limiter module initialized")
	return nil
}

func (m *LimiterModule) Commands() []core.BotCommand {
	return []core.BotCommand{
		{Command: "/mystats", Description: "Посмотреть свою статистику и лимиты"},
	}
}

func (m *LimiterModule) Enabled(chatID int64) (bool, error) {
	return true, nil
}

func (m *LimiterModule) OnMessage(ctx *core.MessageContext) error {
	msg := ctx.Message
	if msg.Private() {
		return nil
	}

	chatID := msg.Chat.ID
	userID := msg.Sender.ID

	m.logger.Debug("limiter: received message",
		zap.Int64("chat_id", chatID),
		zap.Int64("user_id", userID),
		zap.String("username", msg.Sender.Username),
		zap.String("text", msg.Text),
	)

	isVIP, err := m.vipRepo.IsVIP(chatID, userID)
	if err != nil {
		m.logger.Error("limiter: failed to check VIP status", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.Error(err))
	}
	if isVIP {
		m.logger.Debug("limiter: user is VIP, skipping limits", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID))
		return nil
	}

	contentType := m.detectContentType(msg)
	m.logger.Debug("limiter: detected content type", zap.String("content_type", contentType))
	if contentType == "" {
		m.logger.Warn("limiter: unknown content type, skipping", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID))
		return nil
	}

	limit, err := m.contentLimitsRepo.GetLimitForContentType(chatID, &userID, contentType)
	if err != nil {
		m.logger.Error("limiter: failed to get limit", zap.Error(err), zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		return nil
	}
	m.logger.Debug("limiter: got limit", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType), zap.Int("limit", limit))

	if limit == -1 {
		m.logger.Info("limiter: content type is banned, deleting message", zap.Int64("chat_id", chatID), zap.String("content_type", contentType))
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("limiter: failed to delete banned message", zap.Error(err), zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		} else {
			m.logger.Info("limiter: banned message deleted successfully", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		}
		return nil
	}

	if limit == 0 {
		m.logger.Debug("limiter: limit is zero, skipping", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		return nil
	}

	counter, err := m.contentLimitsRepo.GetCounter(chatID, userID, contentType)
	if err != nil {
		m.logger.Error("limiter: failed to get counter", zap.Error(err), zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		return nil
	}
	m.logger.Debug("limiter: got counter", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType), zap.Int("counter", counter))

	if counter >= limit {
		m.logger.Info("limiter: content limit exceeded, deleting message", zap.Int("counter", counter), zap.Int("limit", limit), zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("limiter: failed to delete message after limit exceeded", zap.Error(err), zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		} else {
			m.logger.Info("limiter: message deleted after limit exceeded", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
		}
		return ctx.SendReply(fmt.Sprintf("⛔️ @%s, вы превысили дневной лимит (%d/%d)", msg.Sender.Username, counter, limit))
	}

	if err := m.contentLimitsRepo.IncrementCounter(chatID, userID, contentType); err != nil {
		m.logger.Error("limiter: failed to increment counter", zap.Error(err), zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
	} else {
		m.logger.Debug("limiter: counter incremented", zap.Int64("chat_id", chatID), zap.Int64("user_id", userID), zap.String("content_type", contentType))
	}

	newCounter := counter + 1
	if newCounter == limit-2 || newCounter == limit-1 {
		_ = ctx.SendReply(fmt.Sprintf("⚠️ @%s, у вас осталось %d из %d", msg.Sender.Username, limit-newCounter, limit))
	}

	return nil
}

func (m *LimiterModule) Shutdown() error {
	m.logger.Info("limiter module shutdown")
	return nil
}

func (m *LimiterModule) detectContentType(msg *telebot.Message) string {
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
	return ""
}

func (m *LimiterModule) RegisterCommands(bot *telebot.Bot) {
	bot.Handle("/mystats", m.handleMyStats)
}

func (m *LimiterModule) RegisterAdminCommands(bot *telebot.Bot) {
	bot.Handle("/setlimit", m.handleSetLimit)
	bot.Handle("/setvip", m.handleSetVIP)
	bot.Handle("/removevip", m.handleRemoveVIP)
	bot.Handle("/listvips", m.handleListVIPs)
}

func (m *LimiterModule) handleMyStats(c telebot.Context) error {
	if c.Message().Private() {
		return c.Send("📊 В личных сообщениях лимиты не применяются.")
	}

	chatID := c.Chat().ID
	userID := c.Sender().ID

	isVIP, err := m.vipRepo.IsVIP(chatID, userID)
	if err != nil {
		return c.Send("❌ Ошибка получения статуса")
	}

	if isVIP {
		return c.Send("👑 *VIP-статус активен*\n\nВсе лимиты для вас отключены!", &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	}

	limits, err := m.contentLimitsRepo.GetLimits(chatID, &userID)
	if err != nil {
		return c.Send("❌ Не удалось получить лимиты")
	}

	text := "📊 *Ваша статистика:*\n\n"
	types := []struct {
		name, field string
		value       int
	}{
		{"текст", "text", limits.LimitText},
		{"фото", "photo", limits.LimitPhoto},
		{"видео", "video", limits.LimitVideo},
		{"стикеры", "sticker", limits.LimitSticker},
	}

	for _, t := range types {
		if t.value == -1 {
			text += fmt.Sprintf("🚫 %s: *ЗАПРЕЩЕНО*\n", t.name)
		} else if t.value == 0 {
			text += fmt.Sprintf("♾ %s: *без лимита*\n", t.name)
		} else {
			counter, _ := m.contentLimitsRepo.GetCounter(chatID, userID, t.field)
			emoji := "✅"
			if counter >= t.value {
				emoji = "⛔️"
			} else if counter >= t.value-2 {
				emoji = "⚠️"
			}
			text += fmt.Sprintf("%s %s: %d из %d\n", emoji, t.name, counter, t.value)
		}
	}

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

func (m *LimiterModule) handleSetLimit(c telebot.Context) error {
	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	args := strings.Fields(c.Text())
	if len(args) != 3 {
		return c.Send("Использование: /setlimit <type> <value>\nПример: /setlimit photo 5 (для всех) или /setlimit voice 10 (через ответ на сообщение пользователя)")
	}

	contentType := args[1]
	limitValue, err := strconv.Atoi(args[2])
	if err != nil || limitValue < -1 {
		return c.Send("❌ Неверное значение лимита")
	}

	chatID := c.Chat().ID
	var userID *int64
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

func (m *LimiterModule) handleSetVIP(c telebot.Context) error {
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
	grantedBy := c.Sender().ID
	reason := "VIP статус предоставлен администратором"

	if err := m.vipRepo.GrantVIP(chatID, userID, grantedBy, reason); err != nil {
		return c.Send("❌ Не удалось предоставить VIP-статус")
	}

	return c.Send("👑 VIP-статус предоставлен!")
}

func (m *LimiterModule) handleRemoveVIP(c telebot.Context) error {
	admins, err := m.bot.AdminsOf(c.Chat())
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	isAdmin := false
	for _, admin := range admins {
		if admin.User.ID == c.Sender().ID {
			isAdmin = true
			break
		}
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
		if err == sql.ErrNoRows {
			return c.Send("ℹ️ У этого пользователя нет VIP-статуса")
		}
		return c.Send("❌ Не удалось отозвать VIP-статус")
	}

	return c.Send("✅ VIP-статус отозван")
}

func (m *LimiterModule) handleListVIPs(c telebot.Context) error {
	admins, err := m.bot.AdminsOf(c.Chat())
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	isAdmin := false
	for _, admin := range admins {
		if admin.User.ID == c.Sender().ID {
			isAdmin = true
			break
		}
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

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}
