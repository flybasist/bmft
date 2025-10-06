# BMFT — Bot Moderator Framework for Telegram

**Модульный бот для управления Telegram-чатами на Go**

[![Go Version](https://img.shields.io/badge/Go-1.25.1+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16+-316192?style=flat&logo=postgresql)](https://www.postgresql.org/)
[![License](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Status](https://img.shields.io/badge/Status-Alpha-yellow.svg)](https://github.com)

---

## 📖 О проекте

**BMFT** (Bot Moderator For Telegram) — модульный бот для управления Telegram-чатами на Go с plugin-based архитектурой.

> 🟡 **Статус:** Alpha  
> Активная разработка и тестирование новых функций.  
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

| **Модуль** | **Статус** | **Описание** |
|:-----------|:-----------|:-------------|
| Core       | ✅ Done    | Registry, Config, Logging |
| Welcome    | ✅ Done    | Приветствия, /start, /help |
| Limiter    | ✅ Done    | Content Limits (12 types, VIP bypass) |
| Reactions  | ✅ Done    | Keyword Triggers, Regex, Cooldown |
| TextFilter | ✅ Done    | Banned Words Filter |
| Statistics | ✅ Done    | Chat Stats, Message Counts |
| Scheduler  | ✅ Done    | Cron Jobs from DB |
| AdminTools | 📋 Planned | Advanced Admin Management |
| AntiSpam   | 📋 Planned | Flood Protection |

**Легенда:**  
✅ Готово к использованию | 🔄 В процессе рефакторинга | 🔮 Запланировано

> ⚠️ **Важно:** Модули с статусом 🔄 работают, но активно дорабатываются и улучшаются.  
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

#### VIP Management
| Команда | Описание |
|:--------|:---------|
| `/setvip @user [reason]` | Назначить VIP статус (обход всех лимитов) |
| `/removevip @user` | Снять VIP статус |
| `/listvips` | Показать всех VIP пользователей |

#### Content Limits
| Команда | Описание |
|:--------|:---------|
| `/setlimit <type> <value> [@user]` | Установить лимит на тип контента |
| `/mystats` | Показать личную статистику и лимиты |

#### Keyword Reactions
| Команда | Описание |
|:--------|:---------|
| `/addreaction <pattern> <response> <desc>` | Добавить автореакцию на слово |
| `/listreactions` | Показать все реакции |
| `/removereaction <id>` | Удалить реакцию |

#### Text Filter (Banned Words)
| Команда | Описание |
|:--------|:---------|
| `/addban <pattern> <action>` | Добавить запрещенное слово |
| `/listbans` | Показать все запреты |
| `/removeban <id>` | Удалить запрет |

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
- **14 таблиц** — для хранения данных модулей (без дублирования)
- **3 партиции** — для эффективного хранения сообщений по месяцам

> ✅ **v0.6.0:** Структура БД оптимизирована, убраны дубли, все работает правильно.

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

**Цель:** Создание полнофункционального модульного бота для управления чатами.

---

## 📧 Контакты

- **GitHub:** [@flybasist](https://github.com/flybasist)
- **Telegram:** [@flybasist](https://t.me/flybasist)
- **Email:** flybasist92@gmail.com

Нашли баг? Есть идея? [Создайте issue!](https://github.com/flybasist/bmft/issues)

---

## �️ Лицензия

Этот проект распространяется под лицензией [GNU GPLv3](https://www.gnu.org/licenses/gpl-3.0.html).

Вы можете использовать, модифицировать и распространять этот код, при условии, что производные работы также будут открыты под лицензией GPLv3. Это означает, что если вы вносите изменения и распространяете модифицированную версию, вы обязаны предоставить исходный код этих изменений.

В случае использования кода **внутри организации** без его распространения — раскрытие изменений не требуется.

**Автор:** Alexander Ognev (aka FlyBasist)  
**Год:** 2025

---

**⭐ Если проект оказался полезен — поставьте звезду на GitHub! ⭐**

---

### 🇺🇸 English

This project is licensed under the [GNU GPLv3](https://www.gnu.org/licenses/gpl-3.0.html).

You are free to use, modify, and distribute this code under the condition that any derivative works are also licensed under GPLv3. This means if you make changes and distribute your modified version, you must make the source code of those changes available.

If you use the code **within your organization** without distributing it externally, you are not required to disclose your modifications.

**Author:** Alexander Ognev (aka FlyBasist)  
**Year:** 2025
