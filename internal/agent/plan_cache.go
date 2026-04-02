package agent

import (
	"crypto/sha256"
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"
)

// PlanCache stores and retrieves cached plans from SQLite.
// Plans are keyed by keyword hash + model to enable reuse across sessions
// while invalidating on model changes.
type PlanCache struct {
	db *sql.DB
}

// CachedPlan represents a stored plan.
type CachedPlan struct {
	ID                 int64
	CacheKey           string
	Model              string
	OriginalRequest    string
	AcceptanceCriteria string
	CreatedAt          time.Time
	HitCount           int
}

// Stop words removed from requests before hashing.
var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "is": true, "are": true,
	"was": true, "were": true, "be": true, "been": true, "being": true,
	"have": true, "has": true, "had": true, "do": true, "does": true,
	"did": true, "will": true, "would": true, "could": true, "should": true,
	"may": true, "might": true, "shall": true, "can": true,
	"this": true, "that": true, "these": true, "those": true,
	"i": true, "me": true, "my": true, "we": true, "our": true, "you": true, "your": true,
	"it": true, "its": true, "he": true, "she": true, "they": true, "them": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "from": true, "up": true, "about": true,
	"into": true, "through": true, "during": true, "before": true, "after": true,
	"and": true, "but": true, "or": true, "nor": true, "not": true, "so": true,
	"if": true, "then": true, "than": true, "too": true, "very": true,
	"just": true, "also": true, "please": true, "help": true,
}

// NewPlanCache creates a plan cache backed by the given database.
// Creates the plan_cache table if it doesn't exist.
func NewPlanCache(db *sql.DB) *PlanCache {
	if db == nil {
		return nil
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS plan_cache (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		cache_key TEXT NOT NULL,
		model TEXT NOT NULL,
		original_request TEXT NOT NULL,
		acceptance_criteria TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_used DATETIME DEFAULT CURRENT_TIMESTAMP,
		hit_count INTEGER DEFAULT 0,
		UNIQUE(cache_key, model)
	)`); err != nil {
		log.Printf("[PlanCache] failed to create table: %v", err)
		return nil
	}

	db.Exec(`CREATE INDEX IF NOT EXISTS idx_plan_cache_key_model ON plan_cache(cache_key, model)`)

	return &PlanCache{db: db}
}

// Store saves a plan to the cache. Upserts on (cache_key, model) conflict.
func (pc *PlanCache) Store(cacheKey, model, request, criteria string) error {
	if pc == nil || pc.db == nil {
		return nil
	}

	_, err := pc.db.Exec(`
		INSERT INTO plan_cache (cache_key, model, original_request, acceptance_criteria)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(cache_key, model) DO UPDATE SET
			acceptance_criteria = excluded.acceptance_criteria,
			original_request = excluded.original_request,
			last_used = CURRENT_TIMESTAMP`,
		cacheKey, model, request, criteria)
	if err != nil {
		return fmt.Errorf("store plan: %w", err)
	}

	log.Printf("[PlanCache] stored plan (key=%s, model=%s)", cacheKey[:8], model)
	return nil
}

// Lookup finds a cached plan matching the key and model.
// Returns nil if no match found (cache miss).
func (pc *PlanCache) Lookup(cacheKey, model string) (*CachedPlan, error) {
	if pc == nil || pc.db == nil {
		return nil, nil
	}

	var plan CachedPlan
	err := pc.db.QueryRow(`
		SELECT id, cache_key, model, original_request, acceptance_criteria, created_at, hit_count
		FROM plan_cache
		WHERE cache_key = ? AND model = ?`,
		cacheKey, model).Scan(
		&plan.ID, &plan.CacheKey, &plan.Model,
		&plan.OriginalRequest, &plan.AcceptanceCriteria,
		&plan.CreatedAt, &plan.HitCount)

	if err == sql.ErrNoRows {
		return nil, nil // cache miss
	}
	if err != nil {
		return nil, fmt.Errorf("lookup plan: %w", err)
	}

	return &plan, nil
}

// RecordHit increments the hit count and updates last_used timestamp.
func (pc *PlanCache) RecordHit(id int64) {
	if pc == nil || pc.db == nil {
		return
	}
	pc.db.Exec(`UPDATE plan_cache SET hit_count = hit_count + 1, last_used = CURRENT_TIMESTAMP WHERE id = ?`, id)
}

// ExtractCacheKey generates a deterministic cache key from a task request.
// Removes stop words, sorts remaining words, and returns a SHA256-based hash.
// Same task phrased differently produces the same key.
func ExtractCacheKey(request string) string {
	lower := strings.ToLower(request)

	// Extract words, remove punctuation
	var words []string
	for _, word := range strings.Fields(lower) {
		word = strings.Trim(word, ".,;:!?()[]{}\"'`-")
		if len(word) < 2 {
			continue
		}
		if stopWords[word] {
			continue
		}
		words = append(words, word)
	}

	// Sort for order-independence
	sort.Strings(words)

	// Hash
	joined := strings.Join(words, " ")
	hash := sha256.Sum256([]byte(joined))
	return fmt.Sprintf("%x", hash[:8]) // 16 hex chars
}
