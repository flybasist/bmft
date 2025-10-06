# Changelog

Все значимые изменения в проекте BMFT будут документироваться в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и проект следует [Semantic Versioning](https://semver.org/lang/ru/).

---

## [Unreleased]

### В разработке
- Рефакторинг модуля Limiter (лимиты на типы контента, VIP система)
- Рефакторинг модуля Reactions (regex реакции, текстовые нарушения)
- Рефакторинг модуля Statistics (детальная статистика по типам)
- Рефакторинг модуля Scheduler (гибкий планировщик на БД)

### Планируется
- **Webhook Mode:** Переход с Long Polling на Webhook для production
- **Redis:** Кеширование часто запрашиваемых данных
- **Grafana:** Визуализация метрик и статистики
- **CI/CD:** Автоматизированные тесты и деплой

---

## [0.6.0] - 2025-10-06

### Changed
- 🚀 **Bot Init:** `internal/bot/init.go` — централизованная инициализация Telegram бота
- 📝 **Logging:** Переход на структурированные логи с уровнями (ERROR, WARN, INFO, DEBUG)

### Added
- **Limiter:**
  - Лимиты на контент (текст, фото, стикеры, голосовые)
  - Прогресс-бары для пользователей (`/mycontentusage`)
  - Команды: `/setcontentlimit`, `/mycontentusage`, `/listcontentlimits`

- **Reactions:**
  - Приветствия новых участников (welcome/goodbye messages)
  - Случайные стикеры/фото на входящие сообщения
  - Версионная команда `/version`

- **Statistics:**
  - Команды: `/activestats`, `/totalmessages`

### Fixed
- ✅ **Graceful Shutdown:** Корректное завершение с context.Context
- ✅ **Telegram Polling:** Убран legacy `bot.Start()`

---

## [0.5.0] - 2025-10-05

### Added
- 🧩 **Module Registry:** Plugin-based архитектура
  - Централизованный реестр модулей в `internal/modules/registry.go`
  - Инициализация через `RegisterAllModules()`
  - Поддержка модулей: `limiter`, `reactions`, `statistics`, `scheduler`, `chatexport`

- **Violations (Regex Reactions):**
  - Обработка regex-паттернов с violation=21
  - Автоматическая реакция на запрещённые слова/фразы
  - Интеграция с базой данных через `db.GetAllActiveRegexPatterns()`

- **Edit Handler:**
  - Отредактированные сообщения проходят через ту же логику обработки
  - Проверка лимитов, regex-паттернов, статистика

### Changed
- 📁 Рефакторинг структуры проекта:
  - `internal/bot/` — инициализация Telegram-бота
  - `internal/handlers/` — обработчики событий
  - `internal/modules/` — модули функциональности

---

## [0.4.1] - 2025-10-04

### Fixed
- ✅ SQL синтаксис ошибка в `limiter_config` UNIQUE constraint
- ✅ Отсутствующий volume mount для `migrations/`

### Known Issues
- ⚠️ Отсутствует VIP система (обход лимитов для администраторов)

---

## [0.4.0] - 2025-10-04

### Added
- ✅ **Полный функционал реализован** — все основные модули работают

- **Scheduler (Планировщик задач):**
  - Поддержка `file_id` для стикеров/фото (без необходимости хранить файлы)
  - Команды: `/schedule`, `/listtasks`, `/deletetask`
  - Cron-формат для периодических задач

- **ChatExport (Экспорт данных):**
  - Экспорт статистики чата в CSV
  - Команда: `/exportchat`

- **Limiter (Лимиты на контент):**
  - Лимиты на типы контента (текст, стикеры, фото, голосовые)
  - Команды: `/setcontentlimit`, `/mycontentusage`, `/listcontentlimits`

- **Statistics (Статистика):**
  - Команды: `/activestats`, `/totalmessages`

### Changed
- 🚀 **Docker Compose:** Обновлён `docker-compose.yml` с автомиграциями
- 📝 **Migrations:** Автоматическое применение SQL-миграций при старте
- 🔧 **Config:** Поддержка `config/config.json` с database credentials

### Technical Implementation
- Полная интеграция с PostgreSQL 16
- Автоматические миграции через volume mount
- Graceful shutdown с `context.Context`

---

## [0.3.1] - 2025-10-03

### Fixed
- ✅ Docker volume mount для миграций
- ✅ PostgreSQL подключение через `host.docker.internal`

---

## [0.3.0] - 2025-10-03

### Added
- 📁 **migrations/:** SQL-миграции для автоматического создания схемы БД
- 📝 **migrations/README.md:** Инструкции по автомиграциям
- 🐳 **docker-compose.yml:** Контейнеризация с PostgreSQL 16

- **Scheduler Module:**
  - `/schedule <time> <message>` — Отложенная отправка сообщений
  - `/schedule <cron> <message>` — Периодические задачи (cron-формат)
  - `/listtasks` — Список всех задач планировщика в чате

---

## [0.2.0] - 2025-10-02

### Added
- 🗄️ **PostgreSQL Integration:**
  - База данных для хранения статистики, лимитов, настроек
  - Миграции через `migrations/001_initial_schema.sql`

- **Commands:**
  - `/version` — Версия бота и информация о системе
  - `/activestats` — Активность пользователей за последние 7 дней
  - `/totalmessages` — Общее количество сообщений в чате

### Changed
- Переход от In-Memory к PostgreSQL для статистики

---

## [0.1.0] - 2025-10-01

### Added
- 🎉 **Первый релиз!**
- ✅ Базовая структура проекта на Go
- ✅ Подключение к Telegram Bot API через `telebot.v3`
- ✅ Обработка входящих сообщений
- ✅ Базовая конфигурация через JSON

---

## Versioning Strategy

Проект следует [Semantic Versioning](https://semver.org/lang/ru/):

- **MAJOR (X.0.0):** Несовместимые изменения API, критические рефакторинги
- **MINOR (0.X.0):** Новые функции с сохранением обратной совместимости
- **PATCH (0.0.X):** Исправления багов, мелкие улучшения

**Текущий статус:** Alpha (v0.x.x) — активная разработка, API может меняться.
