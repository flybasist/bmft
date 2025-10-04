# Phase 2: Limiter Module — Детальный План

**Дата начала:** 4 октября 2025  
**Ветка:** phase2-limiter-module  
**Статус:** 📋 Планирование  
**Предполагаемое время:** ~3 часа

---

## 🎯 Цель Phase 2

Создать модуль для контроля лимитов пользователей:
- Ограничение количества запросов к AI (GPT)
- Подсчёт использованных запросов
- Уведомления о достижении лимита
- Административные команды управления лимитами

---

## 📋 10 шагов реализации

### ✅ Шаг 1: Миграция БД — таблица лимитов (15 мин)
**Файл:** `migrations/003_create_limits_table.sql`

**Что делаем:**
```sql
CREATE TABLE user_limits (
    user_id BIGINT PRIMARY KEY,
    username VARCHAR(255),
    daily_limit INT NOT NULL DEFAULT 10,
    monthly_limit INT NOT NULL DEFAULT 300,
    daily_used INT NOT NULL DEFAULT 0,
    monthly_used INT NOT NULL DEFAULT 0,
    last_reset_daily TIMESTAMP NOT NULL DEFAULT NOW(),
    last_reset_monthly TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_user_limits_daily_reset ON user_limits(last_reset_daily);
CREATE INDEX idx_user_limits_monthly_reset ON user_limits(last_reset_monthly);
```

**Зачем:**
- `daily_limit` — дневной лимит запросов (по умолчанию 10)
- `monthly_limit` — месячный лимит запросов (по умолчанию 300)
- `daily_used` / `monthly_used` — сколько использовано
- `last_reset_*` — когда был последний сброс (для автоматического обнуления)

**Проверка:**
```bash
docker-compose exec db psql -U bmft -d bmft -c "\d user_limits"
```

---

### ✅ Шаг 2: Repository для лимитов (20 мин)
**Файл:** `internal/postgresql/repositories/limit_repository.go`

**Что делаем:**
```go
type LimitRepository struct {
    db *sql.DB
    logger *zap.Logger
}

func NewLimitRepository(db *sql.DB, logger *zap.Logger) *LimitRepository

// Методы:
func (r *LimitRepository) GetOrCreateLimit(userID int64, username string) (*UserLimit, error)
func (r *LimitRepository) CheckAndIncrement(userID int64) (bool, *LimitInfo, error)
func (r *LimitRepository) GetLimitInfo(userID int64) (*LimitInfo, error)
func (r *LimitRepository) SetDailyLimit(userID int64, limit int) error
func (r *LimitRepository) SetMonthlyLimit(userID int64, limit int) error
func (r *LimitRepository) ResetDailyIfNeeded(userID int64) error
func (r *LimitRepository) ResetMonthlyIfNeeded(userID int64) error
```

**Структуры:**
```go
type UserLimit struct {
    UserID           int64
    Username         string
    DailyLimit       int
    MonthlyLimit     int
    DailyUsed        int
    MonthlyUsed      int
    LastResetDaily   time.Time
    LastResetMonthly time.Time
}

type LimitInfo struct {
    DailyRemaining   int
    MonthlyRemaining int
    DailyUsed        int
    MonthlyUsed      int
    DailyLimit       int
    MonthlyLimit     int
}
```

**Проверка:**
- Unit-тесты для каждого метода

---

### ✅ Шаг 3: Интерфейс Module для Limiter (10 мин)
**Файл:** `internal/modules/limiter/limiter.go`

**Что делаем:**
```go
package limiter

import (
    "github.com/flybasist/bmft/internal/core"
    "github.com/flybasist/bmft/internal/postgresql/repositories"
    "go.uber.org/zap"
)

type LimiterModule struct {
    limitRepo *repositories.LimitRepository
    logger    *zap.Logger
}

func New(limitRepo *repositories.LimitRepository, logger *zap.Logger) *LimiterModule {
    return &LimiterModule{
        limitRepo: limitRepo,
        logger:    logger,
    }
}

// Реализация core.Module интерфейса
func (m *LimiterModule) Name() string {
    return "limiter"
}

func (m *LimiterModule) Init() error {
    m.logger.Info("limiter module initialized")
    return nil
}

func (m *LimiterModule) OnMessage(ctx *core.MessageContext) error {
    // Логика проверки лимита ТОЛЬКО если текст начинается с "GPT:"
    // или если модуль AI активен
    return nil
}

func (m *LimiterModule) Shutdown() error {
    m.logger.Info("limiter module shutdown")
    return nil
}
```

