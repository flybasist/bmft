# Миграции базы данных

## 📋 Текущий подход (Development)

### Один файл = вся схема

```
migrations/
└── 001_initial_schema.sql  (~400 строк)
```

**Содержит:**
- ✅ Phase 1: Core Framework (chats, users, modules, event_log)
- ✅ Phase 2: Limiter Module (user_limits)
- ✅ Phase 3: Reactions Module (reactions_config, reactions_log)
- ✅ Phase 4-5: Statistics & Scheduler (готовые таблицы)

---

## 🚀 Как применить миграции

### Вариант 1: golang-migrate (рекомендуется)

```bash
# Установка (если ещё не установлен)
# macOS:
brew install golang-migrate

# Linux:
curl -L https://github.com/golang-migrate/migrate/releases/download/v4.17.0/migrate.linux-amd64.tar.gz | tar xvz
sudo mv migrate /usr/local/bin/

# Применить миграции
migrate -path migrations -database "postgres://bmft:secret@localhost:5432/bmft?sslmode=disable" up

# Откатить последнюю миграцию (если нужно)
migrate -path migrations -database "postgres://bmft:secret@localhost:5432/bmft?sslmode=disable" down 1

# Проверить версию схемы
migrate -path migrations -database "postgres://bmft:secret@localhost:5432/bmft?sslmode=disable" version
```

### Вариант 2: psql (ручной импорт)

```bash
# Подключись к PostgreSQL
docker exec -it bmft_postgres psql -U bmft -d bmft

# Импортируй схему
\i /docker-entrypoint-initdb.d/001_initial_schema.sql

# Или из командной строки:
docker exec -i bmft_postgres psql -U bmft -d bmft < migrations/001_initial_schema.sql
```

### Вариант 3: Автоматически через docker-entrypoint-initdb.d

**⚠️ Работает только при первом запуске контейнера!**

В `docker-compose.env.yaml` раскомментируй:
```yaml
volumes:
  - ./data/postgres:/var/lib/postgresql/data
  - ./migrations:/docker-entrypoint-initdb.d:ro  # ← Раскомментировать
```

При первом запуске PostgreSQL автоматически выполнит все `.sql` файлы из этой папки.

---

## 🔍 Проверка что миграции применились

```bash
# Подключись к БД
docker exec -it bmft_postgres psql -U bmft -d bmft

# Проверь список таблиц
bmft=# \dt

# Должно быть:
# chats, users, chat_admins, chat_modules, event_log
# user_limits
# reactions_config, reactions_log
# statistics_daily, statistics_monthly
# scheduler_tasks, scheduler_log

# Проверь структуру таблицы
bmft=# \d reactions_config

# Выход
bmft=# \q
```

---

## 🎯 Development workflow

### При первом запуске проекта:

1. Запусти PostgreSQL:
   ```bash
   docker-compose -f docker-compose.env.yaml up -d
   ```

2. Примени миграции:
   ```bash
   migrate -path migrations -database "postgres://bmft:secret@localhost:5432/bmft?sslmode=disable" up
   ```

3. Запусти бота:
   ```bash
   # Локально (для отладки):
   go run cmd/bot/main.go
   
   # Или в Docker:
   docker-compose -f docker-compose.bot.yaml up -d
   ```

### При добавлении новой таблицы (Phase 4+):

Пока проект в разработке (до v1.0.0) просто добавляй новые таблицы в `001_initial_schema.sql`.

**Для тестирования изменений:**
```bash
# Останови бота
docker-compose -f docker-compose.bot.yaml down

# Останови БД и удали данные (ВНИМАНИЕ: потеряешь все данные!)
docker-compose -f docker-compose.env.yaml down -v

# Или вручную очисти папку данных:
rm -rf data/postgres/*

# Запусти БД заново
docker-compose -f docker-compose.env.yaml up -d

# Применить миграции
migrate -path migrations -database "postgres://bmft:secret@localhost:5432/bmft?sslmode=disable" up

# Запусти бота
docker-compose -f docker-compose.bot.yaml up -d
```

---

## 📦 Production workflow (ПОСЛЕ v1.0.0)

После первого релиза будем использовать инкрементальные миграции:

```
migrations/
├── 001_initial_schema.sql        # Phase 1-3 (v0.3.0)
├── 002_add_statistics.sql        # Phase 4 (v0.4.0)
├── 003_add_scheduler.sql         # Phase 5 (v0.5.0)
└── 004_add_reaction_groups.sql   # Feature (v1.1.0)
```

### Защита от частичных миграций:

Если приложение обнаружит:
- ❌ Некоторые таблицы есть, но не все
- ❌ Таблица есть, но не хватает колонок
- ❌ Типы данных не совпадают

То выдаст ошибку:
```
FATAL: Database schema validation failed
Expected tables: [chats, users, chat_modules, ...]
Found: [chats, users]
Missing: [chat_modules, ...]

Please drop database and restart:
  docker-compose down -v
  docker-compose up -d
```

---

## 🔄 Для разработки (Development)

### Вайп и пересоздание БД:

```bash
# Остановить и удалить все данные
docker-compose down -v

# Запустить заново (БД будет создана автоматически)
docker-compose up -d

# Бот сам применит миграции при старте
./bin/bot
```

**Это безопасно для dev окружения!** Все данные тестовые.

---

## 📦 Для продакшена (Production) - позже

В будущем когда пойдём на прод, добавим:

1. **Версионирование миграций:**
   ```
   migrations/
   ├── v0.1.0_initial_schema.sql
   ├── v0.2.0_add_reactions.sql
   └── v0.3.0_add_statistics.sql
   ```

2. **Миграции без даунтайма:**
   - Добавление колонок с DEFAULT значениями
   - Создание новых таблиц без влияния на старые
   - Миграция данных в фоне

3. **Rollback механизм:**
   ```
   migrations/
   ├── up/
   │   └── 001_add_feature.sql
   └── down/
       └── 001_rollback_feature.sql
   ```

**Но это всё потом!** Пока мы в dev режиме - один файл миграции.

---

## ✅ Текущий статус

- ✅ Один файл `001_initial_schema.sql` содержит всё
- ✅ Phase 1-2 готовы
- ✅ Phase 3 готова (таблицы reactions уже есть)
- ✅ Phase 4-5 готовы (таблицы statistics, scheduler уже есть)

**Валидация схемы:** Будет добавлена в Phase 4

---

## 📖 Ссылки

- **Schema:** `migrations/001_initial_schema.sql`
- **Validator:** (будет создан в Phase 4)
- **Docker Compose:** `docker-compose.yaml`
