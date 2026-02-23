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
// Использует messageRepo.GetTodayCountByType() для подсчёта сообщений.
type LimiterModule struct {
	db                *sql.DB
	vipRepo           *repositories.VIPRepository
	contentLimitsRepo *repositories.ContentLimitsRepository
	messageRepo       *repositories.MessageRepository
	eventRepo         *repositories.EventRepository
	logger            *zap.Logger
	bot               *tele.Bot
}

// New создаёт новый экземпляр LimiterModule.
// messageRepo — общий экземпляр из initModules (не создаём дубликат).
func New(db *sql.DB, vipRepo *repositories.VIPRepository, contentLimitsRepo *repositories.ContentLimitsRepository, messageRepo *repositories.MessageRepository, eventRepo *repositories.EventRepository, logger *zap.Logger, bot *tele.Bot) *LimiterModule {
	return &LimiterModule{
		db:                db,
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		messageRepo:       messageRepo,
		eventRepo:         eventRepo,
		logger:            logger,
		bot:               bot,
	}
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
		msg += "• <code>document</code>, <code>location</code>, <code>contact</code>\n\n"

		msg += "<b>⚠️ ОСОБЫЙ ТИП - banned_words:</b>\n"
		msg += "• <code>/setlimit banned_words 3</code> - макс 3 мата/день, потом бан\n"
		msg += "ℹ️ Работает только если включён profanityfilter\n"
		msg += "ℹ️ Для включения: <code>/setprofanity delete</code>\n\n"
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

		msg += "🔹 <code>/setvip</code> — Выдать VIP-статус (только админы)\n"
		msg += "   VIP-пользователи игнорируют все лимиты\n"
		msg += "   📌 Ответьте на сообщение пользователя и напишите <code>/setvip</code>\n\n"

		msg += "🔹 <code>/removevip</code> — Снять VIP-статус (только админы)\n"
		msg += "   📌 Ответьте на сообщение пользователя и напишите <code>/removevip</code>\n\n"

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
	// Пропускаем приватные сообщения и команды.
	// Приватные: лимиты бессмысленны в ЛС с ботом.
	// Команды: админ должен ВСЕГДА иметь возможность управлять ботом,
	// даже если текстовый лимит исчерпан. Без этой проверки
	// команды /setlimit, /setvip и т.д. удалялись limiter-ом как обычный текст.
	if ctx.Message.Private() || (ctx.Message.Text != "" && strings.HasPrefix(ctx.Message.Text, "/")) {
		return nil
	}

	chatID := ctx.Chat.ID
	// ThreadID уже вычислен в middleware и закеширован — без лишнего SQL-запроса.
	threadID := ctx.ThreadID
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
	contentType := core.DetectContentType(ctx.Message)
	if contentType == "unknown" {
		return nil
	}

	// Получаем лимиты (с fallback: персональные → общие, топик → чат).
	// Передаём &userID для проверки персональных лимитов (установленных через /setlimit reply).
	// GetLimits автоматически откатывается к общим, если персональных нет.
	// Раньше передавали nil — персональные лимиты полностью игнорировались.
	limits, err := m.contentLimitsRepo.GetLimits(chatID, threadID, &userID)
	if err != nil {
		m.logger.Error("failed to get limits", zap.Error(err))
		return nil
	}

	// Получаем текущий счётчик из messages (за сегодня)
	// Statistics уже сохранил текущее сообщение (statistics → limiter в пайплайне),
	// поэтому counter уже включает текущее сообщение
	counter, err := m.messageRepo.GetTodayCountByType(chatID, threadID, userID, contentType)
	if err != nil {
		m.logger.Error("failed to get today counter", zap.Error(err))
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

	// Используем WarningThreshold из БД (по умолчанию 2).
	// Предупреждаем когда до лимита осталось warning_threshold сообщений.
	warnThreshold := limits.WarningThreshold
	if warnThreshold <= 0 {
		warnThreshold = 2 // fallback на случай некорректного значения
	}

	// Отправляем предупреждения в чате, если близко к лимиту
	if limitValue > 0 && counter <= limitValue {
		remaining := limitValue - counter
		if remaining >= 0 && remaining < warnThreshold {
			warning := fmt.Sprintf("⚠️ %s, %s: %d из %d (осталось %d)",
				core.DisplayName(ctx.Sender), contentType, counter, limitValue, remaining)
			if err := ctx.Send(warning); err != nil {
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

		// Удаляем сообщение (ctx.DeleteMessage автоматически ставит ctx.MessageDeleted = true)
		if err := ctx.DeleteMessage(); err != nil {
			m.logger.Error("failed to delete message", zap.Error(err))
		}

		// Предупреждение отправляем только ОДИН раз — при первом превышении.
		// Иначе пользователь может заспамить чат удалениями (видно: 6/5, 7/5, 8/5...).
		// Для limitValue > 0: предупреждаем при counter == limitValue + 1.
		// Для limitValue == -1 (запрещено): предупреждаем при counter == 1.
		firstExceeded := (limitValue > 0 && counter == limitValue+1) || (limitValue == -1 && counter == 1)
		if firstExceeded {
			warning := fmt.Sprintf("❌ %s, лимит на %s достигнут (%d/%d)", core.DisplayName(ctx.Sender), contentType, counter, limitValue)
			if limitValue == -1 {
				warning = fmt.Sprintf("❌ %s, %s запрещено в этом чате", core.DisplayName(ctx.Sender), contentType)
			}
			if err := ctx.Send(warning); err != nil {
				m.logger.Error("failed to send warning", zap.Error(err))
			}
		}

		// MessageDeleted пропагируется через middleware → Reactions увидит и скорректирует.
		return nil
	}

	return nil
}

// handleMyStats показывает статистику пользователя
func (m *LimiterModule) handleMyStats(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

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

	// Один SQL-запрос для всех типов контента (вместо 12 отдельных)
	counters, err := m.messageRepo.GetTodayCountsAllTypes(chatID, threadID, userID)
	if err != nil {
		m.logger.Error("failed to get today counts", zap.Error(err))
		return c.Send("❌ Не удалось получить статистику")
	}

	for _, t := range types {
		counter := counters[t.field]
		switch {
		case t.value == -1:
			text += fmt.Sprintf("%s %s: %d из 0 (запрещено)\n", t.emoji, t.name, counter)
		case t.value == 0:
			text += fmt.Sprintf("%s %s: %d (без лимита)\n", t.emoji, t.name, counter)
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
	threadID := core.GetThreadID(m.db, c)

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
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleSetLimit called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	// Убеждаемся что chat_id существует в таблице chats (для foreign key)
	_, _ = m.db.Exec(`
		INSERT INTO chats (chat_id, chat_type, title)
		VALUES ($1, 'unknown', 'unknown')
		ON CONFLICT (chat_id) DO NOTHING
	`, chatID)

	args := c.Args()
	if len(args) != 2 {
		return c.Send("Использование: /setlimit <тип> <значение>\nДля персонального лимита: ответьте этой командой на сообщение пользователя")
	}

	contentType := args[0]

	// Валидация типа контента до записи в БД.
	// Без этой проверки SetLimit возвращал "unknown content type",
	// а пользователь видел непонятное "не удалось установить лимит".
	validContentTypes := map[string]bool{
		"text": true, "photo": true, "video": true, "sticker": true,
		"animation": true, "voice": true, "video_note": true, "audio": true,
		"document": true, "location": true, "contact": true, "banned_words": true,
	}
	if !validContentTypes[contentType] {
		return c.Send("❌ Неизвестный тип: " + contentType + "\n\nДопустимые: text, photo, video, sticker, animation, voice, video_note, audio, document, location, contact, banned_words")
	}

	limitValue, err := strconv.Atoi(args[1])
	if err != nil || limitValue < -1 {
		return c.Send("❌ Неверное значение лимита")
	}

	var userID *int64

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

	// Контекстные предупреждения для специальных типов лимитов
	if contentType == "banned_words" && limitValue > 0 {
		msg += "\n\n⚠️ Для работы этого лимита необходимо включить фильтр мата: `/setprofanity delete`"
	}
	if contentType == "text" && limitValue == -1 {
		msg += "\n\n⚠️ При полном запрете текста фильтры (`/addban`, `/setprofanity`) не смогут проверять текстовые сообщения — Limiter удаляет их до проверки фильтрами."
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleSetVIP устанавливает VIP-статус
func (m *LimiterModule) handleSetVIP(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleSetVIP called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	// Убеждаемся что chat_id существует в таблице chats (для foreign key)
	_, _ = m.db.Exec(`
		INSERT INTO chats (chat_id, chat_type, title)
		VALUES ($1, 'unknown', 'unknown')
		ON CONFLICT (chat_id) DO NOTHING
	`, chatID)

	if c.Message().ReplyTo == nil {
		return c.Send("❌ Ответьте этой командой на сообщение пользователя")
	}

	userID := c.Message().ReplyTo.Sender.ID

	args := c.Args()
	reason := "Установлено администратором"
	// args — все аргументы после /setvip (без самой команды).
	// Используем args целиком: args[0] — первое слово причины, не user_id.
	// Раньше было args[1:] — первое слово причины терялось.
	if len(args) > 0 {
		reason = strings.Join(args, " ")
	}

	if err := m.vipRepo.GrantVIP(chatID, threadID, userID, c.Sender().ID, reason); err != nil {
		return c.Send("❌ Не удалось установить VIP-статус")
	}

	// Логируем событие
	_ = m.eventRepo.Log(chatID, c.Sender().ID, "limiter", "grant_vip",
		fmt.Sprintf("Granted VIP to user %d (chat=%d, thread=%d, reason: %s)", userID, chatID, threadID, reason))

	displayName := core.DisplayName(c.Message().ReplyTo.Sender)

	var msg string
	if threadID != 0 {
		msg = fmt.Sprintf("✅ VIP-статус выдан пользователю %s **для этого топика**\n\n💡 Теперь он игнорирует все лимиты в этом топике.\nДля выдачи VIP на весь чат используйте команду в основном чате.", displayName)
	} else {
		msg = fmt.Sprintf("✅ VIP-статус выдан пользователю %s **для всего чата**\n\n💡 Теперь он игнорирует все лимиты во всех топиках.", displayName)
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleRemoveVIP снимает VIP-статус
func (m *LimiterModule) handleRemoveVIP(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleRemoveVIP called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

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

	displayName := core.DisplayName(c.Message().ReplyTo.Sender)

	var msg string
	if threadID != 0 {
		msg = fmt.Sprintf("✅ VIP-статус снят с %s **для этого топика**\n\n💡 Чтобы снять VIP на весь чат, используйте команду в основном чате.", displayName)
	} else {
		msg = fmt.Sprintf("✅ VIP-статус снят с %s **для всего чата**", displayName)
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeMarkdown})
}

// handleListVIPs показывает список VIP-пользователей
func (m *LimiterModule) handleListVIPs(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleListVIPs called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

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

	text := fmt.Sprintf("👑 <b>VIP-пользователи %s:</b>\n\n", location)
	for i, vip := range vips {
		// Получаем имя через Telegram API (таблица users удалена в миграции 002).
		// Аналогично /topchat в statistics.
		displayName := fmt.Sprintf("ID: <code>%d</code>", vip.UserID)
		chatMember, apiErr := m.bot.ChatMemberOf(c.Chat(), &tele.User{ID: vip.UserID})
		if apiErr == nil && chatMember != nil && chatMember.User != nil {
			if chatMember.User.Username != "" {
				displayName = fmt.Sprintf("@%s", chatMember.User.Username)
			} else if chatMember.User.FirstName != "" {
				displayName = chatMember.User.FirstName
			}
		}
		text += fmt.Sprintf("%d. %s\n   Причина: %s\n\n", i+1, displayName, vip.Reason)
	}

	return c.Send(text, &tele.SendOptions{ParseMode: tele.ModeHTML})
}
