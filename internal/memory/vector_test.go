package memory

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestRetrieveRelevantUsesLexicalRanking(t *testing.T) {
	db := newTestDB(t)
	vm := NewVectorMemory(db)

	if err := vm.Store("Discussing Go concurrency with goroutines and channels", "user", "session-a", nil); err != nil {
		t.Fatal(err)
	}
	if err := vm.Store("Python data science notebook workflow", "assistant", "session-a", nil); err != nil {
		t.Fatal(err)
	}
	if err := vm.Store("Go interfaces and channel fan-out patterns", "assistant", "session-a", nil); err != nil {
		t.Fatal(err)
	}

	results, err := vm.RetrieveRelevant("go channels", "session-a", 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected relevant results")
	}
	if results[0].Content != "Go interfaces and channel fan-out patterns" &&
		results[0].Content != "Discussing Go concurrency with goroutines and channels" {
		t.Fatalf("unexpected top result: %q", results[0].Content)
	}
}

func TestRetrieveRelevantFallsBackToRecent(t *testing.T) {
	db := newTestDB(t)
	vm := NewVectorMemory(db)

	if err := vm.Store("first message", "user", "session-b", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := vm.Store("second message", "assistant", "session-b", nil); err != nil {
		t.Fatal(err)
	}

	results, err := vm.RetrieveRelevant("zzzz-no-match", "session-b", 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("expected recent fallback, got %d results", len(results))
	}
	if results[0].Content != "first message" || results[1].Content != "second message" {
		t.Fatalf("unexpected fallback ordering: %+v", results)
	}
}

func TestRetrieveRelevantRespectsSessionBoundary(t *testing.T) {
	db := newTestDB(t)
	vm := NewVectorMemory(db)

	if err := vm.Store("redis cache invalidation details", "user", "session-c", nil); err != nil {
		t.Fatal(err)
	}
	if err := vm.Store("redis cache invalidation details", "user", "session-d", nil); err != nil {
		t.Fatal(err)
	}

	results, err := vm.RetrieveRelevant("redis invalidation", "session-c", 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	schema := `
	CREATE TABLE memory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		content TEXT NOT NULL,
		embedding BLOB,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		session_id TEXT,
		role TEXT,
		metadata TEXT
	);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}
