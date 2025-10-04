# Ответы на твои вопросы + Модульность

## ✅ Ответы:

### 01 - Webhook vs Long Polling
**Ответ:** Будем использовать **Long Polling** (как в Python версии).

**Пояснение:** Webhook требует:
- Публичный домен с SSL сертификатом
- Telegram отправляет POST запросы на твой URL
- Меньше latency, но сложнее setup

Long Polling (текущий подход):
- Бот сам запрашивает обновления у Telegram
- Не нужен публичный домен
- Проще для разработки и деплоя
- Достаточно для твоего RPS (~0.004)

### 02 - Структура БД
**Ответ:** ✅ Создана **оптимизированная схема** в `migrations/001_initial_schema.sql`

**Ключевые улучшения:**
1. **Единая таблица `messages`** вместо `{chat_id}` таблиц
   - Партиционирование по дате (messages_2025_10, messages_2025_11, etc.)
   - Легче делать аналитику
   - Проще бэкапы

2. **Нормализованная таблица `limiter_config`**
   - Одна строка = один лимит
   - Не 12 колонок (audio, photo, video...), а:
     ```sql
     {chat_id, user_id, content_type, daily_limit}
     ```

3. **Таблица `chat_modules`** для управления модулями
   - Каждый чат включает нужные модули
   - `config JSONB` для специфичных настроек модуля

4. **Таблица `chat_admins`** для управления правами
   - Админы чатов настраивают лимиты самостоятельно
   - Не нужно писать тебе в личку

### 03 - Scheduled tasks
**Ответ:** ✅ Создан **модуль Scheduler**

**Функционал:**
- Cron-задачи через `github.com/robfig/cron/v3`
- Таблица `scheduler_tasks` с полями:
  - `cron_expression`: `"0 9 * * *"` (каждый день в 9:00)
  - `task_type`: `"sticker"`, `"text"`, `"poll"`
  - `task_data`: JSONB с file_id стикера или текстом
- Команды управления: `/addtask`, `/listtasks`, `/deltask`

**Пример из Python:**
```python
# scheduletask.py
schedule.every().day.at("09:00").do(send_sticker, chatid, sticker_id)
```

**Аналог в Go:**
```go
// internal/modules/scheduler/cron.go
c := cron.New()
c.AddFunc("0 9 * * *", func() {
    sendSticker(chatID, stickerID)
})
c.Start()
```

### 04 - Приоритеты
**Ответ:** ✅ План миграции создан — **8 фаз, 15-20 дней**

**MVP (7-10 дней):**
- Фаза 1: Базовый каркас (без Kafka, telebot.v3)
- Фаза 2: Модуль Limiter (лимиты на контент)
- Фаза 3: Модуль Reactions (реакции на слова)
- Фаза 4: Модуль Statistics (статистика)

**Full (12-16 дней):**
- Фаза 5: Модуль Scheduler (задачи по расписанию)
- Фаза 7: Админ-панель (управление через команды)

**Production (15-20 дней):**
- Фаза 8: Метрики, CI/CD, бэкапы

### 05 - Админка
**Ответ:** ✅ Пока **команды в Telegram**

**Будущее:** Telegram Mini App (WebView)
- Это встроенный браузер внутри Telegram
- Можно сделать веб-интерфейс с React/Vue
- Открывается по кнопке в чате с ботом
- Полноценная админка с графиками и настройками

---

## 🧩 Модульность: Plugin-based архитектура

### Концепция "Каркас + Плагины"

```
ЯДРО (Core Framework)
├── Bot initialization
├── Module registry
├── Message routing
└── Database layer

ПЛАГИНЫ (Modules)
├── Limiter        ← включить/выключить per chat
├── Reactions      ← включить/выключить per chat
├── AntiSpam       ← включить/выключить per chat
├── Statistics     ← включить/выключить per chat
├── Scheduler      ← включить/выключить per chat
└── Custom Module  ← твой собственный модуль
```

### Как это работает:

#### 1. Каждый модуль = интерфейс

