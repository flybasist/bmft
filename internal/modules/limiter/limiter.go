package limiter

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/flybasist/bmft/internal/core"
	"github.com/flybasist/bmft/internal/postgresql/repositories"
	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// LimiterModule управляет лимитами на контент в чатах.
// Русский комментарий: v0.8.0 - использует messageRepo.GetTodayCountByType()
// для подсчёта сообщений вместо отдельной таблицы content_counters.
type LimiterModule struct {
	db                *sql.DB
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	messageRepo       *repositories.MessageRepository
	eventRepo         *repositories.EventRepository
	logger            *zap.Logger
	bot               *tele.Bot
}

// New создаёт новый экземпляр LimiterModule
func New(db *sql.DB, vipRepo *repositories.VIPRepository, contentLimitsRepo *repositories.ContentLimitsRepository, eventRepo *repositories.EventRepository, logger *zap.Logger, bot *tele.Bot) *LimiterModule {
	return &LimiterModule{
		db:                db,
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		messageRepo:       repositories.NewMessageRepository(db, logger),
		eventRepo:         eventRepo,
		logger:            logger,
		bot:               bot,
	}
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
	// /limiter — справка по модулю
	bot.Handle("/limiter", func(c tele.Context) error {
		m.logger.Info("handling /limiter command",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int64("user_id", c.Sender().ID))

		msg := "🚦 <b>Модуль Limiter</b> — Контроль лимитов контента\n\n"
		msg += "Устанавливает ограничения на количество сообщений разных типов в день.\n\n"
		msg += "<b>Доступные команды:</b>\n\n"

		msg += "🔹 <code>/setlimit &lt;тип&gt; &lt;кол-во&gt;</code> — Установить лимит (только админы)\n\n"
		msg += "<b>Доступные типы:</b>\n"
		msg += "• <code>text</code>, <code>photo</code>, <code>video</code>, <code>sticker</code>\n"
		msg += "• <code>animation</code>, <code>voice</code>, <code>video_note</code>, <code>audio</code>\n"
		msg += "• <code>document</code>, <code>location</code>, <code>contact</code>\n"
		msg += "• <code>banned_words</code> — лимит на маты из profanityfilter\n\n"
		msg += "📌 Примеры:\n"
		msg += "• <code>/setlimit photo 10</code> — макс 10 фото/день для всех\n"
		msg += "• <code>/setlimit sticker 20</code> — макс 20 стикеров/день\n"
		msg += "• <code>/setlimit banned_words 3</code> — 3 мата/день (потом бан)\n"
		msg += "• <code>/setlimit text 0</code> — 0 = отключить лимит\n"
		msg += "• <code>/setlimit photo -1</code> — -1 = полный запрет\n\n"

		msg += "🔹 <code>/mystats</code> — Показать ваши текущие лимиты\n"
		msg += "   Отображает все установленные лимиты и сколько осталось до превышения\n"
		msg += "   📌 Пример: <code>/mystats</code>\n\n"

		msg += "🔹 <code>/getlimit</code> — Посмотреть текущие лимиты чата\n"
		msg += "   Показывает все установленные лимиты для этого топика или чата\n"
		msg += "   📌 Пример: <code>/getlimit</code>\n\n"

		msg += "🔹 <code>/setvip @username</code> — Выдать VIP-статус (только админы)\n"
		msg += "   VIP-пользователи игнорируют все лимиты\n"
		msg += "   📌 Примеры:\n"
		msg += "   • <code>/setvip @username</code> — выдать VIP\n"
		msg += "   • Ответить на сообщение и написать <code>/setvip</code>\n\n"

		msg += "🔹 <code>/removevip @username</code> — Снять VIP-статус (только админы)\n"
		msg += "   📌 Примеры:\n"
		msg += "   • <code>/removevip @username</code>\n"
		msg += "   • Ответить на сообщение и написать <code>/removevip</code>\n\n"

		msg += "🔹 <code>/listvips</code> — Список всех VIP-пользователей\n"
		msg += "   📌 Пример: <code>/listvips</code>\n\n"

		msg += "⚙️ <b>Работа с топиками:</b>\n"
		msg += "• Команда в <b>топике</b> настраивает лимиты только для этого топика\n"
		msg += "• Команда в <b>основном чате</b> настраивает лимиты для всего чата\n"
		msg += "• Если лимит для топика не установлен, используется общий лимит чата\n\n"

		msg += "⚠️ <i>Предупреждения:</i> После 2-х превышений лимита пользователь получает предупреждение."

		m.logger.Info("sending /limiter help message",
			zap.Int64("chat_id", c.Chat().ID),
			zap.Int("msg_length", len(msg)))

		return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
	})

	bot.Handle("/mystats", m.handleMyStats)
	bot.Handle("/getlimit", m.handleGetLimit)
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
	threadID := ctx.Message.ThreadID
	userID := ctx.Sender.ID

	// Проверяем VIP-статус (с fallback: топик → чат)
	isVIP, err := m.vipRepo.IsVIP(chatID, threadID, userID)
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

	// Получаем лимиты (с fallback: топик → чат)
	limits, err := m.contentLimitsRepo.GetLimits(chatID, threadID, nil)
	if err != nil {
		m.logger.Error("failed to get limits", zap.Error(err))
		return nil
	}

	// Получаем текущий счётчик из messages (за сегодня)
	counter, err := m.messageRepo.GetTodayCountByType(chatID, threadID, userID, contentType)
	if err != nil {
		m.logger.Error("failed to get today counter", zap.Error(err))
		return nil
	}

	// counter уже включает текущее сообщение (так как Statistics его уже сохранил)
	// Но если Statistics ещё не обработал, добавляем +1
	// TODO: Правильнее координировать порядок модулей через pipeline
	counter++ // Предполагаем что текущее сообщение ещё не учтено

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
		// Логируем превышение лимита
		m.logger.Info("limit exceeded, deleting message",
			zap.Int64("user_id", ctx.Sender.ID),
			zap.String("username", ctx.Sender.Username),
			zap.Int64("chat_id", ctx.Chat.ID),
			zap.String("content_type", contentType),
			zap.Int("counter", counter),
			zap.Int("limit", limitValue))

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

	return nil
}

