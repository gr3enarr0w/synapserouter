package tasks

import (
	"database/sql"
	"fmt"
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
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	return err
}

// CreateTask creates a new background task
func (m *Manager) CreateTask(id, worktreePath, message string) (*BackgroundTask, error) {
	now := time.Now()
	task := &BackgroundTask{
		ID:           id,
		Status:       StatusPending,
		WorktreePath: worktreePath,
		StartTime:    now,
		Message:      message,
	}

	_, err := m.db.Exec(`
		INSERT INTO background_tasks (id, status, worktree_path, start_time, message)
		VALUES (?, ?, ?, ?, ?)
	`, task.ID, task.Status, task.WorktreePath, task.StartTime, task.Message)

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

// GetTasks retrieves all tasks, optionally filtered by status
func (m *Manager) GetTasks(status TaskStatus) ([]*BackgroundTask, error) {
	var rows *sql.Rows
	var err error

	if status == "" {
		rows, err = m.db.Query(`
			SELECT id, status, worktree_path, start_time, end_time, message, pr_url, error
			FROM background_tasks
			ORDER BY start_time DESC
		`)
	} else {
		rows, err = m.db.Query(`
			SELECT id, status, worktree_path, start_time, end_time, message, pr_url, error
			FROM background_tasks
			WHERE status = ?
			ORDER BY start_time DESC
		`, status)
	}

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*BackgroundTask
	for rows.Next() {
		task := &BackgroundTask{}
		err := rows.Scan(&task.ID, &task.Status, &task.WorktreePath, &task.StartTime,
			&task.EndTime, &task.Message, &task.PRURL, &task.Error)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}

	return tasks, rows.Err()
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
