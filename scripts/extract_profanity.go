//go:build tools
// +build tools

// Утилита для регенерации словаря мата (internal/profanity/dictionary.dat.gz)
// из исходников проекта denexapp/russian-bad-words.
//
// Запуск:
//   go run -tags tools ./scripts/extract_profanity.go <path-to-russian-bad-words>
//
// Build-tag `tools` исключает файл из обычной сборки бота (`go build ./...`),
// чтобы не было конфликта `package main` с cmd/bot.

package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// parseTypeScriptFiles парсит .ts файлы и извлекает строковые значения
func parseTypeScriptFiles(repoPath string) ([]string, error) {
	var allWords []string

	// Регулярка для извлечения строковых значений
	stringPattern := regexp.MustCompile(`:\s*['"]([а-яёА-ЯЁ]+)['"]`)

	wordsDir := filepath.Join(repoPath, "src", "words")

	err := filepath.Walk(wordsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".ts" && filepath.Base(path) != "index.ts" {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			// Извлекаем все строки с русскими буквами
			matches := stringPattern.FindAllStringSubmatch(string(content), -1)
			for _, match := range matches {
				if len(match) > 1 {
					word := strings.TrimSpace(strings.ToLower(match[1]))
					if word != "" {
						allWords = append(allWords, word)
					}
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Дедупликация
	wordSet := make(map[string]bool)
	for _, word := range allWords {
		wordSet[word] = true
	}

	uniqueWords := make([]string, 0, len(wordSet))
	for word := range wordSet {
		uniqueWords = append(uniqueWords, word)
	}

	return uniqueWords, nil
}

// createCompressedDict создает сжатый файл словаря
func createCompressedDict(words []string, outputPath string) error {
	// Создаем директорию
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Создаем файл
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	// Создаем gzip writer
	gz := gzip.NewWriter(f)
	defer gz.Close()

	// Записываем JSON
	encoder := json.NewEncoder(gz)
	if err := encoder.Encode(words); err != nil {
		return fmt.Errorf("failed to encode json: %w", err)
	}

	return nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run extract_profanity.go <path-to-russian-bad-words>")
		os.Exit(1)
	}

	repoPath := os.Args[1]
	outputPath := filepath.Join("internal", "profanity", "dictionary.dat.gz")

	fmt.Printf("Parsing TypeScript files from: %s\n", repoPath)
	words, err := parseTypeScriptFiles(repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Extracted %d unique words\n", len(words))

	fmt.Printf("Creating compressed dictionary: %s\n", outputPath)
	if err := createCompressedDict(words, outputPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating dictionary: %v\n", err)
		os.Exit(1)
	}

	// Проверяем размер файла
	stat, _ := os.Stat(outputPath)
	fmt.Printf("\n✅ Dictionary created successfully!\n")
	fmt.Printf("   File: %s\n", outputPath)
	fmt.Printf("   Size: %d bytes (compressed)\n", stat.Size())
	fmt.Printf("   Words: %d\n\n", len(words))

	fmt.Println("📝 Attribution required:")
	fmt.Println("   Source: https://github.com/denexapp/russian-bad-words")
	fmt.Println("   Author: Denis Mukhametov")
	fmt.Println("   License: MIT")
}
