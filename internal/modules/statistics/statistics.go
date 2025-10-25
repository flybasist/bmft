package statistics

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
)

// StatisticsModule реализует модуль статистики.
// Собирает статистику по сообщениям через таблицу content_counters.
// При каждом сообщении инкрементирует счётчики по типам контента (text, photo, video и т.д.).
// Предоставляет команды: /mystats (личная статистика), /chatstats (статистика чата), /topchat (топ активных).
type StatisticsModule struct {
	db          *sql.DB
	bot         *tele.Bot
	logger      *zap.Logger
	statsRepo   *repositories.StatisticsRepository
	moduleRepo  *repositories.ModuleRepository
	eventRepo   *repositories.EventRepository
	userMsgRepo *repositories.UserMessageRepository
}

// New создаёт новый экземпляр модуля статистики.
func New(
	db *sql.DB,
	statsRepo *repositories.StatisticsRepository,
	moduleRepo *repositories.ModuleRepository,
	eventRepo *repositories.EventRepository,
	logger *zap.Logger,
	bot *tele.Bot,
) *StatisticsModule {
	return &StatisticsModule{
		db:          db,
		logger:      logger,
		statsRepo:   statsRepo,
		moduleRepo:  moduleRepo,
		eventRepo:   eventRepo,
		bot:         bot,
		userMsgRepo: repositories.NewUserMessageRepository(db, logger),
	}
}

// SetAdminUsers устанавливает список администраторов модуля.

// Init инициализирует модуль.
func (m *StatisticsModule) Init(deps core.ModuleDependencies) error {
	m.bot = deps.Bot
	m.logger.Info("statistics module initialized")
	return nil
}

// OnMessage обрабатывает входящее сообщение.
// Русский комментарий: При каждом сообщении инкрементим счётчик в БД.
func (m *StatisticsModule) OnMessage(ctx *core.MessageContext) error {
	if ctx.Message == nil || ctx.Sender == nil {
		m.logger.Warn("statistics: empty message or sender", zap.Any("ctx", ctx))
		return nil
	}

	m.logger.Debug("statistics: received message",
		zap.Int64("chat_id", ctx.Chat.ID),
		zap.Int64("user_id", ctx.Sender.ID),
		zap.String("username", ctx.Sender.Username),
		zap.String("text", ctx.Message.Text),
	)

	contentType := m.detectContentType(ctx.Message)
	m.logger.Debug("statistics: detected content type", zap.String("content_type", contentType))

	// Сохраняем пользователя (upsert)
	err := m.userMsgRepo.UpsertUser(ctx.Sender.ID, ctx.Sender.Username, ctx.Sender.FirstName)
	if err != nil {
		m.logger.Error("statistics: failed to upsert user",
			zap.Int64("user_id", ctx.Sender.ID),
			zap.String("username", ctx.Sender.Username),
			zap.Error(err))
	}

	// Сохраняем сообщение
	err = m.userMsgRepo.InsertMessage(ctx.Chat.ID, ctx.Sender.ID, ctx.Message.ID, contentType)
	if err != nil {
		m.logger.Error("statistics: failed to insert message",
			zap.Int64("chat_id", ctx.Chat.ID),
			zap.Int64("user_id", ctx.Sender.ID),
			zap.Int("message_id", ctx.Message.ID),
			zap.Error(err))
	}

	err = m.statsRepo.IncrementCounter(ctx.Chat.ID, ctx.Sender.ID, contentType)
	if err != nil {
		m.logger.Error("statistics: failed to increment statistics",
			zap.Int64("chat_id", ctx.Chat.ID),
			zap.Int64("user_id", ctx.Sender.ID),
			zap.String("content_type", contentType),
			zap.Error(err))
		return err
	} else {
		m.logger.Debug("statistics: counter incremented",
			zap.Int64("chat_id", ctx.Chat.ID),
			zap.Int64("user_id", ctx.Sender.ID),
			zap.String("content_type", contentType),
		)
	}

	return nil
}

// detectContentType определяет тип контента сообщения.
func (m *StatisticsModule) detectContentType(msg *tele.Message) string {
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
	if msg.Poll != nil {
		return "poll"
	}
	if msg.Text != "" {
		return "text"
	}
	return "other"
}

// Commands возвращает список команд модуля.
func (m *StatisticsModule) Commands() []core.BotCommand {
	return []core.BotCommand{
		{Command: "/mystats", Description: "Моя статистика за сегодня"},
		{Command: "/myweek", Description: "Моя статистика за неделю"},
		{Command: "/chatstats", Description: "Статистика чата (админ)"},
		{Command: "/topchat", Description: "Топ активных пользователей (админ)"},
	}
}

