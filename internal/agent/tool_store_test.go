package agent

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func setupToolStoreTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestToolOutputStore_CreateAndStore(t *testing.T) {
	db := setupToolStoreTestDB(t)
	store, err := NewToolOutputStore(db)
	if err != nil {
		t.Fatalf("NewToolOutputStore failed: %v", err)
	}
	if store == nil {
		t.Fatal("store should not be nil")
	}

	id, err := store.Store("session-1", "bash", "go test ./...",
		"exit 0 | 14 lines", "full output here", 0, 100)
	if err != nil {
		t.Fatalf("Store failed: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive id, got %d", id)
	}
}

func TestToolOutputStore_Retrieve(t *testing.T) {
	db := setupToolStoreTestDB(t)
	store, _ := NewToolOutputStore(db)

	id, _ := store.Store("session-1", "bash", "go test",
		"summary", "the full output content", 0, 25)

	output, err := store.Retrieve("session-1", id)
	if err != nil {
		t.Fatalf("Retrieve failed: %v", err)
	}
	if output != "the full output content" {
		t.Errorf("wrong output: %s", output)
	}
}

func TestToolOutputStore_RetrieveWrongSession(t *testing.T) {
	db := setupToolStoreTestDB(t)
	store, _ := NewToolOutputStore(db)

	id, _ := store.Store("session-1", "bash", "cmd", "sum", "full", 0, 4)

	_, err := store.Retrieve("session-2", id)
	if err == nil {
		t.Error("should fail when retrieving from wrong session")
	}
}

func TestToolOutputStore_Search(t *testing.T) {
	db := setupToolStoreTestDB(t)
	store, _ := NewToolOutputStore(db)

	store.Store("s1", "bash", "go test", "test summary", "full1", 0, 5)
	store.Store("s1", "grep", "pattern=TODO", "grep summary", "full2", 0, 5)
	store.Store("s1", "bash", "go build", "build summary", "full3", 0, 5)

	// Search all
	results, err := store.Search("s1", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 {
		t.Errorf("expected 3 results, got %d", len(results))
	}

	// Search by tool name
	bashResults, err := store.Search("s1", "bash", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(bashResults) != 2 {
		t.Errorf("expected 2 bash results, got %d", len(bashResults))
	}

	// Search wrong session
	empty, err := store.Search("s2", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 results for wrong session, got %d", len(empty))
	}
}

func TestToolOutputStore_NilSafe(t *testing.T) {
	store, err := NewToolOutputStore(nil)
	if err == nil {
		t.Error("nil db should return error")
	}
	if store != nil {
		t.Error("nil db should return nil store")
	}

	// All methods should be safe to call on nil
	var nilStore *ToolOutputStore
	id, err := nilStore.Store("s", "t", "a", "s", "f", 0, 0)
	if err != nil || id != 0 {
		t.Error("nil store.Store should return 0, nil")
	}
	results, err := nilStore.Search("s", "", 5)
	if err != nil || results != nil {
		t.Error("nil store.Search should return nil, nil")
	}
}

func TestToolOutputStore_TableAutoCreated(t *testing.T) {
	// Verify the table is created automatically (no migration needed)
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "fresh.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// No migrations run — just create the store
	store, err := NewToolOutputStore(db)
	if err != nil {
		t.Fatalf("NewToolOutputStore failed: %v", err)
	}

	// Should work without migration
	id, err := store.Store("s1", "bash", "cmd", "sum", "full", 0, 4)
	if err != nil {
		t.Fatalf("Store should work on fresh DB (auto-create): %v", err)
	}
	if id <= 0 {
		t.Error("should return positive id")
	}

	// Verify file exists and has data
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("DB file should exist")
	}
}
