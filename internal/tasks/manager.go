package tasks

import (
	"database/sql"
	"fmt"
	"os"
	"time"
)

// TaskStatus represents the status of a background task
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

// BackgroundTask represents a background agent task
type BackgroundTask struct {
	ID           string
	Status       TaskStatus
	WorktreePath string
	StartTime    time.Time
	EndTime      sql.NullTime
	Message      string
	PRURL        sql.NullString
	Error        sql.NullString
}

// Manager handles background task persistence
type Manager struct {
	db *sql.DB
}

// NewManager creates a new task manager
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// Init creates the background_tasks table if it doesn't exist
func (m *Manager) Init() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS background_tasks (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			worktree_path TEXT NOT NULL,
			start_time DATETIME NOT NULL,
			end_time DATETIME,
			message TEXT NOT NULL,
			pr_url TEXT,
			error TEXT,
			pid INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return err
	}
	// Add pid column if missing (migration for existing databases)
	_, _ = m.db.Exec(`ALTER TABLE background_tasks ADD COLUMN pid INTEGER DEFAULT 0`)
	return nil
}

// CreateTask creates a new background task with PID tracking
func (m *Manager) CreateTask(id, worktreePath, message string) (*BackgroundTask, error) {
	now := time.Now()
	pid := os.Getpid()
	task := &BackgroundTask{
		ID:           id,
		Status:       StatusPending,
		WorktreePath: worktreePath,
		StartTime:    now,
		Message:      message,
	}

	_, err := m.db.Exec(`
		INSERT INTO background_tasks (id, status, worktree_path, start_time, message, pid)
		VALUES (?, ?, ?, ?, ?, ?)
	`, task.ID, task.Status, task.WorktreePath, task.StartTime, task.Message, pid)

	if err != nil {
		return nil, err
	}

	return task, nil
}

// UpdateTask updates a task's status and optional fields
func (m *Manager) UpdateTask(id string, status TaskStatus, prURL, errMsg string) error {
	query := `UPDATE background_tasks SET status = ?`
	args := []interface{}{status}

	if status == StatusCompleted || status == StatusFailed {
		query += `, end_time = ?`
		args = append(args, time.Now())
	}

	if prURL != "" {
		query += `, pr_url = ?`
		args = append(args, prURL)
	}

	if errMsg != "" {
		query += `, error = ?`
		args = append(args, errMsg)
	}

	query += ` WHERE id = ?`
	args = append(args, id)

	_, err := m.db.Exec(query, args...)
	return err
}

// SetPID updates the PID for a running task
func (m *Manager) SetPID(id string, pid int) error {
	_, err := m.db.Exec(`UPDATE background_tasks SET pid = ? WHERE id = ?`, pid, id)
	return err
}

// GetTask retrieves a single task by ID
func (m *Manager) GetTask(id string) (*BackgroundTask, error) {
	row := m.db.QueryRow(`
		SELECT id, status, worktree_path, start_time, end_time, message, pr_url, error
		FROM background_tasks
		WHERE id = ?
	`, id)

	task := &BackgroundTask{}
	err := row.Scan(&task.ID, &task.Status, &task.WorktreePath, &task.StartTime,
		&task.EndTime, &task.Message, &task.PRURL, &task.Error)

	if err != nil {
		return nil, err
	}

	return task, nil
}

// GetTasks retrieves all tasks, optionally filtered by status.
// Automatically marks "running" tasks as "failed" if their PID is dead.
func (m *Manager) GetTasks(status TaskStatus) ([]*BackgroundTask, error) {
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = m.db.Query(`
			SELECT id, status, worktree_path, start_time, end_time, message, pr_url, error, COALESCE(pid, 0)
			FROM background_tasks
			ORDER BY start_time DESC
		`)
	} else {
		rows, err = m.db.Query(`
			SELECT id, status, worktree_path, start_time, end_time, message, pr_url, error, COALESCE(pid, 0)
			FROM background_tasks
			WHERE status = ?
			ORDER BY start_time DESC
		`, status)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var taskList []*BackgroundTask
	for rows.Next() {
		task := &BackgroundTask{}
		var pid int
		err := rows.Scan(&task.ID, &task.Status, &task.WorktreePath, &task.StartTime,
			&task.EndTime, &task.Message, &task.PRURL, &task.Error, &pid)
		if err != nil {
			return nil, err
		}

		// Liveness check: if task says "running", check if it completed or died
		if task.Status == StatusRunning {
			// Check for completion marker first
			doneMarker := task.WorktreePath + "/.synroute-task-done"
			if _, err := os.Stat(doneMarker); err == nil {
				_ = m.UpdateTask(task.ID, StatusCompleted, "", "")
				task.Status = StatusCompleted
			} else if pid > 0 {
				proc, err := os.FindProcess(pid)
				if err != nil || proc.Signal(nil) != nil {
					_ = m.UpdateTask(task.ID, StatusFailed, "", fmt.Sprintf("process died (PID %d not found)", pid))
					task.Status = StatusFailed
				}
			} else if time.Since(task.StartTime) > 24*time.Hour {
				_ = m.UpdateTask(task.ID, StatusFailed, "", "stale task (no PID, older than 24h)")
				task.Status = StatusFailed
			}
		}

		taskList = append(taskList, task)
	}

	return taskList, rows.Err()
}

// FormatTask returns a formatted string representation of a task
func FormatTask(task *BackgroundTask) string {
	endTime := "running"
	if task.EndTime.Valid {
		endTime = task.EndTime.Time.Format(time.RFC3339)
	}

	prURL := "-"
	if task.PRURL.Valid {
		prURL = task.PRURL.String
	}

	errStr := ""
	if task.Error.Valid {
		errStr = " error=" + task.Error.String
	}

	return fmt.Sprintf("%-20s %-10s %-40s %-25s %-25s %s%s",
		task.ID, task.Status, task.WorktreePath,
		task.StartTime.Format(time.RFC3339), endTime, prURL, errStr)
}
