# ✅ Миграции реорганизованы

**Дата:** 4 октября 2025, 17:06  
**Ветка:** phase3-reactions-module  
**Статус:** ГОТОВО ✅

---

## 📋 Что сделано

### 1. Объединены миграции ✅

**Было:**
```
migrations/
├── 001_initial_schema.sql  (~350 строк)
└── 003_create_limits_table.sql  (~30 строк)
```

**Стало:**
```
migrations/
├── 001_initial_schema.sql  (~400 строк) ✅
└── README.md  (новый) ✅
```

### 2. Обновлён 001_initial_schema.sql ✅

**Добавлено:**
- ✅ Новая шапка с описанием всех Phase
- ✅ Секция "LIMITER MODULE (Phase 2)"
- ✅ Таблица `user_limits` с комментариями
- ✅ Индексы для оптимизации
- ✅ Комментарий о автоматической проверке схемы

**Результат:** Один файл содержит всю схему БД (Phase 1-5)

### 3. Создан README.md для migrations/ ✅

**Содержит:**
- 📖 Объяснение нового подхода
- 🚀 Как работает автоматическая валидация
- 🔄 Инструкция по вайпу БД для dev
- 📦 План для продакшена (позже)

---

## 🎯 Новый подход (как в твоём проекте)

### Принцип:
> При первом запуске приложение:
> 1. Проверит схему БД
> 2. Выполнит миграцию `001_initial_schema.sql` если таблиц нет
> 3. Валидирует что все необходимые таблицы и колонки присутствуют
> 4. Остановится с ошибкой если обнаружит частично созданную/некорректную схему

### Для разработки:
```bash
# Вайпаем БД и создаём заново
docker-compose down -v
docker-compose up -d

# Бот сам применит миграции
./bin/bot
```

**Безопасно для dev!** Все данные тестовые.

---

## 📊 Содержимое 001_initial_schema.sql

### Phase 1: Core Framework
- ✅ `chats` - метаинформация о чатах
- ✅ `users` - кэш пользователей
- ✅ `chat_admins` - администраторы чатов
- ✅ `chat_modules` - включение/выключение модулей
- ✅ `event_log` - audit log всех событий

### Phase 2: Limiter Module
- ✅ `user_limits` - лимиты пользователей (daily/monthly)

### Phase 3: Reactions Module
- ✅ `reactions_config` - настройки реакций (regex patterns)
- ✅ `reactions_log` - история срабатываний

### Phase 4: Statistics Module
- ✅ `statistics_daily` - суточная статистика
- ✅ `statistics_monthly` - месячная статистика

### Phase 5: Scheduler Module
- ✅ `scheduler_tasks` - задачи по расписанию

### Дополнительно:
- ✅ `bot_settings` - глобальные настройки бота
- ✅ Views: `v_active_modules`, `v_daily_chat_stats`
- ✅ Triggers: `update_updated_at_column()`
- ✅ Seed data: доступные модули, версия, timezone

---

## ✅ Что дальше

### Phase 4: Добавить валидацию схемы

Создать `internal/postgresql/schema_validator.go`:

```go
package postgresql

import (
    "database/sql"
    "fmt"
)

// ValidateSchema проверяет что все необходимые таблицы и колонки есть
func ValidateSchema(db *sql.DB) error {
    requiredTables := []string{
        "chats", "users", "chat_admins", "chat_modules",
        "user_limits", "event_log",
        "reactions_config", "reactions_log",
        "statistics_daily", "statistics_monthly",
        "scheduler_tasks", "bot_settings",
    }
    
    for _, table := range requiredTables {
        var exists bool
        query := `
            SELECT EXISTS (
                SELECT FROM information_schema.tables 
                WHERE table_schema = 'public' 
                AND table_name = $1
            );
        `
        if err := db.QueryRow(query, table).Scan(&exists); err != nil {
            return fmt.Errorf("failed to check table %s: %w", table, err)
        }
        
        if !exists {
            return fmt.Errorf("required table missing: %s\n\nPlease drop database and restart:\n  docker-compose down -v\n  docker-compose up -d", table)
        }
    }
    
    return nil
}

// CheckIfMigrationNeeded проверяет нужна ли миграция
func CheckIfMigrationNeeded(db *sql.DB) error {
    var count int
    query := `
        SELECT COUNT(*) 
        FROM information_schema.tables 
        WHERE table_schema = 'public';
    `
    if err := db.QueryRow(query).Scan(&count); err != nil {
        return fmt.Errorf("failed to count tables: %w", err)
    }
    
    if count == 0 {
        // БД пустая - нужна миграция
        return nil
    }
    
    if count < 12 {
        // Частичная схема - ошибка!
        return fmt.Errorf("partial database schema detected (%d tables)\n\nPlease drop database and restart:\n  docker-compose down -v\n  docker-compose up -d", count)
    }
    
    // Схема уже применена
    return nil
}
```

### В main.go добавить:

```go
// После подключения к БД:

// 1. Проверяем нужна ли миграция
if err := postgresql.CheckIfMigrationNeeded(db); err != nil {
    if err.Error() == "partial database schema detected" {
        return err // Останавливаемся
    }
    
    // БД пустая - применяем миграцию
    logger.Info("running initial migration...")
    migrationFile := "migrations/001_initial_schema.sql"
    
    sqlContent, err := os.ReadFile(migrationFile)
    if err != nil {
        return fmt.Errorf("failed to read migration file: %w", err)
    }
    
    if _, err := db.Exec(string(sqlContent)); err != nil {
        return fmt.Errorf("failed to run migration: %w", err)
    }
    
    logger.Info("migration completed successfully")
}

// 2. Валидируем схему
if err := postgresql.ValidateSchema(db); err != nil {
    return fmt.Errorf("database schema validation failed: %w", err)
}

logger.Info("database schema validated successfully")
```

**Но это в Phase 4!** Сейчас работаем без валидации (docker-compose сам применяет миграции).

---

## 📝 Изменённые файлы (готовы к commit)

### Изменённые:
1. `migrations/001_initial_schema.sql` (+50 строк) ✅

### Новые:
2. `migrations/README.md` (100 строк) ✅

### Удалённые:
3. `migrations/003_create_limits_table.sql` ❌

**Итого:** 3 файла изменено, +150 строк

---

## 🎉 Готовность

✅ **Миграции реорганизованы**  
✅ **Один файл = вся схема**  
✅ **README с инструкциями**  
✅ **Готово к commit**  
✅ **Готово к Phase 3**

---

## 📌 Commit message:

```bash
git add migrations/
git commit -m "refactor: объединены миграции в один файл

- Объединены 001_initial_schema.sql + 003_create_limits_table.sql
- Один файл теперь содержит всю схему (Phase 1-5)
- Добавлен migrations/README.md с инструкциями
- Удалён 003_create_limits_table.sql

Новый подход:
- Для dev: вайпаем БД и создаём заново
- Для prod: будет добавлена валидация схемы (Phase 4)
- Защита от частичных миграций (будет в Phase 4)

Refs: подход из другого проекта (автоматическая валидация)
"
```

---

**Подготовил:** GitHub Copilot  
**Время:** 5 минут  
**Статус:** READY TO COMMIT ✅
