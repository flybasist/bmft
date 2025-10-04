# Phase 1: Core Framework — Implementation Checklist

**Цель:** Создать базу для модульной системы с telebot.v3 и Long Polling  
**Срок:** 2-3 дня  
**Статус:** 🟡 Not Started

---

## Прогресс выполнения

```
[✓] Шаг 1: Удалить Kafka инфраструктуру (30 мин) - COMPLETED
[✓] Шаг 2: Добавить зависимости (5 мин) - COMPLETED
[✓] Шаг 3: Создать core структуру (1-2 часа) - COMPLETED
[✓] Шаг 4: Обновить config (30 мин) - COMPLETED
[✓] Шаг 5: Реализовать бота с Long Polling (2-3 часа) - COMPLETED
[✓] Шаг 6: Database helpers (1 час) - COMPLETED
[✓] Шаг 6.1: Сборка проекта - COMPLETED (bin/bot, 10M)
[✓] Шаг 7: Тестирование - COMPLETED (config tests: 100% pass)
[✓] Шаг 8: Обновление документации - COMPLETED
[✓] Шаг 9: Docker setup - COMPLETED
[ ] Шаг 10: Финальная проверка (30 мин)
```

### Завершённые этапы:

**✅ Шаги 1-6 (Build Phase): ~70% Phase 1 завершено**
- Удалена вся Kafka инфраструктура (internal/kafkabot, internal/logger, docker-compose файлы)
- Добавлены telebot.v3 v3.3.8 и robfig/cron v3.0.1
- Создана core модульная система (interface.go, registry.go, middleware.go)
- Очищен config от Kafka переменных, добавлен PollingTimeout
- Реализован полноценный бот с 5 командами (/start, /help, /modules, /enable, /disable)
- Создан repository слой (ChatRepository, ModuleRepository, EventRepository)
- **Проект успешно компилируется**: `bin/bot` (10M)---

## 📦 Step 2: Add Dependencies (5 минут)

```bash
# Добавить telebot.v3
- [ ] go get gopkg.in/telebot.v3@latest

# Добавить cron (для будущего scheduler module)
- [ ] go get github.com/robfig/cron/v3@latest

# Добавить migrate (для миграций)
- [ ] go get -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Очистить неиспользуемые зависимости
- [ ] go mod tidy

# Проверить
- [ ] go list -m all | grep -E 'telebot|cron|migrate'
```

**Ожидаемый результат в go.mod:**
```
gopkg.in/telebot.v3 v3.x.x
github.com/robfig/cron/v3 v3.x.x
github.com/golang-migrate/migrate/v4 v4.x.x
```

---

## 🏗️ Step 3: Create Core Structure (1-2 часа)

### Create directories:
```bash
- [ ] mkdir -p internal/modules
- [ ] mkdir -p internal/core
```

### Create core files:

#### internal/core/interface.go (Module interface)
```go
- [ ] type Module interface (5 methods)
- [ ] type ModuleDependencies struct (DB, Bot, Logger, Config)
- [ ] type BotCommand struct (Command, Description)
```

#### internal/core/context.go (MessageContext)
```go
- [ ] type MessageContext struct
- [ ] func (ctx *MessageContext) SendReply(text string) error
- [ ] func (ctx *MessageContext) DeleteMessage() error
- [ ] func (ctx *MessageContext) LogEvent(eventType, details string) error
```

#### internal/core/registry.go (Module Registry)
```go
- [ ] type ModuleRegistry struct (modules map[string]Module)
- [ ] func NewRegistry() *ModuleRegistry
- [ ] func (r *ModuleRegistry) Register(name string, module Module)
- [ ] func (r *ModuleRegistry) InitAll(deps ModuleDependencies) error
- [ ] func (r *ModuleRegistry) OnMessage(ctx MessageContext) error
- [ ] func (r *ModuleRegistry) ShutdownAll() error
```

