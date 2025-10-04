# Phase 3 — Reactions Module 🎉

**Статус:** ✅ Завершена  
**Дата:** 2025-01-20  
**Компоненты:** `internal/modules/reactions/reactions.go`, `migrations/001_initial_schema.sql` (reactions_config, reactions_log)

---

## 📋 Описание

Модуль **Reactions** реализует автоматические реакции бота на сообщения пользователей по regex/exact/contains паттернам. Это прямой порт функционала из Python бота (rts_bot/checkmessage.py + rts_bot/reaction.py).

### Основные возможности:

- ✅ **3 типа триггеров:** regex (регулярные выражения), exact (точное совпадение), contains (содержит подстроку)
- ✅ **3 типа реакций:** text (отправить текст), sticker (отправить стикер), delete (удалить сообщение)
- ✅ **Cooldown система:** Настраиваемый интервал между реакциями (по умолчанию 10 минут)
- ✅ **VIP bypass:** Флаг `is_vip` для пропуска cooldown
- ✅ **Логирование:** Все реакции сохраняются в `reactions_log` для антифлуда и статистики
- ✅ **Admin commands:** 4 команды для управления реакциями

---

## 🗄️ Схема БД

### Таблица `reactions_config`

Хранит конфигурацию реакций для каждого чата.

```sql
CREATE TABLE reactions_config (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL REFERENCES chats(chat_id) ON DELETE CASCADE,
    user_id BIGINT DEFAULT NULL, -- NULL = для всех пользователей
    content_type VARCHAR(20) NOT NULL, -- 'text', 'photo', 'video', etc.
    trigger_type VARCHAR(20) NOT NULL, -- 'regex', 'exact', 'contains'
    trigger_pattern TEXT NOT NULL, -- regex или текст для поиска
    reaction_type VARCHAR(20) NOT NULL, -- 'text', 'sticker', 'delete'
    reaction_data TEXT, -- текст ответа или file_id стикера
    violation_code INT DEFAULT 0, -- код нарушения для статистики
    cooldown_minutes INT DEFAULT 10, -- антифлуд: минуты между реакциями
    is_enabled BOOLEAN DEFAULT true,
    is_vip BOOLEAN DEFAULT false, -- пропускает cooldown
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
```

### Таблица `reactions_log`

Логирует все срабатывания реакций для cooldown проверки.

```sql
CREATE TABLE reactions_log (
    id BIGSERIAL PRIMARY KEY,
    chat_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    reaction_id BIGINT NOT NULL REFERENCES reactions_config(id) ON DELETE CASCADE,
    message_id BIGINT NOT NULL,
    triggered_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_reactions_log_cooldown ON reactions_log(chat_id, reaction_id, triggered_at DESC);
```

---

## 🔧 Admin Commands

Все команды доступны только администраторам (проверка через `isAdmin(userID)`).

### 1. `/addreaction` — Добавить реакцию

**Формат:**
```
/addreaction <contentType> <triggerType> <pattern> <reactionType> <data> [cooldown]
```

**Параметры:**
- `contentType`: `text`, `photo`, `video`, `document`, `sticker`, `voice`
- `triggerType`: `regex`, `exact`, `contains`
- `pattern`: regex выражение или текст для поиска
- `reactionType`: `text`, `sticker`, `delete`
- `data`: текст ответа или file_id стикера (для delete пусто `""`)
- `cooldown`: минуты между реакциями (опционально, по умолчанию 10)

**Примеры:**
```bash
# Regex: Ответить "Здравствуй!" на любое приветствие
/addreaction text regex (?i)(привет|здравствуй|hi|hello) text "Здравствуй!" 10

# Contains: Удалить сообщение со словом "спам"
/addreaction text contains спам delete "" 5

# Exact: Отправить стикер на точное слово "test"
/addreaction text exact test sticker CAACAgIAAxkBAAIC... 0
```

**Валидация:**
- ✅ Проверка что `triggerType` = `regex` → компилирует regex паттерн
- ✅ Проверка допустимых значений для всех enum полей
- ✅ Cooldown должен быть >= 0

### 2. `/listreactions` — Список реакций

Показывает все реакции для текущего чата.

