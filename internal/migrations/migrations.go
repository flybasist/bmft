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
// Русский комментарий: В режиме горячей разработки (main ветка) мы всегда используем
// только 001_initial_schema.sql и вайпаем базу при изменениях структуры.
// В продакшене (когда будут боевые данные) появятся 002, 003 и т.д. миграции.
var ExpectedSchema = []ExpectedTable{
	// Core tables (Phase 1)
	{Name: "chats", Columns: []string{"id", "chat_id", "chat_type", "title", "is_active"}},
	{Name: "users", Columns: []string{"id", "user_id", "username", "first_name"}},
	{Name: "chat_admins", Columns: []string{"id", "chat_id", "user_id", "can_manage_modules"}},
	{Name: "chat_modules", Columns: []string{"id", "chat_id", "module_name", "is_enabled", "config"}},
	{Name: "messages", Columns: []string{"id", "chat_id", "user_id", "message_id", "content_type"}},

	// Limiter Module (Phase 2)
	{Name: "limiter_config", Columns: []string{"id", "chat_id", "content_type", "daily_limit"}},
	{Name: "limiter_counters", Columns: []string{"id", "chat_id", "user_id", "content_type", "counter_date"}},
	{Name: "user_limits", Columns: []string{"user_id", "username", "daily_limit", "monthly_limit", "daily_used", "monthly_used"}},

	// Reactions Module (Phase 3)
	{Name: "reactions_config", Columns: []string{"id", "chat_id", "content_type", "trigger_type", "trigger_pattern", "reaction_type", "reaction_data"}},
	{Name: "reactions_log", Columns: []string{"id", "chat_id", "user_id", "message_id", "keyword", "violation_code"}},

	// Statistics Module (Phase 4)
	{Name: "statistics_daily", Columns: []string{"id", "chat_id", "user_id", "stat_date", "message_count"}},

	// Scheduler Module (Phase 5)
	{Name: "scheduler_tasks", Columns: []string{"id", "chat_id", "task_name", "cron_expression", "task_type", "task_data", "is_enabled"}},

	// AntiSpam Module (Future)
	{Name: "antispam_config", Columns: []string{"id", "chat_id", "rule_name", "rule_type"}},

	// System tables
	{Name: "event_log", Columns: []string{"id", "event_type", "chat_id", "user_id", "module_name"}},
	{Name: "bot_settings", Columns: []string{"id", "key", "value"}},
}

// RunMigrationsIfNeeded проверяет схему БД и выполняет миграции если требуется
// Возвращает ошибку если схема несовместима или миграция не удалась
// Русский комментарий: Вызывается при старте бота сразу после подключения к PostgreSQL.
func RunMigrationsIfNeeded(db *sql.DB, logger *zap.Logger) error {
	logger.Info("starting database schema validation and migrations")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Проверяем какие таблицы существуют
	existingTables, err := getExistingTables(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to get existing tables: %w", err)
	}

	logger.Info("found existing tables", zap.Int("count", len(existingTables)), zap.Strings("tables", existingTables))

	// 2. Анализируем состояние схемы
	schemaState := analyzeSchemaState(existingTables)

	switch schemaState {
	case SchemaEmpty:
		logger.Info("database schema is empty, running initial migration from 001_initial_schema.sql")
		return runInitialMigration(ctx, db, logger)

	case SchemaComplete:
		logger.Info("database schema is complete, validating structure")
		return validateExistingSchema(ctx, db, logger)

	case SchemaPartial:
		return fmt.Errorf("database schema is partially created - this indicates corrupted migration state. "+
			"Expected tables: %v, found: %v. Please DROP DATABASE and recreate",
			getExpectedTableNames(), existingTables)

	case SchemaUnknown:
		logger.Warn("database contains extra tables not part of expected schema",
			zap.Strings("extra_tables", findUnknownTables(existingTables)))
		// Продолжаем работу, но логируем warning
		return validateExistingSchema(ctx, db, logger)
	}

	return nil
}

// SchemaState представляет состояние схемы БД
type SchemaState int

const (
	SchemaEmpty    SchemaState = iota // Таблиц нет
	SchemaComplete                    // Все таблицы есть
	SchemaPartial                     // Некоторые таблицы есть
	SchemaUnknown                     // Есть неожиданные таблицы
)