**Проверка:**
- Модуль компилируется
- Реализует интерфейс `core.Module`

---

### ✅ Шаг 4: Логика проверки лимита (20 мин)
**Файл:** `internal/modules/limiter/limiter.go` (расширение)

**Что делаем:**
```go
func (m *LimiterModule) OnMessage(ctx *core.MessageContext) error {
    msg := ctx.Message
    
    // Проверяем только если:
    // 1. Это личное сообщение (не группа)
    // 2. Или это сообщение с запросом к AI
    if !m.shouldCheckLimit(msg) {
        return nil
    }
    
    userID := msg.Sender.ID
    username := msg.Sender.Username
    
    // Проверяем и сбрасываем лимиты если нужно
    if err := m.limitRepo.ResetDailyIfNeeded(userID); err != nil {
        m.logger.Error("failed to reset daily limit", zap.Error(err))
    }
    if err := m.limitRepo.ResetMonthlyIfNeeded(userID); err != nil {
        m.logger.Error("failed to reset monthly limit", zap.Error(err))
    }
    
    // Проверяем лимит и инкрементируем
    allowed, info, err := m.limitRepo.CheckAndIncrement(userID)
    if err != nil {
        m.logger.Error("failed to check limit", zap.Error(err))
        return err
    }
    
    if !allowed {
        return m.sendLimitExceededMessage(ctx, info)
    }
    
    // Если осталось мало запросов — предупреждаем
    if info.DailyRemaining <= 2 || info.MonthlyRemaining <= 10 {
        m.sendLimitWarning(ctx, info)
    }
    
    return nil
}

func (m *LimiterModule) shouldCheckLimit(msg *telebot.Message) bool {
    // Проверяем только личные сообщения или сообщения с командой AI
    return msg.Private() || strings.HasPrefix(msg.Text, "GPT:")
}

func (m *LimiterModule) sendLimitExceededMessage(ctx *core.MessageContext, info *LimitInfo) error {
    text := fmt.Sprintf(
        "⛔️ Лимит исчерпан!\n\n" +
        "📊 Дневной лимит: %d/%d\n" +
        "📊 Месячный лимит: %d/%d\n\n" +
        "Попробуйте позже или обратитесь к администратору.",
        info.DailyUsed, info.DailyLimit,
        info.MonthlyUsed, info.MonthlyLimit,
    )
    
    return ctx.SendReply(text)
}

func (m *LimiterModule) sendLimitWarning(ctx *core.MessageContext, info *LimitInfo) {
    text := fmt.Sprintf(
        "⚠️ У вас осталось:\n" +
        "📊 Дневной: %d/%d запросов\n" +
        "📊 Месячный: %d/%d запросов",
        info.DailyRemaining, info.DailyLimit,
        info.MonthlyRemaining, info.MonthlyLimit,
    )
    
    ctx.SendReply(text)
}
```

**Проверка:**
- Модуль корректно блокирует при превышении лимита
- Отправляет предупреждения

---

### ✅ Шаг 5: Команда /limits (15 мин)
**Файл:** `internal/modules/limiter/commands.go`

**Что делаем:**
```go
package limiter

import (
    "fmt"
    "gopkg.in/telebot.v3"
)

func (m *LimiterModule) RegisterCommands(bot *telebot.Bot) {
    bot.Handle("/limits", m.handleLimitsCommand)
}

func (m *LimiterModule) handleLimitsCommand(c telebot.Context) error {
    userID := c.Sender().ID
    username := c.Sender().Username
    
    // Получаем информацию о лимитах
    info, err := m.limitRepo.GetLimitInfo(userID)
    if err != nil {
        m.logger.Error("failed to get limit info", zap.Error(err))
        return c.Send("❌ Не удалось получить информацию о лимитах")
    }
    
    text := fmt.Sprintf(
        "📊 Ваши лимиты:\n\n" +
        "🔵 Дневной лимит:\n" +
        "   Использовано: %d/%d\n" +
        "   Осталось: %d\n\n" +
        "🟢 Месячный лимит:\n" +
        "   Использовано: %d/%d\n" +
        "   Осталось: %d\n\n" +
        "💡 Лимиты обновляются автоматически каждый день/месяц.",
        info.DailyUsed, info.DailyLimit, info.DailyRemaining,
        info.MonthlyUsed, info.MonthlyLimit, info.MonthlyRemaining,
    )
    
    return c.Send(text)
}
```

**Интеграция в main.go:**
```go
// После создания бота регистрируем команды модуля
limiterModule.RegisterCommands(bot)
```

**Проверка:**
- Команда `/limits` показывает текущие лимиты

