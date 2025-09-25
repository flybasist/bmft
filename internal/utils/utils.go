package utils

import (
	"encoding/json"
	"errors"
	"fmt"
)

func Truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "...(truncated)"
}

// intToStr — безопасно преобразует числовое значение в строку
func IntToStr(v any) string {
	switch val := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", val)
	case int:
		return fmt.Sprint(val)
	case int64:
		return fmt.Sprint(val)
	case json.Number:
		return val.String()
	default:
		return fmt.Sprint(v)
	}
}

func CheckContentType(update map[string]any) (string, error) {
	msgRaw, ok := update["message"]
	if !ok {
		return "", errors.New("no message field in update")
	}

	msg, ok := msgRaw.(map[string]any)
	if !ok {
		return "", errors.New("message is not an object")
	}

	if msg["text"] != nil {
		if entities, ok := msg["entities"].([]any); ok {
			for _, e := range entities {
				if ent, ok := e.(map[string]any); ok {
					if ent["type"] == "bot_command" {
						return "command", nil
					}
				}
			}
		}
		return "text", nil
	}

	switch {
	case msg["photo"] != nil:
		return "photo", nil
	case msg["audio"] != nil:
		return "audio", nil
	case msg["voice"] != nil:
		return "voice", nil
	case msg["video"] != nil:
		return "video", nil
	case msg["document"] != nil:
		return "document", nil
	case msg["sticker"] != nil:
		return "sticker", nil
	case msg["contact"] != nil:
		return "contact", nil
	case msg["location"] != nil:
		return "location", nil
	case msg["animation"] != nil:
		return "animation", nil
	case msg["video_note"] != nil:
		return "video_note", nil
	default:
		return "unknown", nil
	}
}
