package agent

import (
	"database/sql"
	"log"
	"sync"
)

// TierLearner tracks task-type × tier success rates for routing optimization.
// Stores outcomes in SQLite, provides success-rate lookups for planner tier assignment.
// Follows the same online correction pattern as the intent router.
type TierLearner struct {
	db    *sql.DB
	mu    sync.RWMutex
	cache map[string]tierStats // "intent:tier" → stats
}

type tierStats struct {
	successes int
	failures  int
}

// NewTierLearner creates a new tier learner backed by SQLite.
// Creates the table if it doesn't exist.
func NewTierLearner(db *sql.DB) *TierLearner {
	if db == nil {
		return nil
	}
	tl := &TierLearner{
		db:    db,
		cache: make(map[string]tierStats),
	}
	tl.initTable()
	tl.loadCache()
	return tl
}

func (tl *TierLearner) initTable() {
	_, err := tl.db.Exec(`
		CREATE TABLE IF NOT EXISTS tier_outcomes (
			intent TEXT NOT NULL,
			tier TEXT NOT NULL,
			successes INTEGER NOT NULL DEFAULT 0,
			failures INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (intent, tier)
		)
	`)
	if err != nil {
		log.Printf("[TierLearner] warning: failed to create table: %v", err)
	}
}

func (tl *TierLearner) loadCache() {
	rows, err := tl.db.Query("SELECT intent, tier, successes, failures FROM tier_outcomes")
	if err != nil {
		return
	}
	defer rows.Close()

	for rows.Next() {
		var intent, tier string
		var s, f int
		if err := rows.Scan(&intent, &tier, &s, &f); err != nil {
			continue
		}
		tl.cache[intent+":"+tier] = tierStats{successes: s, failures: f}
	}
}

// RecordOutcome records a success or failure for a task-type × tier combination.
func (tl *TierLearner) RecordOutcome(intent string, tier ModelTier, success bool) {
	if tl == nil || tl.db == nil {
		return
	}

	tl.mu.Lock()
	defer tl.mu.Unlock()

	key := intent + ":" + string(tier)
	stats := tl.cache[key]
	if success {
		stats.successes++
	} else {
		stats.failures++
	}
	tl.cache[key] = stats

	_, err := tl.db.Exec(`
		INSERT INTO tier_outcomes (intent, tier, successes, failures) VALUES (?, ?, ?, ?)
		ON CONFLICT(intent, tier) DO UPDATE SET
			successes = CASE WHEN ? THEN successes + 1 ELSE successes END,
			failures = CASE WHEN ? THEN failures ELSE failures + 1 END
	`, intent, string(tier),
		boolToInt(success), boolToInt(!success),
		success, success)
	if err != nil {
		log.Printf("[TierLearner] warning: failed to record outcome: %v", err)
	}
}

// SuccessRate returns the success rate for a task-type × tier combination.
// Returns -1 if no data exists (insufficient samples).
// Requires at least 5 samples before returning a rate.
func (tl *TierLearner) SuccessRate(intent string, tier ModelTier) float64 {
	if tl == nil {
		return -1
	}

	tl.mu.RLock()
	defer tl.mu.RUnlock()

	key := intent + ":" + string(tier)
	stats, ok := tl.cache[key]
	if !ok {
		return -1
	}

	total := stats.successes + stats.failures
	if total < 5 {
		return -1 // insufficient data
	}

	return float64(stats.successes) / float64(total)
}

// SuggestTier returns the tier with the highest success rate for a given intent.
// Returns empty string if no data exists or confidence is too low.
func (tl *TierLearner) SuggestTier(intent string) ModelTier {
	if tl == nil {
		return ""
	}

	tl.mu.RLock()
	defer tl.mu.RUnlock()

	bestTier := ModelTier("")
	bestRate := 0.0

	for _, tier := range []ModelTier{TierCheap, TierMid, TierFrontier} {
		key := intent + ":" + string(tier)
		stats, ok := tl.cache[key]
		if !ok {
			continue
		}
		total := stats.successes + stats.failures
		if total < 5 {
			continue
		}
		rate := float64(stats.successes) / float64(total)
		if rate > bestRate {
			bestRate = rate
			bestTier = tier
		}
	}

	// Only suggest if confidence is meaningful (>70% success rate)
	if bestRate < 0.7 {
		return ""
	}
	return bestTier
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