---

### ✅ Шаг 6: Админские команды (20 мин)
**Файл:** `internal/modules/limiter/admin_commands.go`

**Что делаем:**
```go
package limiter

import (
    "fmt"
    "strconv"
    "strings"
    "gopkg.in/telebot.v3"
)

var adminUsers = []int64{
    123456789, // Замени на свой Telegram ID
}

func (m *LimiterModule) isAdmin(userID int64) bool {
    for _, id := range adminUsers {
        if id == userID {
            return true
        }
    }
    return false
}

func (m *LimiterModule) RegisterAdminCommands(bot *telebot.Bot) {
    bot.Handle("/setlimit", m.handleSetLimitCommand)
    bot.Handle("/getlimit", m.handleGetLimitCommand)
}

// /setlimit <user_id> daily <limit>
// /setlimit <user_id> monthly <limit>
func (m *LimiterModule) handleSetLimitCommand(c telebot.Context) error {
    if !m.isAdmin(c.Sender().ID) {
        return c.Send("❌ Эта команда доступна только администраторам")
    }
    
    args := strings.Fields(c.Text())
    if len(args) != 4 {
        return c.Send("Использование: /setlimit <user_id> daily|monthly <limit>")
    }
    
    userID, err := strconv.ParseInt(args[1], 10, 64)
    if err != nil {
        return c.Send("❌ Неверный user_id")
    }
    
    limitType := args[2]
    limit, err := strconv.Atoi(args[3])
    if err != nil {
        return c.Send("❌ Неверный лимит")
    }
    
    switch limitType {
    case "daily":
        if err := m.limitRepo.SetDailyLimit(userID, limit); err != nil {
            m.logger.Error("failed to set daily limit", zap.Error(err))
            return c.Send("❌ Не удалось установить лимит")
        }
        return c.Send(fmt.Sprintf("✅ Дневной лимит для %d установлен: %d", userID, limit))
    
    case "monthly":
        if err := m.limitRepo.SetMonthlyLimit(userID, limit); err != nil {
            m.logger.Error("failed to set monthly limit", zap.Error(err))
            return c.Send("❌ Не удалось установить лимит")
        }
        return c.Send(fmt.Sprintf("✅ Месячный лимит для %d установлен: %d", userID, limit))
    
    default:
        return c.Send("❌ Тип лимита должен быть: daily или monthly")
    }
}

// /getlimit <user_id>
func (m *LimiterModule) handleGetLimitCommand(c telebot.Context) error {
    if !m.isAdmin(c.Sender().ID) {
        return c.Send("❌ Эта команда доступна только администраторам")
    }
    
    args := strings.Fields(c.Text())
    if len(args) != 2 {
        return c.Send("Использование: /getlimit <user_id>")
    }
    
    userID, err := strconv.ParseInt(args[1], 10, 64)
    if err != nil {
        return c.Send("❌ Неверный user_id")
    }
    
    info, err := m.limitRepo.GetLimitInfo(userID)
    if err != nil {
        m.logger.Error("failed to get limit info", zap.Error(err))
        return c.Send("❌ Не удалось получить информацию")
    }
    
    text := fmt.Sprintf(
        "📊 Лимиты пользователя %d:\n\n" +
        "🔵 Дневной: %d/%d (осталось %d)\n" +
        "🟢 Месячный: %d/%d (осталось %d)",
        userID,
        info.DailyUsed, info.DailyLimit, info.DailyRemaining,
        info.MonthlyUsed, info.MonthlyLimit, info.MonthlyRemaining,
    )
    
    return c.Send(text)
}
```

**Проверка:**
- Только админы могут использовать `/setlimit` и `/getlimit`
- Лимиты корректно обновляются

---

### ✅ Шаг 7: Интеграция модуля в main.go (10 мин)
**Файл:** `cmd/bot/main.go`

**Что делаем:**
```go
import (
    "github.com/flybasist/bmft/internal/modules/limiter"
    // ... остальные импорты
)

func run(ctx context.Context, logger *zap.Logger) error {
    // ... существующий код создания БД, бота, registry ...
    
    // Создаём репозиторий лимитов
    limitRepo := repositories.NewLimitRepository(db, logger)
    
    // Создаём модуль лимитов
    limiterModule := limiter.New(limitRepo, logger)
    
    // Регистрируем модуль
    if err := registry.Register(limiterModule); err != nil {
        logger.Fatal("failed to register limiter module", zap.Error(err))
    }
    
    // Регистрируем команды модуля
    limiterModule.RegisterCommands(bot)
    limiterModule.RegisterAdminCommands(bot)
    
    // Включаем модуль по умолчанию во всех чатах
    // (или делаем это через /enable limiter)
    
    // ... остальной код ...
}
```