#### internal/core/middleware.go (Middleware layer)
```go
- [ ] func LoggerMiddleware(next telebot.HandlerFunc) telebot.HandlerFunc
- [ ] func PanicRecoveryMiddleware(next telebot.HandlerFunc) telebot.HandlerFunc
- [ ] func RateLimitMiddleware(next telebot.HandlerFunc) telebot.HandlerFunc
```

---

## ⚙️ Step 4: Update Config (30 минут)

### internal/config/config.go

**Remove:**
```go
- [ ] KAFKA_BROKERS
- [ ] KAFKA_GROUP_CORE
- [ ] KAFKA_GROUP_SEND
- [ ] KAFKA_GROUP_DELETE
- [ ] KAFKA_GROUP_LOGGER
- [ ] DLQ_TOPIC
- [ ] MAX_PROCESS_RETRIES
- [ ] LOG_TOPICS
- [ ] BATCH_INSERT_SIZE
- [ ] BATCH_INSERT_INTERVAL
```

**Add:**
```go
- [ ] POLLING_TIMEOUT (default: 60)
```

**Keep:**
```go
- [x] TELEGRAM_BOT_TOKEN
- [x] POSTGRES_DSN
- [x] LOG_LEVEL
- [x] LOGGER_PRETTY
- [x] SHUTDOWN_TIMEOUT
- [x] METRICS_ADDR
```

### Verify:
```bash
- [ ] go build ./internal/config
- [ ] grep -E 'KAFKA|DLQ|BATCH' internal/config/config.go (не должно быть)
```

---

## 🤖 Step 5: Implement Bot with Long Polling (2-3 часа)

### cmd/bot/main.go (новый файл вместо cmd/telegram_bot/main.go)

```go
- [ ] package main + imports
- [ ] func main() with signal handling (SIGINT/SIGTERM)
- [ ] Load config from .env
- [ ] Initialize logger (zap)
- [ ] Connect to PostgreSQL
- [ ] Create telebot.Bot with Long Polling
- [ ] Create Module Registry
- [ ] Initialize Module Dependencies (DB, Bot, Logger, Config)
- [ ] Register modules (пока пустой список)
- [ ] Call registry.InitAll(deps)
- [ ] Setup middleware (logger, panic recovery)
- [ ] Register basic commands (/start, /help, /modules)
- [ ] Start bot.Start() in goroutine
- [ ] Wait for shutdown signal
- [ ] Graceful shutdown: bot.Stop(), registry.ShutdownAll(), db.Close()
```

### Обработчики команд:

#### /start
```go
- [ ] Приветственное сообщение
- [ ] Сохранить chat_id в таблицу chats (if not exists)
- [ ] Отправить список основных команд
```

#### /help
```go
- [ ] Список всех доступных команд
- [ ] Сгруппировать по модулям (из registry.Modules())
```

#### /modules (только для админов группы)
```go
- [ ] Получить список всех модулей из registry
- [ ] Для каждого модуля показать статус (enabled/disabled для этого чата)
- [ ] Показать краткое описание
```

#### /enable <module> (только для админов)
```go
- [ ] Проверить права админа
- [ ] Проверить что модуль существует в registry
- [ ] Включить модуль: INSERT/UPDATE chat_modules SET is_enabled=true
- [ ] Отправить подтверждение
```

#### /disable <module> (только для админов)
```go
- [ ] Проверить права админа
- [ ] Проверить что модуль существует
- [ ] Отключить модуль: UPDATE chat_modules SET is_enabled=false
- [ ] Отправить подтверждение
```

---

## 🗄️ Step 6: Database Helpers (1 час)

### internal/postgresql/repositories/chat_repository.go (новый файл)

```go
- [ ] type ChatRepository struct { db *sql.DB }
- [ ] func NewChatRepository(db *sql.DB) *ChatRepository
- [ ] func (r *ChatRepository) GetOrCreate(chatID int64, chatType, title string) error
- [ ] func (r *ChatRepository) IsActive(chatID int64) (bool, error)
- [ ] func (r *ChatRepository) Deactivate(chatID int64) error
```

