package limiter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// LimiterModule — модуль контроля лимитов пользователей
type LimiterModule struct {
	limitRepo  *repositories.LimitRepository
	logger     *zap.Logger
	adminUsers []int64
}

// New создаёт новый экземпляр модуля лимитов
func New(limitRepo *repositories.LimitRepository, logger *zap.Logger) *LimiterModule {
	return &LimiterModule{
		limitRepo:  limitRepo,
		logger:     logger,
		adminUsers: []int64{
			// TODO: Заполнить список админов из конфига
		},
	}
}

// Name возвращает имя модуля
func (m *LimiterModule) Name() string {
	return "limiter"
}

// Init инициализирует модуль
func (m *LimiterModule) Init(deps core.ModuleDependencies) error {
	m.logger.Info("limiter module initialized")
	return nil
}

// Commands возвращает список команд модуля
func (m *LimiterModule) Commands() []core.BotCommand {
	return []core.BotCommand{
		{Command: "/limits", Description: "Посмотреть свои лимиты запросов"},
	}
}

// Enabled проверяет, включен ли модуль для чата (всегда включен)
func (m *LimiterModule) Enabled(chatID int64) (bool, error) {
	// Модуль лимитов всегда активен для всех чатов
	return true, nil
}

// OnMessage обрабатывает входящее сообщение
func (m *LimiterModule) OnMessage(ctx *core.MessageContext) error {
	msg := ctx.Message

	// Проверяем лимиты только для личных сообщений или команд AI
	if !m.shouldCheckLimit(msg) {
		return nil
	}

	userID := msg.Sender.ID
	username := msg.Sender.Username

	// Проверяем и инкрементируем лимит
	allowed, info, err := m.limitRepo.CheckAndIncrement(userID, username)
	if err != nil {
		m.logger.Error("failed to check limit",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return err
	}

	// Если лимит исчерпан — блокируем
	if !allowed {
		return m.sendLimitExceededMessage(ctx, info)
	}

	// Если осталось мало запросов — предупреждаем
	if info.DailyRemaining <= 2 || info.MonthlyRemaining <= 10 {
		m.sendLimitWarning(ctx, info)
	}

	return nil
}

// Shutdown завершает работу модуля
func (m *LimiterModule) Shutdown() error {
	m.logger.Info("limiter module shutdown")
	return nil
}

// shouldCheckLimit определяет, нужно ли проверять лимит для сообщения
func (m *LimiterModule) shouldCheckLimit(msg *telebot.Message) bool {
	// Проверяем только личные сообщения
	// В будущем можно добавить проверку для команд AI (GPT:)
	return msg.Private()
}

// sendLimitExceededMessage отправляет сообщение о превышении лимита
func (m *LimiterModule) sendLimitExceededMessage(ctx *core.MessageContext, info *repositories.LimitInfo) error {
	text := fmt.Sprintf(
		"⛔️ *Лимит исчерпан!*\n\n"+
			"📊 Дневной лимит: %d/%d\n"+
			"📊 Месячный лимит: %d/%d\n\n"+
			"Попробуйте позже или обратитесь к администратору.",
		info.DailyUsed, info.DailyLimit,
		info.MonthlyUsed, info.MonthlyLimit,
	)

	return ctx.SendReply(text)
}

// sendLimitWarning отправляет предупреждение о приближении к лимиту
func (m *LimiterModule) sendLimitWarning(ctx *core.MessageContext, info *repositories.LimitInfo) {
	text := fmt.Sprintf(
		"⚠️ *Внимание!* У вас осталось:\n"+
			"📊 Дневной: %d/%d запросов\n"+
			"📊 Месячный: %d/%d запросов",
		info.DailyRemaining, info.DailyLimit,
		info.MonthlyRemaining, info.MonthlyLimit,
	)

	// Не блокируем выполнение если не отправилось
	if err := ctx.SendReply(text); err != nil {
		m.logger.Warn("failed to send limit warning", zap.Error(err))
	}
}

// RegisterCommands регистрирует команды модуля
func (m *LimiterModule) RegisterCommands(bot *telebot.Bot) {
	bot.Handle("/limits", m.handleLimitsCommand)
}

// RegisterAdminCommands регистрирует административные команды
func (m *LimiterModule) RegisterAdminCommands(bot *telebot.Bot) {
	bot.Handle("/setlimit", m.handleSetLimitCommand)
	bot.Handle("/getlimit", m.handleGetLimitCommand)
}

// handleLimitsCommand обрабатывает команду /limits
func (m *LimiterModule) handleLimitsCommand(c telebot.Context) error {
	userID := c.Sender().ID

	// Получаем информацию о лимитах
	info, err := m.limitRepo.GetLimitInfo(userID)
	if err != nil {
		m.logger.Error("failed to get limit info",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return c.Send("❌ Не удалось получить информацию о лимитах")
	}

	text := fmt.Sprintf(
		"📊 *Ваши лимиты:*\n\n"+
			"🔵 *Дневной лимит:*\n"+
			"   Использовано: %d/%d\n"+
			"   Осталось: %d\n\n"+
			"🟢 *Месячный лимит:*\n"+
			"   Использовано: %d/%d\n"+
			"   Осталось: %d\n\n"+
			"💡 _Лимиты обновляются автоматически каждый день/месяц._",
		info.DailyUsed, info.DailyLimit, info.DailyRemaining,
		info.MonthlyUsed, info.MonthlyLimit, info.MonthlyRemaining,
	)

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

// handleSetLimitCommand обрабатывает команду /setlimit
// Формат: /setlimit <user_id> daily|monthly <limit>
func (m *LimiterModule) handleSetLimitCommand(c telebot.Context) error {
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Эта команда доступна только администраторам")
	}

	args := strings.Fields(c.Text())
	if len(args) != 4 {
		return c.Send("📖 *Использование:*\n`/setlimit <user_id> daily|monthly <limit>`\n\n*Примеры:*\n`/setlimit 123456789 daily 20`\n`/setlimit 123456789 monthly 500`",
			&telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	}

	userID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный user_id")
	}

	limitType := args[2]
	limit, err := strconv.Atoi(args[3])
	if err != nil {
		return c.Send("❌ Неверный лимит (должно быть целое число)")
	}

	if limit < 0 {
		return c.Send("❌ Лимит не может быть отрицательным")
	}

	switch limitType {
	case "daily":
		if err := m.limitRepo.SetDailyLimit(userID, limit); err != nil {
			m.logger.Error("failed to set daily limit",
				zap.Int64("user_id", userID),
				zap.Error(err),
			)
			return c.Send("❌ Не удалось установить лимит")
		}
		return c.Send(fmt.Sprintf("✅ Дневной лимит для пользователя `%d` установлен: *%d*",
			userID, limit), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})

	case "monthly":
		if err := m.limitRepo.SetMonthlyLimit(userID, limit); err != nil {
			m.logger.Error("failed to set monthly limit",
				zap.Int64("user_id", userID),
				zap.Error(err),
			)
			return c.Send("❌ Не удалось установить лимит")
		}
		return c.Send(fmt.Sprintf("✅ Месячный лимит для пользователя `%d` установлен: *%d*",
			userID, limit), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})

	default:
		return c.Send("❌ Тип лимита должен быть: `daily` или `monthly`",
			&telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	}
}

// handleGetLimitCommand обрабатывает команду /getlimit
// Формат: /getlimit <user_id>
func (m *LimiterModule) handleGetLimitCommand(c telebot.Context) error {
	if !m.isAdmin(c.Sender().ID) {
		return c.Send("❌ Эта команда доступна только администраторам")
	}

	args := strings.Fields(c.Text())
	if len(args) != 2 {
		return c.Send("📖 *Использование:*\n`/getlimit <user_id>`\n\n*Пример:*\n`/getlimit 123456789`",
			&telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
	}

	userID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil {
		return c.Send("❌ Неверный user_id")
	}

	info, err := m.limitRepo.GetLimitInfo(userID)
	if err != nil {
		m.logger.Error("failed to get limit info",
			zap.Int64("user_id", userID),
			zap.Error(err),
		)
		return c.Send("❌ Не удалось получить информацию")
	}

	text := fmt.Sprintf(
		"📊 *Лимиты пользователя* `%d`:\n\n"+
			"🔵 *Дневной:* %d/%d (осталось %d)\n"+
			"🟢 *Месячный:* %d/%d (осталось %d)",
		userID,
		info.DailyUsed, info.DailyLimit, info.DailyRemaining,
		info.MonthlyUsed, info.MonthlyLimit, info.MonthlyRemaining,
	)

	return c.Send(text, &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
}

// isAdmin проверяет, является ли пользователь администратором
func (m *LimiterModule) isAdmin(userID int64) bool {
	for _, id := range m.adminUsers {
		if id == userID {
			return true
		}
	}
	return false
}

// SetAdminUsers устанавливает список администраторов
func (m *LimiterModule) SetAdminUsers(adminUsers []int64) {
	m.adminUsers = adminUsers
	m.logger.Info("admin users updated", zap.Int("count", len(adminUsers)))
}
