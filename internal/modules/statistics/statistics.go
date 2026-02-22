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
	// /statistics — справка по модулю
	bot.Handle("/statistics", func(c tele.Context) error {
		msg := "📊 <b>Модуль Statistics</b> — Статистика активности\n\n"
		msg += "Собирает и анализирует данные об активности пользователей в чате.\n\n"
		msg += "<b>Доступные команды:</b>\n\n"

		msg += "🔹 <code>/myweek</code> — Ваша статистика за неделю\n"
		msg += "   Подробная аналитика активности за последние 7 дней\n\n"

		msg += "🔹 <code>/chatstats</code> — Статистика чата (только админы)\n"
		msg += "   Общая статистика активности всего чата\n\n"

		msg += "🔹 <code>/topchat</code> — Топ активных пользователей (только админы)\n"
		msg += "   Рейтинг самых активных участников чата\n\n"

		msg += "⚙️ <b>Работа с топиками:</b>\n"
		msg += "• Команда в <b>топике</b> — статистика для топика\n"
		msg += "• Команда в <b>основном чате</b> — статистика для всего чата\n\n"

		msg += "💡 <i>Подсказка:</i> Статистика собирается автоматически для всех сообщений."

		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
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
		isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
		if err != nil {
			return c.Reply("Ошибка проверки прав администратора")
		}
		if !isAdmin {
			return c.Reply("❌ Эта команда доступна только администраторам.")
		}

		// Логируем событие
		_ = m.eventRepo.Log(c.Chat().ID, c.Sender().ID, "statistics", "admin_view_chat_stats",
			"Admin viewed chat statistics")

		var today time.Time
		err = m.db.QueryRow("SELECT CURRENT_DATE").Scan(&today)
		if err != nil {
			m.logger.Error("failed to get CURRENT_DATE from PostgreSQL", zap.Error(err))
			today = time.Now()
		}
		return m.handleChatStats(c, today)
	})

	// /topchat — топ активных пользователей (только админы)
	bot.Handle("/topchat", func(c tele.Context) error {
		isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
		if err != nil {
			m.logger.Error("failed to check user admin status", zap.Error(err))
			return c.Reply("❌ Не удалось проверить права доступа.")
		}
		if !isAdmin {
			return c.Reply("❌ Команда доступна только администраторам.")
		}

		// Логируем событие
		_ = m.eventRepo.Log(c.Chat().ID, c.Sender().ID, "statistics", "admin_view_top_chat",
			"Admin viewed top chat users")

		var today time.Time
		err = m.db.QueryRow("SELECT CURRENT_DATE").Scan(&today)
		if err != nil {
			m.logger.Error("failed to get CURRENT_DATE from PostgreSQL", zap.Error(err))
			today = time.Now()
		}
		return m.handleTopChat(c, today)
	})
}

