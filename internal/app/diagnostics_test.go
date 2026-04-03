package app

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/gr3enarr0w/synapserouter/internal/memory"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Apply minimal schema
	schemaPath := filepath.Join("..", "..", "migrations", "001_init.sql")
	sqlBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatal(err)
	}

	return db
}

func TestRunDiagnostics_WithProviders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ac := &AppContext{
		DB:        db,
		Providers: []providers.Provider{&healthyMockProvider{name: "test-provider"}},
	}

	checks := RunDiagnostics(context.Background(), ac)
	if len(checks) == 0 {
		t.Fatal("expected diagnostic checks")
	}

	// Verify categories present
	categories := make(map[string]bool)
	for _, c := range checks {
		categories[c.Category] = true
	}

	for _, expected := range []string{"environment", "database", "providers"} {
		if !categories[expected] {
			t.Errorf("missing diagnostic category: %s", expected)
		}
	}
}

func TestRunDiagnostics_DatabaseChecks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ac := &AppContext{DB: db}
	checks := RunDiagnostics(context.Background(), ac)

	var dbChecks []DiagnosticCheck
	for _, c := range checks {
		if c.Category == "database" {
			dbChecks = append(dbChecks, c)
		}
	}

	if len(dbChecks) == 0 {
		t.Fatal("expected database diagnostic checks")
	}

	for _, c := range dbChecks {
		if c.Status == "fail" {
			t.Errorf("database check %s failed: %s", c.Name, c.Message)
		}
	}
}

func TestRunDiagnostics_NoProviders(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	ac := &AppContext{
		DB:        db,
		Providers: []providers.Provider{},
	}

	checks := RunDiagnostics(context.Background(), ac)
	found := false
	for _, c := range checks {
		if c.Category == "providers" && c.Name == "count" {
			found = true
			if c.Status != "warn" {
				t.Errorf("expected warn for no providers, got %s", c.Status)
			}
		}
	}
	if !found {
		t.Error("expected provider count check")
	}
}

func TestRunDiagnostics_MemoryCheck(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	// Store a message so memory check has data
	vm := memory.NewVectorMemory(db)
	_ = vm.Store("test content", "user", "test-session", nil)

	ac := &AppContext{
		DB:           db,
		VectorMemory: vm,
	}

	checks := RunDiagnostics(context.Background(), ac)
	found := false
	for _, c := range checks {
		if c.Category == "memory" {
			found = true
			if c.Status == "fail" {
				t.Errorf("memory check failed: %s", c.Message)
			}
		}
	}
	if !found {
		t.Error("expected memory diagnostic check")
	}
}
