# BMFT —## 📖 О проекте

BMFT — модульный бот для управления Telegram-чатами с plugin-based архитектурой.

- 🧩 **Модульная архитектура** — независимые модули (лимитер, реакции, статистика и др.)
- 🎛️ **Per-chat управление** — админы включают/выключают модули командами
- 🗄️ **PostgreSQL** — единая БД с партиционированием
- 📡 **Long Polling** — без webhook и публичного IP
- 🐳 **Docker-ready** — простое развертывание

> 🟡 **Статус:** Alpha. Активная разработка.

## 🏗️ Схема компонентов

```
┌─────────────────────────────┐
│        Telegram API         │
└─────────────┬───────────────┘
          │ Long Polling
          ▼
      ┌─────────────────────┐
      │   Bot (telebot.v3)  │
      └─────────┬───────────┘
        │
        ▼
      ┌─────────────────────────────┐
      │      Module Registry        │◄──── chat_modules (config)
      └─────────┬───────────┬───────┘
        │           │
   ┌────────────┘           └────────────┐
   ▼                                    ▼
┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
│ Limiter  │ │ Reactions│ │Statistics│ │Scheduler │ │Welcome   │
└────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘ └────┬─────┘
     └────────────┴────────────┴────────────┴─────────────┘
              │
              ▼
        ┌─────────────────────┐
        │    PostgreSQL 16+   │
        └─────────────────────┘
```
## 🚀 Быстрый старт

### Требования
- Docker и Docker Compose
- Telegram Bot Token от [@BotFather](https://t.me/BotFather)

### Установка
```bash
# 1. Клонируйте и настройте
git clone <repository-url>
cd bmft
cp .env.example .env  # Укажите TELEGRAM_BOT_TOKEN

# 2. Запустите
docker-compose -f docker-compose.env.yaml up -d  # PostgreSQL
docker-compose -f docker-compose.bot.yaml up -d  # Бот

# 3. Проверьте
docker logs -f bmft_bot
```

## 📚 Документация

Подробная информация в [docs/](docs/):
- [Быстрый старт](docs/QUICK_START.md)
- [Архитектура](docs/ARCHITECTURE.md)
- [Модули](docs/MODULES.md)
- [Тестирование](docs/TESTING_SHORT.md)

История изменений: [CHANGELOG.md](CHANGELOG.md)

## 📧 Контакты

- GitHub: [@flybasist](https://github.com/flybasist)
- Telegram: [@flybasist](https://t.me/flybasist)
- Email: flybasist92@gmail.com

## 📄 Лицензия

Этот проект распространяется под лицензией GNU GPLv3.

Вы можете использовать, модифицировать и распространять этот код, при условии, что производные работы также будут открыты под лицензией GPLv3. Это означает, что если вы вносите изменения и распространяете модифицированную версию, вы обязаны предоставить исходный код этих изменений.

В случае использования кода внутри организации без его распространения — раскрытие изменений не требуется.

Автор: Alexander Ognev (aka FlyBasist)
Год: 2025

⭐ Если проект оказался полезен — поставьте звезду на GitHub! ⭐

🇺🇸 English

This project is licensed under the GNU GPLv3.

You are free to use, modify, and distribute this code under the condition that any derivative works are also licensed under GPLv3. This means if you make changes and distribute your modified version, you must make the source code of those changes available.

If you use the code within your organization without distributing it externally, you are not required to disclose your modifications.

Author: Alexander Ognev (aka FlyBasist), 2025