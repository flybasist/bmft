package core

import (
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

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
