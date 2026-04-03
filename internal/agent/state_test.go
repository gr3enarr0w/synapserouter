package agent

import (
	"bytes"
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS agent_sessions (
			session_id TEXT PRIMARY KEY,
			model TEXT NOT NULL DEFAULT 'auto',
			system_prompt TEXT NOT NULL DEFAULT '',
			work_dir TEXT NOT NULL DEFAULT '.',
			messages TEXT NOT NULL DEFAULT '[]',
			tool_call_log TEXT NOT NULL DEFAULT '[]',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSaveAndLoadState(t *testing.T) {
	db := setupTestDB(t)

	exec := &mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{
				Message: providers.Message{Role: "assistant", Content: "Hello!"},
			}},
		}},
	}

	ag := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), Config{
		Model:        "claude-sonnet-4-6",
		SystemPrompt: "You are a Go expert.",
		MaxTurns:     25,
		WorkDir:      "/tmp/test",
	})

	// Add some conversation
	ag.Run(nil, "hi")

	// Save
	if err := ag.SaveState(db); err != nil {
		t.Fatal(err)
	}

	// Load
	state, err := LoadState(db, ag.SessionID())
	if err != nil {
		t.Fatal(err)
	}

	if state.SessionID != ag.SessionID() {
		t.Errorf("session ID = %q, want %q", state.SessionID, ag.SessionID())
	}
	if state.Model != "claude-sonnet-4-6" {
		t.Errorf("model = %q, want claude-sonnet-4-6", state.Model)
	}
	if len(state.Messages) != 2 { // user + assistant
		t.Errorf("messages = %d, want 2", len(state.Messages))
	}
}

func TestLoadLatestState(t *testing.T) {
	db := setupTestDB(t)

	// Save first session
	ag1 := New(&mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{Message: providers.Message{Role: "assistant", Content: "a"}}},
		}},
	}, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	ag1.Run(nil, "first")
	ag1.SaveState(db)

	// Ensure different timestamp by bumping updated_at for ag2
	ag2 := New(&mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{Message: providers.Message{Role: "assistant", Content: "b"}}},
		}},
	}, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	ag2.Run(nil, "second")
	ag2.SaveState(db)

	// Manually set ag2's updated_at to be later than ag1's
	db.Exec(`UPDATE agent_sessions SET updated_at = datetime('now', '+1 second') WHERE session_id = ?`, ag2.SessionID())

	// Latest should be ag2
	state, err := LoadLatestState(db)
	if err != nil {
		t.Fatal(err)
	}
	if state.SessionID != ag2.SessionID() {
		t.Errorf("latest session = %q, want %q", state.SessionID, ag2.SessionID())
	}
}

func TestRestoreAgent(t *testing.T) {
	db := setupTestDB(t)

	exec := &mockExecutor{
		responses: []providers.ChatResponse{
			{Choices: []providers.Choice{{Message: providers.Message{Role: "assistant", Content: "first"}}}},
			{Choices: []providers.Choice{{Message: providers.Message{Role: "assistant", Content: "restored"}}}},
		},
	}

	// Create and save
	ag := New(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), Config{
		Model:    "auto",
		MaxTurns: 25,
		WorkDir:  "/tmp/test",
	})
	ag.Run(nil, "remember this")
	ag.SaveState(db)

	// Load and restore
	state, err := LoadState(db, ag.SessionID())
	if err != nil {
		t.Fatal(err)
	}

	restored := RestoreAgent(exec, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), state)
	if restored.SessionID() != ag.SessionID() {
		t.Errorf("restored session = %q, want %q", restored.SessionID(), ag.SessionID())
	}

	// Restored agent should have conversation history
	msgs := restored.conversation.Messages()
	if len(msgs) != 2 { // user + assistant from original session
		t.Errorf("restored messages = %d, want 2", len(msgs))
	}
}

func TestListSessions(t *testing.T) {
	db := setupTestDB(t)

	// Save a session
	ag := New(&mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{Message: providers.Message{Role: "assistant", Content: "ok"}}},
		}},
	}, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	ag.Run(nil, "test")
	ag.SaveState(db)

	sessions, err := ListSessions(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Errorf("sessions = %d, want 1", len(sessions))
	}
}

func TestDeleteSession(t *testing.T) {
	db := setupTestDB(t)

	ag := New(&mockExecutor{
		responses: []providers.ChatResponse{{
			Choices: []providers.Choice{{Message: providers.Message{Role: "assistant", Content: "ok"}}},
		}},
	}, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	ag.Run(nil, "test")
	ag.SaveState(db)

	if err := DeleteSession(db, ag.SessionID()); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState(db, ag.SessionID())
	if err == nil {
		t.Error("expected error loading deleted session")
	}
}

func TestLoadStateNotFound(t *testing.T) {
	db := setupTestDB(t)

	_, err := LoadState(db, "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent session")
	}
}

func TestSaveStateNilDB(t *testing.T) {
	ag := New(&mockExecutor{}, tools.NewRegistry(), NewRenderer(&bytes.Buffer{}), DefaultConfig())
	if err := ag.SaveState(nil); err == nil {
		t.Error("expected error with nil DB")
	}
}
