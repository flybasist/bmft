# 🤖 Текущая функциональность бота (Phase 1)

**Версия:** 0.2.1  
**Статус:** ✅ Полностью рабочий  
**Последний коммит:** Phase 1 Complete

---

## 🎯 Что умеет бот СЕЙЧАС:

### 1. **Базовые команды (5 штук):**

#### `/start` — Приветствие и инициализация
**Кто может:** Все пользователи  
**Что делает:**
- Создаёт запись чата в БД (таблица `chats`)
- Показывает welcome message с описанием возможностей
- Логирует событие в `event_log` (audit trail)

**Пример ответа:**
```
🤖 Привет! Я BMFT — модульный бот для управления Telegram-чатами.

📋 Основные команды:
/help — список всех команд
/modules — показать доступные модули (только админы)
/enable <module> — включить модуль (только админы)
/disable <module> — выключить модуль (только админы)

Добавьте меня в группу и дайте права администратора для полной функциональности!
```

---

#### `/help` — Справка
**Кто может:** Все пользователи  
**Что делает:**
- Показывает список всех доступных команд
- Информирует о будущих модулях (limiter, reactions, statistics, scheduler, antispam)

**Пример ответа:**
```
📖 Доступные команды:

🔹 Основные:
/start — приветствие и инициализация
/help — эта справка

🔹 Управление модулями (только админы):
/modules — показать все модули
/enable <module> — включить модуль
/disable <module> — выключить модуль

🔹 Модули будут добавлены в Phase 2-6:
- limiter — лимиты на типы контента
- reactions — автоматические реакции
- statistics — статистика чата
- scheduler — задачи по расписанию
- antispam — антиспам фильтры
```

---

#### `/modules` — Список модулей
**Кто может:** ⚠️ Только администраторы чата (в группах)  
**Что делает:**
- Проверяет права администратора через `bot.AdminsOf()`
- Показывает список зарегистрированных модулей с их статусом (✅ Включен / ❌ Выключен)
- Показывает команды каждого модуля

**Сейчас:**
```
📦 Модули пока не зарегистрированы. Будут добавлены в Phase 2-6.
```

**После Phase 2 (пример):**
```
📦 Доступные модули:

🔹 limiter — ✅ Включен
  Команды: /setlimit, /showlimits, /mystats

🔹 reactions — ❌ Выключен
  Команды: /addreaction, /listreactions, /delreaction
```

---

#### `/enable <module>` — Включить модуль
**Кто может:** ⚠️ Только администраторы чата  
**Что делает:**
- Проверяет что модуль зарегистрирован в `ModuleRegistry`
- Включает модуль для текущего чата (запись в `chat_modules`)
- Логирует событие в `event_log`

**Использование:**
```
/enable limiter
```

**Ответ:**
```
✅ Модуль 'limiter' включен для этого чата.
```

**Если модуль не найден:**
```
❌ Модуль 'limiter' не найден. Используйте /modules для просмотра доступных модулей.
```

---

#### `/disable <module>` — Выключить модуль
**Кто может:** ⚠️ Только администраторы чата  
**Что делает:**
- Выключает модуль для текущего чата (обновление `chat_modules.is_enabled = FALSE`)
- Логирует событие в `event_log`

**Использование:**
```
/disable limiter
```

**Ответ:**
```
❌ Модуль 'limiter' выключен для этого чата.
```

---

### 2. **Обработка всех сообщений:**

#### `OnText` handler
**Что делает:**
- Ловит ВСЕ текстовые сообщения в чате
- Создаёт `MessageContext` с полезными данными (Message, Bot, DB, Logger, Chat, Sender)
- Передаёт сообщение в `ModuleRegistry.OnMessage()`
- `ModuleRegistry` вызывает `OnMessage()` у **всех активных модулей** для этого чата

**Сейчас:** Модулей нет → ничего не происходит  
**После Phase 2:** Limiter Module будет проверять лимиты на фото/видео/стикеры

---

### 3. **Модульная система (готова к расширению):**

#### Module Registry
**Что это:**
- Централизованный менеджер всех модулей
- Управляет lifecycle: `Init()` → `OnMessage()` → `Shutdown()`
- Проверяет активность модуля для каждого чата через `Enabled(chatID)`

