# Changelog

Все заметные изменения в проекте будут документированы в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и этот проект придерживается [Semantic Versioning](https://semver.org/lang/ru/).

---

## [Unreleased]

### Added
- Phase 3: Reactions Module (автоматические реакции на сообщения)
  - Команды: `/addreaction`, `/listreactions`, `/delreaction`, `/testreaction`
  - 3 типа триггеров: regex, exact, contains
  - 3 типа реакций: text, sticker, delete
  - Cooldown система с настраиваемым интервалом (по умолчанию 10 минут)
  - VIP bypass для cooldown (`is_vip` флаг)
  - Логирование в `reactions_log` для антифлуда
  - Таблицы БД: `reactions_config`, `reactions_log`

---

## [0.2.0] - 2025-01-19 (Phase 2)

### Added
- Phase 2: Limiter Module (лимиты запросов пользователей)
  - Команды: `/limits` (пользователь), `/setlimit`, `/getlimit` (админ)
  - Daily/Monthly лимиты per user
  - Таблица БД: `user_limits`
  - Repository: `LimitRepository` с методами CheckAndIncrement, SetDailyLimit, SetMonthlyLimit
  - Graceful degradation при недоступности БД (логирование ошибки, пропуск блокировки)

### Changed
- Миграции: объединены `001_initial_schema.sql` + `003_create_limits_table.sql` в один файл
- Структура миграций: теперь один файл `migrations/001_initial_schema.sql` содержит все фазы (1-5)
- README: добавлены инструкции по миграциям, примеры команд limiter модуля

---

## [0.1.0] - 2025-01-18 (Phase 1)

### Added
- Инициализация проекта BMFT (Bot Moderator Framework for Telegram)
- Core архитектура:
  - Module Registry (`internal/core/registry.go`)
  - Module Interface (`internal/core/interface.go`)
  - Middleware: LoggerMiddleware, PanicRecoveryMiddleware, RateLimitMiddleware
- PostgreSQL интеграция:
  - Repositories: ChatRepository, ModuleRepository, EventRepository
  - PingWithRetry для graceful startup
- Базовые команды: `/start`, `/help`, `/modules`, `/enable`, `/disable`
- Конфигурация через `.env` (Viper)
- Structured logging (zap)
- Docker Compose для PostgreSQL
- Graceful shutdown (SIGINT/SIGTERM handling)
- Миграция БД: `migrations/001_initial_schema.sql`
  - Таблицы: `chats`, `users`, `chat_admins`, `chat_modules`, `event_log`

---

## Legend

- **Added:** новые фичи
- **Changed:** изменения в существующем функционале
- **Deprecated:** скоро будет удалено
- **Removed:** удалено
- **Fixed:** исправление багов
- **Security:** исправления безопасности

---

**Ссылки:**
- [Unreleased]: https://github.com/flybasist/bmft/compare/v0.2.0...HEAD
- [0.2.0]: https://github.com/flybasist/bmft/compare/v0.1.0...v0.2.0
- [0.1.0]: https://github.com/flybasist/bmft/releases/tag/v0.1.0
