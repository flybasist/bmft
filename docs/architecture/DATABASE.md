# База данных BMFT

## Обзор

BMFT использует PostgreSQL 16+ с партиционированием и JSONB metadata для гибкого хранения данных.

**Ключевые особенности:**
- 🔄 **Партиционирование** — автоматическое разбиение по месяцам
- 📊 **JSONB metadata** — расширяемое хранение данных
- 🔀 **Поддержка топиков** — `thread_id` для Telegram Forums
- ♻️ **Автоматическая ротация** — старые данные удаляются автоматически

---

## Схема БД v1.0

### Основные таблицы (12 штук)

| Таблица | Назначение | Топики | Партиции |
|---------|------------|--------|----------|
| `chats` | Реестр чатов | `is_forum` флаг | ❌ |
| `users` | Реестр пользователей | - | ❌ |
| `chat_vips` | VIP-пользователи (обход лимитов) | `thread_id` | ❌ |
| `messages` | **Единый источник правды** с JSONB | `thread_id` | ✅ по месяцам |
| `content_limits` | Настройки лимитов контента | `thread_id` | ❌ |
| `keyword_reactions` | Автореакции на ключевые слова | `thread_id` | ❌ |
| `banned_words` | Фильтр запрещённых слов | `thread_id` | ❌ |
| `scheduled_tasks` | Задачи по расписанию (cron) | `thread_id` | ❌ |
| `event_log` | Аудит событий администраторов | - | ✅ по месяцам |
| `profanity_dictionary` | Глобальный словарь матов | - | ❌ |
| `profanity_settings` | Настройки фильтра матов | `thread_id` | ❌ |
| `bot_settings` | Версия бота, timezone | - | ❌ |

---

## Логика работы с топиками

### thread_id поле

Все таблицы конфигурации поддерживают топики через поле `thread_id`:

```sql
thread_id BIGINT DEFAULT 0
```

**Значения:**
- `thread_id = 0` → настройка для всего чата
- `thread_id > 0` → настройка для конкретного топика

**Приоритет:**
1. Если есть настройка для топика → используется она
2. Если нет → fallback на настройку чата (`thread_id = 0`)
3. Если нет вообще → модуль неактивен

**Примеры использования:**

```sql
-- VIP во всём чате
INSERT INTO chat_vips (chat_id, thread_id, user_id) 
VALUES (-1001234567890, 0, 123456789);

-- VIP только в топике #general (thread_id = 5)
INSERT INTO chat_vips (chat_id, thread_id, user_id) 
VALUES (-1001234567890, 5, 987654321);

-- Лимит на GIF в топике #memes (thread_id = 10)
INSERT INTO content_limits (chat_id, thread_id, limit_animation) 
VALUES (-1001234567890, 10, 3);
```

---

## Партиционирование

### messages — единый источник правды

Таблица `messages` хранит **все** сообщения пользователей и партиционирована по месяцам:

```sql
CREATE TABLE messages (
    id BIGSERIAL,
    chat_id BIGINT NOT NULL,
    thread_id BIGINT DEFAULT 0,
    user_id BIGINT NOT NULL,
    message_id BIGINT NOT NULL,
    content_type VARCHAR(20) NOT NULL,
    text TEXT,
    caption TEXT,
    file_id TEXT,
    metadata JSONB DEFAULT '{}',
    was_deleted BOOLEAN DEFAULT FALSE,
    deletion_reason TEXT,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Партиции
CREATE TABLE messages_2025_11 PARTITION OF messages 
FOR VALUES FROM ('2025-11-01') TO ('2025-12-01');

CREATE TABLE messages_2025_12 PARTITION OF messages 
FOR VALUES FROM ('2025-12-01') TO ('2026-01-01');
```

**Преимущества:**
- ⚡ Быстрые запросы (сканирует только нужные месяцы)
- 🗑️ Мгновенное удаление старых данных (`DROP TABLE`)
- 💾 Легко делать бэкапы отдельных периодов

### event_log — аудит действий

Аналогично партиционирована таблица `event_log`:

```sql
CREATE TABLE event_log (
    id BIGSERIAL,
    chat_id BIGINT NOT NULL,
    user_id BIGINT,
    module_name VARCHAR(50) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    details TEXT,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);
```

**Автоматическое управление:**
- Модуль `Maintenance` создаёт партиции на 3 месяца вперёд
- Удаляет партиции старше `DB_RETENTION_MONTHS` (default: 6 месяцев)

