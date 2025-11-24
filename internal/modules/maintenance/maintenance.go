package maintenance

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
)

// MaintenanceModule обслуживает автоматическую ротацию данных в PostgreSQL.
// Русский комментарий: Создаёт партиции на будущие месяцы и удаляет старые данные.
// Работает в фоновом режиме по расписанию cron.
type MaintenanceModule struct {
	db              *sql.DB
	logger          *zap.Logger
	cron            *cron.Cron
	retentionMonths int // Количество месяцев для хранения данных
}

// New создаёт новый инстанс модуля обслуживания.
func New(db *sql.DB, logger *zap.Logger, retentionMonths int) *MaintenanceModule {
	m := &MaintenanceModule{
		db:              db,
		logger:          logger,
		cron:            cron.New(),
		retentionMonths: retentionMonths,
	}

	logger.Info("maintenance module created", zap.Int("retention_months", retentionMonths))
	return m
}

// Start запускает фоновые задачи обслуживания.
// Русский комментарий: Регистрирует cron-задачи для создания партиций и очистки старых данных.
func (m *MaintenanceModule) Start() error {
	m.logger.Info("starting maintenance module")

	// Задача 1: Создание партиций на 3 месяца вперёд (выполняется каждый день в 03:00)
	_, err := m.cron.AddFunc("0 3 * * *", func() {
		m.logger.Info("running partition creation task")
		if err := m.ensurePartitions(); err != nil {
			m.logger.Error("failed to create partitions", zap.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule partition creation: %w", err)
	}

	// Задача 2: Удаление старых партиций (выполняется каждый день в 04:00)
	_, err = m.cron.AddFunc("0 4 * * *", func() {
		m.logger.Info("running old data cleanup task")
		if err := m.cleanupOldData(); err != nil {
			m.logger.Error("failed to cleanup old data", zap.Error(err))
		}
	})
	if err != nil {
		return fmt.Errorf("failed to schedule data cleanup: %w", err)
	}

	// Запускаем задачи сразу при старте
	m.logger.Info("running initial partition setup")
	if err := m.ensurePartitions(); err != nil {
		m.logger.Error("initial partition creation failed", zap.Error(err))
	}

	m.cron.Start()
	m.logger.Info("maintenance scheduler started successfully")

	return nil
}

// Shutdown выполняет graceful shutdown модуля.
func (m *MaintenanceModule) Shutdown() error {
	m.logger.Info("shutting down maintenance module")
	ctx := m.cron.Stop()
	<-ctx.Done()
	m.logger.Info("maintenance scheduler stopped")
	return nil
}

// ensurePartitions создаёт партиции на 3 месяца вперёд для messages и event_log.
// Русский комментарий: Гарантирует, что всегда есть партиции на будущие месяцы.
func (m *MaintenanceModule) ensurePartitions() error {
	now := time.Now()

	// Создаём партиции на 3 месяца вперёд
	for i := 0; i < 3; i++ {
		month := now.AddDate(0, i, 0)

		// Партиции для messages
		if err := m.createPartition("messages", month); err != nil {
			return fmt.Errorf("failed to create messages partition for %s: %w", month.Format("2006-01"), err)
		}

		// Партиции для event_log
		if err := m.createPartition("event_log", month); err != nil {
			return fmt.Errorf("failed to create event_log partition for %s: %w", month.Format("2006-01"), err)
		}
	}

	m.logger.Info("partition check completed successfully")
	return nil
}

// createPartition создаёт партицию для указанной таблицы и месяца, если её ещё нет.
func (m *MaintenanceModule) createPartition(tableName string, month time.Time) error {
	// Форматируем имя партиции: messages_2025_11
	year, monthNum, _ := month.Date()
	partitionName := fmt.Sprintf("%s_%d_%02d", tableName, year, int(monthNum))

	// Определяем границы партиции
	startDate := time.Date(year, monthNum, 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0)

	// Проверяем, существует ли партиция
	var exists bool
	checkSQL := `
		SELECT EXISTS (
			SELECT 1 FROM pg_tables 
			WHERE schemaname = 'public' AND tablename = $1
		)
	`
	if err := m.db.QueryRow(checkSQL, partitionName).Scan(&exists); err != nil {
		return fmt.Errorf("failed to check partition existence: %w", err)
	}

	if exists {
		// Партиция уже существует
		return nil
	}

	// Создаём партицию
	createSQL := fmt.Sprintf(
		`CREATE TABLE %s PARTITION OF %s FOR VALUES FROM ('%s') TO ('%s')`,
		partitionName,
		tableName,
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"),
	)

	if _, err := m.db.Exec(createSQL); err != nil {
		return fmt.Errorf("failed to create partition: %w", err)
	}

	m.logger.Info("created partition",
		zap.String("table", tableName),
		zap.String("partition", partitionName),
		zap.String("start_date", startDate.Format("2006-01-02")),
		zap.String("end_date", endDate.Format("2006-01-02")),
	)

	return nil
}

// cleanupOldData удаляет партиции старше заданного периода retention.
// Русский комментарий: Удаляет данные старше N месяцев путём удаления целых партиций.
func (m *MaintenanceModule) cleanupOldData() error {
	cutoffDate := time.Now().AddDate(0, -m.retentionMonths, 0)
	cutoffYear, cutoffMonth, _ := cutoffDate.Date()

	m.logger.Info("starting data cleanup",
		zap.Time("cutoff_date", cutoffDate),
		zap.Int("retention_months", m.retentionMonths),
	)

	// Удаляем старые партиции messages
	if err := m.dropOldPartitions("messages", cutoffYear, int(cutoffMonth)); err != nil {
		return fmt.Errorf("failed to drop messages partitions: %w", err)
	}

	// Удаляем старые партиции event_log
	if err := m.dropOldPartitions("event_log", cutoffYear, int(cutoffMonth)); err != nil {
		return fmt.Errorf("failed to drop event_log partitions: %w", err)
	}

	m.logger.Info("data cleanup completed successfully")
	return nil
}

// dropOldPartitions удаляет партиции старше указанной даты.
func (m *MaintenanceModule) dropOldPartitions(tableName string, cutoffYear, cutoffMonth int) error {
	// Получаем список партиций
	query := `
		SELECT tablename 
		FROM pg_tables 
		WHERE schemaname = 'public' 
		  AND tablename LIKE $1
		ORDER BY tablename
	`

	rows, err := m.db.Query(query, tableName+"_%")
	if err != nil {
		return fmt.Errorf("failed to list partitions: %w", err)
	}
	defer rows.Close()

	var droppedCount int
	for rows.Next() {
		var partitionName string
		if err := rows.Scan(&partitionName); err != nil {
			return fmt.Errorf("failed to scan partition name: %w", err)
		}

		// Парсим год и месяц из имени партиции: messages_2025_10
		var year, month int
		_, err := fmt.Sscanf(partitionName, tableName+"_%d_%d", &year, &month)
		if err != nil {
			m.logger.Warn("failed to parse partition name", zap.String("partition", partitionName))
			continue
		}

		// Сравниваем с cutoff датой
		if year < cutoffYear || (year == cutoffYear && month < cutoffMonth) {
			// Удаляем партицию
			dropSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", partitionName)
			if _, err := m.db.Exec(dropSQL); err != nil {
				m.logger.Error("failed to drop partition",
					zap.String("partition", partitionName),
					zap.Error(err),
				)
				continue
			}

			m.logger.Info("dropped old partition",
				zap.String("table", tableName),
				zap.String("partition", partitionName),
				zap.Int("year", year),
				zap.Int("month", month),
			)
			droppedCount++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating partitions: %w", err)
	}

	m.logger.Info("partition cleanup completed",
		zap.String("table", tableName),
		zap.Int("dropped_count", droppedCount),
	)

	return nil
}
