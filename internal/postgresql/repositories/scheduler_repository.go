package repositories

import (
	"database/sql"
	"fmt"
	"time"
)

// ============================================================================
// SchedulerRepository - планировщик задач
// ============================================================================

// SchedulerRepository управляет операциями с таблицей scheduled_tasks.
// Русский комментарий: Репозиторий для работы с задачами планировщика.
// Создаёт, читает, удаляет задачи. Отслеживает последний запуск.
type SchedulerRepository struct {
	db *sql.DB
}

// NewSchedulerRepository создаёт новый инстанс репозитория планировщика.
func NewSchedulerRepository(db *sql.DB) *SchedulerRepository {
	return &SchedulerRepository{
		db: db,
	}
}

// ScheduledTask представляет задачу планировщика.
type ScheduledTask struct {
	ID        int64
	ChatID    int64
	ThreadID  int64 // 0 = основной чат, >0 = конкретный топик
	TaskName  string
	CronExpr  string
	TaskType  string // sticker, text, photo
	TaskData  string // file_id для sticker, текст для text, file_id для photo
	IsActive  bool
	LastRun   *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateTask создаёт новую задачу планировщика.
func (r *SchedulerRepository) CreateTask(chatID int64, threadID int, taskName, cronExpr, taskType, taskData string) (int64, error) {
	query := `
		INSERT INTO scheduled_tasks (chat_id, thread_id, task_name, cron_expression, action_type, action_data, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, true)
		RETURNING id
	`
	var taskID int64
	err := r.db.QueryRow(query, chatID, threadID, taskName, cronExpr, taskType, taskData).Scan(&taskID)
	if err != nil {
		return 0, fmt.Errorf("failed to create scheduled task: %w", err)
	}

	return taskID, nil
}

// GetTask получает задачу по ID.
func (r *SchedulerRepository) GetTask(taskID int64) (*ScheduledTask, error) {
	query := `
		SELECT id, chat_id, thread_id, task_name, cron_expression, action_type, action_data, is_active, last_run, created_at, updated_at
		FROM scheduled_tasks
		WHERE id = $1
	`
	task := &ScheduledTask{}
	err := r.db.QueryRow(query, taskID).Scan(
		&task.ID, &task.ChatID, &task.ThreadID, &task.TaskName, &task.CronExpr,
		&task.TaskType, &task.TaskData, &task.IsActive, &task.LastRun,
		&task.CreatedAt, &task.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	return task, nil
}

// GetChatTasks получает все задачи для чата.
func (r *SchedulerRepository) GetChatTasks(chatID int64, threadID int) ([]*ScheduledTask, error) {
	query := `
		SELECT id, chat_id, thread_id, task_name, cron_expression, action_type, action_data, is_active, last_run, created_at, updated_at
		FROM scheduled_tasks
		WHERE chat_id = $1 AND thread_id = $2
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(query, chatID, threadID)
	if err != nil {
		return nil, fmt.Errorf("failed to get chat tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*ScheduledTask
	for rows.Next() {
		task := &ScheduledTask{}
		err := rows.Scan(
			&task.ID, &task.ChatID, &task.ThreadID, &task.TaskName, &task.CronExpr,
			&task.TaskType, &task.TaskData, &task.IsActive, &task.LastRun,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// GetActiveTasks получает все активные задачи.
func (r *SchedulerRepository) GetActiveTasks() ([]*ScheduledTask, error) {
	query := `
		SELECT id, chat_id, thread_id, task_name, cron_expression, action_type, action_data, is_active, last_run, created_at, updated_at
		FROM scheduled_tasks
		WHERE is_active = true
		ORDER BY chat_id, thread_id, created_at
	`
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*ScheduledTask
	for rows.Next() {
		task := &ScheduledTask{}
		err := rows.Scan(
			&task.ID, &task.ChatID, &task.ThreadID, &task.TaskName, &task.CronExpr,
			&task.TaskType, &task.TaskData, &task.IsActive, &task.LastRun,
			&task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	return tasks, nil
}

// UpdateLastRun обновляет время последнего запуска задачи.
func (r *SchedulerRepository) UpdateLastRun(taskID int64, lastRun time.Time) error {
	query := `UPDATE scheduled_tasks SET last_run = $1, updated_at = NOW() WHERE id = $2`
	_, err := r.db.Exec(query, lastRun, taskID)
	if err != nil {
		return fmt.Errorf("failed to update last run: %w", err)
	}
	return nil
}

// DeleteTask удаляет задачу.
func (r *SchedulerRepository) DeleteTask(taskID int64) error {
	query := `DELETE FROM scheduled_tasks WHERE id = $1`
	result, err := r.db.Exec(query, taskID)
	if err != nil {
		return fmt.Errorf("failed to delete task: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("task not found")
	}

	return nil
}
