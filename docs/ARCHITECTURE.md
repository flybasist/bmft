# Архитектура BMFT

## Стек технологий

| Компонент | Технология |
|-----------|-----------|
| Язык | Go 1.25.5 |
| Telegram API | telebot.v3 (Long Polling) |
| БД | PostgreSQL 16+ (партиционирование, JSONB) |
| Логирование | zap + lumberjack (ротация файлов) |
| Cron | robfig/cron/v3 |
| Конфигурация | .env (godotenv) |
| Деплой | Docker (multi-stage build) |

## Структура проекта

```
bmft/
├── cmd/bot/                     # Точка входа
│   ├── main.go                  # Инициализация, graceful shutdown
│   ├── modules.go               # Создание модулей и pipeline
│   └── handlers.go              # /start, /help, /version
│
├── internal/
│   ├── config/                  # Загрузка конфигурации из .env
│   ├── core/                    # Общие типы и утилиты
│   │   ├── interface.go         # MessageContext (контекст pipeline)
│   │   ├── helpers.go           # GetThreadID, DetectContentType
│   │   ├── middleware.go        # LoggerMiddleware, PanicRecovery
│   │   └── admin_check.go      # IsUserAdmin
│   ├── logx/                    # Настройка zap + lumberjack
│   ├── migrations/              # Автоматические миграции БД
│   ├── profanity/               # Загрузчик словаря мата (embedded)
│   ├── modules/
│   │   ├── statistics/          # Модуль статистики
│   │   ├── limiter/             # Модуль лимитов
│   │   ├── reactions/           # Модуль реакций + фильтры
│   │   ├── scheduler/           # Модуль планировщика
│   │   └── maintenance/         # Модуль обслуживания БД
│   └── postgresql/
│       ├── postgresql.go        # PingWithRetry
│       └── repositories/        # Репозитории (chat, message, vip, etc.)
│
├── migrations/                  # SQL-файлы миграций
├── config/postgres/             # pg_hba.conf для Docker
├── scripts/                     # Вспомогательные скрипты
├── docs/                        # Документация
└── logs/                        # Логи (создаётся автоматически)
```

## Pipeline обработки сообщений

```
Telegram → Long Polling → telebot.v3
                              │
                     LoggerMiddleware
                     PanicRecovery
                              │
                     ┌────────┴────────┐
                     │   statistics    │  ← записывает в messages
                     ├─────────────────┤
                     │    limiter      │  ← проверяет лимиты, может удалить
                     ├─────────────────┤
                     │   reactions     │  ← мат → бан-слова → автоответы
                     └─────────────────┘
```

Каждый модуль получает `*core.MessageContext` и может:
- Читать/анализировать сообщение
- Отвечать или удалять сообщение
- Установить `MessageDeleted = true` — сообщение удалено, следующие модули скорректируют поведение

## Жизненный цикл бота

1. Загрузка конфигурации из `.env`
2. Инициализация логгера (zap + lumberjack)
3. Подключение к PostgreSQL с ретраями
4. Применение миграций (если требуется)
5. Загрузка словаря мата в БД (если пуст)
6. Создание модулей и регистрация команд
7. Запуск pipeline (bot.Use)
8. Запуск Scheduler и Maintenance (cron)
9. Запуск HTTP health-сервера (/healthz)
10. Long Polling → обработка сообщений
11. Graceful shutdown по SIGINT/SIGTERM

## Graceful Shutdown

При получении SIGINT/SIGTERM:
1. Остановка health-сервера
2. Остановка telebot (bot.Stop)
3. Остановка Scheduler и Maintenance (cron.Stop)
4. Закрытие соединения с БД
5. Таймаут: `SHUTDOWN_TIMEOUT` (по умолчанию 15s)