// getExistingTables возвращает список существующих таблиц
func getExistingTables(ctx context.Context, db *sql.DB) ([]string, error) {
	query := `
		SELECT table_name 
		FROM information_schema.tables 
		WHERE table_schema = 'public' 
		AND table_type = 'BASE TABLE'
		ORDER BY table_name`

	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		tables = append(tables, tableName)
	}

	return tables, rows.Err()
}

// analyzeSchemaState анализирует состояние схемы по списку существующих таблиц
func analyzeSchemaState(existingTables []string) SchemaState {
	expectedTables := getExpectedTableNames()

	if len(existingTables) == 0 {
		return SchemaEmpty
	}

	existingSet := make(map[string]bool)
	for _, table := range existingTables {
		existingSet[table] = true
	}

	expectedSet := make(map[string]bool)
	for _, table := range expectedTables {
		expectedSet[table] = true
	}

	// Проверяем есть ли все ожидаемые таблицы
	allExpectedExist := true
	for _, expected := range expectedTables {
		if !existingSet[expected] {
			allExpectedExist = false
			break
		}
	}

	// Проверяем есть ли неожиданные таблицы
	hasUnexpectedTables := false
	for _, existing := range existingTables {
		if !expectedSet[existing] {
			hasUnexpectedTables = true
			break
		}
	}

	if allExpectedExist && !hasUnexpectedTables {
		return SchemaComplete
	}

	if hasUnexpectedTables && allExpectedExist {
		return SchemaUnknown // Есть лишние таблицы но все нужные на месте
	}

	return SchemaPartial // Не все нужные таблицы присутствуют
}

// getExpectedTableNames возвращает список названий ожидаемых таблиц
func getExpectedTableNames() []string {
	var names []string
	for _, table := range ExpectedSchema {
		names = append(names, table.Name)
	}
	return names
}

// findUnknownTables возвращает список таблиц которых нет в ExpectedSchema
func findUnknownTables(existingTables []string) []string {
	expectedSet := make(map[string]bool)
	for _, table := range getExpectedTableNames() {
		expectedSet[table] = true
	}

	var unknown []string
	for _, existing := range existingTables {
		if !expectedSet[existing] {
			unknown = append(unknown, existing)
		}
	}
	return unknown
}

// runInitialMigration выполняет начальную миграцию
func runInitialMigration(ctx context.Context, db *sql.DB, logger *zap.Logger) error {
	logger.Info("executing initial database migration")

	migrationFile := "migrations/001_initial_schema.sql"
	migrationSQL, err := os.ReadFile(migrationFile)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", migrationFile, err)
	}

	// Разбиваем SQL на отдельные команды
	commands := splitSQLCommands(string(migrationSQL))
	logger.Info("parsed migration file", zap.Int("command_count", len(commands)))

	// Выполняем команды последовательно
	for i, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" || strings.HasPrefix(command, "--") {
			continue
		}

		logger.Debug("executing migration command", zap.Int("index", i+1), zap.Int("total", len(commands)))

		if _, err := db.ExecContext(ctx, command); err != nil {
			return fmt.Errorf("failed to execute migration command %d: %w\nCommand: %s", i+1, err, command)
		}
	}

	logger.Info("initial migration completed successfully")

	// Валидируем что все таблицы созданы
	return validateExistingSchema(ctx, db, logger)
}

// splitSQLCommands разбивает SQL файл на отдельные команды
// Русский комментарий: Разделитель — точка с запятой. Игнорируем комментарии.
func splitSQLCommands(sqlContent string) []string {
	// Удаляем многострочные комментарии /* ... */
	sqlContent = removeMultilineComments(sqlContent)

	// Разбиваем по точке с запятой
	rawCommands := strings.Split(sqlContent, ";")

	var commands []string
	for _, cmd := range rawCommands {
		// Удаляем однострочные комментарии --
		lines := strings.Split(cmd, "\n")
		var cleanLines []string
		for _, line := range lines {
			// Удаляем комментарии после --
			if idx := strings.Index(line, "--"); idx >= 0 {
				line = line[:idx]
			}
			line = strings.TrimSpace(line)
			if line != "" {
				cleanLines = append(cleanLines, line)
			}
		}

		if len(cleanLines) > 0 {
			command := strings.Join(cleanLines, "\n")
			commands = append(commands, command)
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
