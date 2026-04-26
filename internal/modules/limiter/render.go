package limiter

import (
	"fmt"
	"html"
	"strconv"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"
)

// UniqueRemoveVIP — Unique значение inline-кнопки 🗑 для удаления VIP в меню.
const UniqueRemoveVIP = "m_rm_vip"

// RenderMyStats возвращает HTML-текст персональной статистики лимитов за сегодня.
// Второй return = isVIP.
func (m *LimiterModule) RenderMyStats(chatID int64, threadID int, userID int64) (string, bool, error) {
	isVIP, err := m.vipRepo.IsVIP(chatID, threadID, userID)
	if err != nil {
		return "", false, fmt.Errorf("ошибка получения статуса")
	}
	if isVIP {
		var scope string
		if threadID != 0 {
			scope = " (топик)"
		} else {
			scope = " (весь чат)"
		}
		return fmt.Sprintf("👑 <b>VIP-статус активен%s</b>\n\nВсе лимиты для вас отключены!", scope), true, nil
	}

	limits, err := m.contentLimitsRepo.GetLimits(chatID, threadID, &userID)
	if err != nil {
		return "", false, fmt.Errorf("не удалось получить лимиты")
	}

	counters, err := m.messageRepo.GetTodayCountsAllTypes(chatID, threadID, userID)
	if err != nil {
		return "", false, fmt.Errorf("не удалось получить статистику")
	}

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
	return text, false, nil
}

// RenderGetLimit возвращает HTML-текст общих лимитов чата/топика.
func (m *LimiterModule) RenderGetLimit(chatID int64, threadID int) (string, error) {
	limits, err := m.contentLimitsRepo.GetLimits(chatID, threadID, nil)
	if err != nil {
		return "", fmt.Errorf("не удалось получить лимиты")
	}

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

	text += "\n\U0001f4a1 \u041d\u0430\u0441\u0442\u0440\u043e\u0438\u0442\u044c: /setlimit"

	return text, nil
}

// RenderListVIPs возвращает HTML-текст списка VIP + slice userID для кнопок 🗑.
// Пустая строка + nil = нет VIP.
func (m *LimiterModule) RenderListVIPs(chatID int64, threadID int, chat *tele.Chat) (string, []int64, error) {
	vips, err := m.vipRepo.ListVIPs(chatID, threadID)
	if err != nil {
		return "", nil, fmt.Errorf("не удалось получить список VIP")
	}
	if len(vips) == 0 {
		return "", nil, nil
	}

	location := "чата"
	if threadID != 0 {
		location = "топика"
	}

	text := fmt.Sprintf("👑 <b>VIP-пользователи %s:</b>\n\n", location)
	userIDs := make([]int64, 0, len(vips))
	for i, vip := range vips {
		displayName := fmt.Sprintf("ID: <code>%d</code>", vip.UserID)
		if chat != nil {
			chatMember, apiErr := m.bot.ChatMemberOf(chat, &tele.User{ID: vip.UserID})
			if apiErr == nil && chatMember != nil && chatMember.User != nil {
				if chatMember.User.Username != "" {
					displayName = fmt.Sprintf("@%s", html.EscapeString(chatMember.User.Username))
				} else if chatMember.User.FirstName != "" {
					displayName = html.EscapeString(chatMember.User.FirstName)
				}
			}
		}
		text += fmt.Sprintf("%d. %s\n   Причина: %s\n\n", i+1, displayName, html.EscapeString(vip.Reason))
		userIDs = append(userIDs, vip.UserID)
	}

	text += "💡 Выдать VIP: /setvip (ответом на сообщение)\n🗑 Снять VIP: кнопки ниже"

	return text, userIDs, nil
}

// RevokeVIPByID снимает VIP по userID. Используется inline-кнопкой 🗑 в меню.
func (m *LimiterModule) RevokeVIPByID(chatID int64, threadID int, userID int64, adminID int64) error {
	if err := m.vipRepo.RevokeVIP(chatID, threadID, userID); err != nil {
		m.logger.Error("RevokeVIPByID failed", zap.Error(err),
			zap.Int64("chat_id", chatID), zap.Int64("user_id", userID))
		return err
	}
	_ = m.eventRepo.Log(chatID, adminID, "limiter", "vip_revoked",
		fmt.Sprintf("VIP revoked for user %d via menu (chat=%d, thread=%d)", userID, chatID, threadID))
	return nil
}

// BuildVIPButtons создаёт inline-кнопки 🗑 для списка VIP (по одной на строку).
func BuildVIPButtons(userIDs []int64) []tele.Btn {
	btns := make([]tele.Btn, 0, len(userIDs))
	for _, uid := range userIDs {
		btns = append(btns, tele.Btn{
			Unique: UniqueRemoveVIP,
			Text:   fmt.Sprintf("🗑 VIP #%d", uid),
			Data:   strconv.FormatInt(uid, 10),
		})
	}
	return btns
}
