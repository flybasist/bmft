# Changelog

Все значимые изменения в проекте BMFT будут документироваться в этом файле.

Формат основан на [Keep a Changelog](https://keepachangelog.com/ru/1.0.0/),
и проект следует [Semantic Versioning](https://semver.org/lang/ru/).

---

## [Unreleased]

### Планируется
- **Webhook Mode:** Переход с Long Polling на Webhook для production
- **Redis:** Кеширование часто запрашиваемых данных
- **Grafana:** Визуализация метрик и статистики
- **CI/CD:** Автоматизированные тесты и деплой

---

## [0.6.0-dev] - 2025-10-06

### 🔄 КРИТИЧЕСКИЙ РЕФАКТОРИНГ — Упрощение архитектуры

**Философия:**
- Ничего не хардкодим - всё через БД и команды ТГ
- Упрощённая логика - без магических чисел
- VIP система - обход всех лимитов
- Понятность > сложность

### ✅ Реализовано

- **VIP система:**
  - Таблица `chat_vips` - отдельная таблица для VIP пользователей
  - VIP обходят ВСЕ лимиты и проверки
  - Репозиторий `VIPRepository` (145 строк) с методами: `IsVIP()`, `GrantVIP()`, `RevokeVIP()`, `ListVIPs()`
  - Команды: `/setvip @user [reason]`, `/removevip @user`, `/listvips`

- **Новые лимиты на контент:**
  - Таблица `content_limits` с 12 отдельными полями для каждого типа
  - Значения: `-1` = ЗАПРЕТ, `0` = БЕЗ ЛИМИТА, `N` = максимум N в день
  - Типы: text, photo, video, sticker, animation, voice, video_note, audio, document, location, contact, banned_words
  - Репозиторий `ContentLimitsRepository` (280 строк) с методами: `GetLimits()`, `SetLimit()`, `GetCounter()`, `IncrementCounter()`
  - Предупреждения: "⚠️ Осталось X из Y"

- **Keyword Reactions:**
  - Таблица `keyword_reactions` - автореакции на ключевые слова (regex/текст)
  - Поддержка cooldown (антифлуд) в секундах
  - Описание реакций для админов, кэширование на 5 минут
  - Заменяет старую логику с violation codes (11,12,13)
  - Команды: `/addreaction <pattern> <response> <description>`, `/listreactions`, `/removereaction <id>`

- **Banned Words (Text Filter):**
  - Таблица `banned_words` - фильтр запрещённых слов
  - Действия: `delete`, `warn`, `delete_warn`
  - Инкрементирует счётчик `banned_words` в content_counters
  - Заменяет старую логику violation code 21
  - Команды: `/addban <pattern> <action>`, `/listbans`, `/removeban <id>`

- **Настройки бота из БД:**
  - Таблица `bot_settings` - глобальные настройки (версия, таймзона, модули)
  - Репозиторий `SettingsRepository` (70 строк)
  - Версия бота теперь берётся из БД, а не хардкодится

- **Планировщик на БД:**
  - Таблица `banned_words` - запрещённые слова с действиями
  - Действия: warn, delete, mute, ban
  - Заменяет старую логику с violation=21

- **Scheduler на БД:**
  - Таблица `scheduled_tasks` (переименована с `scheduler_tasks`)
  - Поля: `cron_expression`, `action_type`, `action_data` (JSONB)
  - Полностью на БД - без хардкода в коде
  - Обновлён `SchedulerRepository`

### Changed

- 🗄️ **Database Schema v0.6.0:**
  - Убраны магические числа `violation` (11,12,13,21)
  - Лимиты: вместо `limiter_config` с content_type → `content_limits` с отдельными полями
  - Счётчики: `content_counters` с полями count_text, count_photo, и т.д.
  - Партиции messages: добавлены на 3 месяца вперёд

- � **Модули полностью переписаны:**
  - `internal/modules/limiter/` (329 строк) - интеграция VIP + ContentLimits
  - `internal/modules/reactions/` (289 строк) - keyword_reactions, кэш, cooldown
  - `internal/modules/textfilter/` (283 строки, НОВЫЙ) - banned_words фильтр
  - `internal/modules/statistics/` - без изменений (уже готов)
  - `internal/modules/scheduler/` - обновлён на scheduled_tasks

- �📝 **Version:** `0.5` → `0.6.0-dev` (из БД через SettingsRepository)

### Removed

- ❌ Старая таблица `limiter_config` (заменена на `content_limits`)
- ❌ Старая таблица `scheduler_tasks` (заменена на `scheduled_tasks`)
- ❌ Дублирующая таблица `statistics_daily` (статистика через `content_counters`)
- ❌ Мертвый код `limit_repository.go` + тесты (заменен на `content_limits.go`)
- ❌ Магические числа violation (11=амига, 12=похмелье, 21=мат)
- ❌ Хардкод версии бота в main.go
- ❌ Хардкод задач scheduler в коде
- ❌ Файлы `docs/ANALYSIS.md`, `docs/ROADMAP.md` (устарели)

### Migration Notes

⚠️ **BREAKING CHANGES:** Схема БД полностью переработана

Для обновления:
```bash
# 1. Удалить старую БД (данные не критичны в dev)
docker-compose down -v

# 2. Запустить с новой схемой
docker-compose up -d

# 3. Проверить миграцию
docker-compose logs bot | grep "initial migration completed successfully"
```

**Результаты финальной миграции (06.01.2025):**
- ✅ Все 14 таблиц созданы успешно (убрали дублирующую statistics_daily)
- ✅ Все 5 модулей зарегистрированы (limiter, reactions, textfilter, statistics, scheduler)
- ✅ VIP система работает
- ✅ Content Limits работают (12 типов контента)
- ✅ Keyword Reactions работают (regex + cooldown)
- ✅ Banned Words Filter работает
- ✅ Statistics работает через content_counters (правильная архитектура)
- ✅ Schema validation пройдена
- ✅ Удален мертвый код (старый LimitRepository)
- ✅ Все комментарии на русском, все логи на английском
- ✅ Бот запущен без ошибок

### Refactoring (06.01.2025)

**Проблема:** Обнаружены ошибки при первом тестировании
1. `reactions` модуль падал с ошибкой `column "cooldown_seconds" does not exist`
2. `statistics` модуль падал с ошибкой `relation "statistics_daily" does not exist`
3. Дублирование данных между `statistics_daily` и `content_counters`
4. Мертвый код `limit_repository.go` (старая версия, не используется)

**Решение:**
1. ✅ Исправлен `reactions.go` - использует правильное название колонки `cooldown`
2. ✅ Удалена таблица `statistics_daily` из схемы БД
3. ✅ Переписан `statistics_repository.go` - теперь использует `content_counters` напрямую
4. ✅ Удален мертвый код `limit_repository.go` + `limit_repository_test.go` (588 строк)
5. ✅ Обновлен `ExpectedSchema` в `migrations.go` - убрано `statistics_daily`
6. ✅ Все комментарии на русском, все логи на английском (проверено)
7. ✅ Актуализирована документация (README, CHANGELOG, docs/)

**Архитектура v0.6.0 (правильная):**
- `messages` - хранит все сообщения (партиционирование по месяцам)
- `content_counters` - дневные счетчики по типам контента (12 полей)
- `statistics_repository` - агрегирует данные из `content_counters` на лету

### Technical Details

**Новые файлы:**
- `internal/postgresql/repositories/vip.go` (145 строк)
- `internal/postgresql/repositories/content_limits.go` (280 строк)
- `internal/postgresql/repositories/settings.go` (70 строк)
- `internal/modules/textfilter/textfilter.go` (283 строки)
- `docs/QUICK_START.md` (350+ строк) - руководство для пользователей
- `docs/TESTING_SHORT.md` (150+ строк) - краткий чеклист тестирования
- `docs/TERMINAL_CHEATSHEET.md` (200+ строк) - команды терминала

**Обновлённые файлы:**
- `internal/modules/limiter/limiter.go` - полностью переписан
- `internal/modules/reactions/reactions.go` - полностью переписан, исправлен баг
- `internal/modules/statistics/statistics.go` - обновлены комментарии
- `internal/postgresql/repositories/statistics_repository.go` - полностью переписан на `content_counters`
- `internal/postgresql/repositories/scheduler_repository.go` - обновлены запросы
- `internal/migrations/migrations.go` - убрано `statistics_daily` из ExpectedSchema
- `migrations/001_initial_schema.sql` (179 строк) - убрана `statistics_daily`
- `cmd/bot/main.go` - интеграция новых репозиториев

**Статистика кода:**
- Написано: ~2200 строк Go кода + документация
- Удалено: ~1400 строк устаревшего/мертвого кода
- Схема БД: 14 таблиц (было 15), 13 индексов, без дублирования данных

---

## [0.6.0] - 2025-10-06 (old)

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
