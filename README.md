# BMFT — Bot Moderator Framework for Telegram

**Модульный бот для управления Telegram-чатами с plugin-based архитектурой.**

[![Go Version](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-12+-316192?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)

## 📖 Описание проекта

**BMFT** (Bot Moderator For Telegram) — это модульная система для управления Telegram-чатами. Каждая фи## 🗄️ База данных

- **Схема:** [`migrations/001_initial_schema.sql`](migrations/001_initial_schema.sql) — полная PostgreSQL схема
- **Миграции:** Используем [golang-migrate](https://github.com/golang-migrate/migrate)
- **Таблицы:** chats, chat_modules, messages, event_log, user_stats и др.тдельный модуль, который можно включать/выключать для каждого чата индивидуально.

**⚡ Quick Start:**
```bash
git clone <repo> && cd bmft
cp .env.example .env  # Укажите TELEGRAM_BOT_TOKEN
docker run -d --name postgres -e POSTGRES_PASSWORD=secret -p 5432:5432 postgres:16
migrate -path migrations -database "postgres://postgres:secret@localhost/postgres?sslmode=disable" up
go run cmd/bot/main.go
```

### 🔌 Доступные модули:

- **Limiter** — лимиты на запросы пользователей (daily/monthly per user) ✅
  - ⚠️ *Примечание:* Content type limiter (photo/video/sticker из Python бота) планируется отдельно
- **Reactions** — автоматические реакции на ключевые слова (regex) 🔜
- **Statistics** — статистика сообщений и активности 🔜
- **Scheduler** — задачи по расписанию (cron-like) 🔜
- **AntiSpam** — антиспам фильтры (в будущем) 🔮
- **Custom** — добавь свой модуль за 5 минут!

### 🎯 Преимущества модульной архитектуры:

1. **Гибкость:** Админ чата сам выбирает нужные модули через команды
2. **Масштабируемость:** Новый модуль = просто реализовать интерфейс
3. **Независимость:** Модули не знают друг о друге
4. **Аналитика:** Все события в единой БД для cross-chat анализа

### Ключевые возможности:

- ✅ **Plugin architecture** — каждая фича = отдельный модуль (limiter, reactions, stats, scheduler)
- ✅ **Per-chat module control** — админ чата сам выбирает нужные модули через команды
- ✅ **Unified database** — все данные в одной PostgreSQL (cross-chat аналитика)
- ✅ **Long Polling** — нет нужды в публичном домене/webhook
- ✅ **Graceful shutdown** — корректная остановка всех модулей при SIGINT/SIGTERM
- ✅ **Structured logging** — zap для операционных логов
- ✅ **Event audit** — все действия модулей логируются в `event_log`

## 🏗 Архитектура

### Plugin-based модульная система:

```
                    ┌──────────────────┐
                    │  Telegram API    │
                    └────────┬─────────┘
                             │ Long Polling
                             ▼
                    ┌──────────────────┐
                    │  Bot (telebot.v3)│
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ Module Registry  │◄──── chat_modules (config)
                    └────────┬─────────┘
                             │
         ┌───────────────────┼───────────────────┐
         ▼                   ▼                   ▼
    ┌─────────┐         ┌─────────┐        ┌─────────┐
    │ Limiter │         │Reactions│        │  Stats  │
    │ Module  │         │ Module  │        │ Module  │
    └────┬────┘         └────┬────┘        └────┬────┘
         │                   │                   │
         └───────────────────┴───────────────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │   PostgreSQL     │
                    │ (unified schema) │
                    └──────────────────┘
```

### Интерфейс модуля:

Каждый модуль реализует простой интерфейс:

```go
type Module interface {
    Init(deps ModuleDependencies) error      // Инициализация при старте
    OnMessage(ctx MessageContext) error      // Обработка сообщения
    Commands() []BotCommand                  // Список команд модуля
    Enabled(chatID int64) bool              // Включен ли для чата
    Shutdown() error                         // Graceful shutdown
}
```

### Компоненты системы:

#### 1. **Core** (`internal/core/`)
- Module Registry — управление жизненным циклом модулей
- Message Router — маршрутизация сообщений к активным модулям
- Module Dependencies — DI контейнер (DB, logger, bot instance)
- Middleware layer — rate limiting, logging, panic recovery

#### 2. **Modules** (`internal/modules/`)
- **limiter** — лимиты на типы контента (фото, видео, стикеры)
- **reactions** — автоматические реакции на ключевые слова (regex)
- **statistics** — статистика сообщений и активности юзеров
- **scheduler** — задачи по расписанию (cron-like)
- **antispam** — антиспам фильтры (в разработке)

#### 3. **PostgreSQL** (`migrations/`)
- Unified schema: `chats`, `users`, `chat_modules`, `messages` (partitioned)
- Per-module tables: `limiter_config`, `reactions_config`, `scheduler_tasks`
- Analytics: `statistics_daily`, `event_log` для audit trail

#### 4. **Config** (`internal/config/`)
- Загрузка конфигурации из `.env`
- Валидация обязательных параметров
- Module-specific settings через JSONB в `chat_modules.config`

## 🚀 Быстрый старт

### Требования:

- Go 1.25+
- PostgreSQL 12+
- Docker (опционально)

### Установка:

```bash
# 1. Клонируйте репозиторий
git clone <repository-url>
cd bmft

# 2. Скопируйте пример конфигурации
cp .env.example .env

# 3. Отредактируйте .env — укажите токен бота и PostgreSQL DSN
nano .env

# 4. Запустите PostgreSQL (если нужно)
docker run -d --name bmft-postgres \
  -e POSTGRES_USER=bmft \
  -e POSTGRES_PASSWORD=secret \
  -e POSTGRES_DB=bmft \
  -p 5432:5432 \
  postgres:16

# 5. Примените миграции
migrate -path migrations -database "postgres://bmft:secret@localhost:5432/bmft?sslmode=disable" up

# 6. Запустите бота
go run cmd/bot/main.go
```

### Локальная разработка:

```bash
# Установите зависимости
go mod download

# Установите переменные окружения
export TELEGRAM_BOT_TOKEN="123456:ABCdefGHIjklMNOpqrsTUVwxyz"
export POSTGRES_DSN="postgres://bmft:secret@localhost:5432/bmft?sslmode=disable"
export LOG_LEVEL="debug"

# Запустите тесты
go test ./...

# Запустите приложение
go run cmd/bot/main.go
```

## ⚙️ Конфигурация

Все настройки задаются через **переменные окружения**:

### Обязательные параметры:

| Переменная | Описание | Пример |
|------------|----------|--------|
| `TELEGRAM_BOT_TOKEN` | Токен Telegram-бота (получить у @BotFather) | `123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11` |
| `POSTGRES_DSN` | Строка подключения к PostgreSQL | `postgres://user:pass@localhost:5432/bmft?sslmode=disable` |

### Опциональные параметры:

| Переменная | Описание | Значение по умолчанию |
|------------|----------|-----------------------|
| `LOG_LEVEL` | Уровень логирования: `debug`, `info`, `warn`, `error` | `info` |
| `LOGGER_PRETTY` | Человекочитаемые логи (для dev) | `false` |
| `SHUTDOWN_TIMEOUT` | Таймаут graceful shutdown | `15s` |
| `METRICS_ADDR` | Адрес HTTP-сервера метрик (placeholder) | `:9090` |
| `POLLING_TIMEOUT` | Таймаут Long Polling в секундах | `60` |

### Пример `.env` файла:

```bash
# Обязательные
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
POSTGRES_DSN=postgres://bmft:bmftpass@postgres:5432/bmft?sslmode=disable

# Опциональные (для разработки)
LOG_LEVEL=debug
LOGGER_PRETTY=true
SHUTDOWN_TIMEOUT=10s
```

## � База данных PostgreSQL

Полная схема в файле `migrations/001_initial_schema.sql`.

### Основные таблицы:

#### `chats` — метаданные чатов
```sql
CREATE TABLE chats (
    chat_id BIGINT PRIMARY KEY,
    chat_type VARCHAR(20),  -- private, group, supergroup, channel
    title TEXT,
    username TEXT,
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);
```

#### `chat_modules` — активные модули для чатов
```sql
CREATE TABLE chat_modules (
    id SERIAL PRIMARY KEY,
    chat_id BIGINT REFERENCES chats(chat_id) ON DELETE CASCADE,
    module_name VARCHAR(50),  -- limiter, reactions, statistics, etc.
    is_enabled BOOLEAN DEFAULT TRUE,
    config JSONB DEFAULT '{}'::jsonb,  -- модуль-специфичные настройки
    UNIQUE(chat_id, module_name)
);
```

#### `messages` — партиционированное хранение сообщений
```sql
CREATE TABLE messages (
    id BIGSERIAL,
    chat_id BIGINT,
    user_id BIGINT,
    message_id BIGINT,
    content_type VARCHAR(20),  -- text, photo, video, sticker, etc.
    text TEXT,
    caption TEXT,
    has_media BOOLEAN DEFAULT FALSE,
    is_deleted BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT NOW(),
    PRIMARY KEY (id, created_at)  -- composite key для партиционирования
) PARTITION BY RANGE (created_at);

-- Партиции по месяцам
CREATE TABLE messages_2025_10 PARTITION OF messages
    FOR VALUES FROM ('2025-10-01') TO ('2025-11-01');
```

#### `limiter_config` — нормализованные лимиты
```sql
CREATE TABLE limiter_config (
    id SERIAL PRIMARY KEY,
    chat_id BIGINT,
    user_group VARCHAR(50) DEFAULT 'allmembers',  -- allmembers, vip, admin
    content_type VARCHAR(20),  -- photo, video, sticker, etc.
    daily_limit INTEGER,  -- -1 = banned, 0 = unlimited, N = limit
    UNIQUE(chat_id, user_group, content_type)
);
```

### Полезные view:

```sql
-- Активные модули по чатам
CREATE VIEW active_modules_by_chat AS
SELECT chat_id, ARRAY_AGG(module_name) as modules
FROM chat_modules
WHERE is_enabled = TRUE
GROUP BY chat_id;

-- Статистика за последний день
CREATE VIEW daily_stats AS
SELECT chat_id, content_type, COUNT(*) as count
FROM messages
WHERE created_at > NOW() - INTERVAL '1 day'
GROUP BY chat_id, content_type;
```

## 📝 Примеры использования

### Для админа чата:

```
# Базовые команды
/start                       # Приветствие и список команд
/help                        # Помощь по командам
/modules                     # Показать доступные модули
/enable limiter              # Включить модуль лимитов
/disable limiter             # Выключить модуль лимитов

# Команды модуля Limiter (Phase 2)
/limits                      # Посмотреть свои лимиты запросов

# Админские команды модуля Limiter
/setlimit <user_id> daily <limit>     # Установить дневной лимит
/setlimit <user_id> monthly <limit>   # Установить месячный лимит
/getlimit <user_id>                   # Посмотреть лимиты пользователя

# Будущие команды (Phase 3-5, Phase AI)
# Phase 3 - Reactions:
/addreaction <type> <regex> <reaction>  # Добавить реакцию (админ)
/listreactions               # Список всех реакций
/delreaction <id>            # Удалить реакцию (админ)
/testreaction <text>         # Проверить какие реакции сработают

# Phase 4 - Statistics:
/mystats                     # Моя статистика за день
/chatstats                   # Статистика чата (админ)

# Phase 5 - Scheduler:
/addtask <name> <cron> <type> <data>  # Добавить задачу (админ)
/listtasks                   # Список задач
/deltask <id>                # Удалить задачу (админ)
/runtask <id>                # Запустить задачу вручную (админ)

# Phase AI - AI Module:
/gpt <question>              # Задать вопрос AI
/reset                       # Сбросить контекст диалога
/context                     # Показать текущий контекст
```

### Для разработчика нового модуля:

```go
// 1. Создайте файл modules/mymodule/module.go
type MyModule struct {
    db  *sql.DB
    bot *telebot.Bot
    log *zap.Logger
}

// 2. Реализуйте интерфейс Module
func (m *MyModule) Init(deps core.ModuleDependencies) error {
    m.db = deps.DB
    m.bot = deps.Bot
    m.log = deps.Logger
    return nil
}

func (m *MyModule) OnMessage(ctx core.MessageContext) error {
    // Ваша логика обработки сообщения
    if ctx.Message.Text == "/mycommand" {
        m.bot.Send(ctx.Message.Chat, "Hello from MyModule!")
    }
    return nil
}

func (m *MyModule) Commands() []core.BotCommand {
    return []core.BotCommand{
        {Command: "/mycommand", Description: "My custom command"},
    }
}

func (m *MyModule) Enabled(chatID int64) bool {
    // Проверка в chat_modules таблице
    return true
}

func (m *MyModule) Shutdown() error {
    return nil
}

// 3. Зарегистрируйте модуль в cmd/bot/main.go
registry.Register("mymodule", &modules.MyModule{})
```

## 🚀 Миграция из Python

Если мигрируете из Python-версии (rts_bot):

```bash
# 1. Создайте новую БД с миграциями
migrate -path migrations -database "$POSTGRES_DSN" up

# 2. Импортируйте конфигурацию (limits + reactions)
python scripts/migrate_config.py --sqlite rtsbot.db --postgres "$POSTGRES_DSN"

# 3. Запустите бота и проверьте работу
go run cmd/bot/main.go

# Старые сообщения НЕ мигрируются (drop), только конфигурация
```

Подробный план миграции: `MIGRATION_PLAN.md`

## 📈 Мониторинг

HTTP-сервер метрик (placeholder) на порту `:9090`:

- `GET /healthz` — health check
- `GET /metrics` — Prometheus метрики (в разработке)

**Event Audit:** Все действия модулей логируются в таблицу `event_log`:

```sql
SELECT * FROM event_log 
WHERE chat_id = -1001234567890 
ORDER BY created_at DESC 
LIMIT 10;

-- Пример лога:
-- event_type=limit_exceeded, module_name=limiter, 
-- details={"user_id": 123, "content_type": "photo", "limit": 5}

## 🧪 Тестирование

```bash
# Запуск всех тестов
go test ./...

# Тесты с покрытием
go test -cover ./...

# Тесты конкретного модуля
go test -v ./internal/modules/limiter/...
```

## 🔧 Разработка

### Структура проекта (после миграции):

```
.
├── cmd/
│   └── bot/
│       └── main.go                # Точка входа
├── internal/
│   ├── config/                    # Конфигурация
│   │   └── config.go
│   ├── core/                      # Module Registry + Interfaces
│   │   ├── interface.go           # Module interface
│   │   ├── registry.go            # Module registry
│   │   └── context.go             # MessageContext
│   ├── modules/                   # Модули (features)
│   │   ├── limiter/               # Лимиты на контент
│   │   │   ├── module.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   └── commands.go
│   │   ├── reactions/             # Keyword reactions
│   │   ├── statistics/            # Статистика
│   │   ├── scheduler/             # Cron tasks
│   │   └── antispam/              # AntiSpam (в разработке)
│   ├── postgresql/                # База данных
│   │   ├── postgresql.go
│   │   └── repositories/
│   ├── logx/                      # Логирование (zap)
│   │   └── logx.go
│   └── utils/                     # Утилиты
│       ├── utils.go
│       └── utils_test.go
├── migrations/                    # Миграции БД
│   └── 001_initial_schema.sql
├── docker-compose.yaml            # PostgreSQL
├── Dockerfile
├── go.mod
└── README.md
```

### Правила разработки:

1. **Комментарии в коде и README — на русском языке**
2. **Runtime-логи и переменные — строго на английском**
3. Код должен быть понятен начинающим
4. Новые функции должны иметь подробные комментарии
5. Перед коммитом: `go vet ./...` и `go fmt ./...`

### Добавление нового модуля:

1. Создайте директорию `internal/modules/mymodule/`
2. Реализуйте интерфейс `core.Module` в `module.go`
3. Добавьте таблицы в новую миграцию (если нужны)
4. Зарегистрируйте в `cmd/bot/main.go`: `registry.Register("mymodule", &mymodule.Module{})`
5. Включите для чата: `/enable mymodule`

```go
func processBusinessLogic(update map[string]any) (map[string]any, error) {
    // Здесь можно реализовать:
    // - Фильтрацию сообщений по типу контента
    // - Начисление/снятие лимитов
    // - Отправку реакций/ответов в топик telegram-send
    // - Анализ нарушений правил чата
    
    return update, nil
}
```

## 🐛 Troubleshooting

### Проблема: Бот не реагирует на сообщения

**Решение:**
1. Проверьте что PostgreSQL запущен: `docker ps | grep postgres`
2. Проверьте миграции: `migrate -path migrations -database "$POSTGRES_DSN" version`
3. Проверьте логи: `docker logs bmft-bot -f` или консоль приложения

### Проблема: Модуль не работает в чате

**Решение:**
1. Проверьте что модуль включен: `/modules` или SQL:
   ```sql
   SELECT * FROM chat_modules WHERE chat_id = YOUR_CHAT_ID;
   ```
2. Включите модуль: `/enable limiter`
3. Проверьте конфигурацию в `chat_modules.config` (JSONB)

### Проблема: Ошибка "chat_id not found"

**Решение:**
Чат автоматически создается при первом сообщении. Если ошибка остается:
```sql
INSERT INTO chats (chat_id, chat_type, title) 
VALUES (YOUR_CHAT_ID, 'group', 'My Chat');
```

## 📝 Roadmap

### Phase 1 — Core Framework ✅ 100% Complete
- [x] Удалить Kafka инфраструктуру (Step 1)
- [x] Интегрировать telebot.v3 (Steps 2-5)
- [x] Создать Module Registry (Step 3)
- [x] Реализовать базовые команды (/start, /help, /modules, /enable, /disable) (Step 5)
- [x] Repository layer (ChatRepository, ModuleRepository, EventRepository) (Step 6)
- [x] Unit tests для config (Step 7)
- [x] Docker setup (Step 9)
- [x] Final verification (Step 10)
- [x] Code cleanup (удалено ~260 строк мёртвого кода)

**📦 Phase 1 Summary:** См. `PHASE1_SUMMARY.md` и `PRE_MERGE_CHECKLIST.md`

### Phase 2 — Limiter Module ✅ 100% Complete
- [x] Создана таблица user_limits (миграция 003)
- [x] LimitRepository (8 методов) — работа с лимитами пользователей
- [x] LimiterModule (17 методов) — модуль контроля лимитов
- [x] Команды: /limits, /setlimit, /getlimit
- [x] Daily counters с автосбросом (24 часа)
- [x] Monthly counters с автосбросом (30 дней)
- [x] Unit-тесты (10 тестов, 485 строк)
- [x] Интеграция с main.go
- [x] Документация обновлена

**📦 Phase 2 Summary:** См. `docs/development/PHASE2_SUMMARY.md`, `docs/development/PHASE2_FINAL_REPORT.md` и `docs/development/PHASE2_AUDIT_REPORT.md`

⚠️ **Важно:** Phase 2 реализует user request limiter (daily/monthly per user). Content type limiter (photo/video/sticker из Python бота) будет добавлен позже.

### Phase 3 (Следующая) — Reactions Module
- [ ] Миграция regex паттернов из Python бота (rts_bot)
- [ ] Cooldown система (10 минут между реакциями)
- [ ] Типы реакций: sticker, text, delete, mute
- [ ] Команды: /addreaction, /listreactions, /delreaction, /testreaction
- [ ] Антифлуд через reactions_log
- [ ] Подсчёт текстовых нарушений (violation_code=21)

### Phase 4 — Statistics Module
- [ ] Агрегация данных из messages → statistics_daily
- [ ] Команды: /mystats (личная статистика), /chatstats (админ)
- [ ] Форматированный вывод статистики по типам контента
- [ ] Top users, most active hours

### Phase 5 — Scheduler Module
- [ ] Cron-планировщик (robfig/cron)
- [ ] Миграция задач из Python scheduletask.py
- [ ] Задачи по расписанию (отправка стикеров, напоминания)
- [ ] Команды: /addtask, /listtasks, /deltask, /runtask

### Phase AI (В будущем) — AI Module
- [ ] OpenAI/Anthropic API интеграция
- [ ] Context Management (история диалогов)
- [ ] Интеграция с Limiter Module (проверка лимитов перед AI запросами)
- [ ] Команды: /gpt, /reset, /context
- [ ] Система промптов и модерация контента

### Phase AntiSpam (Опционально)
- [ ] Flood protection
- [ ] Link filtering
- [ ] User reputation system

### Phase 8 — Admin Panel
- [ ] Web интерфейс для управления
- [ ] Графики и аналитика
- [ ] Bulk configuration

**Полный план:** См. [`docs/architecture/MIGRATION_PLAN.md`](docs/architecture/MIGRATION_PLAN.md)

---

## 📚 Документация

### Для пользователей:
- 📘 [**Быстрый старт**](docs/guides/QUICKSTART.md) — Запуск за 5 минут
- 🤖 [**Что умеет бот сейчас**](docs/guides/CURRENT_BOT_FUNCTIONALITY.md) — Текущая функциональность
- 🔧 [**Troubleshooting**](docs/guides/VSCODE_CACHE_FIX.md) — Решение проблем

### Для разработчиков:
- 🏗️ [**Архитектура**](docs/architecture/ARCHITECTURE.md) — Дизайн системы
- 🗺️ [**План миграции**](docs/architecture/MIGRATION_PLAN.md) — 8 фаз разработки
- 💬 [**FAQ**](docs/FAQ.md) — Вопросы и ответы
- 📊 [**Phase 1 Summary**](docs/development/PHASE1_SUMMARY.md) — Отчёт по Phase 1
- 🚀 [**Phase 2 Transition**](docs/development/PHASE1_TO_PHASE2_TRANSITION.md) — Переход к Phase 2
- 📝 [**CHANGELOG**](docs/CHANGELOG.md) — История изменений

**📖 Полный список документов:** [`docs/README.md`](docs/README.md)

---

## 🤝 Contributing

Хочешь добавить свой модуль или улучшить существующий?

1. Fork проекта
2. Создай feature-ветку: `git checkout -b feature/my-awesome-module`
3. Реализуй модуль в `internal/modules/mymodule/`
4. Добавь тесты: `go test ./internal/modules/mymodule/...`
5. Коммит: `git commit -am 'Add my awesome module'`
6. Push: `git push origin feature/my-awesome-module`
7. Создай Pull Request

**Важно:**
- Комментарии в коде — на русском
- Runtime-логи и переменные — на английском
- Перед PR: `go vet ./...` + `go fmt ./...`

## � Дополнительная документация

- [`ARCHITECTURE.md`](ARCHITECTURE.md) — детальная архитектура модульной системы
- [`MIGRATION_PLAN.md`](MIGRATION_PLAN.md) — полный план миграции (8 фаз, 15-20 дней)
- [`ANSWERS.md`](ANSWERS.md) — ответы на вопросы по архитектурным решениям
- [`migrations/001_initial_schema.sql`](migrations/001_initial_schema.sql) — полная схема БД (443 строки)

## 💬 Контакты

- **Вопросы/баги:** [GitHub Issues](https://github.com/your-repo/bmft/issues)
- **Telegram:** @FlyBasist
- **Email:** flybasist92@gmail.com

---

## 🛡️ Лицензия

Этот проект распространяется под лицензией [GNU GPLv3](https://www.gnu.org/licenses/gpl-3.0.html).

Вы можете использовать, модифицировать и распространять этот код, при условии, что производные работы также будут открыты под лицензией GPLv3. Это означает, что если вы вносите изменения и распространяете модифицированную версию, вы обязаны предоставить исходный код этих изменений.

В случае использования кода **внутри организации** без его распространения — раскрытие изменений не требуется.

**Автор:** Alexander Ognev (aka FlyBasist)  
**Год:** 2025

---

**⭐ Если проект оказался полезен — поставь звезду на GitHub!**

---

### 🇺🇸 English

This project is licensed under the [GNU GPLv3](https://www.gnu.org/licenses/gpl-3.0.html).

You are free to use, modify, and distribute this code under the condition that any derivative works are also licensed under GPLv3. This means if you make changes and distribute your modified version, you must make the source code of those changes available.

If you use the code **within your organization** without distributing it externally, you are not required to disclose your modifications.

**Author:** Alexander Ognev (aka FlyBasist)  
**Year:** 2025
