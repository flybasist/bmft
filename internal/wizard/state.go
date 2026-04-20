// Package wizard реализует FSM для интерактивных команд бота через inline-кнопки.
//
// Архитектура:
//
//   - State хранится в памяти процесса (один инстанс бота, перезапуск =
//     сброс всех wizard'ов — это допустимо, пользователь начнёт заново).
//   - Ключ state — (chatID, userID): один пользователь = один активный wizard
//     в чате. Запуск второго отменяет предыдущий. ThreadID хранится в State.Data.
//   - Idle-таймаут 5 минут: если пользователь не отвечает — wizard отменяется,
//     сообщение wizard'а удаляется.
//   - Anonymous-админы (msg.SenderChat == chat) не могут запускать wizard'ы
//     — у них одинаковый Sender.ID (1087968824) для всех, FSM не сможет
//     различить пользователей. Им предлагается старый синтаксис команды.
//   - Per-step admin recheck: на каждый callback проверяется IsAdmin().
//     Если пользователь потерял права за время wizard'а — wizard отменяется
//     с понятным сообщением.
//
// Wizard'ы работают ТОЛЬКО в группе (не в личке), чтобы не нарушать
// security model: админ в личке не привязан к чату, личка может остаться
// открытой после потери админских прав.
package wizard

import (
	"sync"
	"time"
)

// IdleTimeout — время бездействия, после которого wizard отменяется.
// Пользователь должен либо нажать кнопку, либо отправить текст в этот срок.
const IdleTimeout = 5 * time.Minute

// ConfirmTTL — время жизни информационного подтверждения «✅ Настройки сохранены».
// Через этот срок сообщение бота авто-удаляется, чтобы не засорять чат.
// Для важных подтверждений (списки изменённых VIP, добавленные баны) wizard
// может явно НЕ ставить TTL и оставить сообщение навсегда.
const ConfirmTTL = 60 * time.Second

// StateKey — уникальный идентификатор активного wizard'а.
//
// Один пользователь в одном чате = один активный wizard. Попытка
// запустить второй wizard (в том же или другом топике) автоматически
// отменяет предыдущий. ThreadID хранится в State.Data под ключом
// DataThreadID — это освобождает callback handler'ы от лукапа в БД
// для определения ThreadID.
//
// Примечание: параллельные wizard'ы в разных топиках одного чата одного
// юзера не поддерживаются в этой итерации — реальный use case редкий,
// а простота callback-роутинга важнее.
type StateKey struct {
	ChatID int64
	UserID int64
}

// DataThreadID — ключ в State.Data для сохранённого ThreadID чата
// (определяется в Start через core.GetThreadIDFromMessage).
const DataThreadID = "_threadID"

// State — текущее состояние конкретного wizard'а.
//
// Data — свободная корзина для значений, накопленных wizard'ом
// (например, для welcome: {"enabled": true, "ttl": 300}).
// Конкретные ключи и типы определяет реализация wizard'а.
//
// MessageID — ID сообщения бота, в котором отрисован текущий шаг.
// Wizard использует его для редактирования (next step) или удаления
// (отмена/завершение).
//
// AwaitText — флаг «следующее текстовое сообщение пользователя
// в этом (chat, thread, user) — это ввод для текущего шага».
// Pipeline middleware видит флаг и поглощает сообщение.
type State struct {
	Key       StateKey
	Wizard    string         // имя wizard'а: "welcome", "addtask", ...
	Step      string         // текущий шаг (произвольная строка для wizard'а)
	Data      map[string]any // накопленные значения
	MessageID int            // ID сообщения wizard'а в Telegram
	StartedAt time.Time
	UpdatedAt time.Time
	AwaitText bool        // ожидает текстовый ввод вместо callback
	timer     *time.Timer // idle-таймаут
}

// stateStore — потокобезопасное хранилище state'ов.
// Внутренняя структура Manager'а.
type stateStore struct {
	mu     sync.RWMutex
	states map[StateKey]*State
}

func newStateStore() *stateStore {
	return &stateStore{
		states: make(map[StateKey]*State),
	}
}

// get возвращает state или nil, если wizard не активен.
func (s *stateStore) get(key StateKey) *State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.states[key]
}

// set сохраняет state, перезаписывая существующий.
func (s *stateStore) set(state *State) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state.Key] = state
}

// remove удаляет state и останавливает idle-таймер.
// Возвращает удалённый state (или nil, если не было).
func (s *stateStore) remove(key StateKey) *State {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.states[key]
	if !ok {
		return nil
	}
	if state.timer != nil {
		state.timer.Stop()
	}
	delete(s.states, key)
	return state
}
