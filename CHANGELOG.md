# CHANGELOG

## v1.1.1 — Anti-Spam & Admin Security (2025-07-07)

### 🔴 Критические исправления

- **CommandCooldownMiddleware**: 5 сек per-user per-chat **per-command** — повторные команды удаляются, бот молчит
- **AdminOnlyMiddleware**: все 19 админских команд защищены на уровне middleware с кешем `getChatAdministrators` (TTL 60 сек). Не-админ → команда удаляется, бот молчит
- **Молчащие обработчики**: все admin-хендлеры больше не отправляют `❌ Команда доступна только администраторам` — middleware единственная линия защиты
- **`/profanitystatus` без проверки прав**: команда была доступна всем — добавлена в список AdminOnlyMiddleware

### 🟡 Оптимизация

- **`/mystats` 12 SQL → 1**: заменены 12 отдельных `GetTodayCountByType` на один `GetTodayCountsAllTypes` с `GROUP BY content_type` + `COUNT(*) FILTER`
- **Per-command cooldown**: ключ кулдауна включает имя команды — `/version` и `/help` больше не блокируют друг друга
- **Удалён `IsUserAdmin`**: убрана DEPRECATED функция и 18 избыточных некешированных API-вызовов из admin-хендлеров — AdminOnlyMiddleware единственная проверка прав
- **Рефакторинг `GetThreadID`**: обёртка над `GetThreadIDFromMessage`, убрано дублирование логики

### 🟢 UX

- **`/help` с 🔒 маркерами**: админские команды помечены 🔒, добавлена легенда
- **Справки модулей**: `/listreactions`, `/listbans`, `/listtasks`, `/removereaction` помечены как admin-only

### 🟢 Документация

- **COMMANDS_ACCESS.md**: 5 команд исправлены с «Все» на «Админ» (`/listreactions`, `/listbans`, `/listtasks`, `/chatstats`, `/topchat`, `/profanitystatus`)
- **`mute` → `delete_warn`**: исправлена ошибочная документация действий profanity (MODULES.md)
- **ARCHITECTURE.md**: добавлены CommandCooldown и AdminOnly в pipeline diagram и дерево проекта
- **Версия 1.1.1**: обновлена в README, DATABASE, migrations/README
- **CHANGELOG v1.1**: исправлена секция миграций (002=v1.0→v1.1, 003=v1.1→v1.1.1)

### 🟢 Инфраструктура

- **Версия 1.1.1**: обновлена во всех местах — `main.go` fallback, `Dockerfile LABEL`, `001_initial_schema.sql` DEFAULT
- **Миграция 003**: `UPDATE bot_version = '1.1.1'` для существующих установок, `LatestSchemaVersion = 3`
- **`.gitignore`**: убраны избыточные правила `!cmd/bot/` и `!internal/`
- **`bmft-test`**: удалён забытый бинарник из корня проекта

## v1.1 — Bugfix + Consolidation Release (2025-07-06)

Исправление 18 багов + консолидация модулей для упрощения архитектуры.
Миграция безопасна для существующих данных — применяется автоматически при старте.

### 🔍 Аудит: flow + межмодульные конфликты (пятый проход)

- **🟡 DeleteOnLimit — предупреждение повторялось бесконечно**: `incrementDailyCount` не вызывался на пути удаления → `count == DailyLimit` было **всегда** true. Добавлен вызов `incrementDailyCount` перед `continue` — предупреждение теперь отправляется ровно один раз
- **🟡 Двойной `db.Close()`**: `defer db.Close()` в `run()` + явный `db.Close()` в shutdown goroutine → второй вызов возвращал ошибку. Убран явный вызов, `defer` достаточно
- **🟡 Мёртвый код `SchedulerModule.OnMessage`**: пустой stub, никогда не вызывался из pipeline — удалён
- **🟢 Устаревшие комментарии**: обновлены шаги загрузки в main.go (убраны ссылки на Module Registry), убраны префиксы «Русский комментарий:» из handlers.go, limiter.go, vip_repository.go, убрана ссылка на v0.8.0 из message_repository.go
- **🟢 Устаревшая документация**: ARCHITECTURE.md — `StopPropagation` заменён на `MessageDeleted`, CHANGELOG — убрана ссылка на `StopPropagation=true` из записи третьего прохода

### 🚀 Аудит: deployment (апгрейд + чистая установка) (шестой проход)

- **🟡 `PingWithRetry` — логгер сломан, ретраи молчат**: локальный интерфейс `(msg string, fields ...interface{})` не совпадал с `*zap.Logger` `(msg string, fields ...zap.Field)` → type assertion всегда `false`. Заменено на прямой `*zap.Logger`, добавлены structured-поля (attempt, max_retries, error). Убран устаревший комментарий пакета
- **🟡 Схема не валидировалась после апгрейд-миграции**: `applyPendingMigrations` делал `return` без вызова `validateExistingSchema` — при частичном сбое миграции бот мог стартовать с неполной схемой. Добавлена валидация после успешной миграции
- **🟢 Dockerfile `LABEL version="0.6.0"`**: обновлён до `"1.1"`
- **🟢 Дефолтный пароль `secret` в docker-compose.bot/env.yaml**: не совпадал — при отсутствии `.env` бот не мог подключиться к БД. Все дефолты приведены к `bmft`
- **🟢 `.env.example` переписан с нуля**: покрыты все 16 переменных, 2 сценария, добавлены PROFANITY_DICT_SOURCE и TZ
- **🟢 `.env` исправлен**: были переменные от другого проекта (BOT_TOKEN, DB_HOST, API_PORT и т.д.)
- **🟢 TZ хардкод `Europe/Moscow`**: заменён на `${TZ:-Europe/Moscow}` в обоих docker-compose и Dockerfile

