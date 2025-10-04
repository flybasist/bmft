# BMFT — Модульная архитектура бота

## Структура проекта (Plugin-based)

```
bmft/
├── cmd/
│   └── bot/
│       └── main.go                    # Точка входа
├── internal/
│   ├── core/                          # Ядро фреймворка
│   │   ├── bot.go                     # Основной Bot struct
│   │   ├── context.go                 # MessageContext для модулей
│   │   ├── registry.go                # Реестр модулей
│   │   └── router.go                  # Роутинг сообщений
│   ├── config/                        # Конфигурация
│   │   └── config.go
│   ├── database/                      # Подключение к БД
│   │   ├── postgres.go
│   │   └── migrations.go
│   ├── repository/                    # Общие репозитории
│   │   ├── chats.go
│   │   ├── users.go
│   │   ├── modules.go                 # Управление модулями
│   │   └── event_log.go
│   ├── modules/                       # Все модули
│   │   ├── interface.go               # Интерфейс Module
│   │   ├── limiter/                   # Модуль лимитов
│   │   │   ├── module.go
│   │   │   ├── service.go
│   │   │   ├── repository.go
│   │   │   └── commands.go
│   │   ├── reactions/                 # Модуль реакций
│   │   │   ├── module.go
│   │   │   ├── matcher.go
│   │   │   ├── repository.go
│   │   │   └── commands.go
│   │   ├── antispam/                  # Модуль антиспама
│   │   │   ├── module.go
│   │   │   ├── detector.go
│   │   │   ├── repository.go
│   │   │   └── commands.go
│   │   ├── statistics/                # Модуль статистики
│   │   │   ├── module.go
│   │   │   ├── collector.go
│   │   │   ├── repository.go
│   │   │   └── commands.go
│   │   └── scheduler/                 # Модуль планировщика
│   │       ├── module.go
│   │       ├── cron.go
│   │       ├── repository.go
│   │       └── commands.go
│   └── middleware/                    # Middleware для telebot
│       ├── logging.go
│       ├── recovery.go
│       └── admin_check.go
├── migrations/                        # SQL миграции
│   └── 001_initial_schema.sql
├── docker-compose.yaml
├── Dockerfile
├── go.mod
└── README.md
```

## Интерфейс Module (contracts)

Каждый модуль должен реализовать:

```go
type Module interface {
    // Name возвращает уникальное имя модуля
    Name() string
    
    // Description возвращает описание модуля для админов
    Description() string
    
    // Init инициализирует модуль (подключение к БД, загрузка конфига)
    Init(ctx context.Context, deps *ModuleDependencies) error
    
    // Register регистрирует обработчики в роутере
    Register(bot *telebot.Bot) error
    
    // OnMessage вызывается для каждого входящего сообщения (если модуль включен)
    OnMessage(ctx *MessageContext) error
    
    // Commands возвращает список команд которые добавляет модуль
    Commands() []Command
    
    // Shutdown корректное завершение работы модуля
    Shutdown(ctx context.Context) error
}

type ModuleDependencies struct {
    DB            *sql.DB
    Logger        *zap.Logger
    Config        *config.Config
    ChatsRepo     repository.ChatsRepository
    UsersRepo     repository.UsersRepository
    ModulesRepo   repository.ModulesRepository
    EventLogRepo  repository.EventLogRepository
}

type MessageContext struct {
    Ctx       context.Context
    Message   *telebot.Message
    ChatID    int64
    UserID    int64
    ContentType string
    Modules   *ModuleRegistry // доступ к другим модулям если нужно
    
    // Helper methods
    IsAdmin() bool
    GetChatConfig(moduleName string) (map[string]interface{}, error)
    LogEvent(eventType string, data map[string]interface{}) error
}

type Command struct {
    Command     string   // "/setlimit"
    Description string   // "Настройка лимитов на контент"
    AdminOnly   bool     // Доступна только админам чата
    Handler     telebot.HandlerFunc
}
```

## Пример модуля: Limiter