// handleMyWeekStats обрабатывает команду /myweek.
func (m *StatisticsModule) handleMyWeekStats(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleMyWeekStats called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	userID := c.Sender().ID

	// Статистика за последние 7 дней
	days := 7

	stats, err := m.messageRepo.GetUserStats(chatID, threadID, userID, days)
	if err != nil {
		m.logger.Error("failed to get user week stats", zap.Error(err))
		return c.Reply("❌ Не удалось получить статистику")
	}

	if len(stats) == 0 {
		if threadID != 0 {
			return c.Reply("ℹ️ У вас пока нет сообщений в этом топике за последнюю неделю")
		}
		return c.Reply("ℹ️ У вас пока нет сообщений в этом чате за последнюю неделю")
	}

	// Форматируем ответ
	var sb strings.Builder

	if threadID != 0 {
		sb.WriteString("📊 <b>Твоя статистика за неделю (для этого топика)</b>\n\n")
	} else {
		sb.WriteString("📊 <b>Твоя статистика за неделю (для всего чата)</b>\n\n")
	}

	contentTypeEmoji := map[string]string{
		"text":       "💬",
		"photo":      "📷",
		"video":      "🎥",
		"sticker":    "😊",
		"animation":  "🎬",
		"voice":      "🎤",
		"video_note": "📹",
		"audio":      "🎵",
		"document":   "📄",
		"location":   "📍",
		"contact":    "👤",
		"poll":       "📊",
	}

	total := 0
	for contentType, count := range stats {
		if count > 0 {
			emoji, ok := contentTypeEmoji[contentType]
			if !ok {
				emoji = "📎"
			}
			sb.WriteString(fmt.Sprintf("%s %s: <b>%d</b>\n", emoji, contentType, count))
			total += count
		}
	}

	sb.WriteString(fmt.Sprintf("\n<b>Всего:</b> %d сообщений за 7 дней", total))

	return c.Reply(sb.String(), &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// handleChatStats обрабатывает команду /chatstats.
func (m *StatisticsModule) handleChatStats(c tele.Context, date time.Time) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleChatStats called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	// По умолчанию - статистика за сегодня (1 день)
	days := 1

	// Получаем статистику чата
	stats, err := m.messageRepo.GetChatStats(chatID, threadID, days)
	if err != nil {
		m.logger.Error("failed to get chat stats", zap.Error(err))
		return c.Reply("❌ Не удалось получить статистику чата")
	}

	if len(stats) == 0 {
		if threadID != 0 {
			return c.Reply("ℹ️ В этом топике пока нет сообщений за сегодня")
		}
		return c.Reply("ℹ️ В этом чате пока нет сообщений за сегодня")
	}

	// Форматируем ответ
	var sb strings.Builder

	if threadID != 0 {
		sb.WriteString(fmt.Sprintf("📊 <b>Статистика топика за %s</b>\n\n", date.Format("02.01.2006")))
	} else {
		sb.WriteString(fmt.Sprintf("📊 <b>Статистика чата за %s</b>\n\n", date.Format("02.01.2006")))
	}

	contentTypeEmoji := map[string]string{
		"text":       "💬",
		"photo":      "📷",
		"video":      "🎥",
		"sticker":    "😊",
		"animation":  "🎬",
		"voice":      "🎤",
		"video_note": "📹",
		"audio":      "🎵",
		"document":   "📄",
		"location":   "📍",
		"contact":    "👤",
		"poll":       "📊",
	}

	total := 0
	for contentType, count := range stats {
		emoji, ok := contentTypeEmoji[contentType]
		if !ok {
			emoji = "📎"
		}
		sb.WriteString(fmt.Sprintf("%s %s: <b>%d</b>\n", emoji, contentType, count))
		total += count
	}

	sb.WriteString(fmt.Sprintf("\n<b>Всего:</b> %d сообщений", total))

	return c.Reply(sb.String(), &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// handleTopChat обрабатывает команду /topchat.
func (m *StatisticsModule) handleTopChat(c tele.Context, date time.Time) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleTopChat called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	// По умолчанию - топ за сегодня (1 день), 10 пользователей
	days := 1
	limit := 10

	// Получаем топ пользователей
	topUsers, err := m.messageRepo.GetChatTopUsers(chatID, threadID, days, limit)
	if err != nil {
		m.logger.Error("failed to get chat top users", zap.Error(err))
		return c.Reply("❌ Не удалось получить топ активных пользователей")
	}

	if len(topUsers) == 0 {
		if threadID != 0 {
			return c.Reply("ℹ️ В этом топике пока нет сообщений за сегодня")
		}
		return c.Reply("ℹ️ В этом чате пока нет сообщений за сегодня")
	}

	// Форматируем ответ
	var sb strings.Builder

	if threadID != 0 {
		sb.WriteString(fmt.Sprintf("🏆 <b>Топ активных участников топика за %s</b>\n\n", date.Format("02.01.2006")))
	} else {
		sb.WriteString(fmt.Sprintf("🏆 <b>Топ активных участников чата за %s</b>\n\n", date.Format("02.01.2006")))
	}

	medals := []string{"🥇", "🥈", "🥉"}

	for i, userStat := range topUsers {
		// Получаем информацию о пользователе
		chatMember, err := m.bot.ChatMemberOf(c.Chat(), &tele.User{ID: userStat.UserID})
		username := fmt.Sprintf("User #%d", userStat.UserID)
		if err == nil && chatMember != nil && chatMember.User != nil {
			if chatMember.User.Username != "" {
				username = "@" + chatMember.User.Username
			} else if chatMember.User.FirstName != "" {
				username = chatMember.User.FirstName
				if chatMember.User.LastName != "" {
					username += " " + chatMember.User.LastName
				}
			}
		}

		medal := ""
		if i < 3 {
			medal = medals[i] + " "
		}

		sb.WriteString(fmt.Sprintf("%s<b>%d.</b> %s — <b>%d</b> сообщений\n",
			medal, i+1, username, userStat.MessageCount))
	}

	return c.Reply(sb.String(), &tele.SendOptions{ParseMode: tele.ModeHTML})
}
