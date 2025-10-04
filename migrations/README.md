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

## 🚀 Как работает

### При первом запуске приложение:

1. **Проверяет наличие таблиц** в PostgreSQL
2. **Выполняет миграцию** `001_initial_schema.sql` если БД пустая
3. **Валидирует схему** - проверяет что все таблицы и колонки на месте
4. **Останавливается с ошибкой** если схема неполная или некорректная

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
