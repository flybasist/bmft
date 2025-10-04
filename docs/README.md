# 📚 Документация BMFT

**Добро пожаловать в документацию Bot Moderator Framework for Telegram!**

> 💡 **Совет:** Начни с [Быстрого старта](guides/QUICKSTART.md) если хочешь запустить бота за 5 минут.

---

## 🚀 Быстрый старт

**Новичок? Начни здесь:**

1. 📘 [**Быстрый старт**](guides/QUICKSTART.md) — Запуск бота за 5 минут
   - Как получить токен у @BotFather
   - Запуск PostgreSQL и миграций
   - Первые команды боту
   - Troubleshooting частых проблем

2. 🤖 [**Текущая функциональность**](guides/CURRENT_BOT_FUNCTIONALITY.md) — Что умеет бот сейчас
   - 5 базовых команд
   - Module Registry и архитектура
   - Примеры использования
   - Как протестировать

3. 📖 [**Главный README**](../README.md) — Полная документация проекта
   - Описание модульной архитектуры
   - Установка и конфигурация
   - Roadmap (Phase 1-7)

---

## 🏗️ Архитектура

**Хочешь понять как устроена система?**

- 🔧 [**Архитектура системы**](architecture/ARCHITECTURE.md) — Plugin-based модульная система
  - Module interface и lifecycle
  - Message routing и middleware
  - Примеры создания модулей
  - Dependency injection pattern

- �️ [**План миграции**](architecture/MIGRATION_PLAN.md) — 8-фазный план разработки
  - Phase 0: ✅ Analysis (завершено)
  - Phase 1: ✅ Core Framework (завершено)
  - Phase 2-7: Modules (планируется)
  - Итого: 15-20 дней до production

- 💬 [**FAQ**](FAQ.md) — Вопросы и ответы по архитектуре
  - Почему убрали Kafka?
  - Long Polling vs Webhook
  - Оптимизация схемы БД
  - Модульность vs монолит

---

## �‍💻 Для разработчиков

**Работаешь над проектом?**

### Phase 1 (✅ Завершён):
- 📊 [**Phase 1 Summary**](development/PHASE1_SUMMARY.md) — Полный отчёт
  - 8 коммитов, 2,457 строк кода
  - 10/10 шагов выполнено
  - Docker готов, тесты пройдены

- ✅ [**Phase 1 Checklist**](development/PHASE1_CHECKLIST.md) — Детальный чеклист
  - Все 10 шагов с описанием
  - Оценки времени и результаты

- 🧹 [**Cleanup Report**](development/CLEANUP_REPORT.md) — Отчёт по очистке кода
  - Удалено ~260 строк мёртвого кода
  - Все 9 пунктов качества соблюдены

- ✔️ [**Pre-Merge Checklist**](development/PRE_MERGE_CHECKLIST.md) — Проверка качества
  - Русские комментарии / английские логи
  - Код понятен новичку
  - Без неиспользуемых функций

### Phase 2 (⏳ В планах):
- � [**Переход к Phase 2**](development/PHASE1_TO_PHASE2_TRANSITION.md) — Подготовка
  - Limiter Module (лимиты на контент)
  - 10 шагов реализации
  - Оценка: 2-3 дня

---

## 🛠️ Troubleshooting

**Столкнулся с проблемой?**

- 🔧 [**VS Code Cache Fix**](guides/VSCODE_CACHE_FIX.md) — Решение проблем с кешем IDE
  - Ошибки "undefined" для удалённых файлов
  - Gopls restart и очистка кеша

---

## 📜 История изменений

- 📝 [**CHANGELOG.md**](CHANGELOG.md) — Все версии и изменения
  - v0.2.1 (2025-10-04): Phase 1 Complete ✅
  - v0.2.0 (2025-10-04): Documentation Phase
  - v0.1.0 (2025-08-25): Initial Kafka-based version

---

## 📦 Архив

**Устаревшие документы (для истории):**

- [Старый README](archive/README_OLD.md)
- [Project Summary v0.2.0](archive/PROJECT_SUMMARY.md)
   - Migration notes

---

## ⚙️ Конфигурация

**Настройка переменных окружения:**

1. 🔐 [**.env.example**](.env.example) — Шаблон конфигурации (50 строк)
   - Обязательные параметры (TELEGRAM_BOT_TOKEN, POSTGRES_DSN)
   - Опциональные параметры (LOG_LEVEL, LOGGER_PRETTY, etc.)
   - Production рекомендации
   - Примеры значений

---

## 🎯 Я хочу...

### ... запустить бота за 5 минут
→ [**QUICKSTART.md**](QUICKSTART.md)

### ... понять архитектуру
→ [**ARCHITECTURE.md**](ARCHITECTURE.md) + [**migrations/001_initial_schema.sql**](migrations/001_initial_schema.sql)

### ... создать новый модуль
→ [**ARCHITECTURE.md**](ARCHITECTURE.md) → "How to Create New Module"

### ... узнать почему убрали Kafka
→ [**ANSWERS.md**](ANSWERS.md) → Question 5

### ... начать разработку Phase 1
→ [**PHASE1_CHECKLIST.md**](PHASE1_CHECKLIST.md)

### ... мигрировать из Python-версии
→ [**MIGRATION_PLAN.md**](MIGRATION_PLAN.md)