**Формат:**
```
/listreactions
```

**Вывод:**
```
📋 Реакции чата (3):

✅ #1 | text/regex | `(?i)привет` → text (10m)
✅ #2 | text/contains | `спам` → delete (5m)
❌ #3 | photo/exact | `test` → sticker (0m)

💡 Для удаления: /delreaction <id>
```

- ✅ = реакция включена (`is_enabled=true`)
- ❌ = реакция выключена (`is_enabled=false`)

### 3. `/delreaction` — Удалить реакцию

Удаляет реакцию по ID.

**Формат:**
```
/delreaction <id>
```

**Пример:**
```
/delreaction 5
```

**Проверки:**
- ✅ ID должен существовать
- ✅ Реакция должна принадлежать этому чату (проверка `chat_id`)

### 4. `/testreaction` — Протестировать паттерн

Тестирует regex/exact/contains паттерн на тексте без сохранения в БД.

**Формат:**
```
/testreaction <pattern> <text>
```

**Примеры:**
```bash
/testreaction (?i)привет Привет мир
/testreaction спам это спамное сообщение
```

**Вывод:**
```
🧪 Тест паттерна:

Pattern: `(?i)привет`
Text: `Привет мир`

Результаты:
• regex: ✅ совпадение
• exact: ❌ нет
• contains: ✅ совпадение
```

Если regex невалиден, будет показана ошибка.

---

## 🔄 Логика работы

### 1. Обработка входящего сообщения (`OnMessage`)

```go
func (m *ReactionsModule) OnMessage(ctx *core.MessageContext) error {
    // 1. Проверяем что модуль включён для чата (делает registry автоматически)
    
    // 2. Извлекаем текст из сообщения
    text := extractText(ctx.Message)
    if text == "" {
        return nil // Нет текста - ничего не делаем
    }
    
    // 3. Получаем все реакции для чата из БД
    reactions := getReactionsForChat(ctx.Chat.ID)
    
    // 4. Для каждой реакции:
    for _, reaction := range reactions {
        // a) Проверяем паттерн (regex/exact/contains)
        matched := checkPattern(text, reaction)
        if !matched {
            continue
        }
        
        // b) Проверяем cooldown (пропускаем если недавно было)
        if shouldSkipDueToCooldown(reaction) {
            continue
        }
        
        // c) Выполняем реакцию (text/sticker/delete)
        executeReaction(ctx, reaction)
        
        // d) Логируем в reactions_log
        logReaction(reaction.ID, ctx.Chat.ID, ctx.Sender.ID, ctx.Message.ID)
    }
    
    return nil
}
```

### 2. Проверка паттерна (`checkPattern`)

```go
func (m *ReactionsModule) checkPattern(text string, reaction ReactionConfig) (bool, error) {
    switch reaction.TriggerType {
    case "regex":
        re, err := regexp.Compile(reaction.TriggerPattern)
        if err != nil {
            return false, err
        }
        return re.MatchString(text), nil
        
    case "exact":
        return text == reaction.TriggerPattern, nil
        
    case "contains":
        return strings.Contains(
            strings.ToLower(text),
            strings.ToLower(reaction.TriggerPattern),
        ), nil
    }
}
```

### 3. Cooldown проверка (`shouldSkipDueToCooldown`)

```go
func (m *ReactionsModule) shouldSkipDueToCooldown(...) bool {
    // VIP пользователи пропускают cooldown
    if reaction.IsVIP {
        return false
    }
    
    // Получаем последнее срабатывание этой реакции в чате
    query := `
        SELECT triggered_at FROM reactions_log
        WHERE chat_id = $1 AND reaction_id = $2
        ORDER BY triggered_at DESC LIMIT 1
    `
    var lastTriggered time.Time
    err := m.db.QueryRow(query, chatID, reaction.ID).Scan(&lastTriggered)
    
    if err == sql.ErrNoRows {
        return false // Первый раз срабатывает
    }
    
    // Проверяем прошло ли cooldown_minutes времени
    cooldownDuration := time.Duration(reaction.CooldownMinutes) * time.Minute
    return time.Since(lastTriggered) < cooldownDuration
}
```

