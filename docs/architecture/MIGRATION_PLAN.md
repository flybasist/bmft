# План миграции: Python → Go (модульная архитектура)

## Фаза 0: Подготовка инфраструктуры ✅

- [x] Аудит Python проекта (rts_bot)
- [x] Анализ боевой БД (rtsbot.db)
- [x] Дизайн оптимизированной SQL схемы
- [x] Проектирование модульной архитектуры

**Результаты:**
- 19 чатов, ~26k сообщений
- RPS: ~0.004 (очень низкий)
- **Решение: Kafka не нужна**
- **Решение: Переезд на telebot.v3**
- Создана схема БД: `migrations/001_initial_schema.sql`
- Спроектирована plugin-based архитектура

---

## Фаза 1: Базовый каркас (Core Framework)

**Цель:** Создать модульный каркас без Kafka

### Задачи:

#### 1.1 Удаление Kafka-инфраструктуры
- [ ] Удалить `internal/kafkabot/`
- [ ] Удалить Kafka-related код из `internal/core/`
- [ ] Удалить Kafka-related код из `internal/telegram_bot/`
- [ ] Удалить Kafka из `docker-compose.env.yaml`
- [ ] Обновить `go.mod` — удалить `github.com/segmentio/kafka-go`

#### 1.2 Переезд на telebot.v3
- [ ] Добавить `gopkg.in/telebot.v3` в зависимости
- [ ] Удалить `github.com/go-telegram-bot-api/telegram-bot-api/v5`
- [ ] Создать базовый `cmd/bot/main.go` с telebot.v3
- [ ] Настроить Long Polling (без webhook)

#### 1.3 Core Framework
- [ ] `internal/core/bot.go` — основной Bot wrapper
- [ ] `internal/core/context.go` — MessageContext для модулей
- [ ] `internal/core/registry.go` — ModuleRegistry (управление модулями)
- [ ] `internal/core/interface.go` — интерфейс Module
- [ ] `internal/core/router.go` — роутинг сообщений по модулям

#### 1.4 Database Layer
- [ ] `internal/database/postgres.go` — подключение к PostgreSQL
- [ ] `internal/database/migrations.go` — автоматический запуск миграций
- [ ] Интеграция `github.com/golang-migrate/migrate/v4`
- [ ] Применить `migrations/001_initial_schema.sql`

#### 1.5 Repository Layer (общие)
- [ ] `internal/repository/chats.go` — ChatsRepository
- [ ] `internal/repository/users.go` — UsersRepository
- [ ] `internal/repository/modules.go` — ModulesRepository (управление включением модулей)
- [ ] `internal/repository/event_log.go` — EventLogRepository

#### 1.6 Middleware
- [ ] `internal/middleware/logging.go` — структурированные логи (zap)
- [ ] `internal/middleware/recovery.go` — panic recovery
- [ ] `internal/middleware/admin_check.go` — проверка прав администратора

#### 1.7 Config
- [ ] Обновить `internal/config/config.go` — убрать Kafka, добавить модули
- [ ] Переменные окружения:
  ```
  TELEGRAM_BOT_TOKEN
  POSTGRES_DSN
  LOG_LEVEL
  ADMIN_USER_IDS (твои user_id для супер-админа)
  DEFAULT_MODULES (список модулей по умолчанию)
  ```

#### 1.8 Базовые команды
- [ ] `/start` — приветствие
- [ ] `/help` — показать команды активных модулей
- [ ] `/modules` — управление модулями (только админ чата)
- [ ] `/version` — информация о боте

**Трудозатраты:** 2-3 дня

**Результат:** Работающий скелет бота без функционала, но с модульной архитектурой

---

## Фаза 2: Модуль Limiter (лимиты на контент)

**Цель:** Перенести функционал лимитов из Python

### Задачи:

#### 2.1 Структура модуля
- [ ] `internal/modules/limiter/module.go` — реализация интерфейса Module
- [ ] `internal/modules/limiter/service.go` — бизнес-логика лимитов
- [ ] `internal/modules/limiter/repository.go` — работа с БД (limiter_config, limiter_counters)
- [ ] `internal/modules/limiter/commands.go` — команды `/setlimit`, `/showlimits`, `/mystats`

#### 2.2 Миграция данных
- [ ] Скрипт миграции: `scripts/migrate_limits.py` или `scripts/migrate_limits.go`
- [ ] Миграция из `{chat_id}_limits` → `limiter_config`
- [ ] Конвертация формата:
  ```python
  # Python: одна строка = все лимиты для user_id
  {user_id: "allmembers", photo: 10, video: 5, ...}
  
  # Go: одна строка = один лимит
  {chat_id, user_id: NULL, content_type: "photo", daily_limit: 10}
  {chat_id, user_id: NULL, content_type: "video", daily_limit: 5}
  ```