```go
// internal/modules/limiter/module.go
package limiter

import (
    "context"
    "database/sql"
    "github.com/flybasist/bmft/internal/core"
    "go.uber.org/zap"
    "gopkg.in/telebot.v3"
)

type LimiterModule struct {
    name        string
    description string
    deps        *core.ModuleDependencies
    service     *LimiterService
    repo        *LimiterRepository
    log         *zap.Logger
}

func New() *LimiterModule {
    return &LimiterModule{
        name:        "limiter",
        description: "Лимиты на типы контента (фото, видео, стикеры и т.д.)",
    }
}

func (m *LimiterModule) Name() string { 
    return m.name 
}

func (m *LimiterModule) Description() string { 
    return m.description 
}

func (m *LimiterModule) Init(ctx context.Context, deps *core.ModuleDependencies) error {
    m.deps = deps
    m.log = deps.Logger.Named(m.name)
    m.repo = NewLimiterRepository(deps.DB)
    m.service = NewLimiterService(m.repo, deps.EventLogRepo, m.log)
    
    m.log.Info("module initialized")
    return nil
}

func (m *LimiterModule) Register(bot *telebot.Bot) error {
    // Регистрируем команды модуля
    for _, cmd := range m.Commands() {
        bot.Handle(cmd.Command, cmd.Handler)
    }
    return nil
}

func (m *LimiterModule) OnMessage(ctx *core.MessageContext) error {
    // Проверяем включен ли модуль для этого чата
    enabled, err := m.deps.ModulesRepo.IsModuleEnabled(ctx.Ctx, ctx.ChatID, m.name)
    if err != nil || !enabled {
        return err
    }
    
    // Проверяем VIP статус
    if ctx.IsVIP() {
        return nil // VIP игнорирует лимиты
    }
    
    // Проверяем лимит
    allowed, remaining, limit, err := m.service.CheckLimit(
        ctx.Ctx, 
        ctx.ChatID, 
        ctx.UserID, 
        ctx.ContentType,
    )
    
    if err != nil {
        m.log.Error("failed to check limit", zap.Error(err))
        return err
    }
    
    if !allowed {
        // Лимит превышен - удаляем сообщение
        if err := ctx.Message.Delete(); err != nil {
            m.log.Error("failed to delete message", zap.Error(err))
        }
        
        // Отправляем предупреждение
        msg := fmt.Sprintf("@%s, превышен суточный лимит на %s", 
            ctx.Message.Sender.Username, 
            ctx.ContentType,
        )
        bot.Send(ctx.Message.Chat, msg)
        
        // Логируем событие
        ctx.LogEvent("limit_exceeded", map[string]interface{}{
            "content_type": ctx.ContentType,
            "limit": limit,
        })
        
        return nil // Не обрабатываем дальше
    }
    
    // Предупреждение если близко к лимиту
    if remaining > 0 && remaining <= 2 {
        msg := fmt.Sprintf("@%s, осталось %d из %d %s", 
            ctx.Message.Sender.Username,
            remaining,
            limit,
            ctx.ContentType,
        )
        bot.Send(ctx.Message.Chat, msg)
    }
    
    // Инкрементируем счётчик
    return m.service.IncrementCounter(ctx.Ctx, ctx.ChatID, ctx.UserID, ctx.ContentType)
}

func (m *LimiterModule) Commands() []core.Command {
    return []core.Command{
        {
            Command:     "/setlimit",
            Description: "Установить лимит: /setlimit photo 10",
            AdminOnly:   true,
            Handler:     m.handleSetLimit,
        },
        {
            Command:     "/showlimits",
            Description: "Показать текущие лимиты чата",
            AdminOnly:   false,
            Handler:     m.handleShowLimits,
        },
        {
            Command:     "/mystats",
            Description: "Моя статистика за сутки",
            AdminOnly:   false,
            Handler:     m.handleMyStats,
        },
    }
}

func (m *LimiterModule) Shutdown(ctx context.Context) error {
    m.log.Info("module shutting down")
    return nil
}
```

## Пример команды: /setlimit

```go
// internal/modules/limiter/commands.go
func (m *LimiterModule) handleSetLimit(c telebot.Context) error {
    // Проверяем что пользователь админ
    if !isAdmin(c) {
        return c.Reply("❌ Эта команда доступна только администраторам чата")
    }
    
    // Парсим аргументы: /setlimit photo 10
    args := strings.Fields(c.Message().Text)
    if len(args) != 3 {
        return c.Reply("Использование: /setlimit <тип> <лимит>\n" +
            "Типы: photo, video, sticker, text, audio, voice, document, animation, video_note\n" +
            "Лимит: -1 (запрет), 0 (без лимита), N (суточный лимит)")
    }
    
    contentType := args[1]
    limit, err := strconv.Atoi(args[2])
    if err != nil {
        return c.Reply("❌ Лимит должен быть числом")
    }
    
    // Сохраняем в БД
    err = m.service.SetLimit(
        c.Context(),
        c.Chat().ID,
        nil, // user_id = nil означает "для всех"
        contentType,
        limit,
    )
    
    if err != nil {
        m.log.Error("failed to set limit", zap.Error(err))
        return c.Reply("❌ Ошибка при сохранении лимита")
    }
    
    var msg string
    switch {
    case limit == -1:
        msg = fmt.Sprintf("✅ Контент типа %s полностью запрещён", contentType)
    case limit == 0:
        msg = fmt.Sprintf("✅ Контент типа %s разрешён без ограничений", contentType)
    default:
        msg = fmt.Sprintf("✅ Установлен суточный лимит %d для %s", limit, contentType)
    }
    
    return c.Reply(msg)
}
```