```go
type Module interface {
    Name() string                                  // "limiter"
    Description() string                           // "Лимиты на контент"
    Init(deps *ModuleDependencies) error          // Инициализация
    OnMessage(ctx *MessageContext) error          // Обработка сообщения
    Commands() []Command                           // Команды модуля
    Shutdown() error                               // Корректное завершение
}
```

#### 2. Включение/выключение модулей per chat

```sql
-- Таблица chat_modules
chat_id  | module_name | is_enabled | config
---------+-------------+------------+------------------------
-1001... | limiter     | true       | {"max_warnings": 3}
-1001... | reactions   | true       | {}
-1001... | antispam    | false      | {"threshold": 5}
36382... | limiter     | true       | {}
36382... | reactions   | false      | {}
```

**Команды:**
```
/modules                    # показать доступные модули
/modules enable limiter     # включить модуль limiter
/modules disable antispam   # выключить модуль antispam
```

#### 3. Модули независимы

**Плохой подход (зависимость):**
```go
// ❌ Модуль reactions зависит от limiter
func (r *ReactionsModule) OnMessage(ctx *MessageContext) error {
    limit := ctx.Limiter.GetLimit(...) // зависимость!
}
```

**Правильный подход (независимость):**
```go
// ✅ Модули не знают друг о друге
func (r *ReactionsModule) OnMessage(ctx *MessageContext) error {
    // Модуль работает сам по себе
    // Общие данные в БД, не в других модулях
}
```

#### 4. Создание нового модуля = 5 шагов

**Пример: создаём модуль "Captcha" для новых участников**

**Шаг 1:** Создать структуру
```bash
mkdir -p internal/modules/captcha
touch internal/modules/captcha/{module.go,service.go,repository.go,commands.go}
```

**Шаг 2:** Реализовать интерфейс Module
```go
// internal/modules/captcha/module.go
package captcha

type CaptchaModule struct {
    name string
    // ...
}

func New() *CaptchaModule {
    return &CaptchaModule{name: "captcha"}
}

func (m *CaptchaModule) Name() string { return m.name }
func (m *CaptchaModule) Init(...) error { /* ... */ }
func (m *CaptchaModule) OnMessage(...) error {
    // Логика капчи для новых участников
}
func (m *CaptchaModule) Commands() []Command {
    return []Command{
        {Command: "/setcaptcha", Handler: m.handleSetCaptcha},
    }
}
```

**Шаг 3:** Создать таблицу в БД (миграция)
```sql
-- migrations/002_add_captcha.sql
CREATE TABLE captcha_config (
    chat_id BIGINT PRIMARY KEY,
    captcha_type VARCHAR(20), -- 'math', 'button', 'image'
    timeout_seconds INT DEFAULT 60,
    ban_on_failure BOOLEAN DEFAULT false
);
```

**Шаг 4:** Зарегистрировать модуль
```go
// cmd/bot/main.go
registry.Register(limiter.New())
registry.Register(reactions.New())
registry.Register(captcha.New()) // <- новый модуль
```

**Шаг 5:** Готово! Модуль доступен через `/modules enable captcha`

### Преимущества для аналитики

**Все события логируются:**
```sql
-- Таблица event_log
SELECT * FROM event_log WHERE event_type = 'limit_exceeded';
SELECT * FROM event_log WHERE module_name = 'antispam';
```

**Статистика по модулям:**
```sql
-- Сколько раз срабатывал limiter за неделю?
SELECT COUNT(*) FROM event_log 
WHERE module_name = 'limiter' 
AND created_at > NOW() - INTERVAL '7 days';

-- Какие модули самые активные?
SELECT module_name, COUNT(*) as events
FROM event_log
GROUP BY module_name
ORDER BY events DESC;
```