### 4. Выполнение реакции (`executeReaction`)

```go
func (m *ReactionsModule) executeReaction(ctx *core.MessageContext, reaction ReactionConfig) error {
    switch reaction.ReactionType {
    case "text":
        // Отправляем текстовый ответ
        _, err := ctx.Bot.Send(ctx.Chat, reaction.ReactionData)
        return err
        
    case "sticker":
        // Отправляем стикер по file_id
        sticker := &tele.Sticker{File: tele.File{FileID: reaction.ReactionData}}
        _, err := ctx.Bot.Send(ctx.Chat, sticker)
        return err
        
    case "delete":
        // Удаляем сообщение пользователя
        return ctx.Bot.Delete(ctx.Message)
    }
}
```

---

## 📊 Пример использования

### Сценарий: Автоприветствие и антиспам

**1. Добавляем реакцию на приветствие (regex):**
```bash
/addreaction text regex (?i)(привет|здравствуй|hi|hello) text "Добро пожаловать! 👋" 60
```
- Триггер: regex `(?i)(привет|здравствуй|hi|hello)` (case insensitive)
- Реакция: отправить текст "Добро пожаловать! 👋"
- Cooldown: 60 минут (чтобы не спамить при каждом "привет")

**2. Добавляем удаление спама (contains):**
```bash
/addreaction text contains спам delete "" 0
```
- Триггер: содержит слово "спам" (contains)
- Реакция: удалить сообщение (delete)
- Cooldown: 0 минут (удаляем каждый раз)

**3. Смотрим список реакций:**
```bash
/listreactions
```

**Вывод:**
```
📋 Реакции чата (2):

✅ #1 | text/regex | `(?i)(привет|здравствуй|hi|hello)` → text (60m)
✅ #2 | text/contains | `спам` → delete (0m)

💡 Для удаления: /delreaction <id>
```

**4. Тестируем regex паттерн:**
```bash
/testreaction (?i)привет ПРИВЕТ МИР
```

**Вывод:**
```
🧪 Тест паттерна:

Pattern: `(?i)привет`
Text: `ПРИВЕТ МИР`

Результаты:
• regex: ✅ совпадение
• exact: ❌ нет
• contains: ✅ совпадение
```

**5. Удаляем реакцию:**
```bash
/delreaction 2
```

---

## 🔗 Интеграция с другими модулями

### 1. Event Log

Каждое добавление/удаление реакции логируется через `eventRepo.Log()`:
```go
eventRepo.Log(chatID, userID, "reactions", "add_reaction", 
    fmt.Sprintf("Added reaction #%d: text/regex/(?i)привет", reactionID))
```

### 2. Module Repository

Проверка включён ли модуль через `moduleRepo.IsEnabled()`:
```go
enabled, err := m.moduleRepo.IsEnabled(chatID, "reactions")
if !enabled {
    return nil // Модуль выключен для этого чата
}
```

### 3. Statistics Module (Phase 4)

В будущем `reactions_log` будет использоваться для статистики:
- Топ-10 срабатывающих реакций
- Статистика нарушений по `violation_code`
- Частота срабатывания реакций по времени суток

---

## ⚙️ Технические детали

### Структуры данных

```go
type ReactionsModule struct {
    db         *sql.DB
    logger     *zap.Logger
    moduleRepo *repositories.ModuleRepository
    eventRepo  *repositories.EventRepository
    adminUsers []int64 // Список админов (задаётся через SetAdminUsers)
}

type ReactionConfig struct {
    ID              int64
    ChatID          int64
    ContentType     string  // "text", "photo", "video", etc.
    TriggerType     string  // "regex", "exact", "contains"
    TriggerPattern  string  // паттерн для поиска
    ReactionType    string  // "text", "sticker", "delete"
    ReactionData    string  // текст или file_id
    ViolationCode   int     // код нарушения (для статистики)
    CooldownMinutes int     // минуты между реакциями
    IsEnabled       bool    // включена ли реакция
    IsVIP           bool    // пропускает cooldown
}
```

### Логирование

Все операции логируются через `zap.Logger`:

