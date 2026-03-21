package memory

import (
	"database/sql"
	"time"
)

type SessionTracker struct {
	db *sql.DB
}

func NewSessionTracker(db *sql.DB) *SessionTracker {
	return &SessionTracker{db: db}
}

// Touch upserts a session as active with the current timestamp.
func (st *SessionTracker) Touch(sessionID string) error {
	_, err := st.db.Exec(`
		INSERT INTO sessions (session_id, status, created_at, last_active)
		VALUES (?, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(session_id) DO UPDATE SET
			status = 'active',
			last_active = CURRENT_TIMESTAMP
	`, sessionID)
	return err
}

// End marks a session as ended.
func (st *SessionTracker) End(sessionID string) error {
	_, err := st.db.Exec(`
		UPDATE sessions SET status = 'ended', last_active = CURRENT_TIMESTAMP
		WHERE session_id = ?
	`, sessionID)
	return err
}

// EndInactiveSessions marks sessions as ended if they haven't been active within the timeout.
func (st *SessionTracker) EndInactiveSessions(timeout time.Duration) (int, error) {
	cutoff := time.Now().Add(-timeout)
	result, err := st.db.Exec(`
		UPDATE sessions SET status = 'ended'
		WHERE status = 'active' AND last_active < ?
	`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// FindRecentEnded returns the most recently ended session within the window,
// excluding the given session ID. Returns "" if none found.
func (st *SessionTracker) FindRecentEnded(excludeSessionID string, window time.Duration) (string, error) {
	cutoff := time.Now().Add(-window)
	var sessionID string
	err := st.db.QueryRow(`
		SELECT session_id FROM sessions
		WHERE status = 'ended' AND session_id != ? AND last_active > ?
		ORDER BY last_active DESC LIMIT 1
	`, excludeSessionID, cutoff).Scan(&sessionID)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return sessionID, nil
}
