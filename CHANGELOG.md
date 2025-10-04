# Changelog

Все значимые изменения в проекте BMFT будут документироваться в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и проект следует [Semantic Versioning](https://semver.org/lang/ru/).

## [Unreleased]

### Changed (Breaking Changes)
- **Полная переработка архитектуры:** удален Kafka, реализована plugin-based модульная система
- **Изменение библиотеки:** tgbotapi заменен на telebot.v3
- **Изменение схемы БД:** unified PostgreSQL schema вместо per-chat tables
- **Deployment:** переход на Long Polling вместо webhook

### Added
- ✅ Модульная plugin-based архитектура с Module Registry
- ✅ Unified PostgreSQL schema с партиционированием messages по месяцам
- ✅ Module interface (Init, OnMessage, Commands, Enabled, Shutdown)
- ✅ Per-chat module configuration в таблице `chat_modules`
- ✅ Event audit log для всех действий модулей
- ✅ Comprehensive documentation (README, ARCHITECTURE, MIGRATION_PLAN, ANSWERS, QUICKSTART)
- ✅ SQL migrations (`migrations/001_initial_schema.sql`)
- ✅ `.env.example` с подробными комментариями

### Planned (Phase 1)
- [ ] Core framework: Module Registry implementation
- [ ] telebot.v3 integration с Long Polling
- [ ] Middleware layer (rate limiting, logging, panic recovery)
- [ ] Basic commands: /start, /help, /modules, /enable, /disable
- [ ] Config management для module-specific settings

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