**Проверка:**
- Бот компилируется
- Модуль появляется в списке `/modules`

---

### ✅ Шаг 8: Unit-тесты для LimitRepository (25 мин)
**Файл:** `internal/postgresql/repositories/limit_repository_test.go`

**Что делаем:**
```go
func TestGetOrCreateLimit(t *testing.T) { ... }
func TestCheckAndIncrement_Success(t *testing.T) { ... }
func TestCheckAndIncrement_DailyExceeded(t *testing.T) { ... }
func TestCheckAndIncrement_MonthlyExceeded(t *testing.T) { ... }
func TestSetDailyLimit(t *testing.T) { ... }
func TestSetMonthlyLimit(t *testing.T) { ... }
func TestResetDailyIfNeeded(t *testing.T) { ... }
func TestResetMonthlyIfNeeded(t *testing.T) { ... }
```

**Проверка:**
```bash
go test ./internal/postgresql/repositories/... -v
```

---

### ✅ Шаг 9: Обновление документации (15 мин)

**Файлы для обновления:**

1. **README.md** — добавить команды:
```markdown
### Команды лимитов

- `/limits` — Посмотреть свои лимиты запросов
- `/setlimit <user_id> daily <limit>` — (Админ) Установить дневной лимит
- `/setlimit <user_id> monthly <limit>` — (Админ) Установить месячный лимит
- `/getlimit <user_id>` — (Админ) Посмотреть лимиты пользователя
```

2. **docs/guides/QUICKSTART.md** — добавить раздел "Работа с лимитами"

3. **CHANGELOG.md** — новая версия:
```markdown
## [0.3.0] - 2025-10-04

### Added
- Модуль Limiter: контроль лимитов пользователей
- Таблица `user_limits` в PostgreSQL
- Команды: `/limits`, `/setlimit`, `/getlimit`
- Автоматический сброс дневных/месячных лимитов
- Уведомления о превышении лимита
```

---

### ✅ Шаг 10: Финальное тестирование (20 мин)

**Ручное тестирование:**

1. **Запуск бота:**
   ```bash
   docker-compose up --build
   ```

2. **Проверка команд:**
   - `/start` → Бот отвечает
   - `/modules` → Limiter в списке
   - `/limits` → Показывает лимиты (10/10 дневной, 300/300 месячный)
   - Отправить 11 сообщений подряд → На 11-м должна быть блокировка
   - `/setlimit <your_id> daily 5` → Админ устанавливает лимит 5
   - `/limits` → Проверяем изменение

3. **Проверка БД:**
   ```bash
   docker-compose exec db psql -U bmft -d bmft
   SELECT * FROM user_limits;
   ```

4. **Проверка логов:**
   ```bash
   docker-compose logs -f bot
   ```

5. **Проверка автосброса:**
   - Поменять `last_reset_daily` вручную на вчера
   - Отправить сообщение
   - Проверить что `daily_used` сбросился на 1

**Автоматическое тестирование:**
```bash
go test ./... -v
go build -o bin/bot ./cmd/bot
```

---

## 📊 Чеклист готовности

- [ ] Шаг 1: Миграция создана и применена
- [ ] Шаг 2: LimitRepository реализован
- [ ] Шаг 3: LimiterModule создан
- [ ] Шаг 4: Логика проверки работает
- [ ] Шаг 5: Команда /limits работает
- [ ] Шаг 6: Админские команды работают
- [ ] Шаг 7: Модуль интегрирован в main.go
- [ ] Шаг 8: Unit-тесты написаны и проходят
- [ ] Шаг 9: Документация обновлена
- [ ] Шаг 10: Ручное и автоматическое тестирование пройдено

---

## 🎯 Критерии успеха Phase 2

1. ✅ Бот контролирует лимиты пользователей
2. ✅ Лимиты автоматически сбрасываются ежедневно/ежемесячно
3. ✅ Пользователи получают уведомления о лимитах
4. ✅ Админы могут управлять лимитами через команды
5. ✅ Все тесты проходят
6. ✅ Документация актуальна
7. ✅ Проект готов к продакшену (можно запустить прямо сейчас)

---

## 🚀 После Phase 2

Следующий Phase 3: **AI Module (GPT Integration)**
- Интеграция с OpenAI API
- Контекстная память диалогов
- Система промптов
- Модерация контента

**Но это потом!** Сначала сделаем Phase 2 на 100%. 💪
