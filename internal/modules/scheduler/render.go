package scheduler

import (
	"fmt"
	"html"
	"strconv"

	"go.uber.org/zap"
	tele "gopkg.in/telebot.v3"

	"github.com/flybasist/bmft/internal/core"
)

// UniqueMenuDelTask — Unique для inline-кнопки 🗑 задачи в меню.
const UniqueMenuDelTask = "m_rm_task"

// UniqueMenuRunTask — Unique для inline-кнопки ▶ задачи в меню.
const UniqueMenuRunTask = "m_run_task"

// RenderListTasks возвращает HTML-текст списка задач планировщика.
// Второй return — slice taskID для кнопок ▶/🗑.
// Пустая строка + nil = нет задач.
func (m *SchedulerModule) RenderListTasks(chatID int64, threadID int) (string, []int64, error) {
	tasks, err := m.schedulerRepo.GetChatTasks(chatID, threadID)
	if err != nil {
		return "", nil, fmt.Errorf("не удалось получить задачи")
	}
	if len(tasks) == 0 {
		return "", nil, nil
	}

	location := "чата"
	if threadID != 0 {
		location = "топика"
	}

	text := fmt.Sprintf("📋 <b>Задачи %s (%d):</b>\n\n", location, len(tasks))

	var ids []int64
	const maxLen = 3200
	truncated := false
	for i, task := range tasks {
		status := "✅"
		if !task.IsActive {
			status = "⏸️"
		}

		lastRun := "—"
		if task.LastRun != nil {
			lastRun = task.LastRun.Format(core.DateTimeFormat)
		}

		entry := fmt.Sprintf("%d. %s %s\n   <code>%s</code> · %s · %s\n\n",
			i+1, status, html.EscapeString(task.TaskName),
			html.EscapeString(task.CronExpr), task.TaskType, lastRun)

		if len(text)+len(entry) > maxLen {
			truncated = true
			break
		}
		text += entry
		ids = append(ids, task.ID)
	}
	if truncated {
		text += fmt.Sprintf("<i>…и ещё %d задач</i>", len(tasks)-len(ids))
	}

	return text, ids, nil
}

// DeleteTaskByID удаляет задачу и снимает с cron. Используется inline-кнопкой 🗑 в меню.
func (m *SchedulerModule) DeleteTaskByID(chatID int64, taskID int64, adminID int64) error {
	task, err := m.schedulerRepo.GetTask(taskID)
	if err != nil {
		return fmt.Errorf("задача не найдена")
	}
	if task.ChatID != chatID {
		return fmt.Errorf("задача из другого чата")
	}

	if err := m.schedulerRepo.DeleteTask(taskID); err != nil {
		m.logger.Error("DeleteTaskByID failed", zap.Error(err), zap.Int64("task_id", taskID))
		return err
	}

	m.mu.Lock()
	if entryID, ok := m.taskEntries[taskID]; ok {
		m.cron.Remove(entryID)
		delete(m.taskEntries, taskID)
	}
	m.mu.Unlock()

	_ = m.eventRepo.Log(chatID, adminID, "scheduler", "task_deleted",
		fmt.Sprintf("Task %d deleted via menu", taskID))
	return nil
}

// RunTaskByID запускает задачу немедленно. Используется inline-кнопкой ▶ в меню.
func (m *SchedulerModule) RunTaskByID(chatID int64, taskID int64, adminID int64) (string, error) {
	task, err := m.schedulerRepo.GetTask(taskID)
	if err != nil {
		return "", fmt.Errorf("задача не найдена")
	}
	if task.ChatID != chatID {
		return "", fmt.Errorf("задача из другого чата")
	}

	go m.executeTask(task)

	_ = m.eventRepo.Log(chatID, adminID, "scheduler", "task_run",
		fmt.Sprintf("Task %d (%s) run via menu", taskID, task.TaskName))

	return task.TaskName, nil
}

// BuildTaskButtons создаёт inline-кнопки ▶ и 🗑 для списка задач (по одной строке на задачу).
func BuildTaskButtons(ids []int64) []tele.Btn {
	btns := make([]tele.Btn, 0, len(ids)*2)
	for _, id := range ids {
		idStr := strconv.FormatInt(id, 10)
		btns = append(btns,
			tele.Btn{Unique: UniqueMenuRunTask, Text: fmt.Sprintf("▶ #%d", id), Data: idStr},
			tele.Btn{Unique: UniqueMenuDelTask, Text: fmt.Sprintf("🗑 #%d", id), Data: idStr},
		)
	}
	return btns
}