### internal/postgresql/repositories/module_repository.go (новый файл)

```go
- [ ] type ModuleRepository struct { db *sql.DB }
- [ ] func NewModuleRepository(db *sql.DB) *ModuleRepository
- [ ] func (r *ModuleRepository) IsEnabled(chatID int64, moduleName string) (bool, error)
- [ ] func (r *ModuleRepository) Enable(chatID int64, moduleName string) error
- [ ] func (r *ModuleRepository) Disable(chatID int64, moduleName string) error
- [ ] func (r *ModuleRepository) GetConfig(chatID int64, moduleName string) (map[string]interface{}, error)
- [ ] func (r *ModuleRepository) UpdateConfig(chatID int64, moduleName string, config map[string]interface{}) error
```

### internal/postgresql/repositories/event_repository.go (новый файл)

```go
- [ ] type EventRepository struct { db *sql.DB }
- [ ] func NewEventRepository(db *sql.DB) *EventRepository
- [ ] func (r *EventRepository) Log(chatID, userID int64, moduleName, eventType, details string) error
```

---

## 🧪 Step 7: Testing (1-2 часа)

### Unit tests:

```bash
# Core
- [ ] internal/core/registry_test.go (Register, InitAll, OnMessage)
- [ ] internal/core/context_test.go (SendReply, DeleteMessage, LogEvent)

# Config
- [ ] internal/config/config_test.go (Load from env vars)

# Repositories
- [ ] internal/postgresql/repositories/chat_repository_test.go
- [ ] internal/postgresql/repositories/module_repository_test.go
- [ ] internal/postgresql/repositories/event_repository_test.go
```

### Integration test:

```bash
- [ ] Test bot startup and shutdown
- [ ] Test /start command → chat created in DB
- [ ] Test /modules command → empty list (no modules yet)
- [ ] Test /enable limiter → error "module not found" (expected)
```

### Run tests:
```bash
- [ ] go test ./internal/core/...
- [ ] go test ./internal/config/...
- [ ] go test ./internal/postgresql/repositories/...
- [ ] go test ./...  (all tests pass)
```

---

## 📝 Step 8: Documentation Updates (30 минут)

### Update README.md:
```markdown
- [ ] Секция "Быстрый старт" → обновить команды (no Kafka)
- [ ] Секция "Конфигурация" → убрать KAFKA_* переменные
- [ ] Секция "Архитектура" → убрать диаграмму с Kafka
- [ ] Добавить примеры команд: /start, /modules, /enable
```

### Update CHANGELOG.md:
```markdown
- [ ] [0.2.1] - Phase 1 Complete
- [ ] Changed: Removed Kafka infrastructure
- [ ] Added: telebot.v3 with Long Polling
- [ ] Added: Core module system (Registry, Context, Interface)
- [ ] Added: Basic commands (/start, /help, /modules, /enable, /disable)
- [ ] Added: Repository layer for DB operations
```

---

## 🐳 Step 9: Docker Setup (1 час)

### Create new Dockerfile:
```bash
- [ ] Create Dockerfile (multi-stage build)
- [ ] Stage 1: build Go binary
- [ ] Stage 2: minimal runtime image (alpine)
```

### Create docker-compose.yaml:
```yaml
- [ ] Service: postgres (postgres:16)
- [ ] Service: bot (build from Dockerfile)
- [ ] Depends on: postgres
- [ ] Environment variables from .env
- [ ] Volumes: ./migrations:/migrations (for migrate tool)
- [ ] Health checks
```

### Test Docker setup:
```bash
- [ ] docker-compose build
- [ ] docker-compose up -d postgres
- [ ] Wait for postgres ready
- [ ] docker-compose run --rm bot migrate -path /migrations -database "$POSTGRES_DSN" up
- [ ] docker-compose up bot
- [ ] Test /start command
- [ ] docker-compose down
```

