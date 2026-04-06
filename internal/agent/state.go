package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// SessionState holds a persisted agent session for save/load/resume.
type SessionState struct {
	SessionID    string              `json:"session_id"`
	Model        string              `json:"model"`
	SystemPrompt string              `json:"system_prompt"`
	WorkDir      string              `json:"work_dir"`
	Messages     []providers.Message `json:"messages"`
	ToolCallLog  []string            `json:"tool_call_log"` // IDs of completed tool calls for durable execution
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// SaveState persists the agent's conversation to SQLite.
func (a *Agent) SaveState(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("no database configured")
	}

	msgs := a.conversation.Messages()
	data, err := json.Marshal(msgs)
	if err != nil {
		return fmt.Errorf("marshal messages: %w", err)
	}

	toolLog, err := json.Marshal(a.toolCallLog)
	if err != nil {
		return fmt.Errorf("marshal tool call log: %w", err)
	}

	// Default UserID to "local" for backward compatibility and tests
	userID := a.config.UserID
	if userID == "" {
		userID = "local"
	}

	_, err = db.Exec(`
		INSERT INTO agent_sessions (session_id, user_id, model, system_prompt, work_dir, messages, tool_call_log, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(session_id) DO UPDATE SET
			messages = excluded.messages,
			tool_call_log = excluded.tool_call_log,
			updated_at = CURRENT_TIMESTAMP`,
		a.sessionID, userID, a.config.Model, a.config.SystemPrompt, a.config.WorkDir, string(data), string(toolLog))
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// LoadState restores an agent's conversation from SQLite.
// Filters by user_id for tenant isolation.
func LoadState(db *sql.DB, sessionID, userID string) (*SessionState, error) {
	if db == nil {
		return nil, fmt.Errorf("no database configured")
	}

	var state SessionState
	var messagesJSON, toolCallLogJSON sql.NullString
	err := db.QueryRow(`
		SELECT session_id, model, system_prompt, work_dir, messages, tool_call_log, created_at, updated_at
		FROM agent_sessions WHERE session_id = ? AND user_id = ?`, sessionID, userID).
		Scan(&state.SessionID, &state.Model, &state.SystemPrompt, &state.WorkDir,
			&messagesJSON, &toolCallLogJSON, &state.CreatedAt, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	if err := json.Unmarshal([]byte(messagesJSON.String), &state.Messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}

	if toolCallLogJSON.Valid && toolCallLogJSON.String != "" {
		if err := json.Unmarshal([]byte(toolCallLogJSON.String), &state.ToolCallLog); err != nil {
			return nil, fmt.Errorf("unmarshal tool call log: %w", err)
		}
	}

	return &state, nil
}

// LoadLatestState returns the most recently updated session for a working directory.
// Queries by work_dir to support session recovery in non-git directories.
func LoadLatestState(db *sql.DB, workDir string) (*SessionState, error) {
	if db == nil {
		return nil, fmt.Errorf("no database configured")
	}

	var sessionID string
	err := db.QueryRow(`
		SELECT session_id FROM agent_sessions
		WHERE work_dir = ? ORDER BY updated_at DESC LIMIT 1`, workDir).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no sessions found")
	}
	if err != nil {
		return nil, fmt.Errorf("query latest session: %w", err)
	}

	// Use empty userID for backward compatibility - work_dir is the primary key
	return LoadState(db, sessionID, "")
}

// RestoreAgent creates an agent from a persisted session state.
func RestoreAgent(executor ChatExecutor, registry *tools.Registry, renderer TerminalRenderer, state *SessionState) *Agent {
	config := Config{
		Model:        state.Model,
		SystemPrompt: state.SystemPrompt,
		MaxTurns:     0,
		WorkDir:      state.WorkDir,
	}

	a := New(executor, registry, renderer, config)
	a.sessionID = state.SessionID

	// Restore conversation history
	for _, msg := range state.Messages {
		a.conversation.Add(msg)
	}

	// Restore tool call log for durable execution
	a.toolCallLog = state.ToolCallLog

	return a
}

// ListSessions returns all saved agent sessions for a user, newest first.
// Filters by user_id for tenant isolation.
func ListSessions(db *sql.DB, userID string) ([]SessionState, error) {
	if db == nil {
		return nil, fmt.Errorf("no database configured")
	}

	rows, err := db.Query(`
		SELECT session_id, model, system_prompt, work_dir, created_at, updated_at
		FROM agent_sessions WHERE user_id = ? ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	defer rows.Close()

	var sessions []SessionState
	for rows.Next() {
		var s SessionState
		if err := rows.Scan(&s.SessionID, &s.Model, &s.SystemPrompt, &s.WorkDir,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// DeleteSession removes a saved session.
func DeleteSession(db *sql.DB, sessionID string) error {
	if db == nil {
		return fmt.Errorf("no database configured")
	}
	_, err := db.Exec(`DELETE FROM agent_sessions WHERE session_id = ?`, sessionID)
	return err
}
