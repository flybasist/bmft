// Package migrations обеспечивает автоматическое создание и валидацию схемы БД
// при запуске приложения. Гарантирует совместимость схемы или останавливает запуск.
package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ExpectedTable описывает ожидаемую структуру таблицы для валидации
type ExpectedTable struct {
	Name    string
	Columns []string // Список обязательных колонок
}

// ExpectedSchema содержит описание всех таблиц которые должны существовать
// Русский комментарий: v0.8.0 - упрощённая схема с JSONB metadata + поддержка топиков через thread_id.
// Счётчики заменены на MATERIALIZED VIEW для производительности.
var ExpectedSchema = []ExpectedTable{
	// Core tables
	{Name: "chats", Columns: []string{"chat_id", "chat_type", "title", "is_forum", "is_active"}},
	{Name: "users", Columns: []string{"user_id", "username", "first_name"}},
	{Name: "chat_vips", Columns: []string{"id", "chat_id", "thread_id", "user_id", "granted_at"}},
	{Name: "messages", Columns: []string{"id", "chat_id", "thread_id", "user_id", "message_id", "content_type", "chat_name", "metadata"}},

	// Limiter Module
	{Name: "content_limits", Columns: []string{"id", "chat_id", "thread_id", "limit_text", "limit_photo", "limit_banned_words"}},

	// Reactions Module
	{Name: "keyword_reactions", Columns: []string{"id", "chat_id", "thread_id", "pattern", "response_type", "response_content", "is_active"}},
	{Name: "reaction_triggers", Columns: []string{"chat_id", "reaction_id", "user_id", "last_triggered_at", "trigger_count"}},
	{Name: "reaction_daily_counters", Columns: []string{"chat_id", "reaction_id", "user_id", "counter_date", "count"}},
	{Name: "banned_words", Columns: []string{"id", "chat_id", "thread_id", "pattern", "action", "is_active"}},

	// Scheduler Module
	{Name: "scheduled_tasks", Columns: []string{"id", "chat_id", "cron_expression", "action_type", "is_active"}},

	// System tables
	{Name: "schema_migrations", Columns: []string{"version", "description", "applied_at"}},
	{Name: "bot_settings", Columns: []string{"id", "bot_version", "timezone"}},
	{Name: "event_log", Columns: []string{"id", "chat_id", "module_name", "event_type"}},
}

// LatestSchemaVersion - текущая версия схемы базы данных
// Русский комментарий: Увеличивайте эту константу при добавлении новых миграций
const LatestSchemaVersion = 1

// RunMigrationsIfNeeded проверяет схему БД и выполняет миграции если требуется
// Возвращает ошибку если схема несовместима или миграция не удалась
// Русский комментарий: Вызывается при старте бота сразу после подключения к PostgreSQL.
// Поддерживает версионирование миграций через таблицу schema_migrations.
func RunMigrationsIfNeeded(db *sql.DB, logger *zap.Logger) error {
	logger.Info("starting database schema validation and migrations")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Проверяем текущую версию схемы
	currentVersion, err := getCurrentSchemaVersion(ctx, db)
	if err != nil {
		// Если таблицы schema_migrations нет - это пустая БД
		if err == sql.ErrNoRows || strings.Contains(err.Error(), "does not exist") {
			logger.Info("database is empty (no schema_migrations table), running initial migration")
			return runInitialMigration(ctx, db, logger)
		}
		return fmt.Errorf("failed to get current schema version: %w", err)
	}

	logger.Info("current schema version", zap.Int("version", currentVersion))

	// 2. Проверяем нужно ли применить новые миграции
	if currentVersion < LatestSchemaVersion {
		logger.Info("applying pending migrations",
			zap.Int("current", currentVersion),
			zap.Int("target", LatestSchemaVersion))
		return applyPendingMigrations(ctx, db, logger, currentVersion)
	}

	// 3. Валидируем корректность существующей схемы
	logger.Info("schema is up to date, validating structure")
	return validateExistingSchema(ctx, db, logger)
}

// SchemaState представляет состояние схемы БД
type SchemaState int