#### 2.3 Функционал
- [ ] Проверка лимитов при входящем сообщении
- [ ] Подсчёт за последние 24 часа (используя `limiter_counters`)
- [ ] Удаление сообщения при превышении лимита
- [ ] Предупреждение когда осталось 2 сообщения до лимита
- [ ] Поддержка VIP (игнорирование лимитов)
- [ ] Команда `/setlimit <тип> <лимит>` (только админ)
  - `-1` = запрет
  - `0` = без лимита
  - `N` = суточный лимит
- [ ] Команда `/showlimits` — показать текущие лимиты чата
- [ ] Команда `/mystats` — личная статистика за сутки

#### 2.4 Тестирование
- [ ] Unit-тесты для LimiterService
- [ ] Интеграционные тесты с тестовой БД

**Трудозатраты:** 2-3 дня

**Результат:** Работающие лимиты на контент, полностью аналог Python версии

---

## Фаза 3: Модуль Reactions (реакции на ключевые слова)

**Цель:** Перенести функционал реакций из Python

### Задачи:

#### 3.1 Структура модуля
- [ ] `internal/modules/reactions/module.go`
- [ ] `internal/modules/reactions/matcher.go` — regex matching
- [ ] `internal/modules/reactions/repository.go` — работа с БД (reactions_config, reactions_log)
- [ ] `internal/modules/reactions/commands.go` — команды управления реакциями

#### 3.2 Миграция данных
- [ ] Скрипт миграции: `scripts/migrate_reactions.py` или `scripts/migrate_reactions.go`
- [ ] Миграция из `{chat_id}_reaction` → `reactions_config`
- [ ] Конвертация формата:
  ```python
  # Python
  {contenttype: "text", answertype: "sticker", regex: "\\bамига\\b", 
   answer: "CAACAgIAAxk...", violation: 11}
  
  # Go
  {chat_id, content_type: "text", trigger_type: "regex", 
   trigger_pattern: "\\bамига\\b", reaction_type: "sticker",
   reaction_data: "CAACAgIAAxk...", violation_code: 11}
  ```

#### 3.3 Функционал
- [ ] Проверка regex паттернов в text/caption
- [ ] Отправка стикера или текста в ответ
- [ ] Антифлуд: cooldown 10 минут между реакциями (используя `reactions_log`)
- [ ] Подсчёт нарушений (violation_code=21 для текстовых нарушений)
- [ ] Удаление при превышении лимита текстовых нарушений
- [ ] Команды управления реакциями (только админ):
  - `/addreaction <contenttype> <regex> <reaction_type> <data>`
  - `/listreactions` — показать все реакции
  - `/delreaction <id>` — удалить реакцию
  - `/testreaction <text>` — проверить какие реакции сработают

#### 3.4 Тестирование
- [ ] Unit-тесты для regex matching
- [ ] Тесты антифлуда

**Трудозатраты:** 2-3 дня

**Результат:** Работающие реакции на ключевые слова

---

## Фаза 4: Модуль Statistics (статистика)

**Цель:** Реализовать команды статистики

### Задачи:

#### 4.1 Структура модуля
- [ ] `internal/modules/statistics/module.go`
- [ ] `internal/modules/statistics/collector.go` — сбор статистики
- [ ] `internal/modules/statistics/repository.go` — работа с БД (statistics_daily)
- [ ] `internal/modules/statistics/commands.go`

#### 4.2 Функционал
- [ ] Агрегация данных из `messages` → `statistics_daily` (периодически или по запросу)
- [ ] Команда `/mystats` — личная статистика за сутки
- [ ] Команда `/chatstats` — статистика чата (только админ)
- [ ] Форматированный вывод:
  ```
  @username статистика за сутки
  5 фото из 10
  3 видео из 5
  12 стикеров из 20
  2 текстовых нарушения из 5
  ```

**Трудозатраты:** 1-2 дня

**Результат:** Команды статистики работают

---

## Фаза 5: Модуль Scheduler (задачи по расписанию)

**Цель:** Перенести функционал scheduletask.py

### Задачи:

#### 5.1 Структура модуля
- [ ] `internal/modules/scheduler/module.go`
- [ ] `internal/modules/scheduler/cron.go` — cron scheduler (robfig/cron)
- [ ] `internal/modules/scheduler/repository.go` — работа с БД (scheduler_tasks)
- [ ] `internal/modules/scheduler/commands.go`

#### 5.2 Миграция данных
- [ ] Перенести задачи из Python кода в `scheduler_tasks`
- [ ] Пример: отправка стикера каждый день в 9:00