```go
// Успешная реакция
m.logger.Info("reaction triggered",
    zap.Int64("reaction_id", reaction.ID),
    zap.Int64("chat_id", ctx.Chat.ID),
    zap.Int64("user_id", ctx.Sender.ID),
    zap.String("trigger_type", reaction.TriggerType),
)

// Ошибка выполнения
m.logger.Error("failed to execute reaction",
    zap.Int64("reaction_id", reaction.ID),
    zap.Error(err),
)

// Cooldown skip
m.logger.Debug("skipped reaction due to cooldown",
    zap.Int64("reaction_id", reaction.ID),
    zap.Duration("since_last", time.Since(lastTriggered)),
)
```

### Обработка ошибок

- ❌ Если regex не компилируется → пропускаем реакцию и логируем ошибку
- ❌ Если не удалось отправить sticker/text → логируем но не останавливаем обработку других реакций
- ❌ Если не удалось удалить сообщение → логируем (возможно бот не имеет прав)

---

## 🧪 Тестирование

### Unit тесты (TODO Phase 4)

```go
func TestReactionsModule_CheckPattern(t *testing.T) {
    tests := []struct {
        name     string
        pattern  string
        triggerType string
        text     string
        wantMatch bool
    }{
        {"regex case insensitive", "(?i)привет", "regex", "ПРИВЕТ", true},
        {"exact match", "hello", "exact", "hello", true},
        {"exact no match", "hello", "exact", "Hello", false},
        {"contains match", "спам", "contains", "это спам", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test logic
        })
    }
}
```

### Интеграционные тесты

1. Создать тестового бота с reactions модулем
2. Отправить сообщение "привет" → проверить что бот ответил
3. Отправить ещё раз → проверить что cooldown сработал (не ответил)
4. Подождать cooldown_minutes → отправить снова → проверить что ответил

---

## 📝 Миграция из Python бота

### Аналог Python функций

| Python (rts_bot) | Go (bmft) | Комментарий |
|------------------|-----------|-------------|
| `checkmessage.regextext()` | `checkPattern()` | Проверка regex/exact/contains |
| `checkmessage.sendreaction()` | `executeReaction()` | Отправка text/sticker/delete |
| `checkmessage.basecounttext()` | `shouldSkipDueToCooldown()` | Проверка cooldown через reactions_log |
| `reaction.newmessage()` | `OnMessage()` | Главный обработчик сообщений |
| `reaction.reactionversion()` | `/listreactions` | Список реакций |

### Отличия от Python бота

1. **✅ Улучшено:** В Go cooldown проверяется через БД (`reactions_log`), а не через in-memory счётчики
2. **✅ Улучшено:** Добавлен `/testreaction` для тестирования паттернов без сохранения
3. **✅ Улучшено:** VIP bypass через флаг `is_vip` вместо хардкода списка пользователей
4. **⚠️ Отложено:** Content type limiting (photo/video/sticker) из Python бота планируется отдельно в Phase 5

---

## 🔮 Будущие улучшения (Phase 5+)

- [ ] **Reaction groups:** Группировка реакций (например "приветствия", "мат", "спам")
- [ ] **Rate limiting per user:** Ограничение реакций на одного пользователя (не только per reaction)
- [ ] **Content type matching:** Реакции на photo/video/sticker по дополнительным критериям (размер, caption)
- [ ] **Mute reaction:** Реализовать `reaction_type = "mute"` (временный мут пользователя)
- [ ] **Webhook для реакций:** API endpoint для добавления реакций через внешние системы
- [ ] **Export/Import:** Экспорт/импорт reactions_config в JSON для переноса между чатами

---

## 📚 См. также

- **Общая архитектура:** [README.md](../../README.md)
- **Миграции:** [migrations/001_initial_schema.sql](../../migrations/001_initial_schema.sql)
- **Core интерфейсы:** [internal/core/interface.go](../../internal/core/interface.go)
- **Module Registry:** [internal/core/registry.go](../../internal/core/registry.go)
- **Python reference:** `/flybasist_dev/git/rts_bot/checkmessage.py`, `reaction.py`

---

**Версия:** 1.0.0  
**Автор:** @flybasist  
**Последнее обновление:** 2025-01-20
