package core

import (
	"database/sql"

	"gopkg.in/telebot.v3"
)

// GetThreadID возвращает правильный thread_id с учетом того, является ли чат форумом.
// Русский комментарий: В обычных чатах (не форумах) message.ThreadID может содержать
// мусорные значения (например, ID реплаевого сообщения). Эта функция проверяет is_forum
// в БД и возвращает 0 для обычных чатов, thread_id для форумов.
func GetThreadID(db *sql.DB, c telebot.Context) int64 {
	// Если ThreadID == 0, сразу возвращаем 0 (это точно не топик)
	if c.Message().ThreadID == 0 {
		return 0
	}

	// Проверяем является ли чат форумом
	var isForum bool
	err := db.QueryRow(`SELECT is_forum FROM chats WHERE chat_id = $1`, c.Chat().ID).Scan(&isForum)

	// Если ошибка или не форум - возвращаем 0
	if err != nil {
		// Логируем только если это не "нет строк" (это нормально для новых чатов)
		if err != sql.ErrNoRows {
			// Можно добавить логгер в параметры функции, но пока просто возвращаем 0
		}
		return 0
	}
	if !isForum {
		return 0
	}

	// Это реально форум с топиками
	return int64(c.Message().ThreadID)
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

	// Если ошибка или не форум - возвращаем 0
	if err != nil {
		if err != sql.ErrNoRows {
			// Можно добавить логгер в параметры функции
		}
		return 0
	}
	if !isForum {
		return 0
	}

	// Это реально форум с топиками
	return msg.ThreadID
}

// DetectContentType определяет тип контента сообщения.
// Русский комментарий: Общая функция для определения типа контента.
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
