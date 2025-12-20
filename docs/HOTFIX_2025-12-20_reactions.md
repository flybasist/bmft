# Hotfix: Исправление реакций и парсинга аргументов

## Проблемы которые исправлены:

### 1. ❌ Отсутствие таблиц `reaction_triggers` и `reaction_daily_counters`
**Ошибка:**
```
{"level":"error","caller":"reactions/reactions.go:244","msg":"failed to record trigger","error":"pq: relation \"reaction_triggers\" does not exist"}
```

**Причина:** В миграции 001 была materialized view вместо таблиц

**Решение:** Создана миграция 002 с правильными таблицами

### 2. ❌ Неправильный парсинг команд с кавычками
**Команда:**
```
/addreaction user:303724504 "" "@Astrolux, опять ты что то спылесосил!" "Пасхалка" photo 86400
```

**Парсилось как:**
```
args: ["user:303724504", "\"\"", "\"@Astrolux,", "опять", "ты", "что", "то", "спылесосил!\"", ...]
```

**Должно парситься:**
```
args: ["user:303724504", "", "@Astrolux, опять ты что то спылесосил!", "Пасхалка", "photo", "86400"]
```

**Решение:** Добавлена функция `parseQuotedArgs()` с правильной обработкой кавычек

### 3. ❌ Реакция срабатывала на КАЖДОЕ сообщение пользователя
**Причина:** Из-за неправильного парсинга pattern был пустой (""), что соответствовало любому сообщению

**Результат:** После фикса парсинга pattern будет правильным

## Применение на production

### Шаг 1: Остановить бота
```bash
ssh root@10.60.10.8
cd /opt/bmft
docker-compose -f docker-compose.bot.yaml down
```

### Шаг 2: Пересоздать БД с обновлённой схемой
```bash
# ВНИМАНИЕ: Удаляет все данные! Только для тестового окружения
docker-compose -f docker-compose.env.yaml down -v
docker-compose -f docker-compose.env.yaml up -d

# Подождать пока PostgreSQL запустится
sleep 5
```

### Шаг 3: Пересобрать и запустить бота
```bash
cd /opt/bmft
git pull

# Пересобрать образ (важно - изменился код парсинга!)
docker-compose -f docker-compose.bot.yaml build --no-cache

# Запустить
docker-compose -f docker-compose.bot.yaml up -d

# Проверить логи
docker logs -f bmft_bot
```

### Шаг 4: Удалить некорректные реакции
```bash
# Подключиться к БД
docker exec -it bmft-postgres psql -U bmft -d bmft

# Посмотреть реакции с пустым pattern (они срабатывают на всё)
SELECT id, chat_id, user_id, pattern, response_content, description 
FROM keyword_reactions 
WHERE pattern = '';

# Удалить некорректные реакции
DELETE FROM keyword_reactions WHERE pattern = '';

# Проверить что осталось
SELECT id, chat_id, user_id, pattern, response_type, response_content, description 
FROM keyword_reactions 
ORDER BY id DESC 
LIMIT 10;

\q
```

### Шаг 5: Протестировать
```
/addreaction user:303724504 "" "@Astrolux, опять ты что то спылесосил!" "Пасхалка" photo 86400
```

Теперь должно парситься правильно:
- pattern: "" (пустой - сработает только на фото от Astrolux)
- response_content: "@Astrolux, опять ты что то спылесосил!"
- description: "Пасхалка"
- trigger_content_type: "photo"

## Изменённые файлы:

1. ✅ [migrations/001_initial_schema.sql](../migrations/001_initial_schema.sql)
   - Убран materialized view `daily_reaction_stats`
   - Добавлены таблицы `reaction_triggers` и `reaction_daily_counters`

2. ✅ [migrations/002_add_reaction_tracking_tables.sql](../migrations/002_add_reaction_tracking_tables.sql)
   - Новая миграция для production (применяется без пересоздания БД)

3. ✅ [internal/migrations/migrations.go](../internal/migrations/migrations.go)
   - `LatestSchemaVersion = 2`
   - Добавлены `reaction_triggers` и `reaction_daily_counters` в ExpectedSchema

4. ✅ [internal/modules/reactions/reactions.go](../internal/modules/reactions/reactions.go)
   - Добавлена функция `parseQuotedArgs()` с правильной обработкой кавычек
   - Заменён `c.Args()` на `parseQuotedArgs(c.Text())`

## Проверка после применения:

```bash
# Логи должны быть без ошибок "relation does not exist"
docker logs bmft_bot | grep "failed to record trigger"

# Не должно быть ничего

# Проверить что реакции работают
# В телеграме отправить:
# /addreaction "тест" "работает"
# тест

# Бот должен ответить "работает"
```

## Rollback (если что-то пошло не так):

```bash
# Откатить код
cd /opt/bmft
git checkout HEAD~1

# Пересоздать БД со старой схемой
docker-compose -f docker-compose.env.yaml down -v
docker-compose -f docker-compose.env.yaml up -d

# Перезапустить бота
docker-compose -f docker-compose.bot.yaml build
docker-compose -f docker-compose.bot.yaml up -d
```
