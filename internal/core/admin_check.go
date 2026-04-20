package core

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// IsAnonymousAdmin возвращает true, если сообщение отправлено анонимным админом
// (включён режим "Remain Anonymous"). Такие сообщения приходят от имени
// самой группы: msg.SenderChat == текущий чат. Sender при этом равен
// GroupAnonymousBot (ID 1087968824) — одинаков для всех анонимов, поэтому
// по Sender различить конкретного человека невозможно.
//
// Признак SenderChat == chat нельзя подделать обычным пользователем —
// Telegram проставляет это поле только для post-as-chat сообщений.
//
// Используется в AdminOnlyMiddleware (разрешает команду) и wizard'ами
// (отказ в wizard — нет уникального UserID для state, пользователя отправляем
// на старый синтаксис).
func IsAnonymousAdmin(msg *tele.Message) bool {
	return msg != nil && msg.SenderChat != nil && msg.Chat != nil && msg.SenderChat.ID == msg.Chat.ID
}

// adminCacheEntry — кэш списка админов чата.
type adminCacheEntry struct {
	adminIDs  map[int64]bool
	fetchedAt time.Time
}

// AdminChecker проверяет права администратора с кэшированием.
// Кэш per-chat, TTL задаётся при создании.
// Потокобезопасен (sync.RWMutex).
type AdminChecker struct {
	bot      *tele.Bot
	cache    map[int64]*adminCacheEntry // key = chatID
	mu       sync.RWMutex
	cacheTTL time.Duration
}

// NewAdminChecker создаёт AdminChecker с заданным TTL кэша.
func NewAdminChecker(bot *tele.Bot, cacheTTL time.Duration) *AdminChecker {
	ac := &AdminChecker{
		bot:      bot,
		cache:    make(map[int64]*adminCacheEntry),
		cacheTTL: cacheTTL,
	}

	// Фоновая очистка устаревших записей
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			ac.mu.Lock()
			now := time.Now()
			for chatID, entry := range ac.cache {
				if now.Sub(entry.fetchedAt) > 10*time.Minute {
					delete(ac.cache, chatID)
				}
			}
			ac.mu.Unlock()
		}
	}()

	return ac
}

// IsAdmin проверяет, является ли пользователь админом чата.
// Результат getChatAdministrators кэшируется на cacheTTL per-chat.
func (ac *AdminChecker) IsAdmin(chat *tele.Chat, userID int64) (bool, error) {
	if chat.Type != tele.ChatGroup && chat.Type != tele.ChatSuperGroup {
		return false, nil
	}

	chatID := chat.ID

	// Пробуем из кэша (RLock — читатели не блокируют друг друга)
	ac.mu.RLock()
	entry, exists := ac.cache[chatID]
	if exists && time.Since(entry.fetchedAt) < ac.cacheTTL {
		isAdmin := entry.adminIDs[userID]
		ac.mu.RUnlock()
		return isAdmin, nil
	}
	ac.mu.RUnlock()

	// Кэш пуст или устарел — запрашиваем API
	admins, err := ac.bot.AdminsOf(chat)
	if err != nil {
		return false, err
	}

	adminIDs := make(map[int64]bool, len(admins))
	for _, admin := range admins {
		adminIDs[admin.User.ID] = true
	}

	ac.mu.Lock()
	ac.cache[chatID] = &adminCacheEntry{
		adminIDs:  adminIDs,
		fetchedAt: time.Now(),
	}
	ac.mu.Unlock()

	return adminIDs[userID], nil
}

// adminCommands — список команд, требующих прав администратора.
// Если команда в этом списке и вызвана не-админом — middleware молча удаляет сообщение.
var adminCommands = map[string]bool{
	// core
	"/welcome": true,
	// limiter
	"/setlimit":  true,
	"/setvip":    true,
	"/removevip": true,
	"/listvips":  true,
	// statistics
	"/chatstats": true,
	"/topchat":   true,
	// reactions
	"/addreaction":     true,
	"/listreactions":   true,
	"/removereaction":  true,
	"/addban":          true,
	"/listbans":        true,
	"/removeban":       true,
	"/setprofanity":    true,
	"/removeprofanity": true,
	"/profanitystatus": true,
	// scheduler
	"/listtasks": true,
	"/addtask":   true,
	"/deltask":   true,
	"/runtask":   true,
}