// Enabled проверяет, включен ли модуль для данного чата.
func (m *StatisticsModule) Enabled(chatID int64) (bool, error) {
	enabled, err := m.moduleRepo.IsEnabled(chatID, "statistics")
	if err != nil {
		return false, err
	}
	return enabled, nil
}

// Shutdown выполняет graceful shutdown модуля.
func (m *StatisticsModule) Shutdown() error {
	m.logger.Info("shutting down statistics module")
	return nil
}

// RegisterCommands регистрирует команды модуля в боте.
func (m *StatisticsModule) RegisterCommands(bot *tele.Bot) {
	// /mystats — личная статистика за сегодня
	bot.Handle("/mystats", func(c tele.Context) error {
		// Получаем текущую дату из PostgreSQL
		var today time.Time
		err := m.db.QueryRow("SELECT CURRENT_DATE").Scan(&today)
		if err != nil {
			m.logger.Error("failed to get CURRENT_DATE from PostgreSQL", zap.Error(err))
			today = time.Now().Truncate(24 * time.Hour)
		}
		return m.handleMyStats(c, today)
	})

	// /myweek — личная статистика за неделю
	bot.Handle("/myweek", func(c tele.Context) error {
		return m.handleMyWeekStats(c)
	})
}

// RegisterAdminCommands регистрирует админские команды.
func (m *StatisticsModule) RegisterAdminCommands(bot *tele.Bot) {
	// /chatstats — статистика чата (только админы)
	bot.Handle("/chatstats", func(c tele.Context) error {
		if !m.isChatAdmin(c) {
			return c.Reply("❌ Эта команда доступна только администраторам.")
		}
		var today time.Time
		err := m.db.QueryRow("SELECT CURRENT_DATE").Scan(&today)
		if err != nil {
			m.logger.Error("failed to get CURRENT_DATE from PostgreSQL", zap.Error(err))
			today = time.Now()
		}
		return m.handleChatStats(c, today)
	})

	// /topchat — топ активных пользователей (только админы)
	bot.Handle("/topchat", func(c tele.Context) error {
		if !m.isChatAdmin(c) {
			return c.Reply("❌ Эта команда доступна только администраторам.")
		}
		var today time.Time
		err := m.db.QueryRow("SELECT CURRENT_DATE").Scan(&today)
		if err != nil {
			m.logger.Error("failed to get CURRENT_DATE from PostgreSQL", zap.Error(err))
			today = time.Now()
		}
		return m.handleTopChat(c, today)
	})
}

