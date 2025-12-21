# Логирование в BMFT

## Управление уровнем логирования

Уровень логирования управляется через переменную окружения `LOG_LEVEL` в `.env`:

```bash
# В .env или docker-compose.yaml
LOG_LEVEL=info  # debug | info | warn | error
```

## Уровни логирования

### `debug` - Разработка (максимальная детализация)
```
✅ Все операции с БД (INSERT, SELECT, UPDATE)
✅ Каждое входящее сообщение
✅ Детали обработки каждого модуля
✅ Все промежуточные состояния
✅ Парсинг аргументов команд
✅ Проверки VIP статуса, лимитов
```

**Использование**: Локальная разработка, поиск багов

### `info` - Production (только важные события)
```
✅ Старт/остановка бота
✅ Подключение к БД
✅ Применение миграций
✅ Критичные операции (добавление реакций, лимитов)
✅ Старт/остановка модулей
✅ Cron задачи (создание партиций, очистка)
❌ Каждое сообщение пользователя
❌ Детали SQL запросов
❌ Промежуточные проверки
```

**Использование**: Production, мониторинг работы

### `warn` - Предупреждения
```
✅ Неожиданные ситуации (не критичные)
✅ Устаревшие команды
✅ Лишние таблицы в БД
✅ Некорректные аргументы команд
❌ Нормальная работа
```

### `error` - Только ошибки
```
✅ Ошибки БД
✅ Ошибки API Telegram
✅ Критичные сбои
❌ Всё остальное
```

## Текущее состояние логирования

### ✅ Хорошо реализовано

**Миграции** (`internal/migrations/migrations.go`):
```go
logger.Info("starting database schema validation and migrations")
logger.Info("current schema version", zap.Int("version", currentVersion))
logger.Info("applying pending migrations", ...)
logger.Info("migration applied successfully", ...)
```

**Maintenance** (`internal/modules/maintenance/maintenance.go`):
```go
logger.Info("created partition", zap.String("table", ...), ...)
logger.Info("partition check completed successfully")
logger.Error("failed to create partitions", zap.Error(err))
```

**Reactions** (`internal/modules/reactions/reactions.go`):
```go
logger.Info("handleAddReaction called", ...)
logger.Info("parsed args", zap.Strings("args", args))
logger.Info("inserting reaction into DB", ...)
logger.Info("reaction added successfully", ...)  // ✅ Добавлено
logger.Error("failed to add reaction", zap.Error(err))
```

### ⚠️ Требует внимания

**Statistics** (`internal/modules/statistics/statistics.go`):
- `Debug` логи должны быть только в режиме debug:
  ```go
  logger.Debug("statistics: received message", ...)  // ✅ OK
  logger.Debug("statistics: detected content type", ...) // ✅ OK
  ```
- Но при ошибках нужен `Error`:
  ```go
  logger.Error("statistics: failed to insert message", ...) // ✅ OK
  ```

**Limiter** (`internal/modules/limiter/limiter.go`):
- При превышении лимита нужен `Info`:
  ```go
  logger.Info("limit exceeded", 
      zap.Int64("user_id", userID),
      zap.String("content_type", contentType),
      zap.Int("count", count),
      zap.Int("limit", limit))
  ```

**TextFilter** (`internal/modules/textfilter/textfilter.go`):
- При удалении сообщения нужен `Info`:
  ```go
  logger.Info("banned word detected", 
      zap.String("word", word),
      zap.String("action", action),
      zap.Int64("user_id", userID))
  ```

## Рекомендации для новых модулей

### Правило выбора уровня:

```go
// DEBUG - детали работы, промежуточные состояния
logger.Debug("processing user message", 
    zap.Int64("user_id", userID),
    zap.String("text", msg.Text))

// INFO - важные события, изменения состояния
logger.Info("reaction added", 
    zap.String("pattern", pattern),
    zap.Int64("chat_id", chatID))

// WARN - неожиданное, но не критичное
logger.Warn("unknown command", 
    zap.String("command", cmd))

// ERROR - ошибки, требующие внимания
logger.Error("failed to connect to database", 
    zap.Error(err))
```

### Обязательные поля в логах:

```go
// Для операций с БД
zap.Error(err)              // всегда при ошибке
zap.String("table", "...")  // название таблицы
zap.String("operation", "INSERT/UPDATE/DELETE")

// Для операций пользователей
zap.Int64("user_id", userID)
zap.Int64("chat_id", chatID)
zap.Int("thread_id", threadID)  // если применимо
zap.String("username", username)

// Для команд бота
zap.String("command", "/addreaction")
zap.Strings("args", args)
zap.Bool("is_admin", isAdmin)
```

## Примеры использования

### Локальная разработка (видеть всё)
```bash
# .env
LOG_LEVEL=debug

# Логи покажут:
# DEBUG statistics: received message user_id=123 text="hello"
# DEBUG statistics: detected content type content_type=text
# DEBUG limiter: checking limits user_id=123
# INFO reaction added pattern="test"
```

### Production (только важное)
```bash
# .env
LOG_LEVEL=info

# Логи покажут:
# INFO starting bmft bot version=0.8.0
# INFO current schema version version=1
# INFO reaction added pattern="test"
# ERROR failed to add reaction error="..."
```

### Мониторинг ошибок (минимум шума)
```bash
# .env
LOG_LEVEL=error

# Логи покажут только:
# ERROR failed to connect to database error="..."
# ERROR failed to add reaction error="..."
```

## Быстрая смена уровня без пересборки

```bash
# Остановить бота
docker-compose -f docker-compose.bot.yaml down

# Изменить уровень в .env
echo "LOG_LEVEL=debug" >> .env

# Перезапустить (без --build)
docker-compose -f docker-compose.bot.yaml up -d

# Проверить логи
docker logs -f bmft_bot
```

## Проверочный список для PR

Перед коммитом нового кода проверьте:

- [ ] Все ошибки БД логируются через `logger.Error()`
- [ ] Критичные операции (добавление/удаление) логируются через `logger.Info()`
- [ ] Детали обработки логируются через `logger.Debug()`
- [ ] Используется structured logging (zap.Int64, zap.String, etc)
- [ ] Не используется fmt.Printf() или log.Println()
- [ ] При ошибках всегда присутствует zap.Error(err)
- [ ] ID пользователей и чатов всегда логируются (для расследования инцидентов)