## Core Bot initialization

```go
// cmd/bot/main.go
package main

import (
    "context"
    "os"
    "os/signal"
    "syscall"
    
    "github.com/flybasist/bmft/internal/config"
    "github.com/flybasist/bmft/internal/core"
    "github.com/flybasist/bmft/internal/database"
    "github.com/flybasist/bmft/internal/modules/limiter"
    "github.com/flybasist/bmft/internal/modules/reactions"
    "github.com/flybasist/bmft/internal/modules/antispam"
    "github.com/flybasist/bmft/internal/modules/statistics"
    "github.com/flybasist/bmft/internal/modules/scheduler"
    "go.uber.org/zap"
    "gopkg.in/telebot.v3"
)

func main() {
    // Load config
    cfg, err := config.Load()
    if err != nil {
        panic(err)
    }
    
    // Init logger
    logger, _ := zap.NewProduction()
    defer logger.Sync()
    
    // Connect to database
    db, err := database.Connect(cfg.PostgresDSN)
    if err != nil {
        logger.Fatal("failed to connect to database", zap.Error(err))
    }
    defer db.Close()
    
    // Run migrations
    if err := database.RunMigrations(db, "./migrations"); err != nil {
        logger.Fatal("failed to run migrations", zap.Error(err))
    }
    
    // Create bot
    bot, err := telebot.NewBot(telebot.Settings{
        Token:  cfg.TelegramBotToken,
        Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
    })
    if err != nil {
        logger.Fatal("failed to create bot", zap.Error(err))
    }
    
    // Initialize module registry
    registry := core.NewModuleRegistry(db, logger, cfg)
    
    // Register modules (порядок не важен - модули независимы)
    registry.Register(limiter.New())
    registry.Register(reactions.New())
    registry.Register(antispam.New())
    registry.Register(statistics.New())
    registry.Register(scheduler.New())
    
    // Initialize all modules
    ctx := context.Background()
    if err := registry.InitAll(ctx); err != nil {
        logger.Fatal("failed to initialize modules", zap.Error(err))
    }
    
    // Register module handlers in bot
    if err := registry.RegisterAll(bot); err != nil {
        logger.Fatal("failed to register handlers", zap.Error(err))
    }
    
    // Register core handlers
    registerCoreHandlers(bot, registry, logger)
    
    // Start bot
    logger.Info("bot starting", zap.String("username", bot.Me.Username))
    go bot.Start()
    
    // Graceful shutdown
    sigCh := make(chan os.Signal, 1)
    signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
    <-sigCh
    
    logger.Info("shutting down")
    bot.Stop()
    registry.ShutdownAll(ctx)
}

func registerCoreHandlers(bot *telebot.Bot, registry *core.ModuleRegistry, logger *zap.Logger) {
    // /start
    bot.Handle("/start", func(c telebot.Context) error {
        return c.Reply("👋 Привет! Я модульный бот для управления чатом.\n" +
            "Используй /help для списка команд")
    })
    
    // /help - показывает команды всех активных модулей
    bot.Handle("/help", func(c telebot.Context) error {
        chatID := c.Chat().ID
        modules, _ := registry.GetActiveModules(c.Context(), chatID)
        
        var msg strings.Builder
        msg.WriteString("📚 Доступные команды:\n\n")
        
        for _, mod := range modules {
            msg.WriteString(fmt.Sprintf("*%s*\n", mod.Name()))
            for _, cmd := range mod.Commands() {
                adminMark := ""
                if cmd.AdminOnly {
                    adminMark = " 🔒"
                }
                msg.WriteString(fmt.Sprintf("%s - %s%s\n", cmd.Command, cmd.Description, adminMark))
            }
            msg.WriteString("\n")
        }
        
        return c.Reply(msg.String(), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
    })
    
    // /modules - управление модулями (только админ)
    bot.Handle("/modules", handleModulesCommand(registry))
    
    // OnMessage - роутинг по всем модулям
    bot.Handle(telebot.OnText, handleMessage(registry, logger))
    bot.Handle(telebot.OnPhoto, handleMessage(registry, logger))
    bot.Handle(telebot.OnVideo, handleMessage(registry, logger))
    bot.Handle(telebot.OnSticker, handleMessage(registry, logger))
    // ... другие типы контента
}

func handleMessage(registry *core.ModuleRegistry, logger *zap.Logger) telebot.HandlerFunc {
    return func(c telebot.Context) error {
        msg := c.Message()
        
        // Создаём контекст для модулей
        msgCtx := &core.MessageContext{
            Ctx:         c.Context(),
            Message:     msg,
            ChatID:      msg.Chat.ID,
            UserID:      msg.Sender.ID,
            ContentType: getContentType(msg),
            Modules:     registry,
        }
        
        // Вызываем OnMessage для всех активных модулей
        modules, err := registry.GetActiveModules(msgCtx.Ctx, msgCtx.ChatID)
        if err != nil {
            logger.Error("failed to get active modules", zap.Error(err))
            return nil
        }
        
        for _, mod := range modules {
            if err := mod.OnMessage(msgCtx); err != nil {
                logger.Error("module error", 
                    zap.String("module", mod.Name()),
                    zap.Error(err),
                )
            }
        }
        
        return nil
    }
}
```

