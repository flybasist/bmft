package utils

import "testing"

// Русский комментарий: Юнит-тест для функции определения типа контента.
func TestCheckContentType(t *testing.T) {
	cases := []struct {
		name   string
		update map[string]any
		want   string
	}{
		{"text", map[string]any{"message": map[string]any{"text": "hello"}}, "text"},
		{"command", map[string]any{"message": map[string]any{"text": "/start", "entities": []any{map[string]any{"type": "bot_command"}}}}, "command"},
		{"photo", map[string]any{"message": map[string]any{"photo": []any{"p"}}}, "photo"},
		{"unknown", map[string]any{"message": map[string]any{"foo": 1}}, "unknown"},
	}
	for _, c := range cases {
		got, err := CheckContentType(c.update)
		if err != nil {
			if c.want != "" { // Все кейсы валидные, поэтому ошибка не ожидается
				t.Fatalf("%s: unexpected error: %v", c.name, err)
			}
			continue
		}
		if got != c.want {
			t.Fatalf("%s: want %s got %s", c.name, c.want, got)
		}
	}
}
