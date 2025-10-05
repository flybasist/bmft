# BMFT — Bot Moderator Framework for Telegram

**Модульный бот для управления Telegram-чатами на Go**

[![Go Version](https://img.shields.io/badge/Go-1.25.1+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-316192?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Status](https://img.shields.io/badge/Status-Alpha-yellow.svg)](https://github.com)

---

## 📖 О проекте

**BMFT** (Bot Moderator For Telegram) — полноценный порт оригинального Python-бота [rts_bot](https://github.com/flybasist/rts_bot) на Go с улучшенной архитектурой и модульной системой.

> 🟡 **Статус:** Alpha  
> Идет активный рефакторинг для достижения полного паритета с Python версией.  
> Собираем фидбэк на тестовых чатах.

### ✨ Ключевые особенности

- 🧩 **Модульная архитектура** — каждая функция реализована как независимый модуль
- 🎛️ **Per-chat управление** — админы чатов сами решают, какие модули включать
- 🗄️ **PostgreSQL** — единая база данных для всех чатов с партиционированием
- 📡 **Long Polling** — работает без webhook и публичного IP-адреса
- 🐳 **Docker-ready** — простое развертывание через Docker Compose
- 🔄 **Автомиграции** — схема БД создается автоматически при первом запуске
- 🛑 **Graceful Shutdown** — корректное завершение работы всех модулей

---

## 🚀 Быстрый старт

### Требования

- **Docker** и **Docker Compose**
- **Telegram Bot Token** — получите у [@BotFather](https://t.me/BotFather)

### Установка за 5 шагов

```bash
# 1. Клонируйте репозиторий
git clone <repository-url>
cd bmft

# 2. Настройте конфигурацию
cp .env.example .env
nano .env  # Укажите TELEGRAM_BOT_TOKEN

# 3. Запустите PostgreSQL
docker-compose -f docker-compose.env.yaml up -d

# 4. Запустите бота (миграции применятся автоматически)
docker-compose -f docker-compose.bot.yaml up -d

# 5. Проверьте логи
docker logs -f bmft_bot
```

### Пример конфигурации (.env)

```bash
# === Обязательные параметры ===
TELEGRAM_BOT_TOKEN=123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11
POSTGRES_DSN=postgres://bmft:bmft@postgres:5432/bmft?sslmode=disable

# === Опциональные параметры ===
LOG_LEVEL=info              # Уровень логирования: debug, info, warn, error
LOGGER_PRETTY=false         # Человекочитаемые логи (для разработки)
SHUTDOWN_TIMEOUT=15s        # Таймаут graceful shutdown
POLLING_TIMEOUT=60          # Таймаут Long Polling (секунды)
```

---

## 📦 Модули

Каждый модуль можно включать/выключать через команду `/enable` или `/disable`.

| Модуль | Статус | Описание |
|:-------|:------:|:---------|
| **Core** | ✅ | Базовые команды: `/start`, `/help`, `/modules` |
| **Welcome** | ✅ | Приветствие новых участников, команда `/version` |
| **Limiter** | 🔄 | Лимиты на типы контента (фото, видео, стикеры и т.д.) |
| **Reactions** | 🔄 | Автоматические реакции на ключевые слова с regex |
| **Statistics** | 🔄 | Статистика сообщений по типам контента |
| **Scheduler** | 🔄 | Задачи по расписанию (cron-like) |

**Легенда:**  
✅ Готово к использованию | 🔄 В процессе рефакторинга | 🔮 Запланировано

> ⚠️ **Важно:** Модули с статусом 🔄 работают, но активно дорабатываются для достижения паритета с Python версией.  
> Детали изменений смотрите в [CHANGELOG.md](CHANGELOG.md).

---

## 🎮 Команды

### Для всех пользователей

| Команда | Описание |
|:--------|:---------|
| `/start` | Инициализация чата и приветствие |
| `/help` | Список доступных команд |
| `/version` | Информация о версии бота |

### Для администраторов чата

| Команда | Описание |
|:--------|:---------|
| `/modules` | Список модулей и их статус в текущем чате |
| `/enable <module>` | Включить модуль (например: `/enable limiter`) |
| `/disable <module>` | Выключить модуль |

> 📝 **Примечание:** Модуль-специфичные команды (настройка лимитов, реакций и т.д.) будут добавлены после завершения рефакторинга.

---

## 🏗️ Архитектура

### Схема компонентов

```
┌─────────────────┐
│  Telegram API   │
└────────┬────────┘
         │ Long Polling
         ▼
┌─────────────────┐
│ Bot (telebot.v3)│
└────────┬────────┘
         │
         ▼
┌─────────────────────────────┐
│     Module Registry         │◄──── chat_modules (config)
└────────┬────────────────────┘
         │
    ┌────┴────┬──────────┬──────────┬──────────┐
    ▼         ▼          ▼          ▼          ▼
┌────────┐ ┌──────┐ ┌────────┐ ┌────────┐ ┌────────┐
│Limiter │ │Reacts│ │  Stats │ │Schedule│ │Welcome │
└───┬────┘ └───┬──┘ └────┬───┘ └────┬───┘ └────┬───┘
    └──────────┴─────────┴──────────┴──────────┘
                         │
                         ▼
                 ┌───────────────┐
                 │  PostgreSQL   │
                 └───────────────┘
```

### Интерфейс модуля

Каждый модуль реализует стандартный интерфейс:

```go
type Module interface {
    Init(deps ModuleDependencies) error  // Инициализация при старте
    Name() string                        // Название модуля
    Description() string                 // Описание функционала
    Routes() []telebot.Route             // Обработчики команд
    Cleanup() error                      // Graceful shutdown
}
```

---

## 🗄️ База данных

**PostgreSQL 16** с автоматическими миграциями.

При первом запуске автоматически создается:
- **15 таблиц** — для хранения данных модулей
- **3 партиции** — для эффективного хранения сообщений по месяцам

> ⚠️ **Рефакторинг:** Структура БД активно дорабатывается для достижения паритета с Python версией.  
> Подробности в приватной документации `docs/`.

---

## 🔧 Разработка

### Локальная отладка

```bash
# 1. Запустите только PostgreSQL
docker-compose -f docker-compose.env.yaml up -d

# 2. Измените POSTGRES_DSN в .env на localhost
# Было:  postgres://bmft:bmft@postgres:5432/bmft
# Стало: postgres://bmft:bmft@localhost:5432/bmft

# 3. Запустите бота локально
go run cmd/bot/main.go
```

### Структура проекта

```
bmft/
├── cmd/
│   └── bot/              # 🚀 Точка входа приложения (main.go)
├── internal/
│   ├── config/           # ⚙️  Конфигурация из .env
│   ├── core/             # 🧠 Module Registry, интерфейсы
│   ├── db/               # 🗄️  Работа с PostgreSQL
│   ├── logx/             # 📝 Структурированное логирование (zap)
│   ├── migrations/       # 🔄 Автоматические миграции БД
│   └── modules/          # 📦 Все модули бота
│       ├── limiter/
│       ├── reactions/
│       ├── statistics/
│       └── scheduler/
├── migrations/           # 📄 SQL-файлы миграций
├── docker-compose.*.yaml # 🐳 Docker конфигурация
└── .env                  # 🔐 Переменные окружения
```

---

## 📝 История изменений

Подробная история изменений и известные проблемы:  
👉 [CHANGELOG.md](CHANGELOG.md)

---

## 🤝 Текущая разработка

**Активные задачи:**

- 🔄 **Limiter** — добавление per-type лимитов (стикеры, видео, аудио)
- 🔄 **Reactions** — поддержка regex-паттернов с cooldown-таймерами
- 🔄 **Statistics** — детальная статистика по типам контента
- 🔄 **Scheduler** — гибкий планировщик задач из БД

**Цель:** Достижение 100% паритета функционала с Python версией.

---

## 📧 Контакты

- **GitHub:** [@flybasist](https://github.com/flybasist)
- **Telegram:** [@flybasist](https://t.me/flybasist)
- **Email:** flybasist92@gmail.com

Нашли баг? Есть идея? [Создайте issue!](https://github.com/flybasist/bmft/issues)

---

## 📄 Лицензия

Проект распространяется под лицензией **GNU General Public License v3.0**.

Вы можете использовать, модифицировать и распространять код при условии,  
что производные работы также будут открыты под GPLv3.

Подробности: [LICENSE](LICENSE)

---

<div align="center">

**⭐ Если проект полезен — поставьте звезду на GitHub! ⭐**

</div>