// handleMyStats показывает статистику пользователя
func (m *LimiterModule) handleMyStats(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := int(core.GetThreadID(m.db, c))

	m.logger.Info("handleMyStats called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	userID := c.Sender().ID

	isVIP, err := m.vipRepo.IsVIP(chatID, threadID, userID)
	if err != nil {
		return c.Send("❌ Ошибка получения статуса")
	}

	var vipScope string
	if isVIP {
		if threadID != 0 {
			vipScope = " (топик)"
		} else {
			vipScope = " (весь чат)"
		}
		return c.Send(fmt.Sprintf("👑 *VIP-статус активен%s*\n\nВсе лимиты для вас отключены!", vipScope), &tele.SendOptions{ParseMode: tele.ModeMarkdown})
	}

	limits, err := m.contentLimitsRepo.GetLimits(chatID, threadID, &userID)
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

	var scope string
	if threadID != 0 {
		scope = " (для этого топика)"
	} else {
		scope = " (для всего чата)"
	}

	text := fmt.Sprintf("📊 Ваша статистика за сегодня%s:\n\n", scope)
	for _, t := range types {
		counter, _ := m.messageRepo.GetTodayCountByType(chatID, threadID, userID, t.field)
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

// handleGetLimit показывает текущие лимиты чата (доступно всем пользователям)
func (m *LimiterModule) handleGetLimit(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := int(core.GetThreadID(m.db, c))

	m.logger.Info("handleGetLimit called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	limits, err := m.contentLimitsRepo.GetLimits(chatID, threadID, nil)
	if err != nil {
		return c.Send("❌ Не удалось получить лимиты")
	}

	// Все типы контента для вывода
	types := []struct {
		emoji string
		name  string
		value int
	}{
		{"📝", "Текст", limits.LimitText},
		{"📷", "Фото", limits.LimitPhoto},
		{"🎬", "Видео", limits.LimitVideo},
		{"😀", "Стикеры", limits.LimitSticker},
		{"🎞️", "Гифки", limits.LimitAnimation},
		{"🎤", "Голосовые", limits.LimitVoice},
		{"📎", "Документы", limits.LimitDocument},
		{"🎵", "Аудио", limits.LimitAudio},
		{"📍", "Геолокация", limits.LimitLocation},
		{"👤", "Контакты", limits.LimitContact},
		{"🔞", "Мат", limits.LimitBannedWords},
		{"🎥", "Кружочки", limits.LimitVideoNote},
	}

	var scope string
	if threadID != 0 {
		scope = " (для этого топика)"
	} else {
		scope = " (для всего чата)"
	}

	text := fmt.Sprintf("🚦 Установленные лимиты%s:\n\n", scope)
	hasLimits := false
	for _, t := range types {
		switch {
		case t.value == -1:
			text += fmt.Sprintf("%s %s: запрещено ⛔️\n", t.emoji, t.name)
			hasLimits = true
		case t.value > 0:
			text += fmt.Sprintf("%s %s: %d в день\n", t.emoji, t.name, t.value)
			hasLimits = true
		}
	}

	if !hasLimits {
		text += "✅ Лимиты не установлены. Все типы контента разрешены без ограничений.\n"
	}

	text += "\n💡 Используйте `/mystats` чтобы посмотреть вашу личную статистику"

	return c.Send(text, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleSetLimit устанавливает лимит
func (m *LimiterModule) handleSetLimit(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := int(core.GetThreadID(m.db, c))

	m.logger.Info("handleSetLimit called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

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

	if err := m.contentLimitsRepo.SetLimit(chatID, threadID, userID, contentType, limitValue); err != nil {
		return c.Send("❌ Не удалось установить лимит")
	}

	// Логируем событие
	details := fmt.Sprintf("Set limit: %s=%d (chat=%d, thread=%d)", contentType, limitValue, chatID, threadID)
	if userID != nil {
		details = fmt.Sprintf("Set limit: %s=%d for user %d (chat=%d, thread=%d)", contentType, limitValue, *userID, chatID, threadID)
	}
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "limiter", "set_limit", details)

	var msg string
	if threadID != 0 {
		// Команда выполнена в топике
		if userID == nil {
			msg = fmt.Sprintf("✅ Лимит установлен для **этого топика**\n\n%s: %d в день\n\n💡 Для настройки всего чата используйте команду в основном чате (не в топике)", contentType, limitValue)
		} else {
			msg = fmt.Sprintf("✅ Персональный лимит установлен для пользователя **в этом топике**\n\n%s: %d в день\n\n💡 Для настройки на весь чат используйте команду в основном чате", contentType, limitValue)
		}
	} else {
		// Команда выполнена в основном чате
		if userID == nil {
			msg = fmt.Sprintf("✅ Лимит установлен для **всего чата**\n\n%s: %d в день\n\n💡 Для настройки конкретного топика используйте команду внутри топика", contentType, limitValue)
		} else {
			msg = fmt.Sprintf("✅ Персональный лимит установлен для пользователя **во всём чате**\n\n%s: %d в день", contentType, limitValue)
		}
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleSetVIP устанавливает VIP-статус
func (m *LimiterModule) handleSetVIP(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := int(core.GetThreadID(m.db, c))

	m.logger.Info("handleSetVIP called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

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

	userID := c.Message().ReplyTo.Sender.ID

	args := c.Args()
	reason := "Установлено администратором"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	if err := m.vipRepo.GrantVIP(chatID, threadID, userID, c.Sender().ID, reason); err != nil {
		return c.Send("❌ Не удалось установить VIP-статус")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "limiter", "grant_vip",
		fmt.Sprintf("Granted VIP to user %d (chat=%d, thread=%d, reason: %s)", userID, chatID, threadID, reason))

	username := c.Message().ReplyTo.Sender.Username
	if username == "" {
		username = c.Message().ReplyTo.Sender.FirstName
	}

	var msg string
	if threadID != 0 {
		msg = fmt.Sprintf("✅ VIP-статус выдан пользователю @%s **для этого топика**\n\n💡 Теперь он игнорирует все лимиты в этом топике.\nДля выдачи VIP на весь чат используйте команду в основном чате.", username)
	} else {
		msg = fmt.Sprintf("✅ VIP-статус выдан пользователю @%s **для всего чата**\n\n💡 Теперь он игнорирует все лимиты во всех топиках.", username)
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleRemoveVIP снимает VIP-статус
func (m *LimiterModule) handleRemoveVIP(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := int(core.GetThreadID(m.db, c))

	m.logger.Info("handleRemoveVIP called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

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

	userID := c.Message().ReplyTo.Sender.ID

	if err := m.vipRepo.RevokeVIP(chatID, threadID, userID); err != nil {
		return c.Send("❌ Не удалось снять VIP-статус")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "limiter", "revoke_vip",
		fmt.Sprintf("Revoked VIP from user %d (chat=%d, thread=%d)", userID, chatID, threadID))

	username := c.Message().ReplyTo.Sender.Username
	if username == "" {
		username = c.Message().ReplyTo.Sender.FirstName
	}

	var msg string
	if threadID != 0 {
		msg = fmt.Sprintf("✅ VIP-статус снят с @%s **для этого топика**\n\n💡 Чтобы снять VIP на весь чат, используйте команду в основном чате.", username)
	} else {
		msg = fmt.Sprintf("✅ VIP-статус снят с @%s **для всего чата**", username)
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleListVIPs показывает список VIP-пользователей
func (m *LimiterModule) handleListVIPs(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := int(core.GetThreadID(m.db, c))

	m.logger.Info("handleListVIPs called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	isAdmin, err := core.IsUserAdmin(m.bot, c.Chat(), c.Sender().ID)
	if err != nil {
		return c.Send("Ошибка проверки прав администратора")
	}
	if !isAdmin {
		return c.Send("❌ Команда доступна только администраторам")
	}

	vips, err := m.vipRepo.ListVIPs(chatID, threadID)
	if err != nil {
		return c.Send("❌ Не удалось получить список VIP")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "limiter", "list_vips",
		fmt.Sprintf("Admin viewed VIP list (chat=%d, thread=%d)", chatID, threadID))

	if len(vips) == 0 {
		location := "чате"
		if threadID != 0 {
			location = "топике"
		}
		return c.Send(fmt.Sprintf("ℹ️ В этом %s нет VIP-пользователей", location))
	}

	location := "чата"
	if threadID != 0 {
		location = "топика"
	}

	text := fmt.Sprintf("👑 *VIP-пользователи %s:*\n\n", location)
	for i, vip := range vips {
		text += fmt.Sprintf("%d. User ID: `%d`\n   Причина: %s\n\n", i+1, vip.UserID, vip.Reason)
	}

	return c.Send(text, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}