### ... понять текущее состояние проекта
→ [**PROJECT_SUMMARY.md**](PROJECT_SUMMARY.md)

### ... увидеть все изменения
→ [**CHANGELOG.md**](CHANGELOG.md)

### ... настроить переменные окружения
→ [**.env.example**](.env.example)

### ... понять полную картину
→ [**README.md**](README.md)

---

## 📊 Статистика документации

| Файл | Строк | Размер | Назначение |
|------|-------|--------|------------|
| **PHASE1_CHECKLIST.md** | 811 | 14 KB | Детальный чеклист Phase 1 |
| **README.md** | 594 | 24 KB | Полная документация проекта |
| **ARCHITECTURE.md** | 591 | 20 KB | Архитектура модульной системы |
| **PROJECT_SUMMARY.md** | 410 | 11 KB | Обзор проекта и статистика |
| **ANSWERS.md** | 376 | 15 KB | Q&A по архитектурным решениям |
| **MIGRATION_PLAN.md** | 361 | 15 KB | 8-фазный план миграции |
| **migrations/001_initial_schema.sql** | 342 | 15 KB | PostgreSQL schema (14 таблиц) |
| **QUICKSTART.md** | 167 | 6.1 KB | 5-минутный гайд запуска |
| **CHANGELOG.md** | 102 | 4.3 KB | История изменений |
| **.env.example** | 50 | 1.6 KB | Шаблон конфигурации |
| **INDEX.md** (этот файл) | ~200 | ~8 KB | Навигация по документации |
| **Итого** | **~4,000** | **~135 KB** | |

---

## 🗂️ Структура проекта

```
bmft/
├── 📚 Documentation (эти файлы)
│   ├── INDEX.md                    ← ВЫ ЗДЕСЬ
│   ├── README.md
│   ├── QUICKSTART.md
│   ├── ARCHITECTURE.md
│   ├── MIGRATION_PLAN.md
│   ├── PHASE1_CHECKLIST.md
│   ├── PROJECT_SUMMARY.md
│   ├── ANSWERS.md
│   ├── CHANGELOG.md
│   └── .env.example
│
├── 🗄️ Database
│   └── migrations/
│       └── 001_initial_schema.sql
│
├── 💻 Source Code (будет создано в Phase 1)
│   ├── cmd/
│   │   └── bot/
│   │       └── main.go
│   └── internal/
│       ├── config/
│       ├── core/                   ← Module Registry + Interface
│       ├── modules/                ← Plugin modules (limiter, reactions, etc.)
│       ├── postgresql/
│       │   └── repositories/
│       ├── logx/
│       └── utils/
│
└── 🐳 Infrastructure
    ├── Dockerfile
    ├── docker-compose.yaml
    └── .gitignore
```

---

## 🔗 Связь между документами

```
Workflow для нового разработчика:

1. QUICKSTART.md
   └─> Запустил бота за 5 минут
       └─> README.md
           └─> Понял основы
               └─> ARCHITECTURE.md
                   └─> Понял архитектуру
                       └─> migrations/001_initial_schema.sql
                           └─> Изучил схему БД
                               └─> PHASE1_CHECKLIST.md
                                   └─> Начал разработку Phase 1

Workflow для миграции из Python:

1. MIGRATION_PLAN.md
   └─> Понял план (8 фаз)
       └─> ANSWERS.md (Q5)
           └─> Понял почему убрали Kafka
               └─> migrations/001_initial_schema.sql
                   └─> Понял новую схему БД
                       └─> PHASE1_CHECKLIST.md
                           └─> Начал реализацию

Workflow для создания модуля:

1. ARCHITECTURE.md → "How to Create New Module"
   └─> Понял Module interface
       └─> migrations/001_initial_schema.sql
           └─> Создал таблицы для модуля
               └─> PHASE2_CHECKLIST.md (будет создан)
                   └─> Реализовал модуль
```

---

## 🎯 Следующие шаги

### Сейчас (Phase 1: Core Framework)
→ Читай [**PHASE1_CHECKLIST.md**](PHASE1_CHECKLIST.md) и начинай работу

### После Phase 1 (Phase 2: Limiter Module)
→ Будет создан PHASE2_CHECKLIST.md с детальными шагами

### После Phase 2-7 (Phase 8: Production)
→ Миграция данных, CI/CD, мониторинг

---

## 💡 Tips

1. **Используй навигацию:** INDEX.md → найди нужный документ → читай
2. **Commit часто:** После каждого шага делай commit
3. **Читай комментарии:** Вся схема БД в migrations/001_initial_schema.sql с комментариями
4. **Задавай вопросы:** См. ANSWERS.md — возможно ответ уже есть
5. **Следуй чеклисту:** PHASE1_CHECKLIST.md разбит на 10 steps с time estimates

---

## 📞 Контакты

- **GitHub Issues:** [Create Issue](https://github.com/your-repo/bmft/issues)
- **Telegram:** @FlyBasist
- **Email:** your-email@example.com

---

## 📜 Лицензия

[GNU GPLv3](LICENSE) — см. LICENSE для деталей

---

**Версия:** 0.2.0 (Documentation Phase)  
**Дата:** 4 октября 2025  
**Автор:** Alexander Ognev (FlyBasist)

---

**⭐ Нашёл полезную инфу? Поставь звезду на GitHub!**

[← Назад к README](README.md) | [Начать Quick Start →](QUICKSTART.md)