---

## ✅ Step 10: Verification (30 минут)

### Code quality:
```bash
- [ ] go vet ./...  (no issues)
- [ ] go fmt ./...  (all formatted)
- [ ] golangci-lint run  (optional, но рекомендуется)
```

### Functionality:
```bash
- [ ] Bot starts successfully
- [ ] /start → creates chat in DB
- [ ] /help → shows command list
- [ ] /modules → shows empty list
- [ ] /enable test → error "module not found"
- [ ] Graceful shutdown works (Ctrl+C)
```

### Database state:
```sql
- [ ] SELECT * FROM chats;  (должен быть ваш тестовый чат)
- [ ] SELECT * FROM event_log;  (должны быть события /start, /help, /modules)
```

### Documentation:
```bash
- [ ] README.md updated
- [ ] CHANGELOG.md updated
- [ ] All files committed to Git
```

---

## 🎯 Phase 1 Success Criteria

### Must Have:
- ✅ Kafka полностью удален (код + зависимости + Docker)
- ✅ telebot.v3 интегрирован и работает с Long Polling
- ✅ Core module system создан (Interface, Registry, Context)
- ✅ Базовые команды работают: /start, /help, /modules, /enable, /disable
- ✅ Repository layer для работы с БД
- ✅ Middleware layer (logger, panic recovery)
- ✅ Docker Compose setup (postgres + bot)
- ✅ Тесты проходят: go test ./...
- ✅ Документация обновлена

### Nice to Have:
- ⭐ Metrics endpoint (/healthz, /metrics) работает
- ⭐ CI/CD pipeline (GitHub Actions) для тестов
- ⭐ golangci-lint проходит без ошибок
- ⭐ Code coverage > 60%

---

## 📊 Time Estimates

| Step | Task | Estimate | Status |
|------|------|----------|--------|
| 1 | Remove Kafka infrastructure | 30 min | 🟡 Not Started |
| 2 | Add dependencies | 5 min | 🟡 Not Started |
| 3 | Create core structure | 1-2 hours | 🟡 Not Started |
| 4 | Update config | 30 min | 🟡 Not Started |
| 5 | Implement bot with Long Polling | 2-3 hours | 🟡 Not Started |
| 6 | Database helpers | 1 hour | 🟡 Not Started |
| 7 | Testing | 1-2 hours | 🟡 Not Started |
| 8 | Documentation updates | 30 min | 🟡 Not Started |
| 9 | Docker setup | 1 hour | 🟡 Not Started |
| 10 | Verification | 30 min | 🟡 Not Started |
| **Total** | | **8-12 hours** | **2-3 дня** |

---

## 🚀 Next Steps After Phase 1

### Phase 2: Limiter Module (2-3 дня)
- Миграция лимитов из Python (limiter_config таблица)
- Реализация LimiterModule (implements Module interface)
- Команды: /setlimit, /showlimits, /mystats
- Daily counters с автосбросом в 00:00
- VIP bypass логика

**Результат:** Работающий модуль лимитов с командами управления

---

## 💡 Tips

1. **Commit часто:** После каждого Step делайте commit с понятным сообщением
2. **Тестируйте постепенно:** Не пишите весь код сразу, тестируйте по шагам
3. **Используйте debugger:** VS Code + Delve для отладки Go
4. **Читайте telebot.v3 docs:** https://pkg.go.dev/gopkg.in/telebot.v3
5. **Не бойтесь рефакторить:** Это Phase 1, можно переделывать

---

**Готов начинать? Начинай с Step 1: Remove Kafka Infrastructure!**

```bash
# Quick start:
cd /Users/aleksandrognev/Documents/flybasist_dev/git/bmft
git checkout -b phase1-core-framework
rm -rf internal/kafkabot internal/logger
rm docker-compose.env.yaml docker-compose.bot.yaml Dockerfile.telegram_bot
git status  # Проверь что удалилось
```
