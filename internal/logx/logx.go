package logx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Русский комментарий: Этот пакет инкапсулирует настройку структурированного логирования.
// Вся операционная информация выводится только на английском, но комментарии в коде максимально подробны.
// Мы используем zap для высокой производительности и единообразия формата.

var (
	logger     *zap.Logger
	loggerOnce sync.Once
)

// Init инициализирует глобальный логгер.
// pretty=true включает человекочитаемый (console) вывод, иначе JSON для агрегаторов логов.
func Init(pretty bool, service string) error {
	var initErr error
	loggerOnce.Do(func() {
		cfg := zap.NewProductionConfig()
		if pretty {
			cfg = zap.NewDevelopmentConfig()
			// Уберём лишний stacktrace по умолчанию
			cfg.Development = false
		}
		cfg.EncoderConfig.TimeKey = "ts"
		cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stdout"}

		l, err := cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
		if err != nil {
			initErr = fmt.Errorf("failed to build logger: %w", err)
			return
		}
		logger = l.With(zap.String("service", service))
		logger.Info("logger initialized")
	})
	return initErr
}

// L возвращает активный логгер.
func L() *zap.Logger { return logger }

// Sync безопасно синхронизирует буферы.
func Sync() { _ = logger.Sync() }

// DailyFileWriter — вспомогательный writer для файлов (используется в file duplicator).
// В нашем случае основное структурированное логирование идёт в stdout для контейнеров,
// а файловое логирование — опционально.

type DailyFileWriter struct {
	mu   sync.Mutex
	base string
	f    *os.File
	day  string
}

// NewDailyFileWriter создаёт новый файловый writer.
func NewDailyFileWriter(dir, base string) (*DailyFileWriter, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &DailyFileWriter{base: base}
	return w, w.rotateLocked(dir)
}

func (w *DailyFileWriter) rotateLocked(dir string) error {
	w.day = time.Now().Format("2006-01-02")
	path := filepath.Join(dir, fmt.Sprintf("%s_%s.log", w.base, w.day))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.f = f
	return nil
}

// Write реализует io.Writer.
func (w *DailyFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	cur := time.Now().Format("2006-01-02")
	if cur != w.day {
		_ = w.f.Close()
		if err := w.rotateLocked(filepath.Dir(w.f.Name())); err != nil {
			return 0, err
		}
	}
	return w.f.Write(p)
}

// WithContext добавляет в логгер trace поля из контекста (расширяемо при наличии tracer'а).
func WithContext(ctx context.Context) *zap.Logger {
	// Пока нет trace-id — возвращаем базовый логгер.
	return logger
}