---

## JSONB metadata

### Зачем нужен metadata?

JSONB поля позволяют расширять данные без изменения схемы БД:

```sql
metadata JSONB DEFAULT '{}'
```

**Примеры использования:**

#### messages.metadata
```json
{
  "limiter": {
    "violation_count": 2,
    "last_violation": "2025-11-17T12:00:00Z"
  },
  "statistics": {
    "reaction_time_ms": 150
  }
}
```

#### event_log.metadata
```json
{
  "old_value": "5",
  "new_value": "10",
  "ip_address": "192.168.1.1"
}
```

**Преимущества:**
- ✅ Гибкость — добавляем данные без миграций
- ✅ Быстрый поиск — GIN индексы поддерживают JSONB
- ✅ Расширяемость — каждый модуль хранит свои данные

---

## Индексы

### Основные индексы

```sql
-- messages: быстрые запросы по чату/пользователю
CREATE INDEX idx_messages_chat_user 
ON messages(chat_id, thread_id, user_id, created_at DESC);

-- messages: фильтрация по типу контента
CREATE INDEX idx_messages_content_type 
ON messages(chat_id, thread_id, content_type, created_at DESC);

-- messages: поиск по metadata
CREATE INDEX idx_messages_metadata 
ON messages USING GIN (metadata);

-- event_log: аудит по чату
CREATE INDEX idx_event_log_chat 
ON event_log(chat_id, created_at DESC);

-- event_log: фильтрация по модулю
CREATE INDEX idx_event_log_module 
ON event_log(module_name, created_at DESC);
```

---

## Миграции

### Автоматическое применение

Бот автоматически применяет миграции при старте:

```
migrations/
└── 001_initial_schema.sql  -- Создаёт все 12 таблиц
```

**Процесс:**
1. Бот проверяет версию схемы в `bot_settings`
2. Применяет недостающие миграции
3. Обновляет версию

**Безопасность:**
- ✅ Идемпотентность — повторный запуск безопасен
- ✅ Валидация — проверка наличия всех таблиц
- ✅ Откат — при ошибке миграции бот не стартует

---

## Запросы для анализа

### Размер таблиц

```sql
SELECT 
  schemaname || '.' || tablename AS table,
  pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public'
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC;
```

### Партиции messages

```sql
SELECT 
  tablename,
  pg_size_pretty(pg_total_relation_size('public.' || tablename)) AS size
FROM pg_tables
WHERE schemaname = 'public' 
  AND tablename LIKE 'messages_%'
ORDER BY tablename;
```

### Топ активных чатов

```sql
SELECT 
  chat_id,
  COUNT(*) AS message_count
FROM messages
WHERE created_at >= NOW() - INTERVAL '30 days'
GROUP BY chat_id
ORDER BY message_count DESC
LIMIT 10;
```

### Статистика по типам контента

```sql
SELECT 
  content_type,
  COUNT(*) AS count,
  ROUND(100.0 * COUNT(*) / SUM(COUNT(*)) OVER (), 2) AS percentage
FROM messages
WHERE created_at >= NOW() - INTERVAL '7 days'
GROUP BY content_type
ORDER BY count DESC;
```

---

## Бэкапы

### Полный дамп

```bash
docker exec bmft_postgres pg_dump -U bmft bmft > backup.sql
```

### Бэкап конкретного месяца

```bash
# Только ноябрь 2025
docker exec bmft_postgres pg_dump -U bmft bmft \
  -t messages_2025_11 \
  -t event_log_2025_11 \
  > backup_2025_11.sql
```

### Восстановление

```bash
docker exec -i bmft_postgres psql -U bmft bmft < backup.sql
```

---

## Производительность

### Рекомендации

1. **Регулярный VACUUM:**
   ```sql
   VACUUM ANALYZE messages;
   VACUUM ANALYZE event_log;
   ```

2. **Мониторинг размера партиций:**
   - Контролировать рост данных
   - Своевременно создавать новые партиции

3. **Оптимизация JSONB:**
   - Избегать слишком глубоких структур
   - Использовать GIN индексы для поиска

4. **Connection pooling:**
   - PostgreSQL хорошо работает с пулом 10-20 соединений
   - Не создавать слишком много соединений

---

**См. также:**
- [ROTATION.md](guides/ROTATION.md) — настройка автоматической ротации данных
- [Миграции](../migrations/) — SQL скрипты создания схемы