### 🧪 Аудит: боевое тестирование (седьмой проход)

- **🟡 Спам предупреждениями при превышении лимита**: Limiter отправлял `❌ лимит достигнут` при **каждом** превышении (6/5, 7/5, 8/5...) — чат заспамливался. Теперь предупреждение только при первом превышении (`counter == limitValue + 1`), остальные удаляются молча. Аналогично для `limitValue == -1` (запрещено) — предупреждение только при первом сообщении за день
- **🟡 `/version` показывал "1.0" после апгрейда**: миграция 002 не обновляла `bot_version` в `bot_settings`, fallback в main.go был "1.0". Добавлен UPDATE в 002_migration.sql, fallback исправлен на "1.1"
- **�🟢 Путь к pgdata**: `./data/postgres/pgdata` → `./data/pgdata` — совместимость с v1.0, где данные лежали в `./data/pgdata`
- **🟢 Устаревшая документация**: ARCHITECTURE.md — `StopPropagation` заменён на `MessageDeleted`, CHANGELOG — убрана ссылка на `StopPropagation=true` из записи третьего прохода

### 🔵 Консолидация модулей

- **TextFilter + ProfanityFilter → Reactions**: объединены в единый модуль
  - Pipeline упрощён: `statistics → limiter → reactions` (было 5 модулей, стало 3)
  - Все команды сохранены: /addban, /listbans, /removeban, /setprofanity, /removeprofanity, /profanitystatus
  - /textfilter и /profanity — по-прежнему показывают справку (как разделы /reactions)