// AdminOnlyMiddleware блокирует вызов админских команд не-админами.
// Если пользователь не админ и вызывает команду из adminCommands — сообщение удаляется,
// бот молчит. Использует AdminChecker с кэшем для минимизации API-запросов.
func AdminOnlyMiddleware(ac *AdminChecker, logger *zap.Logger) tele.MiddlewareFunc {
	return func(next tele.HandlerFunc) tele.HandlerFunc {
		return func(c tele.Context) error {
			msg := c.Message()
			if msg == nil {
				return next(c)
			}

			text := msg.Text
			if text == "" || !strings.HasPrefix(text, "/") {
				return next(c)
			}

			// Извлекаем команду (без аргументов и @botname)
			cmd := strings.Fields(text)[0]
			if idx := strings.Index(cmd, "@"); idx != -1 {
				cmd = cmd[:idx]
			}
			cmd = strings.ToLower(cmd)

			if !adminCommands[cmd] {
				return next(c)
			}

			// Anonymous admin: сообщение отправлено от имени самой группы.
			// См. IsAnonymousAdmin для подробностей (security model + почему нельзя подделать).
			if IsAnonymousAdmin(msg) {
				return next(c)
			}

			// Админская команда — проверяем права (с кэшем)
			isAdmin, err := ac.IsAdmin(c.Chat(), c.Sender().ID)
			if err != nil {
				logger.Warn("admin check failed, denying access",
					zap.Error(err),
					zap.Int64("chat_id", msg.Chat.ID),
					zap.Int64("user_id", msg.Sender.ID),
					zap.String("command", cmd),
				)
				_ = c.Delete()
				return nil
			}

			if !isAdmin {
				_ = c.Delete()
				return nil
			}

			return next(c)
		}
	}
}

// AdminOnlyCallback оборачивает обработчик inline-кнопки проверкой админства.
// AdminOnlyMiddleware фильтрует только текстовые команды по списку adminCommands;
// callback'и от inline-клавиатуры она не покрывает. Любой пользователь, увидевший
// сообщение с админскими кнопками (например /listtasks), мог бы их нажать.
//
// Поведение:
//   - не-админ → c.Respond с alert «нет прав» (без удаления исходного сообщения,
//     чтобы не сломать UX другим админам в чате);
//   - анонимный админ (от имени группы) → разрешено (как в AdminOnlyMiddleware);
//   - ошибка проверки → отказ + alert.
func AdminOnlyCallback(ac *AdminChecker, logger *zap.Logger, h tele.HandlerFunc) tele.HandlerFunc {
	return func(c tele.Context) error {
		// У callback'ов c.Sender() = автор нажатия, c.Chat() = чат сообщения.
		if c.Callback() == nil {
			return h(c)
		}
		if IsAnonymousAdmin(c.Message()) {
			// Анонимный админ — нажатие от имени группы; разрешаем как в текстовых командах.
			return h(c)
		}

		isAdmin, err := ac.IsAdmin(c.Chat(), c.Sender().ID)
		if err != nil {
			logger.Warn("callback admin check failed",
				zap.Error(err),
				zap.Int64("chat_id", c.Chat().ID),
				zap.Int64("user_id", c.Sender().ID),
			)
			_ = c.Respond(&tele.CallbackResponse{Text: "❌ Ошибка проверки прав", ShowAlert: true})
			return nil
		}
		if !isAdmin {
			_ = c.Respond(&tele.CallbackResponse{Text: "🚫 Только для администраторов", ShowAlert: true})
			return nil
		}
		return h(c)
	}
}
