package utils

import (
	"encoding/json"
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
