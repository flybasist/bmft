package core

import (
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// ModuleRegistry управляет жизненным циклом всех модулей.
// Русский комментарий: Центральный реестр всех модулей бота.
// Модули регистрируются при старте, инициализируются, получают сообщения, graceful shutdown.
type ModuleRegistry struct {
	modules map[string]Module  // Имя модуля -> инстанс модуля
	deps    ModuleDependencies // Зависимости, передаваемые всем модулям
	logger  *zap.Logger
	mu      sync.RWMutex       // Защита от concurrent access
}

// NewRegistry создаёт новый реестр модулей.
func NewRegistry(deps ModuleDependencies) *ModuleRegistry {
	return &ModuleRegistry{
		modules: make(map[string]Module),
		deps:    deps,
		logger:  deps.Logger,
	}
}

// Register регистрирует модуль в реестре.
// Русский комментарий: Вызывается в main.go для регистрации всех модулей.
// Пример: registry.Register("limiter", &limiter.Module{})
func (r *ModuleRegistry) Register(name string, module Module) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.modules[name]; exists {
		r.logger.Warn("module already registered, overwriting", zap.String("module", name))
	}

	r.modules[name] = module
	r.logger.Info("module registered", zap.String("module", name))
}

// InitAll инициализирует все зарегистрированные модули.
// Русский комментарий: Вызывается после регистрации всех модулей, передаёт им Dependencies.
func (r *ModuleRegistry) InitAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info("initializing modules", zap.Int("count", len(r.modules)))

	for name, module := range r.modules {
		r.logger.Info("initializing module", zap.String("module", name))
		if err := module.Init(r.deps); err != nil {
			return fmt.Errorf("failed to init module %s: %w", name, err)
		}
	}

	r.logger.Info("all modules initialized successfully")
	return nil
}

// OnMessage передаёт входящее сообщение всем активным модулям для обработки.
// Русский комментарий: Вызывается для каждого входящего сообщения из Telegram.
// Модули обрабатывают сообщение по очереди. Если модуль не включен для чата — пропускается.
func (r *ModuleRegistry) OnMessage(ctx *MessageContext) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	chatID := ctx.Chat.ID

	for name, module := range r.modules {
		// Проверяем включен ли модуль для этого чата
		enabled, err := module.Enabled(chatID)
		if err != nil {
			r.logger.Error("failed to check if module enabled",
				zap.String("module", name),
				zap.Int64("chat_id", chatID),
				zap.Error(err))
			continue
		}

		if !enabled {
			continue // Модуль отключен для этого чата
		}

		// Передаём сообщение модулю
		if err := module.OnMessage(ctx); err != nil {
			r.logger.Error("module failed to process message",
				zap.String("module", name),
				zap.Int64("chat_id", chatID),
				zap.Int("message_id", ctx.Message.ID),
				zap.Error(err))
			// Не прерываем обработку, даём другим модулям шанс
		}
	}

	return nil
}

// GetModules возвращает список всех зарегистрированных модулей с их командами.
// Русский комментарий: Используется для команды /modules и /help.
func (r *ModuleRegistry) GetModules() map[string][]BotCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string][]BotCommand)
	for name, module := range r.modules {
		result[name] = module.Commands()
	}
	return result
}

// GetModule возвращает модуль по имени (для тестов и прямого доступа).
func (r *ModuleRegistry) GetModule(name string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	module, exists := r.modules[name]
	return module, exists
}

// ShutdownAll вызывает Shutdown для всех модулей при graceful shutdown.
// Русский комментарий: Вызывается при получении SIGINT/SIGTERM для корректной остановки.
func (r *ModuleRegistry) ShutdownAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	r.logger.Info("shutting down modules", zap.Int("count", len(r.modules)))

	var lastErr error
	for name, module := range r.modules {
		r.logger.Info("shutting down module", zap.String("module", name))
		if err := module.Shutdown(); err != nil {
			r.logger.Error("failed to shutdown module",
				zap.String("module", name),
				zap.Error(err))
			lastErr = err // Запоминаем последнюю ошибку, но продолжаем shutdown остальных
		}
	}

	r.logger.Info("all modules shutdown complete")
	return lastErr
}