## Управление модулями через команды

```go
// /modules - показать статус модулей
// /modules enable limiter - включить модуль
// /modules disable antispam - выключить модуль

func handleModulesCommand(registry *core.ModuleRegistry) telebot.HandlerFunc {
    return func(c telebot.Context) error {
        if !isAdmin(c) {
            return c.Reply("❌ Эта команда доступна только администраторам")
        }
        
        args := strings.Fields(c.Message().Text)
        chatID := c.Chat().ID
        
        // /modules - показать список
        if len(args) == 1 {
            modules, _ := registry.GetAllModules()
            activeModules, _ := registry.GetActiveModules(c.Context(), chatID)
            
            activeMap := make(map[string]bool)
            for _, mod := range activeModules {
                activeMap[mod.Name()] = true
            }
            
            var msg strings.Builder
            msg.WriteString("🔌 Доступные модули:\n\n")
            
            for _, mod := range modules {
                status := "❌"
                if activeMap[mod.Name()] {
                    status = "✅"
                }
                msg.WriteString(fmt.Sprintf("%s *%s*\n%s\n\n", 
                    status, 
                    mod.Name(), 
                    mod.Description(),
                ))
            }
            
            msg.WriteString("Для управления:\n")
            msg.WriteString("/modules enable <имя> - включить\n")
            msg.WriteString("/modules disable <имя> - выключить")
            
            return c.Reply(msg.String(), &telebot.SendOptions{ParseMode: telebot.ModeMarkdown})
        }
        
        // /modules enable|disable <name>
        if len(args) == 3 {
            action := args[1]
            moduleName := args[2]
            
            var enabled bool
            switch action {
            case "enable":
                enabled = true
            case "disable":
                enabled = false
            default:
                return c.Reply("❌ Неизвестное действие. Используй enable или disable")
            }
            
            err := registry.SetModuleEnabled(c.Context(), chatID, moduleName, enabled)
            if err != nil {
                return c.Reply("❌ Ошибка при изменении статуса модуля")
            }
            
            status := "отключён"
            if enabled {
                status = "включён"
            }
            
            return c.Reply(fmt.Sprintf("✅ Модуль %s %s", moduleName, status))
        }
        
        return c.Reply("Использование:\n/modules - список модулей\n/modules enable <имя>\n/modules disable <имя>")
    }
}
```

## Преимущества такой архитектуры

1. **Модульность**: Каждая фича = отдельный модуль, можно включать/выключать
2. **Независимость**: Модули не знают друг о друге
3. **Расширяемость**: Новый модуль = просто реализовать интерфейс
4. **Переиспользование**: Общая логика в core, специфичная в модулях
5. **Тестируемость**: Модули можно тестировать изолированно
6. **Гибкость**: Разные чаты = разные наборы модулей
7. **Аналитика**: Все события логируются в event_log

## Следующие шаги

1. ✅ Создана SQL схема с модульной структурой
2. ⏳ Создать core framework (registry, context, interfaces)
3. ⏳ Реализовать первый модуль (limiter) как пример
4. ⏳ Миграция данных из Python БД
5. ⏳ Реализовать остальные модули