**Методы:**
```go
registry.Register("limiter", &limiter.Module{})  // Регистрация модуля
registry.InitAll()                                // Инициализация всех
registry.OnMessage(ctx)                           // Обработка сообщения
registry.GetModules()                             // Список модулей
registry.ShutdownAll()                            // Graceful shutdown
```

**Текущее состояние:**
```go
// TODO: Регистрируем модули здесь (Phase 2-6)
// registry.Register("limiter", &limiter.Module{})
// registry.Register("reactions", &reactions.Module{})
// registry.Register("statistics", &statistics.Module{})
// registry.Register("scheduler", &scheduler.Module{})
```

---

### 4. **Middleware (3 функции):**

#### LoggerMiddleware
- Логирует каждое входящее сообщение (chat_id, user_id, text preview)
- Помогает в дебаге и аудите

#### PanicRecoveryMiddleware
- Ловит panic в хендлерах
- Предотвращает падение всего бота из-за одной ошибки
- Логирует stack trace

#### RateLimitMiddleware (placeholder)
- Сейчас: просто логирует "rate limit check"
- В будущем: защита от flood (например, 10 команд в минуту)

---

### 5. **Repository Layer (работа с БД):**

#### ChatRepository
**Методы:**
- `GetOrCreate(chatID, chatType, title, username)` — создать чат если не существует
- `IsActive(chatID)` — проверить активность чата
- `Deactivate(chatID)` — деактивировать чат
- `GetChatInfo(chatID)` — получить информацию о чате

#### ModuleRepository
**Методы:**
- `IsEnabled(chatID, moduleName)` — включен ли модуль для чата
- `Enable(chatID, moduleName)` — включить модуль
- `Disable(chatID, moduleName)` — выключить модуль
- `GetConfig(chatID, moduleName)` — получить JSONB конфиг модуля
- `UpdateConfig(chatID, moduleName, config)` — обновить JSONB конфиг
- `GetEnabledModules(chatID)` — список активных модулей

#### EventRepository
**Методы:**
- `Log(chatID, userID, module, eventType, details)` — записать событие в audit log
- `GetRecentEvents(chatID, limit)` — получить последние N событий

**Использование:** Все команды логируются в `event_log` для аналитики и аудита.

---

### 6. **Graceful Shutdown:**

**Что происходит при Ctrl+C (SIGINT) или SIGTERM:**
1. Бот перестаёт принимать новые сообщения (`bot.Stop()`)
2. Все модули корректно завершают работу (`registry.ShutdownAll()`)
3. Закрывается соединение с PostgreSQL (`db.Close()`)
4. Если shutdown превышает таймаут (15 секунд) → forced exit
5. Логируется весь процесс остановки

**Лог примера:**
```
2025-10-04T12:00:00.000Z  INFO  received shutdown signal  {"signal": "interrupt"}
2025-10-04T12:00:00.001Z  INFO  shutting down bot...
2025-10-04T12:00:00.005Z  INFO  shutting down modules...
2025-10-04T12:00:00.010Z  INFO  closing database connection...
2025-10-04T12:00:00.015Z  INFO  bot shutdown complete
```

---

### 7. **Long Polling (60 секунд):**

**Что это:**
- Бот не требует webhook и публичного домена
- Бот сам опрашивает Telegram API каждые 60 секунд
- Идеально для разработки и небольших проектов

**Конфигурация:**
```env
POLLING_TIMEOUT=60  # секунд
```

---

### 8. **База данных PostgreSQL:**

#### Активные таблицы:
1. **`chats`** — метаданные чатов (chat_id, type, title, username)
2. **`chat_modules`** — активные модули для каждого чата + JSONB config
3. **`event_log`** — audit trail всех действий (команды, события модулей)
4. **`messages`** — партиционированное хранилище сообщений (для статистики)
5. **`user_stats`** — агрегированная статистика по пользователям

**Миграции:**
```bash
migrations/001_initial_schema.sql  # ✅ Применена
migrations/002_limiter_tables.sql  # ⏳ Будет в Phase 2
```

