package core

import (
	tele "gopkg.in/telebot.v3"
)

// IsUserAdmin проверяет, является ли пользователь админом или владельцем чата
func IsUserAdmin(bot *tele.Bot, chat *tele.Chat, userID int64) (bool, error) {
	if chat.Type == tele.ChatGroup || chat.Type == tele.ChatSuperGroup {
		admins, err := bot.AdminsOf(chat)
		if err != nil {
			return false, err
		}
		for _, admin := range admins {
			if admin.User.ID == userID {
				return true, nil
			}
		}
		return false, nil
	}
	// В приватных чатах никто не считается админом
	return false, nil
}
