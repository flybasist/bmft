# BMFT — Bot Moderator Framework for Telegram

**Модульный бот для управления Telegram-чатами на Go.**

[![Go Version](https://img.shields.io/badge/Go-1.25.1+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-316192?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Status](https://img.shields.io/badge/Status-Alpha-yellow.svg)](https://github.com)

## 📖 О проекте

**BMFT** (Bot Moderator For Telegram) — это полноценный порт оригинального Python [rts_bot](https://github.com/flybasist/rts_bot) на Go с улучшенной архитектурой.

🟡 **Статус:** Alpha — идет активный рефакторинг после анализа оригинального бота. Собираем фидбэк на тестовых чатах.

### ✨ Ключевые особенности

- **Plugin Architecture** — каждая функция это отдельный модуль
- **Per-Chat Control** — админ чата управляет модулями через команды
- **PostgreSQL** — единая база для всех чатов
- **Long Polling** — не требует webhook/публичного IP
- **Docker Ready** — простое развертывание через Docker Compose
- **Auto Migrations** — схема БД создается автоматически при первом запуске
- **Graceful Shutdown** — корректная остановка при SIGINT/SIGTERM

## 🚀 Быстрый старт

### Требования

- Docker & Docker Compose
- Telegram Bot Token (получить у [@BotFather](https://t.me/BotFather))

### Установка

\`\`\`bash
# 1. Клонируйте репозиторий
git clone <repository-url>
cd bmft

# 2. Создайте конфигурацию
cp .env.example .env
nano .env  # Укажите TELEGRAM_BOT_TOKEN

# 3. Запустите PostgreSQL
docker-compose -f docker-compose.env.yaml up -d

# 4. Запустите бота (миграции выполнятся автоматически)
docker-compose -f docker-compose.bot.yaml up -d

# 5. Проверьте логи
docker logs -f bmft_bot
tail -f ./data/logs/bot.log
\`\`\`

### Конфигурация (.env)

\`\`\`bash
# Обязательные параметры
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
POSTGRES_DSN=postgres://bmft:bmft@postgres:5432/bmft?sslmode=disable

# Опциональные
LOG_LEVEL=info              # debug, info, warn, error
LOGGER_PRETTY=false         # true для dev
SHUTDOWN_TIMEOUT=15s
POLLING_TIMEOUT=60
\`\`\`

## 📦 Модули

| Модуль | Статус | Описание |
|--------|--------|----------|
| **Core** | ✅ | Базовые команды (/start, /help, /modules) |
| **Welcome** | ✅ | Приветствия, /version |
| **Limiter** | 🔄 | Лимиты на типы контента (рефакторится) |
| **Reactions** | 🔄 | Автореакции на слова (рефакторится) |
| **Statistics** | 🔄 | Статистика сообщений (рефакторится) |
| **Scheduler** | 🔄 | Задачи по расписанию (рефакторится) |

**Легенда:** ✅ Готово | 🔄 В разработке | 🔮 Планируется

⚠️ **Примечание:** Модули рефакторятся для достижения паритета с оригинальным Python ботом. См. [CHANGELOG.md](CHANGELOG.md).

## 🎮 Команды

### Для всех

- \`/start\` — инициализация чата
- \`/help\` — список команд
- \`/version\` — версия бота

### Для админов

- \`/modules\` — список модулей и статус
- \`/enable <module>\` — включить модуль
- \`/disable <module>\` — выключить модуль

*Модуль-специфичные команды будут добавлены после завершения рефакторинга.*

## 🏗️ Архитектура

\`\`\`
Telegram API
     │
     ▼
Bot (telebot.v3)
     │
     ▼
Module Registry ◄─── chat_modules (DB)
     │
     ├─► Limiter
     ├─► Reactions
     ├─► Statistics
     └─► Scheduler
          │
          ▼
      PostgreSQL
\`\`\`

Каждый модуль реализует интерфейс:

\`\`\`go
type Module interface {
    Init(deps ModuleDependencies) error
    Name() string
    Description() string
    Routes() []telebot.Route
    Cleanup() error
}
\`\`\`

## 🗄️ База данных

PostgreSQL 16 с автоматическими миграциями. При первом запуске создается полная схема (15 таблиц + 3 партиции).

⚠️ **Текущая структура активно рефакторится** для достижения паритета с Python версией (см. внутреннюю документацию).

## 🔧 Разработка

### Локальная отладка

\`\`\`bash
# Запустите только PostgreSQL
docker-compose -f docker-compose.env.yaml up -d

# Измените POSTGRES_DSN в .env
# postgres://bmft:secret@localhost:5432/bmft?sslmode=disable

# Запустите бота локально
go run cmd/bot/main.go
\`\`\`

### Структура проекта

\`\`\`
bmft/
├── cmd/bot/           # Точка входа
├── internal/
│   ├── config/        # Конфигурация
│   ├── core/          # Ядро бота
│   ├── db/            # База данных
│   ├── logx/          # Логирование
│   ├── migrations/    # Миграции
│   └── modules/       # Модули
├── migrations/        # SQL-файлы миграций
├── docker-compose.*.yaml
└── .env
\`\`\`

## 📝 Changelog

См. [CHANGELOG.md](CHANGELOG.md) для истории изменений и известных проблем.

## 🤝 Разработка

Текущие задачи:

- 🔄 Рефакторинг модуля Limiter (лимиты на типы контента)
- 🔄 Рефакторинг модуля Reactions (regex-реакции с кулдаунами)
- 🔄 Рефакторинг модуля Statistics (детальная статистика по типам)
- 🔄 Рефакторинг модуля Scheduler (гибкое расписание из БД)

## 📧 Контакты

- GitHub: [@flybasist](https://github.com/flybasist)
- Telegram: [@flybasist](https://t.me/flybasist)

## 📄 Лицензия

GNU General Public License v3.0 — см. [LICENSE](LICENSE)
