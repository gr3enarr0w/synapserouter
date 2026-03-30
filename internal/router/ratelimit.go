package router

import (
	"context"
	"database/sql"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ProviderRateLimiter manages per-provider token bucket rate limiters.
// Uses golang.org/x/time/rate with AIMD (Additive Increase, Multiplicative Decrease).
type ProviderRateLimiter struct {
	mu       sync.RWMutex
	limiters map[string]*adaptiveLimiter
	db       *sql.DB
}

type adaptiveLimiter struct {
	limiter        *rate.Limiter
	configRPM      float64
	currentRPM     float64
	consecutive429 int
}

var defaultRPM = map[string]float64{
	"vertex-claude":  5,
	"vertex-gemini":  60,
	"ollama":         60,
	"models-corp":    30,
	"gemini":         15,
	"claude-code":    10,
	"codex":          20,
}

func NewProviderRateLimiter(db *sql.DB) *ProviderRateLimiter {
	return &ProviderRateLimiter{
		limiters: make(map[string]*adaptiveLimiter),
		db:       db,
	}
}

func rpmForProvider(name string) float64 {
	envKey := strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_RPM"
	if v := os.Getenv(envKey); v != "" {
		if rpm, err := strconv.ParseFloat(v, 64); err == nil && rpm > 0 {
			return rpm
		}
	}
	lower := strings.ToLower(name)
	for prefix, rpm := range defaultRPM {
		if strings.HasPrefix(lower, prefix) {
			return rpm
		}
	}
	return 600 // high default for unknown providers (tests, custom endpoints)
}

func (prl *ProviderRateLimiter) GetLimiter(name string) *rate.Limiter {
	prl.mu.RLock()
	if al, ok := prl.limiters[name]; ok {
		prl.mu.RUnlock()
		return al.limiter
	}
	prl.mu.RUnlock()

	prl.mu.Lock()
	defer prl.mu.Unlock()

	if al, ok := prl.limiters[name]; ok {
		return al.limiter
	}

	rpm := rpmForProvider(name)
	rps := rpm / 60.0
	burst := int(math.Max(1, math.Min(rpm/10, 5)))

	limiter := rate.NewLimiter(rate.Limit(rps), burst)
	prl.limiters[name] = &adaptiveLimiter{
		limiter:    limiter,
		configRPM:  rpm,
		currentRPM: rpm,
	}

	log.Printf("[RateLimit] %s: initialized at %.1f RPM (burst=%d)", name, rpm, burst)
	return limiter
}

// Wait blocks until the provider's rate limiter allows a request.
// For high-RPM providers (>100 RPM), skip the wait entirely — they
// don't need pacing and the delay impacts test performance.
func (prl *ProviderRateLimiter) Wait(ctx context.Context, name string) error {
	rpm := rpmForProvider(name)
	if rpm >= 100 {
		return nil // no pacing needed for fast providers / tests
	}
	return prl.GetLimiter(name).Wait(ctx)
}

// RecordRateLimitHit halves the rate after a 429 (AIMD decrease).
func (prl *ProviderRateLimiter) RecordRateLimitHit(name string, retryAfterSecs int) {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	al, ok := prl.limiters[name]
	if !ok {
		return
	}

	al.consecutive429++
	newRPM := al.currentRPM / 2.0
	if newRPM < 0.5 {
		newRPM = 0.5
	}
	al.currentRPM = newRPM

	rps := newRPM / 60.0
	burst := int(math.Max(1, math.Min(newRPM/10, 5)))
	al.limiter.SetLimit(rate.Limit(rps))
	al.limiter.SetBurst(burst)

	log.Printf("[RateLimit] %s: 429 hit (consecutive=%d), reduced to %.1f RPM",
		name, al.consecutive429, newRPM)

	if retryAfterSecs > 0 {
		retryDur := time.Duration(retryAfterSecs) * time.Second
		al.limiter.SetLimit(rate.Limit(1.0 / retryDur.Seconds()))
		log.Printf("[RateLimit] %s: Retry-After %ds, pausing", name, retryAfterSecs)

		go func() {
			time.Sleep(retryDur)
			prl.mu.Lock()
			defer prl.mu.Unlock()
			if al2, ok := prl.limiters[name]; ok {
				rps := al2.currentRPM / 60.0
				burst := int(math.Max(1, math.Min(al2.currentRPM/10, 5)))
				al2.limiter.SetLimit(rate.Limit(rps))
				al2.limiter.SetBurst(burst)
				log.Printf("[RateLimit] %s: retry-after expired, restored to %.1f RPM", name, al2.currentRPM)
			}
		}()
	}
}

// RecordSuccess gradually restores rate (AIMD increase).
func (prl *ProviderRateLimiter) RecordSuccess(name string) {
	prl.mu.Lock()
	defer prl.mu.Unlock()

	al, ok := prl.limiters[name]
	if !ok {
		return
	}

	al.consecutive429 = 0
	if al.currentRPM >= al.configRPM {
		return
	}

	increment := al.configRPM * 0.10
	newRPM := math.Min(al.currentRPM+increment, al.configRPM)
	al.currentRPM = newRPM

	rps := newRPM / 60.0
	burst := int(math.Max(1, math.Min(newRPM/10, 5)))
	al.limiter.SetLimit(rate.Limit(rps))
	al.limiter.SetBurst(burst)
}