**Аналитика сообщений:**
```sql
-- messages таблица — единая для всех чатов
-- можно делать кросс-чатовую аналитику

-- Самые активные часы:
SELECT EXTRACT(HOUR FROM created_at) as hour, COUNT(*)
FROM messages
GROUP BY hour
ORDER BY hour;

-- Распределение типов контента:
SELECT content_type, COUNT(*)
FROM messages
WHERE created_at > NOW() - INTERVAL '30 days'
GROUP BY content_type;

-- Топ пользователей по сообщениям:
SELECT u.username, COUNT(m.id) as msg_count
FROM messages m
JOIN users u ON u.user_id = m.user_id
GROUP BY u.username
ORDER BY msg_count DESC
LIMIT 10;
```

---

## 📦 Пример конфигурации модуля через JSONB

```sql
-- Модуль limiter с кастомными настройками
INSERT INTO chat_modules (chat_id, module_name, is_enabled, config) VALUES
(-1001637543078, 'limiter', true, '{
  "max_warnings": 3,
  "reset_warnings_after_hours": 24,
  "vip_users": [36382182, 227345104]
}');

-- Модуль antispam с настройками
INSERT INTO chat_modules (chat_id, module_name, is_enabled, config) VALUES
(-1001637543078, 'antispam', true, '{
  "flood_threshold": 5,
  "flood_window_seconds": 10,
  "ban_duration_seconds": 3600,
  "whitelist_domains": ["github.com", "stackoverflow.com"]
}');
```

**Чтение конфига в модуле:**
```go
func (m *AntiSpamModule) OnMessage(ctx *MessageContext) error {
    config, _ := ctx.GetChatConfig(m.name)
    
    floodThreshold := config["flood_threshold"].(float64) // 5
    whitelistDomains := config["whitelist_domains"].([]interface{})
    
    // Используем настройки
}
```

---

## 🚀 Итоговая схема модульности

```
┌─────────────────────────────────────────────────────────┐
│                  Telegram Bot API                       │
└────────────────────┬────────────────────────────────────┘
                     │ Long Polling
                     ▼
┌─────────────────────────────────────────────────────────┐
│              Core Framework (Ядро)                      │
│  ┌───────────────────────────────────────────────────┐  │
│  │ Module Registry                                   │  │
│  │ - RegisterModule()                                │  │
│  │ - GetActiveModules(chatID)                        │  │
│  │ - RouteMessage(message) → []Module                │  │
│  └───────────────────────────────────────────────────┘  │
│  ┌───────────────────────────────────────────────────┐  │
│  │ Message Router                                    │  │
│  │ For each active module:                           │  │
│  │   module.OnMessage(context)                       │  │
│  └───────────────────────────────────────────────────┘  │
└────────────────────┬────────────────────────────────────┘
                     │
    ┌────────────────┼────────────────┬────────────────┐
    ▼                ▼                ▼                ▼
┌─────────┐    ┌─────────┐    ┌──────────┐    ┌──────────┐
│ Limiter │    │Reactions│    │ AntiSpam │    │ Custom   │
│ Module  │    │ Module  │    │ Module   │    │ Module   │
└────┬────┘    └────┬────┘    └────┬─────┘    └────┬─────┘
     │              │              │               │
     └──────────────┴──────────────┴───────────────┘
                     │
                     ▼
            ┌─────────────────┐
            │   PostgreSQL    │
            │  - messages     │
            │  - chat_modules │
            │  - limiter_*    │
            │  - reactions_*  │
            │  - event_log    │
            └─────────────────┘
```

**Ключевые моменты:**
1. **Один message → все активные модули** (параллельно или последовательно)
2. **Модули регистрируют команды** → бот автоматически их обрабатывает
3. **Включение/выключение per chat** → админ чата управляет
4. **Аналитика централизована** → все в одной БД
5. **Новый модуль = +1 файл в registry** → никаких изменений в core

---

## ✅ Готовность к следующему шагу

Создано:
- ✅ SQL схема: `migrations/001_initial_schema.sql`
- ✅ Архитектурная документация: `ARCHITECTURE.md`
- ✅ План миграции: `MIGRATION_PLAN.md`
- ✅ Ответы на вопросы: этот файл

**Следующий шаг: Фаза 1 — Базовый каркас**

Начинаем удалять Kafka и создавать core framework?
