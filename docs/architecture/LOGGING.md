# Система логирования BMFT

## Стек

- **zap** — structured logging (JSON в проде, console в dev)
- **lumberjack** — автоматическая ротация файлов логов

## Конфигурация (.env)

| Переменная | По умолчанию | Описание |
|-----------|-------------|----------|
| `LOG_LEVEL` | `info` | Уровень: debug, info, warn, error |
| `LOGGER_PRETTY` | `false` | `true` = console формат (для разработки) |
| `LOG_MAX_SIZE_MB` | `100` | Максимальный размер файла лога |
| `LOG_MAX_BACKUPS` | `3` | Количество старых файлов |
| `LOG_MAX_AGE_DAYS` | `28` | Срок хранения файлов логов |

## Вывод

Логи пишутся одновременно в два места:
- **stdout** — для `docker logs`
- **logs/bot.log** — файл с ротацией

## Формат

### Production (LOGGER_PRETTY=false)
```json
{"level":"info","ts":"2025-07-06T12:00:00.000+0300","caller":"bot/main.go:42","msg":"bot created successfully","bot_username":"bmft_bot"}
```

### Development (LOGGER_PRETTY=true)
```
2025-07-06T12:00:00.000+0300  INFO  bot/main.go:42  bot created successfully  {"bot_username": "bmft_bot"}
```

## Уровни

| Уровень | Когда используется |
|---------|-------------------|
| `debug` | Детальная информация (валидация таблиц, пропуск сообщений) |
| `info` | Основные события (старт, shutdown, команды, pipeline) |
| `warn` | Некритичные ошибки (fallback timezone, пропуск миграции) |
| `error` | Ошибки (БД, удаление сообщений, panic recovery) |

## Язык

Все лог-сообщения на **английском** языке. Комментарии в коде на **русском**.

## Ротация

При достижении `LOG_MAX_SIZE_MB` файл `logs/bot.log` ротируется:
```
logs/bot.log           ← текущий
logs/bot-2025-07-06.log.gz  ← предыдущий (сжат)
```

Подробнее о ротации данных: [../ROTATION.md](../ROTATION.md)
