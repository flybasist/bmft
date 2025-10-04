# Changelog

Все значимые изменения в проекте BMFT будут документироваться в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и проект следует [Semantic Versioning](https://semver.org/lang/ru/).

## [Unreleased]

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
- 📝 `docs/development/PHASE2_LIMITER_MODULE.md` — детальный план реализации (10 шагов)
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

### Planned (Phase 2-7)
- [ ] **Phase 2:** Limiter module (content type limits, daily counters)
- [ ] **Phase 3:** Reactions module (regex patterns, cooldowns)
- [ ] **Phase 4:** Statistics module (daily/weekly stats)
- [ ] **Phase 5:** Scheduler module (cron-like tasks)
- [ ] **Phase 6:** AntiSpam module (flood protection, link filtering)
- [ ] **Phase 7:** Admin panel (web interface, analytics dashboard)

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