const (
	SchemaEmpty    SchemaState = iota // Таблиц нет
	SchemaComplete                    // Все таблицы есть
	SchemaPartial                     // Некоторые таблицы есть
	SchemaUnknown                     // Есть неожиданные таблицы
)

// runMigrationFile выполняет миграцию из файла
func runMigrationFile(ctx context.Context, db *sql.DB, migrationFile string, logger *zap.Logger) error {
	migrationSQL, err := os.ReadFile(migrationFile)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", migrationFile, err)
	}

	// Разбиваем SQL на отдельные команды
	commands := splitSQLCommands(string(migrationSQL))
	logger.Info("parsed migration file",
		zap.String("file", migrationFile),
		zap.Int("command_count", len(commands)))

	// Выполняем команды последовательно
	for i, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" || strings.HasPrefix(command, "--") {
			continue
		}

		// Извлекаем первые 100 символов для логирования
		preview := command
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}

		logger.Info("executing migration command",
			zap.String("file", migrationFile),
			zap.Int("index", i+1),
			zap.Int("total", len(commands)),
			zap.String("preview", preview))

		if _, err := db.ExecContext(ctx, command); err != nil {
			// Логируем ошибку но продолжаем для некритичных команд
			if strings.Contains(command, "IF NOT EXISTS") ||
				strings.Contains(command, "IF EXISTS") ||
				strings.Contains(command, "COMMENT ON") {
				logger.Warn("non-critical migration command failed (continuing)",
					zap.Int("index", i+1),
					zap.Error(err),
					zap.String("command_preview", preview))
				continue
			}
			return fmt.Errorf("failed to execute migration command %d: %w\nCommand: %s", i+1, err, command)
		}
	}

	logger.Info("migration completed successfully", zap.String("file", migrationFile))
	return nil
}

// runInitialMigration выполняет начальную миграцию
func runInitialMigration(ctx context.Context, db *sql.DB, logger *zap.Logger) error {
	logger.Info("executing initial database migration")

	if err := runMigrationFile(ctx, db, "migrations/001_initial_schema.sql", logger); err != nil {
		return err
	}

	logger.Info("initial migration completed successfully")

	// Валидируем что все таблицы созданы
	return validateExistingSchema(ctx, db, logger)
}

// splitSQLCommands разбивает SQL файл на отдельные команды
// Русский комментарий: Разделитель — точка с запятой. Учитываем PL/pgSQL блоки с $$ ... $$
func splitSQLCommands(sqlContent string) []string {
	// Удаляем многострочные комментарии /* ... */
	sqlContent = removeMultilineComments(sqlContent)

	var commands []string
	var currentCommand strings.Builder
	inDollarQuote := false

	lines := strings.Split(sqlContent, "\n")
	for _, line := range lines {
		// Удаляем однострочные комментарии --
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)

		if line == "" {
			continue
		}

		// Проверяем вход/выход из $$ блока (PL/pgSQL функции)
		if strings.Contains(line, "$$") {
			inDollarQuote = !inDollarQuote
		}

		currentCommand.WriteString(line)
		currentCommand.WriteString("\n")

		// Если встретили ; и НЕ внутри $$ блока - это конец команды
		if strings.HasSuffix(line, ";") && !inDollarQuote {
			cmd := strings.TrimSpace(currentCommand.String())
			if cmd != "" && cmd != ";" {
				commands = append(commands, cmd)
			}
			currentCommand.Reset()
		}
	}

	// Добавляем последнюю команду если есть
	if currentCommand.Len() > 0 {
		cmd := strings.TrimSpace(currentCommand.String())
		if cmd != "" && cmd != ";" {
			commands = append(commands, cmd)
		}
	}

	return commands
}

// removeMultilineComments удаляет многострочные комментарии /* ... */
func removeMultilineComments(sql string) string {
	var result strings.Builder
	inComment := false

	for i := 0; i < len(sql); i++ {
		if !inComment && i < len(sql)-1 && sql[i] == '/' && sql[i+1] == '*' {
			inComment = true
			i++ // Пропускаем '*'
			continue
		}

		if inComment && i < len(sql)-1 && sql[i] == '*' && sql[i+1] == '/' {
			inComment = false
			i++ // Пропускаем '/'
			continue
		}

		if !inComment {
			result.WriteByte(sql[i])
		}
	}

	return result.String()
}

