# Быстрый старт BMFT

## Выбор варианта развёртывания

### ⚡ Вариант 1: Готовый бот (рекомендуется)

**Самый простой способ — используйте [@bmft_bot](https://t.me/bmft_bot)**

✅ **Преимущества:**
- Никаких серверов и установок
- Бот уже работает и готов к использованию
- Автоматические обновления
- Техподдержка

❌ **Ограничения:**
- Данные хранятся на серверах автора
- Нельзя модифицировать код
- Премиум-функции платные (в разработке)

**Как начать:**
1. Откройте [@bmft_bot](https://t.me/bmft_bot)
2. Нажмите "Add to Group" и выберите чат
3. Выдайте боту права администратора (удаление сообщений)
4. Готово! Используйте `/help` в чате

---

### 🛠️ Вариант 2: Self-hosted

**Полный контроль — разверните собственную копию**

✅ **Преимущества:**
- Полный контроль над данными
- Можно модифицировать код
- Бесплатно навсегда
- Нет зависимости от третьих сторон

❌ **Требования:**
- Нужен сервер (VPS/dedicated)
- Базовые знания Docker
- Самостоятельная поддержка

---

## Self-hosted: Пошаговая инструкция

### Шаг 1: Требования

**Минимальные требования сервера:**
- 1 CPU core
- 512 MB RAM
- 5 GB диска
- Ubuntu 20.04+ / Debian 11+

**Программное обеспечение:**
```bash
# Установка Docker
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh

# Установка Docker Compose
sudo apt install docker-compose
```

**Telegram Bot Token:**
1. Откройте [@BotFather](https://t.me/BotFather)
2. Отправьте `/newbot`
3. Следуйте инструкциям
4. Сохраните полученный token

---

### Шаг 2: Клонирование и настройка

```bash
# Клонируйте репозиторий
git clone https://github.com/flybasist/bmft.git
cd bmft

# Создайте .env файл
cp .env.example .env
```

**Откройте `.env` и укажите токен:**
```bash
nano .env
```

**Минимальная конфигурация:**
```bash
# Обязательно
TELEGRAM_BOT_TOKEN=YOUR_BOT_TOKEN_HERE

# Для Docker деплоя (рекомендуется)
POSTGRES_DSN=postgres://bmft:bmft@postgres:5432/bmft?sslmode=disable
POSTGRES_USER=bmft
POSTGRES_PASSWORD=bmft
POSTGRES_DB=bmft
POSTGRES_PORT=5432

# Опционально (defaults работают)
LOG_LEVEL=info
LOGGER_PRETTY=false
DB_RETENTION_MONTHS=6
```

---

### Шаг 3: Запуск

**Вариант A: Бот + БД в Docker (рекомендуется)**

```bash
# Запустите PostgreSQL
docker-compose -f docker-compose.env.yaml up -d

# Подождите 5-10 секунд для инициализации БД
sleep 10

# Запустите бота
docker-compose -f docker-compose.bot.yaml up -d

# Проверьте логи
docker logs -f bmft_bot
```

**Ожидаемый вывод:**
```
{"level":"info","ts":"2025-11-17T12:00:00Z","msg":"starting bmft bot"}
{"level":"info","ts":"2025-11-17T12:00:01Z","msg":"connected to postgresql"}
{"level":"info","ts":"2025-11-17T12:00:01Z","msg":"starting scheduler module"}
{"level":"info","ts":"2025-11-17T12:00:01Z","msg":"starting maintenance module"}
{"level":"info","ts":"2025-11-17T12:00:02Z","msg":"bot started successfully","bot_username":"your_bot"}
```

---

**Вариант B: БД в Docker, бот локально (для разработки)**

```bash
# Запустите только PostgreSQL
docker-compose -f docker-compose.env.yaml up -d

# В .env измените POSTGRES_DSN:
# POSTGRES_DSN=postgres://bmft:bmft@localhost:5432/bmft?sslmode=disable

# Запустите бота локально
go run cmd/bot/main.go
```

---

### Шаг 4: Добавление бота в чат

1. Откройте ваш чат в Telegram
2. Добавьте бота через меню → "Add Members"
3. **Важно:** Выдайте боту права администратора:
   - ✅ Delete messages (обязательно!)
   - ✅ Ban users (опционально, для антиспама)

4. Проверьте работу: `/help`

---

### Шаг 5: Базовая настройка

**Проверка модулей:**
```
/statistics  — Справка по статистике
/limiter     — Справка по лимитам
/scheduler   — Справка по задачам
```

**Первоначальная конфигурация:**

```bash
# Установить лимит на стикеры (5 в день на пользователя)
/setlimit sticker 5

# Установить лимит на GIF (3 в день)
/setlimit animation 3

# Дать VIP админам (обход всех лимитов)
/setvip @admin_username

# Добавить автореакцию
/addreaction "привет" "Здарова! 👋"

# Создать задачу по расписанию (ежедневно в 09:00)
/addtask morning "0 9 * * *" text "Доброе утро! ☀️"
```

---

## Управление

### Просмотр логов

```bash
# Логи бота
docker logs -f bmft_bot

# Логи PostgreSQL
docker logs -f bmft_postgres

# Последние 100 строк
docker logs --tail 100 bmft_bot
```

### Перезапуск

```bash
# Перезапустить бота
docker-compose -f docker-compose.bot.yaml restart

# Перезапустить всё
docker-compose -f docker-compose.env.yaml restart
docker-compose -f docker-compose.bot.yaml restart
```

### Остановка

```bash
# Остановить бота
docker-compose -f docker-compose.bot.yaml down

# Остановить всё (БД сохранится)
docker-compose -f docker-compose.env.yaml down
docker-compose -f docker-compose.bot.yaml down

# Удалить ВСЁ включая данные
docker-compose -f docker-compose.env.yaml down -v
docker-compose -f docker-compose.bot.yaml down -v
```

### Обновление

```bash
# Получить новую версию
git pull

# Пересобрать образы
docker-compose -f docker-compose.bot.yaml build --no-cache

# Перезапустить
docker-compose -f docker-compose.bot.yaml up -d
```

---

## Бэкапы

### Автоматический бэкап БД

```bash
# Создать скрипт backup.sh
cat > backup.sh << 'EOF'
#!/bin/bash
DATE=$(date +%Y-%m-%d_%H-%M-%S)
docker exec bmft_postgres pg_dump -U bmft bmft > backup_$DATE.sql
gzip backup_$DATE.sql
# Удалить бэкапы старше 30 дней
find . -name "backup_*.sql.gz" -mtime +30 -delete
EOF

chmod +x backup.sh

# Добавить в cron (каждый день в 03:00)
crontab -e
# 0 3 * * * /path/to/backup.sh
```

### Восстановление из бэкапа

```bash
# Остановить бота
docker-compose -f docker-compose.bot.yaml down

# Восстановить БД
gunzip -c backup_2025-11-17.sql.gz | \
  docker exec -i bmft_postgres psql -U bmft bmft

# Запустить бота
docker-compose -f docker-compose.bot.yaml up -d
```

---

## Мониторинг

### Healthcheck

Бот предоставляет HTTP endpoint для мониторинга:

```bash
# Проверка здоровья (по умолчанию порт 9090)
curl http://localhost:9090/healthz

# Ожидается: {"status":"ok"}
```

**Настройка в .env:**
```bash
METRICS_ADDR=:9090
```

### Prometheus метрики (в разработке)

```bash
curl http://localhost:9090/metrics
```

---

## Troubleshooting

### Бот не отвечает на команды

**Проверка 1: Бот запущен?**
```bash
docker ps | grep bmft_bot
```

**Проверка 2: Логи ошибок?**
```bash
docker logs bmft_bot | grep -i error
```

**Проверка 3: Бот администратор?**
- Зайдите в настройки чата → Administrators
- Проверьте наличие бота в списке
- Убедитесь, что "Delete messages" включено

---

### Ошибка "failed to connect to postgres"

**Причина:** PostgreSQL ещё не запустился

**Решение:**
```bash
# Проверить статус PostgreSQL
docker ps | grep postgres

# Подождать 10 секунд и перезапустить бота
sleep 10
docker-compose -f docker-compose.bot.yaml restart
```

---

### Миграции не применяются

**Симптомы:**
```
ERROR: relation "messages" does not exist
```

**Решение:**
```bash
# Удалить БД и пересоздать
docker-compose -f docker-compose.env.yaml down -v
docker-compose -f docker-compose.env.yaml up -d
sleep 10
docker-compose -f docker-compose.bot.yaml up -d
```

---

### Диск заполняется

**Причина:** Логи или данные в БД

**Решение:**

```bash
# Проверить размер логов
du -sh logs/

# Уменьшить ротацию логов в .env
LOG_MAX_SIZE_MB=50
LOG_MAX_BACKUPS=2

# Уменьшить срок хранения данных
DB_RETENTION_MONTHS=3

# Перезапустить
docker-compose -f docker-compose.bot.yaml restart
```

---

## Следующие шаги

- 📖 [Документация по модулям](modules/MODULES.md)
- 🗄️ [Структура базы данных](architecture/DATABASE.md)
- 🔄 [Настройка ротации данных](ROTATION.md)
- 📋 [Список всех команд](COMMANDS_ACCESS.md)

---

**Нужна помощь?**
- 💬 Telegram: [@flybasist](https://t.me/flybasist)
- 📧 Email: flybasist92@gmail.com
- 🐙 GitHub Issues: [создать issue](https://github.com/flybasist/bmft/issues)
