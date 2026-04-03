package memory

import (
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDBWithSessions(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
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
	CREATE TABLE sessions (
		session_id TEXT PRIMARY KEY,
		status TEXT NOT NULL DEFAULT 'active',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_active DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX idx_sessions_status ON sessions(status, last_active);
	`

	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSessionTouchCreatesAndUpdates(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	if err := st.Touch("sess-1"); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := db.QueryRow("SELECT status FROM sessions WHERE session_id = ?", "sess-1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("expected active, got %s", status)
	}

	// Touch again should not error
	if err := st.Touch("sess-1"); err != nil {
		t.Fatal(err)
	}
}

func TestSessionEnd(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	if err := st.Touch("sess-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.End("sess-1"); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := db.QueryRow("SELECT status FROM sessions WHERE session_id = ?", "sess-1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ended" {
		t.Fatalf("expected ended, got %s", status)
	}
}

func TestFindRecentEndedReturnsEndedSession(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	if err := st.Touch("sess-old"); err != nil {
		t.Fatal(err)
	}
	if err := st.End("sess-old"); err != nil {
		t.Fatal(err)
	}

	found, err := st.FindRecentEnded("sess-new", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if found != "sess-old" {
		t.Fatalf("expected sess-old, got %q", found)
	}
}

func TestFindRecentEndedExcludesSelf(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	if err := st.Touch("sess-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.End("sess-1"); err != nil {
		t.Fatal(err)
	}

	found, err := st.FindRecentEnded("sess-1", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if found != "" {
		t.Fatalf("expected empty (excluded self), got %q", found)
	}
}

func TestFindRecentEndedSkipsActiveSessions(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	// sess-1 is active (not ended)
	if err := st.Touch("sess-1"); err != nil {
		t.Fatal(err)
	}

	found, err := st.FindRecentEnded("sess-2", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if found != "" {
		t.Fatalf("expected empty (active sessions skipped), got %q", found)
	}
}

func TestFindRecentEndedReturnsMostRecent(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	// Insert two ended sessions with different timestamps
	if _, err := db.Exec(`INSERT INTO sessions (session_id, status, last_active) VALUES (?, 'ended', ?)`,
		"sess-older", time.Now().Add(-10*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (session_id, status, last_active) VALUES (?, 'ended', ?)`,
		"sess-newer", time.Now().Add(-2*time.Minute)); err != nil {
		t.Fatal(err)
	}

	found, err := st.FindRecentEnded("sess-current", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if found != "sess-newer" {
		t.Fatalf("expected sess-newer, got %q", found)
	}
}

func TestEndInactiveSessions(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	// Insert an active session that was last active 40 minutes ago
	if _, err := db.Exec(`INSERT INTO sessions (session_id, status, last_active) VALUES (?, 'active', ?)`,
		"sess-stale", time.Now().Add(-40*time.Minute)); err != nil {
		t.Fatal(err)
	}
	// Insert an active session that was last active 5 minutes ago
	if err := st.Touch("sess-recent"); err != nil {
		t.Fatal(err)
	}

	n, err := st.EndInactiveSessions(30 * time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 ended, got %d", n)
	}

	// Verify stale session is ended
	var status string
	if err := db.QueryRow("SELECT status FROM sessions WHERE session_id = ?", "sess-stale").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ended" {
		t.Fatalf("expected ended, got %s", status)
	}

	// Verify recent session is still active
	if err := db.QueryRow("SELECT status FROM sessions WHERE session_id = ?", "sess-recent").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		t.Fatalf("expected active, got %s", status)
	}
}

func TestRetrieveRecentFromSession(t *testing.T) {
	db := newTestDBWithSessions(t)
	vm := NewVectorMemory(db)

	if err := vm.Store("hello from old session", "user", "sess-old", nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	if err := vm.Store("response from old session", "assistant", "sess-old", nil); err != nil {
		t.Fatal(err)
	}
	// Different session — should not be returned
	if err := vm.Store("unrelated message", "user", "sess-other", nil); err != nil {
		t.Fatal(err)
	}

	msgs, err := vm.RetrieveRecentFromSession("sess-old", 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	// Should be in chronological order
	if msgs[0].Content != "hello from old session" {
		t.Fatalf("expected first message to be user message, got %q", msgs[0].Content)
	}
	if msgs[1].Content != "response from old session" {
		t.Fatalf("expected second message to be assistant message, got %q", msgs[1].Content)
	}
}

func TestCrossSessionContinuityFlow(t *testing.T) {
	db := newTestDBWithSessions(t)
	vm := NewVectorMemory(db)
	st := NewSessionTracker(db)

	// Simulate session 1: store messages and end
	if err := st.Touch("sess-1"); err != nil {
		t.Fatal(err)
	}
	if err := vm.Store("The secret code is PHOENIX", "user", "sess-1", nil); err != nil {
		t.Fatal(err)
	}
	if err := vm.Store("Got it, I'll remember PHOENIX", "assistant", "sess-1", nil); err != nil {
		t.Fatal(err)
	}
	if err := st.End("sess-1"); err != nil {
		t.Fatal(err)
	}

	// Session 2 starts: no history in current session
	if err := st.Touch("sess-2"); err != nil {
		t.Fatal(err)
	}

	// Current session has no messages
	current, err := vm.RetrieveRecent("sess-2", 4, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(current) != 0 {
		t.Fatalf("expected 0 current messages, got %d", len(current))
	}

	// Cross-session: find ended session and retrieve its context
	endedSession, err := st.FindRecentEnded("sess-2", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if endedSession != "sess-1" {
		t.Fatalf("expected sess-1, got %q", endedSession)
	}

	crossMsgs, err := vm.RetrieveRecentFromSession(endedSession, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(crossMsgs) != 2 {
		t.Fatalf("expected 2 cross-session messages, got %d", len(crossMsgs))
	}
	if crossMsgs[0].Content != "The secret code is PHOENIX" {
		t.Fatalf("expected PHOENIX message, got %q", crossMsgs[0].Content)
	}
}

func TestMultiTabIsolation(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	// Both tabs are active simultaneously
	if err := st.Touch("tab-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.Touch("tab-2"); err != nil {
		t.Fatal(err)
	}

	// Tab 2 should NOT find tab 1 (tab 1 is still active)
	found, err := st.FindRecentEnded("tab-2", 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if found != "" {
		t.Fatalf("expected empty (both active), got %q", found)
	}
}

func TestEndedSessionNotRetouchedByCarryOver(t *testing.T) {
	db := newTestDBWithSessions(t)
	st := NewSessionTracker(db)

	// Session 1 ended
	if err := st.Touch("sess-1"); err != nil {
		t.Fatal(err)
	}
	if err := st.End("sess-1"); err != nil {
		t.Fatal(err)
	}

	// Session 2 touches itself (does not re-activate sess-1)
	if err := st.Touch("sess-2"); err != nil {
		t.Fatal(err)
	}

	var status string
	if err := db.QueryRow("SELECT status FROM sessions WHERE session_id = ?", "sess-1").Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "ended" {
		t.Fatalf("sess-1 should still be ended, got %s", status)
	}
}
