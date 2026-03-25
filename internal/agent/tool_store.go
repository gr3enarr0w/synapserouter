package agent

import (
	"database/sql"
	"fmt"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// ToolOutputStore persists full tool outputs in the DB while conversation
// gets only summaries. Enables the recall tool to retrieve past results.
type ToolOutputStore struct {
	db *sql.DB
}

// ToolOutputStore implements tools.ToolOutputSearcher interface.

// NewToolOutputStore creates a store backed by the given database.
// Ensures the tool_outputs table exists (safe to call multiple times).
func NewToolOutputStore(db *sql.DB) *ToolOutputStore {
	if db == nil {
		return nil
	}
	// Auto-create table if not exists (handles case where migration hasn't run)
	db.Exec(`CREATE TABLE IF NOT EXISTS tool_outputs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		tool_name TEXT NOT NULL,
		args_summary TEXT,
		summary TEXT NOT NULL,
		full_output TEXT,
		exit_code INTEGER DEFAULT 0,
		output_size INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_tool_outputs_session ON tool_outputs(session_id, created_at DESC)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_tool_outputs_tool ON tool_outputs(session_id, tool_name)`)
	return &ToolOutputStore{db: db}
}

// Store saves a tool output and returns its ID.
func (s *ToolOutputStore) Store(sessionID, toolName, argsSummary, summary, fullOutput string, exitCode, outputSize int) (int64, error) {
	if s == nil || s.db == nil {
		return 0, nil
	}
	result, err := s.db.Exec(`
		INSERT INTO tool_outputs (session_id, tool_name, args_summary, summary, full_output, exit_code, output_size)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, sessionID, toolName, argsSummary, summary, fullOutput, exitCode, outputSize)
	if err != nil {
		return 0, fmt.Errorf("failed to store tool output: %w", err)
	}
	return result.LastInsertId()
}

// Retrieve returns the full output for a given tool output ID.
func (s *ToolOutputStore) Retrieve(sessionID string, id int64) (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("no database configured")
	}
	var output string
	err := s.db.QueryRow(`
		SELECT full_output FROM tool_outputs
		WHERE id = ? AND session_id = ?
	`, id, sessionID).Scan(&output)
	if err != nil {
		return "", fmt.Errorf("tool output not found: %w", err)
	}
	return output, nil
}

// Search finds recent tool outputs matching a tool name and/or session.
func (s *ToolOutputStore) Search(sessionID, toolName string, limit int) ([]tools.ToolOutputResult, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	var rows *sql.Rows
	var err error
	if toolName != "" {
		rows, err = s.db.Query(`
			SELECT id, tool_name, args_summary, summary, exit_code, output_size, created_at
			FROM tool_outputs
			WHERE session_id = ? AND tool_name = ?
			ORDER BY created_at DESC LIMIT ?
		`, sessionID, toolName, limit)
	} else {
		rows, err = s.db.Query(`
			SELECT id, tool_name, args_summary, summary, exit_code, output_size, created_at
			FROM tool_outputs
			WHERE session_id = ?
			ORDER BY created_at DESC LIMIT ?
		`, sessionID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []tools.ToolOutputResult
	for rows.Next() {
		var e tools.ToolOutputResult
		if err := rows.Scan(&e.ID, &e.ToolName, &e.ArgsSummary, &e.Summary, &e.ExitCode, &e.OutputSize, &e.CreatedAt); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}
