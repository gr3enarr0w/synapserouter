package agent

import (
	"database/sql"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	_ "modernc.org/sqlite"
)

func TestLoadLatestStateByWorkDir(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create sessions table
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS agent_sessions (
		session_id TEXT PRIMARY KEY,
		user_id TEXT,
		model TEXT,
		system_prompt TEXT,
		work_dir TEXT,
		messages TEXT,
		tool_call_log TEXT,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}

	workDir := "/tmp/test-project-abc"

	// Create two agents with different work dirs
	config1 := DefaultConfig()
	config1.WorkDir = workDir
	config1.DB = db
	ag1 := New(&mockExecutor{}, nil, nil, config1)
	ag1.conversation.Add(providers.Message{Role: "user", Content: "hello from project"})
	ag1.SaveState(db)

	config2 := DefaultConfig()
	config2.WorkDir = "/tmp/other-project"
	config2.DB = db
	ag2 := New(&mockExecutor{}, nil, nil, config2)
	ag2.conversation.Add(providers.Message{Role: "user", Content: "hello from other"})
	ag2.SaveState(db)

	// LoadLatestState by work_dir should find ag1's session
	state, err := LoadLatestState(db, workDir)
	if err != nil {
		t.Fatalf("LoadLatestState failed: %v", err)
	}

	if state.WorkDir != workDir {
		t.Errorf("WorkDir = %q, want %q", state.WorkDir, workDir)
	}

	if len(state.Messages) == 0 {
		t.Error("expected at least one message in restored state")
	}
}
