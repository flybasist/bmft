# BMFT — Quick Start Guide

**5 минут до первого запуска бота!**

## 🚀 Минимальная установка

```bash
# 1. Клонируйте репозиторий
git clone <your-repo-url>
cd bmft

# 2. Создайте .env файл
cat > .env << 'EOF'
TELEGRAM_BOT_TOKEN=YOUR_BOT_TOKEN_HERE
POSTGRES_DSN=postgres://bmft:secret@localhost:5432/bmft?sslmode=disable
LOG_LEVEL=debug
LOGGER_PRETTY=true
EOF

# 3. Запустите PostgreSQL
docker run -d --name bmft-postgres \
  -e POSTGRES_USER=bmft \
  -e POSTGRES_PASSWORD=secret \
  -e POSTGRES_DB=bmft \
  -p 5432:5432 \
  postgres:16

# 4. Примените миграции
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
migrate -path migrations -database "postgres://bmft:secret@localhost:5432/bmft?sslmode=disable" up

# 5. Запустите бота
go run cmd/bot/main.go
```

## 📝 Получение токена бота

1. Найдите [@BotFather](https://t.me/BotFather) в Telegram
2. Отправьте команду `/newbot`
3. Введите имя бота (например: "My Moderator Bot")
4. Введите username бота (должен заканчиваться на `bot`, например: `my_moderator_bot`)
5. Скопируйте токен (выглядит как `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`)
6. Вставьте токен в `.env` файл: `TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrsTUVwxyz`

## ✅ Проверка работы

1. Найдите вашего бота в Telegram по username
2. Отправьте команду `/start` — бот должен ответить
3. Добавьте бота в группу
4. Дайте боту права администратора (удаление сообщений, если нужен limiter)
5. Отправьте `/modules` — увидите список доступных модулей

## 🔧 Первая настройка

```
# В группе:
/modules                  # Посмотреть все модули
/enable limiter          # Включить модуль лимитов
/setlimit photo 10       # Установить лимит: 10 фото в день
/setlimit sticker 0      # Снять лимит на стикеры (безлимит)
/setlimit video -1       # Забанить видео полностью
/showlimits              # Посмотреть текущие лимиты

# Личные сообщения боту:
/start                   # Приветствие
/help                    # Помощь
```

## 🐛 Если что-то не работает

### Бот не отвечает на команды:

```bash
# Проверьте логи
docker logs bmft-bot -f

# Проверьте что PostgreSQL запущен
docker ps | grep postgres

# Проверьте что миграции применены
migrate -path migrations -database "$POSTGRES_DSN" version
```

### Ошибка "chat not found":

```sql
-- Подключитесь к PostgreSQL
docker exec -it bmft-postgres psql -U bmft -d bmft

-- Добавьте чат вручную
INSERT INTO chats (chat_id, chat_type, title) 
VALUES (-1001234567890, 'supergroup', 'My Group');

-- Включите модуль
INSERT INTO chat_modules (chat_id, module_name, is_enabled) 
VALUES (-1001234567890, 'limiter', true);
```

### Модуль не работает:

```sql
-- Проверьте что модуль включен
SELECT * FROM chat_modules WHERE chat_id = YOUR_CHAT_ID;

-- Включите модуль вручную
INSERT INTO chat_modules (chat_id, module_name, is_enabled) 
VALUES (YOUR_CHAT_ID, 'limiter', true)
ON CONFLICT (chat_id, module_name) 
DO UPDATE SET is_enabled = true;
```

## 📚 Что дальше?

- [README.md](README.md) — полная документация проекта
- [ARCHITECTURE.md](ARCHITECTURE.md) — как устроена модульная архитектура
- [MIGRATION_PLAN.md](MIGRATION_PLAN.md) — план миграции из Python-версии
- [migrations/001_initial_schema.sql](migrations/001_initial_schema.sql) — полная схема БД

## 🎯 Основные команды бота

| Команда | Описание | Где работает |
|---------|----------|--------------|
| `/start` | Приветствие | Везде |
| `/help` | Список команд | Везде |
| `/modules` | Доступные модули | Группы (admin) |
| `/enable <module>` | Включить модуль | Группы (admin) |
| `/disable <module>` | Выключить модуль | Группы (admin) |
| `/setlimit <type> <N>` | Установить лимит | Группы (admin) |
| `/showlimits` | Показать лимиты | Группы |
| `/mystats` | Моя статистика | Группы |
| `/statistics` | Статистика чата | Группы |

**Значения лимитов:**
- `-1` — контент забанен полностью
- `0` — безлимит (unlimited)
- `N` (>0) — разрешено N штук в день

**Типы контента для лимитов:**
- `photo`, `video`, `sticker`, `animation`, `voice`, `video_note`, `audio`, `document`

## 🔥 Быстрый тест

```bash
# 1. Запустите бота
go run cmd/bot/main.go

# 2. В другой вкладке терминала — проверьте БД
docker exec -it bmft-postgres psql -U bmft -d bmft

# 3. В psql проверьте что таблицы созданы
\dt

# Должны увидеть:
# chats, users, chat_modules, messages, limiter_config, reactions_config, etc.

# 4. Добавьте тестового пользователя в Telegram боту
# Отправьте /start боту

# 5. Проверьте что сообщение записалось
SELECT * FROM chats ORDER BY created_at DESC LIMIT 1;
```

---

**Готово!** Ваш бот запущен и готов к работе 🎉

Если возникли проблемы — см. полную документацию в [README.md](README.md)
