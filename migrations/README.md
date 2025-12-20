# Database Migrations

Система версионирования миграций для BMFT бота.

## Структура

- `001_initial_schema.sql` - Начальная схема БД (версия 1)
- `schema_migrations` - Таблица отслеживания версий

## Как работает

1. При старте бот проверяет таблицу `schema_migrations`
2. Если таблицы нет → запускает `001_initial_schema.sql`
3. Если версия < `LatestSchemaVersion` → применяет недостающие миграции
4. После применения миграции записывается версия в `schema_migrations`

## Добавление новой миграции

### 1. Создайте новый SQL файл

```bash
# Например, для версии 2:
touch migrations/002_add_new_feature.sql
```

### 2. Напишите SQL команды

```sql
-- migrations/002_add_new_feature.sql
-- Описание: Добавление новой функциональности

ALTER TABLE messages ADD COLUMN new_field TEXT;
CREATE INDEX idx_messages_new_field ON messages(new_field);

-- Запись версии миграции
INSERT INTO schema_migrations (version, description) 
VALUES (2, 'Add new_field to messages table')
ON CONFLICT (version) DO NOTHING;
```

### 3. Обновите константу версии

В `internal/migrations/migrations.go`:

```go
const LatestSchemaVersion = 2  // было 1
```

### 4. Обновите ExpectedSchema (если нужно)

Если добавили обязательные поля/таблицы, добавьте их в `ExpectedSchema`:

```go
var ExpectedSchema = []ExpectedTable{
    // ...
    {Name: "messages", Columns: []string{"id", "chat_id", "new_field", "metadata"}},
    // ...
}
```

### 5. Тестирование

```bash
# Сборка
go build -o bmft ./cmd/bmft/

# Запуск (миграция применится автоматически)
./bmft
```

## Откат миграций

⚠️ **Автоматический откат не поддерживается!**

Для отката нужно:
1. Вручную откатить изменения в БД
2. Удалить запись из `schema_migrations`
3. Убедиться что схема соответствует `ExpectedSchema`

## Проверка текущей версии

```sql
SELECT version, description, applied_at 
FROM schema_migrations 
ORDER BY version DESC;
```

## Примеры миграций

### Добавление поля

```sql
-- 002_add_field.sql
ALTER TABLE messages ADD COLUMN IF NOT EXISTS new_field TEXT;
CREATE INDEX IF NOT EXISTS idx_messages_new_field ON messages(new_field);

INSERT INTO schema_migrations (version, description) 
VALUES (2, 'Add new_field to messages')
ON CONFLICT (version) DO NOTHING;
```

### Создание таблицы

```sql
-- 003_create_table.sql
CREATE TABLE IF NOT EXISTS new_table (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

INSERT INTO schema_migrations (version, description) 
VALUES (3, 'Create new_table')
ON CONFLICT (version) DO NOTHING;
```

### Изменение типа поля

```sql
-- 004_change_type.sql
ALTER TABLE messages 
    ALTER COLUMN old_field TYPE BIGINT USING old_field::BIGINT;

INSERT INTO schema_migrations (version, description) 
VALUES (4, 'Change old_field type to BIGINT')
ON CONFLICT (version) DO NOTHING;
```

## Безопасность

✅ Используйте `IF EXISTS` / `IF NOT EXISTS`
✅ Используйте `ON CONFLICT DO NOTHING`
✅ Тестируйте на копии БД перед продом
✅ Делайте бэкап перед миграцией на проде
✅ Миграции должны быть идемпотентными (можно запускать повторно)

## Логирование

Бот логирует все операции миграций:

```
INFO starting database schema validation and migrations
INFO current schema version version=1
INFO schema is up to date, validating structure
INFO schema validation completed successfully tables=12
```

При применении новой миграции:

```
INFO applying pending migrations current=1 target=2
INFO applying migration version=2 file=migrations/002_migration.sql
INFO migration applied successfully version=2
```
