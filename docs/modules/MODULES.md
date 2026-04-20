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

**Команды:** `/limiter`, `/mystats`, `/getlimit`, `/setlimit` (🧙 wizard), `/setvip` (🧙 wizard с reply), `/removevip`, `/listvips`

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
- Автоответы: `/reactions`, `/addreaction` (🧙 wizard), `/listreactions` (inline 🗑), `/removereaction`
- Фильтр слов: `/textfilter`, `/addban` (🧙 wizard), `/listbans` (inline 🗑), `/removeban`
- Фильтр мата: `/profanity`, `/setprofanity` (🧙 wizard), `/profanitystatus`, `/removeprofanity`

---

## 4. Scheduler

**Назначение:** Выполнение задач по расписанию (cron).

- Задачи хранятся в БД (таблица `scheduled_tasks`)
- Формат расписания: cron-выражения (5 полей)
- Время: Europe/Moscow (UTC+3)
- При shutdown все задачи корректно останавливаются

**Команды:** `/scheduler`, `/addtask` (🧙 wizard), `/listtasks` (inline 🗑), `/deltask`, `/runtask`

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

---

## Интерактивные wizard'ы

Админские команды помеченные «🧙 wizard» в **групповых чатах** поддерживают
пошаговый мастер с inline-кнопками (вызов без аргументов). Реализация — пакет
`internal/wizard`:

- **State store** (в памяти): одна активная сессия на (chat×user); idle-timeout 5 мин.
- **TextInterceptMiddleware**: встраивается в pipeline после PanicRecovery; если у юзера
  есть активный wizard и пришёл не-командный текст — роутится в обработчик шага.
- **Отмена**: любая команда (`/cancel`, `/help` и т.д.) или кнопка «❌ Отменить».
- **Ограничения**: не работает в личке и для анонимных админов.
- **Старый синтаксис сохраняется**: вызов с аргументами обрабатывается легаси-handler'ом
  модуля.

Список wizard'ов: `/welcome`, `/setprofanity`, `/setvip`, `/setlimit`, `/addban`,
`/addtask`, `/addreaction`.

## Inline-кнопки в списках

Команды `/listtasks`, `/listbans`, `/listreactions` выводят inline-кнопки 🗑 рядом с
записями (до 50 кнопок на сообщение; для `/listreactions` — только при выводе в одно
сообщение). Обработчики callback'ов защищены `core.AdminOnlyCallback`, выполняют DELETE
в своём chat_id и при успехе показывают alert.
