package telegram_bot

// SendMessage — структура для сообщений, которые мы получаем из Kafka
// и затем отправляем в Telegram.
type SendMessage struct {
	ChatID  int64  `json:"chat_id"`
	Text    string `json:"text,omitempty"`
	Sticker string `json:"sticker,omitempty"`
	TypeMsg string `json:"type_msg"`
}

// DeleteMessage — структура для удаления сообщений в Telegram
type DeleteMessage struct {
	ChatID    int64 `json:"chat_id"`
	MessageID int   `json:"message_id"`
}