// validateExistingSchema проверяет что все ожидаемые таблицы и колонки существуют
func validateExistingSchema(ctx context.Context, db *sql.DB, logger *zap.Logger) error {
	logger.Info("validating existing database schema")

	for _, expectedTable := range ExpectedSchema {
		// Проверяем существование таблицы
		tableExists, err := checkTableExists(ctx, db, expectedTable.Name)
		if err != nil {
			return fmt.Errorf("failed to check table existence for %s: %w", expectedTable.Name, err)
		}

		if !tableExists {
			return fmt.Errorf("expected table %s does not exist", expectedTable.Name)
		}

		// Проверяем существование обязательных колонок
		for _, column := range expectedTable.Columns {
			columnExists, err := checkColumnExists(ctx, db, expectedTable.Name, column)
			if err != nil {
				return fmt.Errorf("failed to check column existence for %s.%s: %w", expectedTable.Name, column, err)
			}

			if !columnExists {
				return fmt.Errorf("expected column %s.%s does not exist", expectedTable.Name, column)
			}
		}

		logger.Debug("table validated", zap.String("table", expectedTable.Name), zap.Int("columns", len(expectedTable.Columns)))
	}

	logger.Info("schema validation completed successfully", zap.Int("tables", len(ExpectedSchema)))
	return nil
}

// checkTableExists проверяет существование таблицы
func checkTableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 
			FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = $1
		)`

	var exists bool
	err := db.QueryRowContext(ctx, query, tableName).Scan(&exists)
	return exists, err
}

// checkColumnExists проверяет существование колонки в таблице
func checkColumnExists(ctx context.Context, db *sql.DB, tableName, columnName string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 
			FROM information_schema.columns 
			WHERE table_schema = 'public' 
			AND table_name = $1 
			AND column_name = $2
		)`

	var exists bool
	err := db.QueryRowContext(ctx, query, tableName, columnName).Scan(&exists)
	return exists, err
}

// getCurrentSchemaVersion возвращает текущую версию схемы из таблицы schema_migrations
func getCurrentSchemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	query := `SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1`
	var version int
	err := db.QueryRowContext(ctx, query).Scan(&version)
	if err != nil {
		return 0, err
	}
	return version, nil
}

// applyPendingMigrations применяет все миграции от currentVersion+1 до LatestSchemaVersion
func applyPendingMigrations(ctx context.Context, db *sql.DB, logger *zap.Logger, currentVersion int) error {
	for version := currentVersion + 1; version <= LatestSchemaVersion; version++ {
		migrationFile := fmt.Sprintf("migrations/%03d_migration.sql", version)

		// Проверяем существование файла
		if _, err := os.Stat(migrationFile); os.IsNotExist(err) {
			logger.Warn("migration file not found, skipping",
				zap.Int("version", version),
				zap.String("file", migrationFile))
			continue
		}

		logger.Info("applying migration",
			zap.Int("version", version),
			zap.String("file", migrationFile))

		if err := runMigrationFile(ctx, db, migrationFile, logger); err != nil {
			return fmt.Errorf("failed to apply migration %d: %w", version, err)
		}

		// Записываем успешно примененную миграцию
		description := fmt.Sprintf("Migration %d", version)
		if err := recordMigration(ctx, db, version, description); err != nil {
			return fmt.Errorf("failed to record migration %d: %w", version, err)
		}

		logger.Info("migration applied successfully", zap.Int("version", version))
	}

	return nil
}

// recordMigration записывает информацию о примененной миграции
func recordMigration(ctx context.Context, db *sql.DB, version int, description string) error {
	query := `
		INSERT INTO schema_migrations (version, description, applied_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (version) DO NOTHING
	`
	_, err := db.ExecContext(ctx, query, version, description)
	return err
}
