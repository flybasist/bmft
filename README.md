# BMFT - Bot Moderator For Telegram 

📖 **О проекте**

BMFT — модульный бот для управления Telegram-чатами с упрощённой архитектурой.

**🚀 Быстрый старт:** Добавьте [@bmft_bot](https://t.me/bmft_bot) в ваш чат прямо сейчас!

- 🧩 **Модульная архитектура** — независимые модули (лимитер, реакции, статистика и др.)
- 🎛️ **Per-chat управление** — админы настраивают модули через команды
- 🗄️ **PostgreSQL 16+** — единая БД с JSONB metadata и партиционированием
- 📡 **Long Polling** — без webhook и публичного IP
- 🐳 **Docker-ready** — простое развертывание для self-hosted

> **Версия:** 1.1.1  
> **Готовый бот:** [@bmft_bot](https://t.me/bmft_bot)  
> **Self-hosted:** Инструкции ниже

##  Быстрый старт

### ⚡ Вариант 1: Готовый бот (рекомендуется)

**Самый простой способ — используйте [@bmft_bot](https://t.me/bmft_bot)**

1. Откройте [@bmft_bot](https://t.me/bmft_bot)
2. Нажмите "Add to Group" и выберите чат
3. Выдайте боту права администратора (удаление сообщений)
4. Готово! Используйте `/help` в чате

**Никаких серверов, установок, конфигов — бот уже работает!**

---

### 🛠️ Вариант 2: Self-hosted

**Полный контроль — разверните собственную копию**

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

**Подробная документация:** [docs/README.md](docs/README.md)

## 💡 Основные возможности

### 5 модулей бота

| Модуль | Описание | Команды |
|--------|----------|---------|
| **Statistics** | Статистика активности | `/myweek`, `/chatstats` |
| **Limiter** | Лимиты на контент + VIP | `/setlimit`, `/setvip` |
| **Reactions** | Реакции, фильтры и модерация | `/addreaction`, `/addban`, `/setprofanity` |
| **Scheduler** | Задачи по расписанию | `/addtask` (cron) |
| **Maintenance** | Автоочистка данных | Работает в фоне |

Модуль **Reactions** объединяет: автоответы на ключевые слова, фильтр запрещённых слов, фильтр ненормативной лексики.

### Поддержка топиков

Все модули работают с Telegram Forums:
- `thread_id = 0` → настройка для всего чата
- `thread_id > 0` → настройка для топика
- Например: VIP только в #general, лимит GIF в #memes

### Автоматическая ротация

- ✅ Логи ротируются при достижении 100MB
- ✅ Старые данные в БД удаляются автоматически
- ✅ Настраивается через `.env`

---

## 📧 Контакты и поддержка

**Автор:** Alexander Ognev (FlyBasist)

- 🤖 **Готовый бот:** [@bmft_bot](https://t.me/bmft_bot)
- 💬 **Telegram:** [@flybasist](https://t.me/flybasist)
- 🐙 **GitHub:** [@flybasist](https://github.com/flybasist)
- 📧 **Email:** flybasist92@gmail.com

### 💰 Поддержка проекта

Если BMFT полезен для вас — поддержите развитие:

- 💳 **Финансовая поддержка** — свяжитесь в [@flybasist](https://t.me/flybasist)
- 🎯 **Спонсирование фич** — нужна конкретная функция? Обсудим!
- 🤝 **Коммерческое сотрудничество** — интеграции, кастомизация

**Приоритет в разработке получают спонсируемые функции!**

## � Лицензия

### 🤖 Использование готового бота

[@bmft_bot](https://t.me/bmft_bot) — бесплатный для базовых функций

- ✅ Все основные модули доступны бесплатно
- 💎 Премиум-модули (в разработке) — платная подписка
- 🎯 Спонсорство фич — индивидуальные условия

### 🔓 Open Source (GNU GPLv3)

Этот проект распространяется под лицензией GNU GPLv3.

**Вы можете:**
- ✅ Использовать код бесплатно
- ✅ Модифицировать под свои нужды
- ✅ Форкать и разворачивать self-hosted копию
- ✅ Использовать в коммерческих целях

**Условия:**
- ⚠️ Производные работы должны быть открыты под GPLv3
- ℹ️ Использование внутри компании без распространения — раскрытие изменений не требуется

---

**Автор:** Alexander Ognev (aka FlyBasist), 2025

⭐ **Если проект полезен:**
- Добавьте [@bmft_bot](https://t.me/bmft_bot) в ваш чат
- Поставьте звезду на [GitHub](https://github.com/flybasist/bmft)
- Поддержите развитие проекта финансово

---

🇺🇸 **English**

This project is licensed under the GNU GPLv3.

You are free to use, modify, and distribute this code under the condition that any derivative works are also licensed under GPLv3. This means if you make changes and distribute your modified version, you must make the source code of those changes available.

If you use the code within your organization without distributing it externally, you are not required to disclose your modifications.

Author: Alexander Ognev (aka FlyBasist), 2025