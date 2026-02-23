# Схема базы данных BMFT v1.1.1

PostgreSQL 16+ с партиционированием по месяцам.

## Таблицы

### Core

| Таблица | Описание |
|---------|----------|
| `chats` | Реестр чатов (chat_id, chat_type, title, is_forum, is_active) |
| `chat_vips` | VIP-пользователи per-chat/per-topic |
| `messages` | Все сообщения — партиционирована по месяцам (RANGE по created_at) |
| `bot_settings` | Версия бота, timezone, available_modules |
| `schema_migrations` | Версионирование миграций |
| `event_log` | Audit trail — партиционирована по месяцам |

### Limiter

| Таблица | Описание |
|---------|----------|
| `content_limits` | Лимиты per-chat/per-topic/per-user с warning_threshold |

### Reactions

| Таблица | Описание |
|---------|----------|
| `keyword_reactions` | Паттерны и ответы (автоответы, бан-слова, фильтры) |
| `reaction_triggers` | Счётчики срабатываний per-user |
| `reaction_daily_counters` | Дневные счётчики срабатываний |

### Profanity

| Таблица | Описание |
|---------|----------|
| `profanity_dictionary` | Глобальный словарь (~5000 слов, embedded) |
| `profanity_settings` | Per-chat/per-topic настройки (action: delete/warn/mute) |

### Scheduler

| Таблица | Описание |
|---------|----------|
| `scheduled_tasks` | Задачи cron per-chat |

## Партиционирование

Таблицы `messages` и `event_log` партиционированы по `RANGE (created_at)`:

```
messages_2025_07  (2025-07-01 .. 2025-08-01)
messages_2025_08  (2025-08-01 .. 2025-09-01)
...
```

Партиции создаются автоматически модулем **Maintenance** (на 3 месяца вперёд). Старые партиции удаляются через `DB_RETENTION_MONTHS` (по умолчанию 6).

## JSONB Metadata

Таблица `messages` хранит метаданные модулей в поле `metadata` (JSONB):

```json
{
  "limiter": {"content_type": "photo", "limit_value": 10, "counter": 5},
  "profanity": {"matched_words": ["слово"], "action": "delete"},
  "reactions": {"reaction_id": 42, "response_type": "text"},
  "statistics": {"file_id": "AgACAgIAA...", "file_unique_id": "AQADAgAT"}
}
```

## Fallback-логика лимитов

`content_limits.GetLimits()` использует 4-уровневый fallback:

1. Per-user + per-topic → если найден, используется
2. Per-topic (без user) → fallback
3. Per-user + весь чат (thread_id=0) → fallback
4. Весь чат (thread_id=0, без user) → последний fallback

## Миграции

- `001_initial_schema.sql` — полная актуальная схема v1.1.1 (для новых установок)
- `002_migration.sql` — обновление v1.0 → v1.1
- `003_migration.sql` — обновление v1.1 → v1.1.1 (version bump)

Миграции применяются автоматически при старте бота (`migrations.RunMigrationsIfNeeded`).
