// Package limiter — лимиты на типы контента и VIP-обход.
package limiter

import (
	"database/sql"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

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
	chatRepo          *repositories.ChatRepository
	logger            *zap.Logger
	bot               *tele.Bot
}

// New создаёт новый экземпляр LimiterModule.
// messageRepo — общий экземпляр из initModules (не создаём дубликат).
func New(db *sql.DB, vipRepo *repositories.VIPRepository, contentLimitsRepo *repositories.ContentLimitsRepository, messageRepo *repositories.MessageRepository, eventRepo *repositories.EventRepository, chatRepo *repositories.ChatRepository, logger *zap.Logger, bot *tele.Bot) *LimiterModule {
	return &LimiterModule{
		db:                db,
		vipRepo:           vipRepo,
		contentLimitsRepo: contentLimitsRepo,
		messageRepo:       messageRepo,
		eventRepo:         eventRepo,
		chatRepo:          chatRepo,
		logger:            logger,
		bot:               bot,
	}
}

// RegisterCommands регистрирует пользовательские команды
func (m *LimiterModule) RegisterCommands(bot *tele.Bot) {
	// Все view-команды обслуживаются inline-меню (cmd/bot/menus.go).
}

// RegisterAdminCommands регистрирует административные команды
func (m *LimiterModule) RegisterAdminCommands(bot *tele.Bot) {
	bot.Handle("/setlimit", m.handleSetLimit)
	bot.Handle("/setvip", m.handleSetVIP)
}

// HandleSetVIP — публичная обёртка над handleSetVIP для wizard-фолбэка
// (cmd/bot/wizards.go::wrapSetVIPWithWizard). Используется, когда есть
// аргументы или нет ReplyTo — старый синтаксис должен работать как раньше.
func (m *LimiterModule) HandleSetVIP(c tele.Context) error {
	return m.handleSetVIP(c)
}

// HandleSetLimit — публичная обёртка над handleSetLimit для wizard-фолбэка
// (cmd/bot/wizards.go::wrapSetLimitWithWizard). Используется, когда есть
// аргументы (старый синтаксис /setlimit <type> <value>) или контекст
// не подходит для wizard'а.
func (m *LimiterModule) HandleSetLimit(c tele.Context) error {
	return m.handleSetLimit(c)
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

	// VIP-статус уже проверен в middleware и передан через ctx (было 2 запроса на сообщение).
	if ctx.IsVIP {
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

// handleSetLimit устанавливает лимит
func (m *LimiterModule) handleSetLimit(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleSetLimit called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	if err := m.chatRepo.EnsureExists(chatID); err != nil {
		m.logger.Error("failed to ensure chat exists", zap.Error(err))
	}

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
		"via_bot": true, "via": true,
	}
	if !validContentTypes[contentType] {
		return c.Send("❌ Неизвестный тип: " + contentType + "\n\nДопустимые: text, photo, video, sticker, animation, voice, video_note, audio, document, location, contact, banned_words, via_bot")
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
			msg = fmt.Sprintf("✅ Лимит установлен для <b>этого топика</b>\n\n%s: %d в день\n\n💡 Для настройки всего чата используйте команду в основном чате (не в топике)", contentType, limitValue)
		} else {
			msg = fmt.Sprintf("✅ Персональный лимит установлен для пользователя <b>в этом топике</b>\n\n%s: %d в день\n\n💡 Для настройки на весь чат используйте команду в основном чате", contentType, limitValue)
		}
	} else {
		// Команда выполнена в основном чате
		if userID == nil {
			msg = fmt.Sprintf("✅ Лимит установлен для <b>всего чата</b>\n\n%s: %d в день\n\n💡 Для настройки конкретного топика используйте команду внутри топика", contentType, limitValue)
		} else {
			msg = fmt.Sprintf("✅ Персональный лимит установлен для пользователя <b>во всём чате</b>\n\n%s: %d в день", contentType, limitValue)
		}
	}

	// Контекстные предупреждения для специальных типов лимитов
	if contentType == "banned_words" && limitValue > 0 {
		msg += "\n\n⚠️ Для работы этого лимита необходимо включить фильтр мата: /setprofanity"
	}
	if contentType == "text" && limitValue == -1 {
		msg += "\n\n⚠️ При полном запрете текста фильтры (/addban, /setprofanity) не смогут проверять текстовые сообщения — Limiter удаляет их до проверки фильтрами."
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
}

// handleSetVIP устанавливает VIP-статус
func (m *LimiterModule) handleSetVIP(c tele.Context) error {
	chatID := c.Chat().ID
	threadID := core.GetThreadID(m.db, c)

	m.logger.Info("handleSetVIP called", zap.Int64("chat_id", chatID), zap.Int("thread_id", threadID), zap.Int64("user_id", c.Sender().ID))

	if err := m.chatRepo.EnsureExists(chatID); err != nil {
		m.logger.Error("failed to ensure chat exists", zap.Error(err))
	}

	if c.Message().ReplyTo == nil {
		return core.SendWithTTL(c,
			"❌ Ответьте этой командой на сообщение пользователя.\n💡 В группе можно также написать <code>/setvip</code> без reply — бот запустит пошаговый мастер.",
			15*time.Second, m.logger, &tele.SendOptions{ParseMode: tele.ModeHTML})
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
		msg = fmt.Sprintf("✅ VIP-статус выдан пользователю %s <b>для этого топика</b>\n\n💡 Теперь он игнорирует все лимиты в этом топике.\nДля выдачи VIP на весь чат используйте команду в основном чате.", html.EscapeString(displayName))
	} else {
		msg = fmt.Sprintf("✅ VIP-статус выдан пользователю %s <b>для всего чата</b>\n\n💡 Теперь он игнорирует все лимиты во всех топиках.", html.EscapeString(displayName))
	}

	return c.Send(msg, &tele.SendOptions{ParseMode: tele.ModeHTML})
}
