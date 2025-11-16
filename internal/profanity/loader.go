package profanity

import (
	"compress/gzip"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"go.uber.org/zap"
)

//go:embed dictionary.dat.gz
var embeddedDictionary []byte

// DictionarySource определяет источник словаря
type DictionarySource string

const (
	SourceEmbedded DictionarySource = "embedded" // Встроенный словарь
	SourceFile     DictionarySource = "file"     // Файл по пути
	SourceSkip     DictionarySource = "skip"     // Пропустить загрузку
)

// LoaderConfig конфигурация загрузчика
type LoaderConfig struct {
	Source   DictionarySource
	FilePath string // Используется если Source == SourceFile
}

// LoadConfigFromEnv загружает конфигурацию из переменных окружения
func LoadConfigFromEnv() LoaderConfig {
	source := os.Getenv("PROFANITY_DICT_SOURCE")
	if source == "" {
		source = "embedded" // По умолчанию
	}

	return LoaderConfig{
		Source:   DictionarySource(strings.ToLower(source)),
		FilePath: os.Getenv("PROFANITY_DICT_PATH"),
	}
}

// loadWords загружает слова из gzip-сжатого JSON
func loadWords(data []byte) ([]string, error) {
	gr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	var words []string
	decoder := json.NewDecoder(gr)
	if err := decoder.Decode(&words); err != nil {
		return nil, fmt.Errorf("failed to decode json: %w", err)
	}

	return words, nil
}

// loadFromFile загружает словарь из файла
func loadFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return loadWords(data)
}

// loadFromEmbedded загружает встроенный словарь
func loadFromEmbedded() ([]string, error) {
	return loadWords(embeddedDictionary)
}

// EnsureDictionary проверяет и загружает словарь в базу
func EnsureDictionary(ctx context.Context, db *sql.DB, logger *zap.Logger) error {
	config := LoadConfigFromEnv()

	// Проверяем есть ли уже слова в базе
	var count int
	err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM profanity_dictionary").Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to check dictionary count: %w", err)
	}

	if count > 0 {
		logger.Debug("profanity dictionary already loaded", zap.Int("count", count))
		return nil
	}

	// Загружаем слова в зависимости от источника
	var words []string

	switch config.Source {
	case SourceSkip:
		logger.Info("profanity dictionary loading skipped (PROFANITY_DICT_SOURCE=skip)")
		return nil

	case SourceFile:
		if config.FilePath == "" {
			return fmt.Errorf("PROFANITY_DICT_PATH not set when source=file")
		}
		logger.Info("loading profanity dictionary from file", zap.String("path", config.FilePath))
		words, err = loadFromFile(config.FilePath)
		if err != nil {
			return fmt.Errorf("failed to load from file: %w", err)
		}

	case SourceEmbedded:
		fallthrough
	default:
		logger.Info("loading embedded profanity dictionary")
		words, err = loadFromEmbedded()
		if err != nil {
			return fmt.Errorf("failed to load embedded dictionary: %w", err)
		}
	}

	if len(words) == 0 {
		logger.Warn("no words to load into profanity dictionary")
		return nil
	}

	// Вставляем слова в базу
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO profanity_dictionary (pattern, is_regex, severity)
		VALUES ($1, false, 'moderate')
		ON CONFLICT (pattern) DO NOTHING
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	inserted := 0
	for _, word := range words {
		if word == "" {
			continue
		}
		result, err := stmt.ExecContext(ctx, word)
		if err != nil {
			logger.Warn("failed to insert word", zap.Error(err), zap.String("word", word))
			continue
		}
		rows, _ := result.RowsAffected()
		inserted += int(rows)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	logger.Info("profanity dictionary loaded successfully",
		zap.Int("total", len(words)),
		zap.Int("inserted", inserted),
	)

	return nil
}