---

### 9. **Docker готовность:**

#### Dockerfile
- Multi-stage build (golang:1.25-alpine → alpine:latest)
- Статический бинарник (размер ~10M)
- Non-root user (bmft:bmft, uid 1000)
- Healthcheck на `:9090/healthz` (для будущего metrics server)

#### docker-compose.yaml
- **postgres** — PostgreSQL 16-alpine с persistent volume
- **bot** — наш бот с автозапуском и health checks
- Environment variables из `.env` файла
- Logging rotation (10MB, 3 файла)

**Запуск:**
```bash
docker-compose up -d
docker-compose logs -f bot  # Смотрим логи
```

---

### 10. **Логирование (zap):**

#### Structured logging с полями:
```go
logger.Info("handling /start command",
    zap.Int64("chat_id", c.Chat().ID),
    zap.Int64("user_id", c.Sender().ID),
)
```

#### Режимы вывода:
- **Production mode** (`LOGGER_PRETTY=false`) — JSON для агрегаторов (ELK, Loki)
- **Development mode** (`LOGGER_PRETTY=true`) — человекочитаемый консольный вывод

#### Уровни логирования:
```env
LOG_LEVEL=debug   # debug, info, warn, error
```

**Пример JSON лога:**
```json
{
  "ts": "2025-10-04T12:00:00.000Z",
  "level": "info",
  "msg": "bot started successfully",
  "bot_username": "bmft_test_bot",
  "bot_id": 123456789
}
```

---

## ✅ Что бот УЖЕ умеет (Phase 1-2 завершены):

### ✅ Core Framework (Phase 1) — 100% Complete
- Модульная plugin-based архитектура
- Module Registry с lifecycle management
- Команды: `/start`, `/help`, `/modules`, `/enable`, `/disable`
- PostgreSQL интеграция с migrations
- Repository Layer (ChatRepository, ModuleRepository, EventRepository)
- Graceful shutdown с таймаутом
- Structured logging (zap)
- Middleware: Logger, PanicRecovery, RateLimit
- Docker готовность (Dockerfile + docker-compose)

### ✅ Limiter Module (Phase 2) — 100% Complete
- Лимиты на запросы к боту (daily/monthly per user)
- Команды: `/limits`, `/setlimit <user_id> daily|monthly <limit>`, `/getlimit <user_id>`
- Автосброс счётчиков (daily в 00:00 UTC, monthly каждый месяц)
- Unit-тесты (10 тестов, 485 строк)
- Интеграция с main.go

**⚠️ Важно:** Текущая Phase 2 реализует user request limiter. Content type limiter (photo/video/sticker из Python бота) будет добавлен позже как отдельный модуль.

---

## 🚫 Что бот НЕ умеет (будет в Phase 3-5, Phase AI):

### ❌ Reactions Module (Phase 3 — Следующая)
- Автоматические реакции на ключевые слова (regex)
- Миграция паттернов из Python бота (rts_bot)
- Команды: `/addreaction`, `/listreactions`, `/delreaction`, `/testreaction`
- Cooldown система (10 минут между реакциями)
- Антифлуд через reactions_log
- Подсчёт текстовых нарушений (violation_code=21)

### ❌ Statistics Module (Phase 4)
- Статистика сообщений и активности
- Команды: `/mystats` (личная), `/chatstats` (админ)
- Агрегация из messages → statistics_daily
- Top users, most active hours
- Форматированный вывод по типам контента

### ❌ Scheduler Module (Phase 5)
- Задачи по расписанию (cron-like планировщик)
- Миграция задач из Python scheduletask.py
- Scheduled stickers, announcements, reminders
- Команды: `/addtask`, `/listtasks`, `/deltask`, `/runtask`

### ❌ AI Module (Phase AI — В будущем)
- OpenAI/Anthropic API интеграция
- Context Management (история диалогов)
- Команды: `/gpt`, `/reset`, `/context`
- Система промптов и модерация контента
- Интеграция с Limiter Module для проверки лимитов перед AI запросами

### ❌ AntiSpam Module (Опционально)
- Flood protection
- Link filtering
- User reputation system