// handleMyStats обрабатывает команду /mystats.
func (m *StatisticsModule) handleMyStats(c tele.Context, date time.Time) error {
	chatID := c.Chat().ID
	userID := c.Sender().ID

	// Проверяем что модуль включён
	enabled, err := m.Enabled(chatID)
	if err != nil {
		m.logger.Error("failed to check if module enabled", zap.Error(err))
		return c.Reply("Произошла ошибка при проверке модуля.")
	}
	if !enabled {
		return c.Reply("📊 Модуль статистики отключен для этого чата. Админ может включить: /enable statistics")
	}

	// Получаем статистику
	// Передаём только дату (без времени)
	dateOnly := date.Truncate(24 * time.Hour)
	stats, err := m.statsRepo.GetUserStats(userID, chatID, dateOnly)
	if err != nil {
		m.logger.Error("failed to get user stats", zap.Error(err))
		return c.Reply("Произошла ошибка при получении статистики.")
	}

	if stats == nil || stats.TotalCount == 0 {
		return c.Reply(fmt.Sprintf("📊 У вас пока нет статистики за %s", date.Format("02.01.2006")))
	}

	// Форматируем ответ
	msg := m.formatUserStats(stats, date)

	// Логируем событие
	_ = m.eventRepo.Log(chatID, userID, "statistics", "view_my_stats", "User viewed personal statistics")

	return c.Reply(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// handleMyWeekStats обрабатывает команду /myweek.
func (m *StatisticsModule) handleMyWeekStats(c tele.Context) error {
	chatID := c.Chat().ID
	userID := c.Sender().ID

	// Проверяем что модуль включён
	enabled, err := m.Enabled(chatID)
	if err != nil {
		m.logger.Error("failed to check if module enabled", zap.Error(err))
		return c.Reply("Произошла ошибка при проверке модуля.")
	}
	if !enabled {
		return c.Reply("📊 Модуль статистики отключен для этого чата. Админ может включить: /enable statistics")
	}

	// Получаем статистику за неделю
	stats, err := m.statsRepo.GetUserWeeklyStats(userID, chatID)
	if err != nil {
		m.logger.Error("failed to get user weekly stats", zap.Error(err))
		return c.Reply("Произошла ошибка при получении статистики.")
	}

	if stats == nil || stats.TotalCount == 0 {
		return c.Reply("📊 У вас пока нет статистики за последние 7 дней")
	}

	// Форматируем ответ (используем ту же функцию, но с меткой "неделя")
	msg := m.formatUserStatsWeekly(stats)

	// Логируем событие
	_ = m.eventRepo.Log(chatID, userID, "statistics", "view_weekly_stats", "User viewed weekly statistics")

	return c.Reply(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// handleChatStats обрабатывает команду /chatstats.
func (m *StatisticsModule) handleChatStats(c tele.Context, date time.Time) error {
	chatID := c.Chat().ID
	userID := c.Sender().ID

	// Проверяем что модуль включён
	enabled, err := m.Enabled(chatID)
	if err != nil {
		m.logger.Error("failed to check if module enabled", zap.Error(err))
		return c.Reply("Произошла ошибка при проверке модуля.")
	}
	if !enabled {
		return c.Reply("📊 Модуль статистики отключен для этого чата. Админ может включить: /enable statistics")
	}

	// Получаем статистику чата
	stats, err := m.statsRepo.GetChatStats(chatID, date)
	if err != nil {
		m.logger.Error("failed to get chat stats", zap.Error(err))
		return c.Reply("Произошла ошибка при получении статистики.")
	}

	if stats == nil || stats.TotalCount == 0 {
		return c.Reply(fmt.Sprintf("📊 В чате пока нет статистики за %s", date.Format("02.01.2006")))
	}

	// Форматируем ответ
	msg := m.formatChatStats(stats, date)

	// Логируем событие
	_ = m.eventRepo.Log(chatID, userID, "statistics", "view_chat_stats", "Admin viewed chat statistics")

	return c.Reply(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// handleTopChat обрабатывает команду /topchat.
func (m *StatisticsModule) handleTopChat(c tele.Context, date time.Time) error {
	chatID := c.Chat().ID
	userID := c.Sender().ID

	// Проверяем что модуль включён
	enabled, err := m.Enabled(chatID)
	if err != nil {
		m.logger.Error("failed to check if module enabled", zap.Error(err))
		return c.Reply("Произошла ошибка при проверке модуля.")
	}
	if !enabled {
		return c.Reply("📊 Модуль статистики отключен для этого чата. Админ может включить: /enable statistics")
	}

	// Получаем топ-10 пользователей
	topUsers, err := m.statsRepo.GetTopUsers(chatID, date, 10)
	if err != nil {
		m.logger.Error("failed to get top users", zap.Error(err))
		return c.Reply("Произошла ошибка при получении топа пользователей.")
	}

	if len(topUsers) == 0 {
		return c.Reply(fmt.Sprintf("📊 В чате пока нет активности за %s", date.Format("02.01.2006")))
	}

	// Форматируем ответ
	msg := m.formatTopUsers(topUsers, date)

	// Логируем событие
	_ = m.eventRepo.Log(chatID, userID, "statistics", "view_top_users", "Admin viewed top users")

	return c.Reply(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// formatUserStats форматирует статистику пользователя.
func (m *StatisticsModule) formatUserStats(stats *repositories.UserDailyStats, date time.Time) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 <b>Твоя статистика за %s</b>\n\n", date.Format("02.01.2006")))

	if stats.TextCount > 0 {
		sb.WriteString(fmt.Sprintf("💬 Текст: <b>%d</b> сообщений\n", stats.TextCount))
	}
	if stats.PhotoCount > 0 {
		sb.WriteString(fmt.Sprintf("📷 Фото: <b>%d</b>\n", stats.PhotoCount))
	}
	if stats.VideoCount > 0 {
		sb.WriteString(fmt.Sprintf("🎥 Видео: <b>%d</b>\n", stats.VideoCount))
	}
	if stats.StickerCount > 0 {
		sb.WriteString(fmt.Sprintf("😊 Стикеры: <b>%d</b>\n", stats.StickerCount))
	}
	if stats.VoiceCount > 0 {
		sb.WriteString(fmt.Sprintf("🎤 Войс: <b>%d</b>\n", stats.VoiceCount))
	}
	if stats.OtherCount > 0 {
		sb.WriteString(fmt.Sprintf("📎 Прочее: <b>%d</b>\n", stats.OtherCount))
	}

	sb.WriteString(fmt.Sprintf("\n<b>Всего: %d сообщений</b>", stats.TotalCount))

	return sb.String()
}

// formatUserStatsWeekly форматирует недельную статистику пользователя.
func (m *StatisticsModule) formatUserStatsWeekly(stats *repositories.UserDailyStats) string {
	var sb strings.Builder

	sb.WriteString("📊 <b>Твоя статистика за последние 7 дней</b>\n\n")

	if stats.TextCount > 0 {
		sb.WriteString(fmt.Sprintf("💬 Текст: <b>%d</b> сообщений\n", stats.TextCount))
	}
	if stats.PhotoCount > 0 {
		sb.WriteString(fmt.Sprintf("📷 Фото: <b>%d</b>\n", stats.PhotoCount))
	}
	if stats.VideoCount > 0 {
		sb.WriteString(fmt.Sprintf("🎥 Видео: <b>%d</b>\n", stats.VideoCount))
	}
	if stats.StickerCount > 0 {
		sb.WriteString(fmt.Sprintf("😊 Стикеры: <b>%d</b>\n", stats.StickerCount))
	}
	if stats.VoiceCount > 0 {
		sb.WriteString(fmt.Sprintf("🎤 Войс: <b>%d</b>\n", stats.VoiceCount))
	}
	if stats.OtherCount > 0 {
		sb.WriteString(fmt.Sprintf("📎 Прочее: <b>%d</b>\n", stats.OtherCount))
	}

	sb.WriteString(fmt.Sprintf("\n<b>Всего: %d сообщений</b>", stats.TotalCount))

	return sb.String()
}

// formatChatStats форматирует статистику чата.
func (m *StatisticsModule) formatChatStats(stats *repositories.ChatDailyStats, date time.Time) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📊 <b>Статистика чата за %s</b>\n\n", date.Format("02.01.2006")))

	if stats.TextCount > 0 {
		sb.WriteString(fmt.Sprintf("💬 Текст: <b>%d</b> сообщений\n", stats.TextCount))
	}
	if stats.PhotoCount > 0 {
		sb.WriteString(fmt.Sprintf("📷 Фото: <b>%d</b>\n", stats.PhotoCount))
	}
	if stats.VideoCount > 0 {
		sb.WriteString(fmt.Sprintf("🎥 Видео: <b>%d</b>\n", stats.VideoCount))
	}
	if stats.StickerCount > 0 {
		sb.WriteString(fmt.Sprintf("😊 Стикеры: <b>%d</b>\n", stats.StickerCount))
	}
	if stats.VoiceCount > 0 {
		sb.WriteString(fmt.Sprintf("🎤 Войс: <b>%d</b>\n", stats.VoiceCount))
	}
	if stats.OtherCount > 0 {
		sb.WriteString(fmt.Sprintf("📎 Прочее: <b>%d</b>\n", stats.OtherCount))
	}

	sb.WriteString(fmt.Sprintf("\n<b>Всего: %d сообщений</b>\n", stats.TotalCount))
	sb.WriteString(fmt.Sprintf("👥 Активных пользователей: <b>%d</b>", stats.UserCount))

	return sb.String()
}

// formatTopUsers форматирует топ пользователей.
func (m *StatisticsModule) formatTopUsers(topUsers []repositories.TopUser, date time.Time) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🏆 <b>Топ-10 активных за %s</b>\n\n", date.Format("02.01.2006")))

	medals := []string{"🥇", "🥈", "🥉"}

	for i, user := range topUsers {
		medal := ""
		if i < 3 {
			medal = medals[i] + " "
		} else {
			medal = fmt.Sprintf("%d. ", i+1)
		}

		displayName := user.FirstName
		if user.Username != "" {
			displayName = "@" + user.Username
		}

		sb.WriteString(fmt.Sprintf("%s<b>%s</b>: %d сообщений\n", medal, displayName, user.MessageCount))
	}

	return sb.String()
}

// isChatAdmin проверяет, является ли пользователь админом чата в Telegram.
func (m *StatisticsModule) isChatAdmin(c tele.Context) bool {
	if c.Chat().Type == tele.ChatPrivate {
		return true // В личке все команды доступны
	}

	admins, err := m.bot.AdminsOf(c.Chat())
	if err != nil {
		m.logger.Error("failed to get chat admins", zap.Error(err))
		return false
	}

	for _, admin := range admins {
		if admin.User.ID == c.Sender().ID {
			return true
		}
	}

	return false
}
