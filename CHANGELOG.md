# Changelog

Все значимые изменения в проекте BMFT будут документироваться в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и проект следует [Semantic Versioning](https://semver.org/lang/ru/).

## [Unreleased]

### Added
- ✅ **Phase 3.5: Text Violations Counter** — счётчик текстовых нарушений с автоудалением
  - `internal/modules/reactions/text_violations.go` — логика проверки violation_code=21 (268 строк)
  - `internal/modules/reactions/commands_violations.go` — команды управления (259 строк)
  - Методы: `checkTextViolation()`, `getTextViolationLimit()`, `getTextViolationCount()`, `incrementTextViolationCounter()`, `isVIPUser()`
  - **Python паритет:** Полная миграция функционала из `checkmessage.py::regextext()` с violation=21
  - **Команды:** `/mytextviolations`, `/settextlimit`, `/chattextviolations`
  - **Логика:** 0 = без лимита, N = лимит нарушений/день (default: 10)
  - **Features:** Автоудаление при превышении, предупреждения за 2 до лимита, VIP bypass
  - **DB Schema:** Расширена таблица `reactions_log` колонками `violation_code`, `keyword`, `emojis_added`, `created_at`
  - **Integration:** Проверка violation_code==21 в `reactions.go::OnMessage()` перед обработкой реакции
- ✅ **Phase 2.5: Content Type Limiter** — лимиты на типы контента (photo/video/sticker/etc)
  - `internal/modules/limiter/content_limiter.go` — логика проверки лимитов (198 строк)
  - `internal/modules/limiter/commands_content.go` — команды управления (276 строк)
  - Методы в LimitRepository: `GetContentLimit()`, `GetContentCount()`, `IncrementContentCounter()`, `IsVIP()`, `SaveContentLimit()`, `GetAllContentLimits()`
  - **Python паритет:** Полная миграция функционала из `reaction.py::newmessage()`
  - **Команды:** `/setcontentlimit`, `/mycontentusage`, `/listcontentlimits`
  - **Логика:** -1 = запрет, 0 = без лимита, N = лимит сообщений/день
  - **Features:** Автоматическое удаление при превышении, предупреждения за 2 до лимита, VIP bypass
- 🔄 **Auto-Migration System** — автоматическое применение миграций при старте бота
  - `internal/migrations/migrations.go` — Migration Manager (358 строк)
  - Автоматическое определение состояния схемы: empty/complete/partial/unknown
  - Валидация всех 18 таблиц и их колонок при старте
  - Защита от partial migrations: ошибка если схема создана частично
  - Production-ready: в будущем будет использовать версионированные миграции (002, 003, etc.)

### Features
- 🔄 **Automatic Schema Creation:** Если БД пустая — создаёт все таблицы из 001_initial_schema.sql
- 🔄 **Schema Validation:** При старте проверяет наличие всех таблиц и колонок
- 🔄 **Safety Checks:** Останавливает бот если схема создана не полностью (partial state)
- 🔄 **Development Workflow:** Hot development — удаляй БД и перезапускай бота для обновления схемы
- 🔄 **Production Workflow:** Будущие изменения через версионированные миграции (002, 003, etc.)

### Changed
- 📝 **README.md:** Удалены все упоминания ручного `migrate -path migrations` CLI
- 📝 **README.md:** Обновлена секция "База данных PostgreSQL" - все таблицы теперь соответствуют 001_initial_schema.sql
- 📝 **README.md:** Добавлена документация по всем 18 таблицам и 2 VIEW
- 📝 **migrations/README.md:** Добавлены инструкции по автомиграциям

### Fixed
- ✅ **Documentation Drift:** README описывал устаревшую схему БД - теперь полностью соответствует SQL файлам
- ✅ **ExpectedSchema:** Обновлена с 9 таблиц до 18 реальных таблиц из 001_initial_schema.sql
- ✅ **VIEW Names:** Исправлены названия view в документации (v_active_modules, v_daily_chat_stats)

## [0.5.0] - 2025-01-XX (Phase 5: Scheduler Module)

### Added
- ✅ **Scheduler Module** — планировщик задач на основе cron для автоматических действий в чате
  - `internal/postgresql/repositories/scheduler_repository.go` — SchedulerRepository (187 строк, 7 методов)
  - `internal/modules/scheduler/scheduler.go` — SchedulerModule (370 строк)

### Features
- ⏰ **Cron-планировщик:** Интеграция robfig/cron/v3 для выполнения задач по расписанию
- ⏰ **Типы задач:** sticker, text, photo (отправка контента в чат по cron выражению)
- ⏰ **Автозагрузка:** Активные задачи загружаются при старте бота и регистрируются в cron
- ⏰ **Управление задачами:** Создание, удаление, ручной запуск задач (только админы)
- ⏰ **Graceful shutdown:** Корректная остановка cron при завершении работы бота
- ⏰ **Валидация:** Проверка cron выражений при создании задачи
- ⏰ **История:** Запись последнего времени выполнения задачи (last_run)

### Commands
- `/addtask <name> "<cron>" <type> <data>` — (Админ) Добавить задачу планировщика
- `/listtasks` — Список всех задач планировщика в чате
- `/deltask <id>` — (Админ) Удалить задачу по ID
- `/runtask <id>` — (Админ) Запустить задачу вручную (вне расписания)

### Database
- Использует таблицу `scheduler_tasks` из миграции 003
- Колонки: id, chat_id, task_name, cron_expr, task_type, task_data, is_active, last_run
- Индексы: chat_id, is_active для эффективной выборки активных задач

### Technical Details
- **Repository методы:**
  - `CreateTask()` — создать новую задачу
  - `GetTask()` — получить задачу по ID
  - `GetChatTasks()` — все задачи чата
  - `GetActiveTasks()` — только активные задачи
  - `UpdateLastRun()` — обновить время последнего запуска
  - `DeleteTask()` — удалить задачу
  - `SetActive()` — включить/выключить задачу
- **Модуль интегрирован:** Зарегистрирован в Module Registry, команды добавлены
- **Cron управление:** Использует cron.ParseStandard() для валидации, cron.AddFunc() для регистрации

### Migration from Python
- ✅ Полная миграция функционала из Python rts_bot/scheduletask.py
- ✅ Поддержка file_id для стикеров/фото (как в Python версии)
- ✅ Cron выражения вместо simple schedule library

### Documentation
- 📝 README.md обновлён: Phase 5 отмечена как завершённая ✅
- 📝 Команды scheduler добавлены в секцию Available Commands
- 📝 CHANGELOG.md обновлён: версия 0.5.0

## [0.4.0] - 2025-10-04 (Phase 4: Statistics Module)

### Added
- ✅ **Statistics Module** — статистика активности пользователей в чатах
  - `internal/postgresql/repositories/statistics_repository.go` — StatisticsRepository (250+ строк)
  - `internal/modules/statistics/statistics.go` — StatisticsModule (470+ строк)

### Features
- 📊 **Личная статистика:** `/mystats` — сколько сообщений отправил за день/неделю
- 📊 **Статистика чата:** `/chatstats` — общая активность в чате (только админы)
- 📊 **Топ пользователей:** `/topchat` — топ 10 активных пользователей (админы)
- 📊 **Автосбор:** При каждом сообщении инкрементирует счётчик в statistics_daily
- 📊 **Форматирование:** Красивый вывод с эмодзи и группировкой по типам контента

### Commands
- `/mystats` — Посмотреть свою статистику за день и неделю
- `/chatstats` — (Админ) Общая статистика чата за день
- `/topchat` — (Админ) Топ 10 активных пользователей за день

### Database
- Использует таблицу `statistics_daily` для кэшированной агрегации
- Автоматический сбор данных при каждом сообщении
- Оптимизированные запросы с JOIN для получения username

### Technical Details
- **Repository методы:**
  - `RecordMessage()` — записать сообщение в статистику
  - `GetUserDailyStats()` — статистика пользователя за день
  - `GetUserWeeklyStats()` — статистика пользователя за неделю
  - `GetChatDailyStats()` — статистика чата за день
  - `GetTopUsers()` — топ активных пользователей
- **Модуль интегрирован:** Зарегистрирован в Module Registry
- **OnMessage:** Автоматически собирает статистику при каждом сообщении

### Documentation
- 📝 README.md обновлён: Phase 4 завершена ✅
- 📝 CHANGELOG.md обновлён: версия 0.4.0

## [0.3.0] - 2025-10-04 (Phase 2: Limiter Module)

### Added
- ✅ **Limiter Module** — контроль лимитов пользователей на запросы к AI
  - `migrations/003_create_limits_table.sql` — таблица user_limits с индексами
  - `internal/postgresql/repositories/limit_repository.go` — LimitRepository (362 строки, 8 методов)
  - `internal/modules/limiter/limiter.go` — LimiterModule (273 строки)
  - Unit-тесты: `limit_repository_test.go` (486 строк, 10 тестов)

### Features
- 🎯 **Дневные лимиты:** По умолчанию 10 запросов в день, автоматический сброс через 24 часа
- 🎯 **Месячные лимиты:** По умолчанию 300 запросов в месяц, автоматический сброс через 30 дней
- 🎯 **Проверка и инкремент:** Атомарная операция CheckAndIncrement() с блокировкой при превышении
- 🎯 **Уведомления:** Автоматические уведомления при превышении лимита и предупреждения при 20% остатке

### Commands
- `/limits` — Посмотреть свои текущие лимиты (дневной и месячный)
- `/setlimit <user_id> daily <limit>` — (Админ) Установить дневной лимит пользователю
- `/setlimit <user_id> monthly <limit>` — (Админ) Установить месячный лимит пользователю
- `/getlimit <user_id>` — (Админ) Посмотреть лимиты конкретного пользователя

### Database
```sql
-- Новая таблица user_limits
- user_id (PK), username
- daily_limit, monthly_limit (с дефолтами 10/300)
- daily_used, monthly_used (счётчики)
- last_reset_daily, last_reset_monthly (для автосброса)
- Индексы на last_reset_* для быстрого поиска устаревших записей
```

### Technical Details
- **Repository методы:**
  - `GetOrCreate()` — получить или создать запись лимита
  - `CheckAndIncrement()` — проверить лимит и увеличить счётчик (атомарно)
  - `GetLimitInfo()` — получить информацию о лимитах
  - `SetDailyLimit()`, `SetMonthlyLimit()` — админские функции
  - `ResetDailyIfNeeded()`, `ResetMonthlyIfNeeded()` — автоматический сброс
- **Модуль интегрирован:** Зарегистрирован в Module Registry, команды добавлены в бота
- **Покрытие тестами:** 10 unit-тестов для всех методов репозитория

### Documentation
- 📝 README.md обновлён: добавлены команды Limiter модуля
- 📝 CHANGELOG.md обновлён: версия 0.3.0

## [0.2.1] - 2025-01-04 (Phase 1 Implementation - 100% Complete)

### Changed (Breaking Changes)
- **Полная переработка архитектуры:** удален Kafka, реализована plugin-based модульная система
- **Изменение библиотеки:** tgbotapi v5 заменен на telebot.v3 v3.3.8
- **Изменение entry point:** cmd/telegram_bot → cmd/bot
- **Deployment:** переход на Long Polling вместо webhook (60s timeout)
- **Config:** удалены все Kafka-related переменные (KAFKA_BROKERS, KAFKA_GROUP_*, DLQ_TOPIC, etc.)
- **Binary size:** ~10M (включает все зависимости)

### Removed
- ❌ **Kafka infrastructure:** internal/kafkabot/, internal/logger/
- ❌ **Old bot:** internal/telegram_bot/, cmd/telegram_bot/
- ❌ **Docker:** docker-compose.env.yaml, docker-compose.bot.yaml, Dockerfile.telegram_bot
- ❌ **Dependencies:** segmentio/kafka-go v0.4.48 (библиотека полностью удалена)

### Added (Phase 1 Complete - Steps 1-7)
- ✅ **Core framework** (728 lines):
  - `internal/core/interface.go` — Module interface (5 methods) + ModuleDependencies (DI)
  - `internal/core/registry.go` — ModuleRegistry с lifecycle management
  - `internal/core/middleware.go` — LoggerMiddleware, PanicRecoveryMiddleware, RateLimitMiddleware
- ✅ **Bot implementation** (462 lines):
  - `cmd/bot/main.go` — telebot.v3 с Long Polling, graceful shutdown
  - Commands: `/start`, `/help`, `/modules`, `/enable <module>`, `/disable <module>`
  - Admin permission checks через `bot.AdminsOf(chat)`
  - Event logging для audit trail
- ✅ **Repository layer** (265 lines):
  - `internal/postgresql/repositories/chat_repository.go` — Chat CRUD
  - `internal/postgresql/repositories/module_repository.go` — Module state + JSONB config
  - `internal/postgresql/repositories/event_repository.go` — Event logging
- ✅ **Dependencies:**
  - gopkg.in/telebot.v3 v3.3.8 (Telegram bot framework)
  - github.com/robfig/cron/v3 v3.0.1 (для будущего scheduler module)
- ✅ **Config updates:**
  - Removed: 9 Kafka-related fields
  - Added: `POLLING_TIMEOUT` (default: 60 seconds)
  - Defaults: `SHUTDOWN_TIMEOUT=15s`, `METRICS_ADDR=:9090`
- ✅ **Utility functions:**
  - `internal/logx/logx.go`: NewLogger() — инициализация zap logger
  - `internal/postgresql/postgresql.go`: PingWithRetry() — проверка подключения к БД
- ✅ **Testing:**
  - `internal/config/config_test.go` — 5 unit tests (все проходят ✅)
  - Tests: Load(), validate(), defaults, error handling, polling timeout parsing
- ✅ **Documentation:**
  - `PHASE1_CHECKLIST.md` — детальный чеклист (811 lines, 75% выполнено)
  - All previous docs remain accurate (README, ARCHITECTURE, MIGRATION_PLAN)

### Fixed
- 🔧 Duplicate package declarations в generated files (автоматически исправлено)
- 🔧 Config default values (ShutdownTimeout 15s, MetricsAddr :9090)

### In Progress (Phase 1 - Steps 8-10 Remaining)
- [ ] **Step 8:** Documentation updates (README quick start, CHANGELOG)
- [ ] **Step 9:** Docker setup (Dockerfile multi-stage, docker-compose.yaml)
- [ ] **Step 10:** Final verification (go vet, go fmt, functional testing)

### Completed
- [x] **Phase 1:** Core Framework (100% ✅)
- [x] **Phase 2:** Limiter module (user request limits, daily/monthly counters) (100% ✅)

### Planned (Phase 3-5, Phase AI)
- [ ] **Phase 3:** Reactions module (regex patterns, cooldowns, Python migration) ← СЛЕДУЮЩАЯ
- [ ] **Phase 4:** Statistics module (daily/weekly stats, /mystats, /chatstats)
- [ ] **Phase 5:** Scheduler module (cron-like tasks, scheduled stickers)
- [ ] **Phase AI:** AI Module (OpenAI/Anthropic, context management, /gpt) ← В БУДУЩЕМ
- [ ] **Phase AntiSpam:** AntiSpam module (flood protection, link filtering) ← ОПЦИОНАЛЬНО

### Removed
- ❌ Apache Kafka и Zookeeper (overkill для RPS ~0.004)
- ❌ segmentio/kafka-go dependency
- ❌ tgbotapi v5 (заменен на telebot.v3)
- ❌ Per-chat table pattern в SQLite (заменено на unified schema)

---

## [0.2.0] - 2025-10-04 - Documentation Phase

### Added
- Comprehensive architecture documentation (2481 lines total)
- Database migration script with optimized schema
- 8-phase migration plan from Python version
- Q&A document with architectural decisions
- Quick start guide for new developers

### Changed
- Updated README with modular architecture focus
- Replaced Kafka-centric description with plugin-based approach
- Added examples for module development

---

## [0.1.0] - 2025-08-25 - Initial Kafka-based Version

### Added
- Initial Kafka-based architecture
- PostgreSQL integration
- Telegram Bot API client with tgbotapi v5
- Basic message processing pipeline
- Docker Compose setup

### Features
- Message ingestion via Telegram Bot API
- Kafka-based message bus
- PostgreSQL persistence
- Graceful shutdown
- Structured logging with zap

---

## Versioning Strategy

Starting from v0.2.0, we follow Semantic Versioning:

- **MAJOR** version: incompatible API changes
- **MINOR** version: new features in backward-compatible manner
- **PATCH** version: backward-compatible bug fixes

### Pre-1.0 versions:
- `0.x.x` - Development versions with possible breaking changes
- `1.0.0` - First stable release (after Phase 7 completion)

---

## Migration Notes

### From v0.1.0 to v0.2.0

**Breaking changes:**
1. Kafka removed — new architecture does NOT use Kafka
2. tgbotapi replaced with telebot.v3
3. Database schema completely redesigned

**Migration path:**
- See `MIGRATION_PLAN.md` for detailed 8-phase migration guide
- Use `scripts/migrate_config.py` to import limits and reactions from SQLite
- Old messages are NOT migrated (drop policy)

**Environment variables changed:**
- Removed: `KAFKA_BROKERS`, `KAFKA_GROUP_*`, `DLQ_TOPIC`, `LOG_TOPICS`
- Added: `POLLING_TIMEOUT`
- Kept: `TELEGRAM_BOT_TOKEN`, `POSTGRES_DSN`, `LOG_LEVEL`, `LOGGER_PRETTY`

---

## Links

- [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/)
- [Semantic Versioning](https://semver.org/lang/ru/)
- [GitHub Repository](https://github.com/your-repo/bmft)
