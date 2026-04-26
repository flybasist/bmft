package statistics

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
)

// RenderMyWeek возвращает HTML-текст статистики пользователя за 7 дней.
// chatID, threadID, userID — координаты запроса.
// Возвращает пустую строку, если данных нет (вызывающий код покажет «нет данных»).
func (m *StatisticsModule) RenderMyWeek(chatID int64, threadID int, userID int64) (string, error) {
	stats, err := m.messageRepo.GetUserStats(chatID, threadID, userID, 7)
	if err != nil {
		m.logger.Error("RenderMyWeek: get stats", zap.Error(err))
		return "", fmt.Errorf("не удалось получить статистику")
	}
	if len(stats) == 0 {
		return "", nil
	}

	var sb strings.Builder
	if threadID != 0 {
		sb.WriteString("📊 <b>Твоя статистика за неделю (для этого топика)</b>\n\n")
	} else {
		sb.WriteString("📊 <b>Твоя статистика за неделю (для всего чата)</b>\n\n")
	}

	total := 0
	for contentType, count := range stats {
		if count > 0 {
			sb.WriteString(fmt.Sprintf("%s %s: <b>%d</b>\n", contentTypeEmoji(contentType), contentType, count))
			total += count
		}
	}
	sb.WriteString(fmt.Sprintf("\n<b>Всего:</b> %d сообщений за 7 дней", total))
	return sb.String(), nil
}

// RenderChatStats возвращает HTML-текст общей статистики чата/топика за сегодня.
func (m *StatisticsModule) RenderChatStats(chatID int64, threadID int) (string, error) {
	date := time.Now()
	stats, err := m.messageRepo.GetChatStats(chatID, threadID, 1)
	if err != nil {
		m.logger.Error("RenderChatStats: get stats", zap.Error(err))
		return "", fmt.Errorf("не удалось получить статистику")
	}
	if len(stats) == 0 {
		return "", nil
	}

	var sb strings.Builder
	if threadID != 0 {
		sb.WriteString(fmt.Sprintf("📊 <b>Статистика топика за %s</b>\n\n", date.Format(core.DateFormat)))
	} else {
		sb.WriteString(fmt.Sprintf("📊 <b>Статистика чата за %s</b>\n\n", date.Format(core.DateFormat)))
	}

	total := 0
	for contentType, count := range stats {
		sb.WriteString(fmt.Sprintf("%s %s: <b>%d</b>\n", contentTypeEmoji(contentType), contentType, count))
		total += count
	}
	sb.WriteString(fmt.Sprintf("\n<b>Всего:</b> %d сообщений", total))
	return sb.String(), nil
}

// RenderTopChat возвращает HTML-текст топ-10 активных участников за сегодня.
// chat нужен для ChatMemberOf (получение имён через Telegram API).
func (m *StatisticsModule) RenderTopChat(chatID int64, threadID int, chat *tele.Chat) (string, error) {
	date := time.Now()
	topUsers, err := m.messageRepo.GetChatTopUsers(chatID, threadID, 1, 10)
	if err != nil {
		m.logger.Error("RenderTopChat: get top users", zap.Error(err))
		return "", fmt.Errorf("не удалось получить топ")
	}
	if len(topUsers) == 0 {
		return "", nil
	}

	var sb strings.Builder
	if threadID != 0 {
		sb.WriteString(fmt.Sprintf("🏆 <b>Топ активных участников топика за %s</b>\n\n", date.Format(core.DateFormat)))
	} else {
		sb.WriteString(fmt.Sprintf("🏆 <b>Топ активных участников чата за %s</b>\n\n", date.Format(core.DateFormat)))
	}

	medals := []string{"🥇", "🥈", "🥉"}
	for i, userStat := range topUsers {
		username := fmt.Sprintf("User #%d", userStat.UserID)
		if chat != nil {
			chatMember, apiErr := m.bot.ChatMemberOf(chat, &tele.User{ID: userStat.UserID})
			if apiErr == nil && chatMember != nil && chatMember.User != nil {
				if chatMember.User.Username != "" {
					username = "@" + chatMember.User.Username
				} else if chatMember.User.FirstName != "" {
					username = chatMember.User.FirstName
					if chatMember.User.LastName != "" {
						username += " " + chatMember.User.LastName
					}
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
	return sb.String(), nil
}

// contentTypeEmoji возвращает emoji для типа контента.
func contentTypeEmoji(ct string) string {
	emojis := map[string]string{
		"text": "💬", "photo": "📷", "video": "🎥", "sticker": "😊",
		"animation": "🎬", "voice": "🎤", "video_note": "📹", "audio": "🎵",
		"document": "📄", "location": "📍", "contact": "👤", "poll": "📊",
	}
	if e, ok := emojis[ct]; ok {
		return e
	}
	return "📎"
}
