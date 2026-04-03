package usage

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Tracker struct {
	db                *sql.DB
	warningThreshold  float64 // 0.80 = 80%
	criticalThreshold float64 // 0.95 = 95%
}

type ProviderQuota struct {
	Provider     string
	CurrentUsage int64
	DailyLimit   int64
	MonthlyLimit int64
	ResetTime    time.Time
	Tier         string
	UsagePercent float64
}

func NewTracker(dbPath string) (*Tracker, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Run migrations
	if err := runMigrations(db); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return &Tracker{
		db:                db,
		warningThreshold:  getEnvFloat64("USAGE_WARNING_THRESHOLD", 0.80),
		criticalThreshold: 0.95,
	}, nil
}

func runMigrations(db *sql.DB) error {
	// Read and execute migration file
	// For now, we'll assume migrations are already run via the SQL file
	// In production, you'd use a proper migration tool
	return nil
}

func (t *Tracker) RecordUsage(provider string, tokens int64, requestID, model string) error {
	tx, err := t.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }() // no-op after successful commit

	// Insert usage record
	_, err = tx.Exec(`
		INSERT INTO usage (provider, tokens, timestamp, request_id, model)
		VALUES (?, ?, ?, ?, ?)
	`, provider, tokens, time.Now(), requestID, model)
	if err != nil {
		return err
	}

	// Update daily aggregate
	today := time.Now().Format("2006-01-02")
	_, err = tx.Exec(`
		INSERT INTO daily_usage (provider, date, total_tokens, request_count)
		VALUES (?, ?, ?, 1)
		ON CONFLICT(provider, date) DO UPDATE SET
			total_tokens = total_tokens + ?,
			request_count = request_count + 1
	`, provider, today, tokens, tokens)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (t *Tracker) GetProviderQuota(provider string) (*ProviderQuota, error) {
	var quota ProviderQuota
	quota.Provider = provider

	// Get quota configuration
	err := t.db.QueryRow(`
		SELECT daily_limit, monthly_limit, reset_time, tier
		FROM provider_quotas
		WHERE provider = ?
	`, provider).Scan(&quota.DailyLimit, &quota.MonthlyLimit, &quota.ResetTime, &quota.Tier)

	if err != nil {
		return nil, fmt.Errorf("failed to get quota for %s: %w", provider, err)
	}

	// Get current daily usage
	today := time.Now().Format("2006-01-02")
	err = t.db.QueryRow(`
		SELECT COALESCE(total_tokens, 0)
		FROM daily_usage
		WHERE provider = ? AND date = ?
	`, provider, today).Scan(&quota.CurrentUsage)

	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// Calculate usage percentage
	if quota.DailyLimit > 0 {
		quota.UsagePercent = float64(quota.CurrentUsage) / float64(quota.DailyLimit)
	}

	return &quota, nil
}

func (t *Tracker) GetAllQuotas() (map[string]*ProviderQuota, error) {
	rows, err := t.db.Query(`SELECT provider FROM provider_quotas WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	quotas := make(map[string]*ProviderQuota)
	for rows.Next() {
		var provider string
		if err := rows.Scan(&provider); err != nil {
			continue
		}

		quota, err := t.GetProviderQuota(provider)
		if err != nil {
			log.Printf("Warning: failed to get quota for %s: %v", provider, err)
			continue
		}

		quotas[provider] = quota
	}

	return quotas, nil
}

func (t *Tracker) IsProviderAvailable(provider string) bool {
	quota, err := t.GetProviderQuota(provider)
	if err != nil {
		log.Printf("Error checking quota for %s: %v", provider, err)
		return false
	}

	// Available if under warning threshold
	return quota.UsagePercent < t.warningThreshold
}

func (t *Tracker) GetBestProvider(providers []string) (string, error) {
	quotas, err := t.GetAllQuotas()
	if err != nil {
		return "", err
	}

	// Find provider with lowest usage percentage
	var bestProvider string
	lowestUsage := 1.0

	for _, provider := range providers {
		quota, exists := quotas[provider]
		if !exists {
			continue
		}

		if quota.UsagePercent < lowestUsage {
			lowestUsage = quota.UsagePercent
			bestProvider = provider
		}

		// If we found one under warning threshold, use it
		if quota.UsagePercent < t.warningThreshold {
			log.Printf("[Usage] Selected %s (%.1f%% usage)", provider, quota.UsagePercent*100)
			return provider, nil
		}
	}

	if bestProvider != "" {
		log.Printf("[Usage] All providers above threshold, using %s (%.1f%% usage)",
			bestProvider, lowestUsage*100)
		return bestProvider, nil
	}

	return "", fmt.Errorf("no available providers")
}

func (t *Tracker) ResetDailyQuotas() error {
	_, err := t.db.Exec(`
		UPDATE provider_quotas
		SET reset_time = datetime('now', '+1 day')
		WHERE reset_time < datetime('now')
	`)
	return err
}

func (t *Tracker) GetUsageStats(provider string, since time.Time) (int64, int, error) {
	var totalTokens int64
	var requestCount int

	err := t.db.QueryRow(`
		SELECT COALESCE(SUM(tokens), 0), COUNT(*)
		FROM usage
		WHERE provider = ? AND timestamp >= ?
	`, provider, since).Scan(&totalTokens, &requestCount)

	return totalTokens, requestCount, err
}

func (t *Tracker) Close() error {
	return t.db.Close()
}

func (t *Tracker) WarningThreshold() float64 {
	return t.warningThreshold
}

func getEnvFloat64(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		log.Printf("Ignoring invalid %s value %q", key, raw) //nolint:G706 // key is a hardcoded env var name, raw from env var not user input
		return fallback
	}

	return value
}