#### 5.3 Функционал
- [ ] Запуск cron-планировщика при старте модуля
- [ ] Выполнение задач по расписанию
- [ ] Команды управления (только админ):
  - `/addtask <name> <cron> <type> <data>` 
  - `/listtasks`
  - `/deltask <id>`
  - `/runtask <id>` — запустить вручную

**Трудозатраты:** 1-2 дня

**Результат:** Планировщик задач работает

---

## Фаза 6: Модуль AntiSpam (опционально, для будущего)

**Цель:** Добавить антиспам функционал

### Задачи:

#### 6.1 Структура модуля
- [ ] `internal/modules/antispam/module.go`
- [ ] `internal/modules/antispam/detector.go` — детекторы спама
- [ ] `internal/modules/antispam/repository.go` — работа с БД (antispam_config)

#### 6.2 Функционал
- [ ] Детектор flood (N сообщений за M секунд)
- [ ] Детектор дубликатов
- [ ] Детектор ссылок/упоминаний
- [ ] Действия: warn, delete, mute, ban
- [ ] Команды настройки (только админ)

**Трудозатраты:** 2-3 дня (низкий приоритет)

---

## Фаза 7: Админ-панель и расширенные команды

**Цель:** Полноценное управление через Telegram команды

### Задачи:

#### 7.1 Управление VIP-статусом
- [ ] Команда `/setvip @username` (только супер-админ)
- [ ] Команда `/removevip @username`
- [ ] Команда `/listvips`

#### 7.2 Управление администраторами
- [ ] Команда `/addadmin @username` (только владелец чата)
- [ ] Команда `/removeadmin @username`
- [ ] Команда `/listadmins`
- [ ] Синхронизация с Telegram API (getChatAdministrators)

#### 7.3 Inline-кнопки для удобства
- [ ] Inline keyboard для `/modules`
- [ ] Inline keyboard для `/setlimit`
- [ ] Inline keyboard для `/addreaction`

#### 7.4 Приветствия и события
- [ ] Приветствие при добавлении бота в чат
- [ ] Приветствие новых участников чата
- [ ] Обработка left_chat_member (прощание)

**Трудозатраты:** 2-3 дня

---

## Фаза 8: Production-ready

**Цель:** Подготовка к продакшену

### Задачи:

#### 8.1 Мониторинг и метрики
- [ ] Prometheus метрики:
  - Количество сообщений по типам
  - Количество срабатываний лимитов
  - Количество срабатываний реакций
  - Latency обработки сообщений
- [ ] Health endpoints (`/healthz`, `/readyz`)
- [ ] Grafana dashboard

#### 8.2 Логирование и трейсинг
- [ ] Структурированные логи (zap) — уже есть
- [ ] Опционально: OpenTelemetry для distributed tracing

#### 8.3 Оптимизация БД
- [ ] Индексы проверены (уже есть в схеме)
- [ ] Партиционирование `messages` работает
- [ ] VACUUM / ANALYZE для поддержания производительности

#### 8.4 Deployment
- [ ] CI/CD pipeline (GitHub Actions)
- [ ] Docker образ оптимизирован (multi-stage build)
- [ ] docker-compose для production
- [ ] Автоматические бэкапы PostgreSQL

#### 8.5 Документация
- [ ] README с полной инструкцией развёртывания
- [ ] Документация команд для администраторов чатов
- [ ] Примеры конфигурации модулей

**Трудозатраты:** 3-4 дня

---

## Итоговые трудозатраты

| Фаза | Описание | Трудозатраты | Приоритет |
|------|----------|--------------|-----------|
| 0 | Подготовка | ✅ Завершено | - |
| 1 | Базовый каркас | 2-3 дня | 🔴 Критично |
| 2 | Модуль Limiter | 2-3 дня | 🔴 Критично |
| 3 | Модуль Reactions | 2-3 дня | 🟠 Высокий |
| 4 | Модуль Statistics | 1-2 дня | 🟠 Высокий |
| 5 | Модуль Scheduler | 1-2 дня | 🟡 Средний |
| 6 | Модуль AntiSpam | 2-3 дня | 🟢 Низкий (будущее) |
| 7 | Админ-панель | 2-3 дня | 🟠 Высокий |
| 8 | Production-ready | 3-4 дня | 🟡 Средний |

**MVP (фазы 1-4):** 7-10 дней
**Full (фазы 1-7):** 12-16 дней
**Production (фазы 1-8):** 15-20 дней

---

## Следующий шаг: Фаза 1 — Базовый каркас

**Готов начинать по твоей команде!**

Начинаем с удаления Kafka и создания core framework?
