package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// SessionState holds a persisted agent session for save/load/resume.
type SessionState struct {
	SessionID    string              `json:"session_id"`
	Model        string              `json:"model"`
	SystemPrompt string              `json:"system_prompt"`
	WorkDir      string              `json:"work_dir"`
	Messages     []providers.Message `json:"messages"`
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

	_, err = db.Exec(`
		INSERT INTO agent_sessions (session_id, model, system_prompt, work_dir, messages, updated_at)
		VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(session_id) DO UPDATE SET
			messages = excluded.messages,
			updated_at = CURRENT_TIMESTAMP`,
		a.sessionID, a.config.Model, a.config.SystemPrompt, a.config.WorkDir, string(data))
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

// LoadState restores an agent's conversation from SQLite.
func LoadState(db *sql.DB, sessionID string) (*SessionState, error) {
	if db == nil {
		return nil, fmt.Errorf("no database configured")
	}

	var state SessionState
	var messagesJSON string
	err := db.QueryRow(`
		SELECT session_id, model, system_prompt, work_dir, messages, created_at, updated_at
		FROM agent_sessions WHERE session_id = ?`, sessionID).
		Scan(&state.SessionID, &state.Model, &state.SystemPrompt, &state.WorkDir,
			&messagesJSON, &state.CreatedAt, &state.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session not found: %s", sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("load session: %w", err)
	}

	if err := json.Unmarshal([]byte(messagesJSON), &state.Messages); err != nil {
		return nil, fmt.Errorf("unmarshal messages: %w", err)
	}

	return &state, nil
}

// LoadLatestState returns the most recently updated session.
func LoadLatestState(db *sql.DB) (*SessionState, error) {
	if db == nil {
		return nil, fmt.Errorf("no database configured")
	}

	var sessionID string
	err := db.QueryRow(`
		SELECT session_id FROM agent_sessions
		ORDER BY updated_at DESC LIMIT 1`).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no sessions found")
	}
	if err != nil {
		return nil, fmt.Errorf("query latest session: %w", err)
	}

	return LoadState(db, sessionID)
}

// RestoreAgent creates an agent from a persisted session state.
func RestoreAgent(executor ChatExecutor, registry *tools.Registry, renderer *Renderer, state *SessionState) *Agent {
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

	return a
}

// ListSessions returns all saved agent sessions, newest first.
func ListSessions(db *sql.DB) ([]SessionState, error) {
	if db == nil {
		return nil, fmt.Errorf("no database configured")
	}

	rows, err := db.Query(`
		SELECT session_id, model, system_prompt, work_dir, created_at, updated_at
		FROM agent_sessions ORDER BY updated_at DESC`)
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