- **Удалены мёртвые таблицы**: `users` (никогда не использовалась), `daily_content_stats` (никогда не SELECT'илась)
- **banned_words → keyword_reactions**: данные мигрированы в единую таблицу через колонку `action`
- **Удалена функция** `refresh_stats_views()` (зависела от удалённой `daily_content_stats`)
- **Удалён мёртвый тип** `TextFilterMetadata` из message_repository.go

### 🔴 Критические исправления

- **Healthcheck**: добавлен HTTP-сервер на `:9090` с `/healthz` — Docker HEALTHCHECK теперь работает
- **Graceful shutdown**: заменён сломанный `select { default: }` на реальное ожидание завершения через done-channel
- **Scheduler /deltask**: задача теперь удаляется и из cron в памяти, а не только из БД
- **action_data**: тип колонки изменён с JSONB на TEXT — Go-код писал туда строки, не JSON
- **refresh_stats_views()**: убрана ссылка на несуществующий `daily_reaction_stats`

### 🟡 Важные исправления

- **Limiter off-by-one**: убран лишний `counter++` — Statistics уже сохранил сообщение до Limiter
- **GetThreadID**: возвращаемый тип унифицирован в `int` (было `int64`), каскадные исправления в 6+ файлах
- **Логгер**: убран `AddCallerSkip(1)` — логи теперь показывают правильный файл/строку
- **Scheduler help**: время исправлено с UTC+0 на Europe/Moscow (UTC+3)
- **Pipeline комментарий**: исправлен порядок на `statistics → limiter → reactions`

### 🟢 Очистка кода

- **/version**: убран устаревший текст из rts_bot v0.5
- **.env.example**: убран дублирующийся `LOG_LEVEL`
- **reactions.go**: убрана бессмысленная переменная `countUserID`
- **Мёртвый код**: удалены `GetLimitForContentType()`, `SchemaState` + константы
- **go.mod**: `lumberjack` помечен как direct (был indirect)

### 🔧 UX-исправления (справки, флоу, проверки)

- **handleUserJoined**: бот создаёт запись чата в БД при добавлении — `/setlimit` и `/setvip` больше не падают в новых чатах
- **handleSetLimit / handleSetVIP**: добавлена проверка существования чата в `chats` (INSERT ON CONFLICT)
- **/help**: добавлены отсутствующие команды `/mystats` и `/getlimit` в секцию Limiter
- **Справка /setvip и /removevip**: убраны несуществующие примеры с `@username` — бот принимает только reply
- **banned_words + text -1**: при установке специальных лимитов теперь показываются контекстные предупреждения
- **/removereaction и /removeban**: удаление по `chat_id + id` без привязки к `thread_id` — админ может удалить любую реакцию/запрет своего чата из любого места
- **OnEdited**: убран обработчик редактирования — отредактированные сообщения дублировали статистику и списывали лимиты
- **Справка /reactions**: добавлена документация приоритета обработки (profanity → textfilter → autoreply)
- **/listvips**: вместо голых User ID теперь показывает @username или имя
- **/mystats**: убран путающий формат «X из 0 (без лимита)» → «X (без лимита)»
- **/setlimit**: убран несуществующий аргумент `[@username]` из usage text

### � Архитектурные исправления (второй проход)

- **🔴 /listvips SQL crash**: удалён `LEFT JOIN users` — таблица `users` была удалена в миграции 002. Имена VIP теперь получаются через Telegram API (`ChatMemberOf`)
- **🔴 is_forum никогда не записывалось**: `GetOrCreate` теперь принимает `isForum` и записывает значение в БД. Добавлен хелпер `CheckIsForum()` — прямой запрос `getChat` к Telegram API (telebot.v3 v3.3.8 не экспортирует `IsForum` в Chat struct)
- **🟠 Команды проходили через limiter**: добавлен пропуск приватных сообщений и команд (`/...`) в `OnMessage` — админ больше не теряет управление при исчерпании лимита
- **🟠 Персональные лимиты не проверялись**: `GetLimits` теперь получает `&userID` вместо `nil` — персональные лимиты работают
- **🟠 Limiter использовал сырой ThreadID**: заменён на `core.GetThreadIDFromMessage()` — корректный учёт топиков
- **🟡 delete_warn: reply на удалённое**: предупреждение теперь отправляется в чат напрямую (`Bot.Send`), а не как reply на удалённое сообщение
- **🟡 /setvip теряло первое слово reason**: исправлен срез `args[1:]` → `args` — полный текст причины сохраняется
- **🟢 /setlimit с неверным типом**: добавлена валидация типа контента с выводом списка допустимых значений
- **🟢 /addtask text-mode**: добавлена валидация длины имени (аналогично reply-mode)
- **🟡 /mystats — счётчик мата всегда 0**: `banned_words` считался через `GetTodayCountByType` (по content_type), но маты хранятся в metadata. Заменён на `GetTodayCountByMetadata(profanity, true)` — теперь показывает реальное число нарушений

### 🔧 Межмодульные исправления (третий проход)

- **🟠 /setprofanity и /addtask — FK crash**: добавлен страховочный `INSERT INTO chats ON CONFLICT DO NOTHING` перед записью в `profanity_settings` и `scheduled_tasks` — команды работают даже если записи чата нет в БД
- **🟡 Пустой username → «@, лимит на...»**: добавлен хелпер `core.DisplayName()` — возвращает `@username`, иначе `FirstName`, иначе `"Пользователь"`. Заменено в 6 местах (limiter warning, limit exceeded, forbidden, filter warn, filter delete_warn, profanity warning, profanity ban)
- **🟡 Мат в caption не считался при удалении Limiter-ом**: Reactions.OnMessage теперь проверяет мат даже если сообщение удалено Limiter-ом (в режиме «только подсчёт»). Metadata обновляется, banned_words инкрементируется, бан при превышении срабатывает. Действие (delete/warn) не выполняется — сообщение уже удалено
### 🏗️ Архитектурная переработка pipeline (четвёртый проход)

- **🔴 StopPropagation — мёртвый код**: каждый middleware создавал свой `MessageContext` — флаг Limiter-а не доходил до Reactions. Полная переработка: `StopPropagation` → `MessageDeleted`, пропагация между модулями через `c.Set()`/`c.Get()` из telebot.v3. Pipeline **всегда** вызывает `next(c)` — каждый модуль получает шанс обработать сообщение
- **🟠 Предупреждения без ThreadID в форумах**: `ctx.Send()`, `ctx.SendReply()`, `ctx.SendOptions()` теперь автоматически включают `ThreadID` в `SendOptions` — сообщения бота попадают в правильный топик. ThreadID вычисляется **один раз** в middleware и кешируется для всех модулей (−2 SQL-запроса на сообщение)
- **🟠 @FirstName в setvip/removevip**: при пустом username отображалось `@FirstName` вместо просто имени. Заменено на `core.DisplayName()` в `handleSetVIP` и `handleRemoveVIP`
- **🟠 Shutdown — Maintenance не останавливался**: если `Scheduler.Shutdown()` возвращал ошибку, `Maintenance.Shutdown()` не вызывался. Заменён на паттерн `firstErr` — все модули останавливаются независимо
- **🟡 3 дубля MessageRepository**: Statistics, Limiter и Reactions создавали по своему экземпляру `MessageRepository`. Теперь один общий, инициализированный в `initModules()`
- **🟡 reaction_triggers.user_id не обновлялся**: `ON CONFLICT DO UPDATE` не включал `user_id = EXCLUDED.user_id` — при повторном триггере другим пользователем ID автора оставался старым
### �📦 Миграции

- `002_migration.sql` — v1.0 → v1.1 (консолидация banned_words → keyword_reactions, DROP мёртвых таблиц, bugfixes)

## v1.0 — Initial Release

Первый стабильный релиз BMFT.
Модули: statistics, limiter, reactions, scheduler, textfilter, profanityfilter, maintenance.
