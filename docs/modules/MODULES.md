# Модули BMFT

BMFT состоит из 5 модулей, каждый отвечает за свою область функциональности.

## Pipeline обработки сообщений

Каждое входящее сообщение проходит через 3 модуля в фиксированном порядке:

```
statistics → limiter → reactions
```

1. **Statistics** — записывает сообщение в БД (всегда первый)
2. **Limiter** — проверяет лимиты, может удалить и остановить pipeline
3. **Reactions** — фильтры (мат, бан-слова) + автоответы на ключевые слова

Модули **Scheduler** и **Maintenance** работают в фоне и не участвуют в pipeline.

---

## 1. Statistics

**Назначение:** Сбор статистики активности пользователей.

- Записывает каждое сообщение в таблицу `messages` с JSONB metadata
- Определяет тип контента (photo, video, sticker, text и т.д.)
- Извлекает file_id для медиа-контента
- Поддерживает топики (thread_id)

**Команды:** `/statistics`, `/myweek`, `/chatstats`, `/topchat`

---

## 2. Limiter

**Назначение:** Контроль лимитов на типы контента с VIP-обходом.

- Лимиты настраиваются per-chat и per-topic
- VIP-пользователи игнорируют все лимиты
- Предупреждение перед достижением лимита (порог из БД)
- Особый тип `banned_words` — лимит на мат (работает вместе с Reactions)
- При превышении лимита сообщение удаляется, pipeline останавливается

**Команды:** `/limiter`, `/mystats`, `/getlimit`, `/setlimit`, `/setvip`, `/removevip`, `/listvips`

---

## 3. Reactions

**Назначение:** Автоответы, фильтрация запрещённых слов и ненормативной лексики.

Объединяет три подсистемы в одном модуле:

### 3a. Фильтр мата (Profanity)
- Встроенный словарь ~5000 слов (embedded в бинарник)
- Действия: `delete`, `warn`, `delete_warn`
- Предупреждение перед баном (WarningThreshold из content_limits)
- Лимит на количество матов в день (тип `banned_words` в Limiter)

### 3b. Фильтр запрещённых слов (TextFilter)
- Кастомные слова/фразы per-chat
- Хранятся в `keyword_reactions` с `action = 'delete'`
- При срабатывании сообщение удаляется

### 3c. Автоответы на ключевые слова
- Паттерн → ответ (текст, стикер, GIF)
- Поддержка regex, cooldown, per-user реакции
- Хранятся в `keyword_reactions` с `action = 'reply'`

**Порядок проверки:** мат → бан-слова → автоответы

**Команды:**
- Автоответы: `/reactions`, `/addreaction`, `/listreactions`, `/removereaction`
- Фильтр слов: `/textfilter`, `/addban`, `/listbans`, `/removeban`
- Фильтр мата: `/profanity`, `/setprofanity`, `/profanitystatus`, `/removeprofanity`

---

## 4. Scheduler

**Назначение:** Выполнение задач по расписанию (cron).

- Задачи хранятся в БД (таблица `scheduled_tasks`)
- Формат расписания: cron-выражения (5 полей)
- Время: Europe/Moscow (UTC+3)
- При shutdown все задачи корректно останавливаются

**Команды:** `/scheduler`, `/addtask`, `/listtasks`, `/deltask`, `/runtask`

---

## 5. Maintenance

**Назначение:** Фоновое обслуживание БД.

- Автоматическое создание партиций `messages` и `event_log` на будущие месяцы
- Удаление старых партиций (старше `DB_RETENTION_MONTHS`)
- Запуск по cron: ежедневно в 03:00 MSK
- Не имеет команд — работает полностью автоматически

---

## Зависимости между модулями

```
Statistics ← Limiter (использует счётчик из messages)
Statistics ← Reactions (использует счётчик из messages)
Limiter ← Reactions (banned_words лимит работает вместе с profanity)
```

Все модули используют общие пакеты: `core` (helpers, middleware), `postgresql/repositories`.
