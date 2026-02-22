# Документация BMFT

Bot Moderator For Telegram — система модерации и автоматизации для Telegram групп.

## 🚀 Быстрый старт

**Используете бот впервые?** → [QUICKSTART.md](QUICKSTART.md)

**Хотите посмотреть команды?** → [COMMANDS_ACCESS.md](COMMANDS_ACCESS.md)

---

## 📖 Основная документация

### Для пользователей

- **[QUICKSTART.md](QUICKSTART.md)** — установка и первичная настройка (self-hosted или готовый бот)
- **[COMMANDS_ACCESS.md](COMMANDS_ACCESS.md)** — все команды с примерами использования

### Для администраторов

- **[ROTATION.md](ROTATION.md)** — настройка ротации логов и данных в БД
- **[modules/MODULES.md](modules/MODULES.md)** — описание всех 5 модулей и их работы

### Для разработчиков

- **[ARCHITECTURE.md](ARCHITECTURE.md)** — архитектура проекта
- **[architecture/DATABASE.md](architecture/DATABASE.md)** — схема БД, партиционирование, индексы
- **[architecture/LOGGING.md](architecture/LOGGING.md)** — система логирования (уровни, structured logging)

---

## 🗂️ Структура документации

```
docs/
├── README.md                    ← Вы здесь
├── QUICKSTART.md                ← Начните отсюда
├── COMMANDS_ACCESS.md           ← Справочник команд
├── ROTATION.md                  ← Настройка ротации
├── ARCHITECTURE.md              ← Архитектура проекта
├── BMFT_PRESENTATION.md         ← Презентация проекта
│
├── architecture/                ← Техническая документация
│   ├── DATABASE.md              ← Схема БД
│   └── LOGGING.md               ← Система логирования
│
└── modules/                     ← Описание модулей
    └── MODULES.md               ← Все 5 модулей
```

---

## 🔧 Модули бота

Pipeline обработки сообщений: `statistics → limiter → reactions`

| # | Модуль | Описание |
|---|--------|----------|
| 1 | **Statistics** | Сбор статистики активности (всегда активен) |
| 2 | **Limiter** | Лимиты на типы контента с VIP-обходом |
| 3 | **Reactions** | Автоответы, фильтр запрещённых слов, фильтр мата |
| 4 | **Scheduler** | Задачи по расписанию (cron) |
| 5 | **Maintenance** | Фоновое обслуживание БД (партиции, ротация) |

Модуль **Reactions** объединяет: автоответы на ключевые слова, фильтр запрещённых слов (`/addban`), фильтр ненормативной лексики (`/setprofanity`).

---

## 💡 Принципы работы

### Модульность
- Каждый модуль независим
- Активация через наличие конфигурации в БД
- Нет глобального enable/disable

### Топики (Telegram Forums)
- Все модули поддерживают topics
- Fallback логика: топик → чат → дефолт
- Независимая настройка для каждого топика

### База данных
- PostgreSQL 16+ с партиционированием
- `messages` — единый источник правды
- JSONB metadata для гибкости

---

## 🔗 Полезные ссылки

- **GitHub:** [github.com/flybasist/bmft](https://github.com/flybasist/bmft)
- **Готовый бот:** [@bmft_bot](https://t.me/bmft_bot)
- **Миграции:** [../migrations/](../migrations/)