---

## 🎯 Как протестировать бот прямо сейчас:

### 1. Подготовка:
```bash
# Убедитесь что PostgreSQL запущен
docker run -d --name postgres \
  -e POSTGRES_PASSWORD=secret \
  -p 5432:5432 \
  postgres:16

# Примените миграцию
migrate -path migrations \
  -database "postgres://postgres:secret@localhost/postgres?sslmode=disable" up

# Скопируйте .env.example в .env и укажите TELEGRAM_BOT_TOKEN
cp .env.example .env
nano .env  # Укажите токен от @BotFather
```

### 2. Запуск:
```bash
go run cmd/bot/main.go
```

**Ожидаемый вывод:**
```
2025-10-04T12:00:00.000Z  INFO  starting bmft bot  {"log_level": "debug", "polling_timeout": 60}
2025-10-04T12:00:01.000Z  INFO  connected to postgresql
2025-10-04T12:00:02.000Z  INFO  bot created successfully  {"bot_username": "your_bot", "bot_id": 123456}
2025-10-04T12:00:02.100Z  INFO  bot started, polling for updates...
```

### 3. Тестирование команд:

#### В личке с ботом:
```
/start     → Welcome message
/help      → Список команд
/modules   → "Модули пока не зарегистрированы"
```

#### В группе (добавьте бота и дайте права админа):
```
/start     → Welcome message
/help      → Список команд
/modules   → Проверка прав админа + список модулей
/enable limiter   → "Модуль 'limiter' не найден" (модуль пока не зарегистрирован)
```

### 4. Проверка БД:
```sql
-- Посмотреть созданные чаты
SELECT * FROM chats;

-- Посмотреть события
SELECT * FROM event_log ORDER BY created_at DESC LIMIT 10;

-- Посмотреть модули (пока пусто)
SELECT * FROM chat_modules;
```

### 5. Graceful Shutdown:
```bash
# Нажмите Ctrl+C в терминале с ботом
^C

# Увидите логи остановки:
INFO  received shutdown signal  {"signal": "interrupt"}
INFO  shutting down bot...
INFO  shutting down modules...
INFO  closing database connection...
INFO  bot shutdown complete
```

---

## 📊 Итоговая таблица возможностей:

| Функция | Статус | Описание |
|---------|--------|----------|
| `/start` | ✅ Работает | Приветствие + создание чата в БД |
| `/help` | ✅ Работает | Справка по командам |
| `/modules` | ✅ Работает | Список модулей (пока пустой) |
| `/enable <module>` | ✅ Работает | Включение модуля (проверка прав) |
| `/disable <module>` | ✅ Работает | Выключение модуля |
| Обработка сообщений | ✅ Работает | Передача в ModuleRegistry |
| Admin права проверка | ✅ Работает | Через `bot.AdminsOf()` |
| Audit logging | ✅ Работает | Все действия в `event_log` |
| Middleware | ✅ Работает | Logger, Panic Recovery, Rate Limit |
| Graceful Shutdown | ✅ Работает | SIGINT/SIGTERM handling |
| Long Polling | ✅ Работает | 60 секунд таймаут |
| PostgreSQL | ✅ Работает | 5 таблиц + Repository layer |
| Docker | ✅ Работает | Dockerfile + docker-compose |
| Structured Logging | ✅ Работает | zap (JSON/Console) |
| **Модули** | ❌ Пусто | Будут в Phase 2-6 |

---

## 🎉 Вывод:

**Бот полностью работоспособен!** 

Он успешно:
- ✅ Запускается и подключается к Telegram
- ✅ Обрабатывает 5 базовых команд
- ✅ Работает с PostgreSQL через Repository layer
- ✅ Логирует все действия в audit trail
- ✅ Проверяет права администратора
- ✅ Готов к подключению модулей (архитектура готова)
- ✅ Корректно завершается при остановке

**Но:** Функциональных модулей (limiter, reactions и т.д.) пока нет.  
**Это будет добавлено в Phase 2-6.**

**Phase 1 = базовый каркас готов ✅**  
**Phase 2 = добавим первый реальный модуль (Limiter) 🚀**
