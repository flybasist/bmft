package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

const (
	logFolder        = "log"
	logRetentionDays = 30
	maxLogFileSizeMB = 5 // опционально, для проверки размера
)

func InitLogger(name string) {
	now := time.Now()
	currentDate := now.Format("2006-01-02") // формат даты для имени файла

	// Создание директории логов, если не существует
	if _, err := os.Stat(logFolder); os.IsNotExist(err) {
		os.Mkdir(logFolder, 0755)
	}

	// Удаление старых логов
	files, _ := os.ReadDir(logFolder)
	for _, file := range files {
		fileDateStr := file.Name()
		// ожидаем имя вроде bot_2024-04-01.log
		if len(fileDateStr) >= len(name)+11 {
			dateStr := fileDateStr[len(name)+1 : len(name)+11]
			fileDate, err := time.Parse("2006-01-02", dateStr)
			if err == nil && now.Sub(fileDate) > time.Hour*24*time.Duration(logRetentionDays) {
				os.Remove(filepath.Join(logFolder, file.Name()))
			}
		}
	}

	// Формирование имени лог-файла
	logFileName := fmt.Sprintf("%s_%s.log", name, currentDate)
	logPath := filepath.Join(logFolder, logFileName)

	// Открытие файла
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("не удалось открыть лог-файл: %v", err)
	}

	// Установка вывода логов в файл
	log.SetOutput(logFile)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Логгер инициализирован")
}
