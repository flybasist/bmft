package core

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gopkg.in/telebot.v3"
)

// DateFormat — единый формат даты для сообщений пользователям (DD.MM.YYYY).
// Используется в statistics, scheduler, wizard'ах.
const DateFormat = "02.01.2006"

// DateTimeFormat — формат даты с временем (DD.MM.YYYY HH:MM).
const DateTimeFormat = "02.01.2006 15:04"

// ScheduleDelete планирует удаление сообщения через ttl. Неблокирующий:
// использует time.AfterFunc, таймер переживает обычные операции бота.
// При shutdown незавершённые таймеры тихо провалятся (Delete вернёт ошибку, лог Debug).
// Используется welcome-обработчиком и будет использоваться wizard'ами
// для авто-удаления информационных подтверждений.
func ScheduleDelete(bot *telebot.Bot, msg *telebot.Message, ttl time.Duration, logger *zap.Logger) {
	if msg == nil {
		return
	}
	time.AfterFunc(ttl, func() {
		if err := bot.Delete(msg); err != nil {
			logger.Debug("failed to delete scheduled message",
				zap.Error(err),
				zap.Int("message_id", msg.ID),
				zap.Int64("chat_id", msg.Chat.ID),
			)
		}
	})
}

// InfoResponseTTL — время жизни информационных ответов бота в группах.
// Справочные и статистические ответы автоматически удаляются, чтобы не засорять чат.
// В личных чатах TTL не применяется.
const InfoResponseTTL = 2 * time.Minute

// SendWithTTL отправляет сообщение; в группах планирует удаление через ttl.
// В личных чатах или при ttl ≤ 0 — обычный c.Send без удаления.
func SendWithTTL(c telebot.Context, what interface{}, ttl time.Duration, logger *zap.Logger, opts ...interface{}) error {
	chat := c.Chat()
	if ttl <= 0 || chat == nil || (chat.Type != telebot.ChatGroup && chat.Type != telebot.ChatSuperGroup) {
		return c.Send(what, opts...)
	}
	sent, err := c.Bot().Send(chat, what, opts...)
	if err != nil {
		return err
	}
	ScheduleDelete(c.Bot(), sent, ttl, logger)
	return nil
}

// ReplyWithTTL отправляет reply на сообщение; в группах планирует удаление через ttl.
// В личных чатах или при ttl ≤ 0 — обычный c.Reply без удаления.
func ReplyWithTTL(c telebot.Context, what interface{}, ttl time.Duration, logger *zap.Logger, opts ...interface{}) error {
	chat := c.Chat()
	msg := c.Message()
	if ttl <= 0 || chat == nil || (chat.Type != telebot.ChatGroup && chat.Type != telebot.ChatSuperGroup) || msg == nil {
		return c.Reply(what, opts...)
	}
	sent, err := c.Bot().Reply(msg, what, opts...)
	if err != nil {
		return err
	}
	ScheduleDelete(c.Bot(), sent, ttl, logger)
	return nil
}

// ParseQuotedTokens разбирает строку на токены с учётом двойных кавычек.
// Примеры:
//
//	`hello world`        → ["hello", "world"]
//	`"hello world"`      → ["hello world"]
//	`a "b c" d`          → ["a", "b c", "d"]
//	`""`                 → []   (пустые кавычки не создают токен)
//
// Не выполняет escape внутри кавычек и не обрабатывает одиночные кавычки
// — реальные команды бота этого не требуют. Для сложных случаев (cron в
// кавычках после имени) scheduler использует собственный parseAddTaskArgs.
func ParseQuotedTokens(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var tokens []string
	var current strings.Builder
	inQuote := false

	for i := 0; i < len(text); i++ {
		ch := text[i]
		switch ch {
		case '"':
			inQuote = !inQuote
		case ' ', '\t':
			if inQuote {
				current.WriteByte(ch)
			} else if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

// DisplayName возвращает отображаемое имя пользователя для сообщений бота.
// @username если есть, иначе FirstName, иначе "Пользователь".
// Без этого хелпера при пустом Username получалось: «⚠️ @, лимит на...»
func DisplayName(user *telebot.User) string {
	if user.Username != "" {
		return "@" + user.Username
	}
	if user.FirstName != "" {
		return user.FirstName
	}
	return "Пользователь"
}

// CheckIsForum проверяет, является ли чат форумом (с топиками) через Telegram API.
// telebot.v3 v3.3.8 НЕ экспортирует IsForum в Chat struct,
// поэтому делаем прямой запрос getChat и парсим is_forum из ответа.
// Вызывается только в handleUserJoined и /start (редкие события), overhead минимален.
func CheckIsForum(bot *telebot.Bot, chatID int64) bool {
	data, err := bot.Raw("getChat", map[string]string{
		"chat_id": strconv.FormatInt(chatID, 10),
	})
	if err != nil {
		return false
	}
	var resp struct {
		Result struct {
			IsForum bool `json:"is_forum"`
		} `json:"result"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return false
	}
	return resp.Result.IsForum
}

// GetThreadID возвращает правильный thread_id с учетом того, является ли чат форумом.
// Обёртка над GetThreadIDFromMessage для использования в admin-хендлерах, где доступен telebot.Context.
func GetThreadID(db *sql.DB, c telebot.Context) int {
	return GetThreadIDFromMessage(db, c.Message())
}

// GetThreadIDFromMessage возвращает правильный thread_id для сообщений в pipeline.
// Используется в OnMessage, где нет telebot.Context, а есть только Message и DB.
func GetThreadIDFromMessage(db *sql.DB, msg *telebot.Message) int {
	// Если ThreadID == 0, сразу возвращаем 0 (это точно не топик)
	if msg.ThreadID == 0 {
		return 0
	}

	// Проверяем является ли чат форумом
	var isForum bool
	err := db.QueryRow(`SELECT is_forum FROM chats WHERE chat_id = $1`, msg.Chat.ID).Scan(&isForum)

	// Если ошибка (включая sql.ErrNoRows) или не форум — возвращаем 0.
	// Логгер сюда не пробрасываем намеренно: функция вызывается на каждом сообщении,
	// а ошибки этого запроса (нет записи о чате) — нормальная ситуация при первом обращении.
	if err != nil || !isForum {
		return 0
	}

	// Это реально форум с топиками
	return msg.ThreadID
}

// DetectContentType определяет тип контента сообщения.
// Общая функция для определения типа контента.
// Используется в модулях limiter и statistics.
func DetectContentType(msg *telebot.Message) string {
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
