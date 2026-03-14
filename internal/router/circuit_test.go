package router

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func setupCircuitTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "circuit.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}

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

func TestCircuitBreaker_Reset(t *testing.T) {
	db := setupCircuitTestDB(t)
	defer db.Close()

	cb := NewCircuitBreaker(db, "nanogpt")

	// Open the circuit
	cb.Open(5 * time.Minute)
	state, _ := cb.GetState()
	if state != StateOpen {
		t.Fatalf("expected open, got %s", state)
	}

	// Reset
	if err := cb.Reset(); err != nil {
		t.Fatal(err)
	}

	state, _ = cb.GetState()
	if state != StateClosed {
		t.Errorf("expected closed after reset, got %s", state)
	}

	// Verify failure count is zero
	var failureCount int
	db.QueryRow(`SELECT failure_count FROM circuit_breaker_state WHERE provider = ?`, "nanogpt").Scan(&failureCount)
	if failureCount != 0 {
		t.Errorf("expected 0 failure count after reset, got %d", failureCount)
	}
}

func TestCircuitBreaker_Reset_AfterFailures(t *testing.T) {
	db := setupCircuitTestDB(t)
	defer db.Close()

	cb := NewCircuitBreaker(db, "nanogpt")

	// Record failures
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Reset
	if err := cb.Reset(); err != nil {
		t.Fatal(err)
	}

	state, _ := cb.GetState()
	if state != StateClosed {
		t.Errorf("expected closed after reset, got %s", state)
	}
}

func TestResetAllCircuitStates(t *testing.T) {
	db := setupCircuitTestDB(t)
	defer db.Close()

	// Open a couple of circuits
	NewCircuitBreaker(db, "nanogpt").Open(5 * time.Minute)
	NewCircuitBreaker(db, "gemini").Open(5 * time.Minute)

	// Reset all
	providers, err := ResetAllCircuitStates(db)
	if err != nil {
		t.Fatal(err)
	}

	if len(providers) == 0 {
		t.Error("expected at least one provider reset")
	}

	// Verify all are closed
	states, err := GetAllCircuitStates(db)
	if err != nil {
		t.Fatal(err)
	}

	for name, state := range states {
		if state != StateClosed {
			t.Errorf("expected %s to be closed, got %s", name, state)
		}
	}
}

func TestRouter_ResetCircuitBreaker(t *testing.T) {
	db := setupCircuitTestDB(t)
	defer db.Close()

	dbPath := filepath.Join(t.TempDir(), "tracker.db")
	// Copy schema to tracker db path
	sqlBytes, _ := os.ReadFile(filepath.Join("..", "..", "migrations", "001_init.sql"))
	trackerDB, _ := sql.Open("sqlite3", dbPath)
	trackerDB.Exec(string(sqlBytes))
	trackerDB.Close()

	provider := &testProvider{}
	r := createTestRouter(t, db, []testProvider{*provider})

	// Open a circuit
	r.circuitBreakers["nanogpt"].Open(5 * time.Minute)

	// Reset it
	err := r.ResetCircuitBreaker("nanogpt")
	if err != nil {
		t.Fatal(err)
	}

	if r.circuitBreakers["nanogpt"].IsOpen() {
		t.Error("expected circuit to be closed after reset")
	}
}

func TestRouter_ResetCircuitBreaker_NotFound(t *testing.T) {
	db := setupCircuitTestDB(t)
	defer db.Close()

	r := createTestRouter(t, db, []testProvider{{}} )

	err := r.ResetCircuitBreaker("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent provider")
	}
}

func TestRouter_ResetAllCircuitBreakers(t *testing.T) {
	db := setupCircuitTestDB(t)
	defer db.Close()

	r := createTestRouter(t, db, []testProvider{{}})

	r.circuitBreakers["nanogpt"].Open(5 * time.Minute)

	reset, err := r.ResetAllCircuitBreakers()
	if err != nil {
		t.Fatal(err)
	}

	if len(reset) == 0 {
		t.Error("expected at least one reset provider")
	}
}

func createTestRouter(t *testing.T, db *sql.DB, tps []testProvider) *Router {
	t.Helper()
	breakers := make(map[string]*CircuitBreaker)

	for i := range tps {
		name := tps[i].Name()
		breakers[name] = NewCircuitBreaker(db, name)
	}

	return &Router{
		db:              db,
		circuitBreakers: breakers,
		healthCache:     make(map[string]*cachedHealth),
	}
}
