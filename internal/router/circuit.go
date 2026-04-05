package router

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

type CircuitState string

const (
	StateClosed   CircuitState = "closed"
	StateOpen     CircuitState = "open"
	StateHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	db                *sql.DB
	provider          string
	failureThreshold  int
	cooldownDuration  time.Duration
}

func NewCircuitBreaker(db *sql.DB, provider string) *CircuitBreaker {
	threshold := 5
	if v := os.Getenv("CIRCUIT_FAILURE_THRESHOLD"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			threshold = n
		}
	}
	cooldown := 5 * time.Minute
	if v := os.Getenv("CIRCUIT_COOLDOWN_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cooldown = time.Duration(n) * time.Second
		}
	}
	return &CircuitBreaker{
		db:                db,
		provider:          provider,
		failureThreshold:  threshold,
		cooldownDuration:  cooldown,
	}
}

func (cb *CircuitBreaker) GetState() (CircuitState, error) {
	var state string
	var openUntil sql.NullTime

	err := cb.db.QueryRow(`
		SELECT state, open_until
		FROM circuit_breaker_state
		WHERE provider = ?
	`, cb.provider).Scan(&state, &openUntil)

	if err == sql.ErrNoRows {
		// No state row yet — provider hasn't been used. Treat as healthy.
		return StateClosed, nil
	}
	if err != nil {
		return StateClosed, err
	}

	// Check if we should transition from open to half-open
	if state == string(StateOpen) && openUntil.Valid && time.Now().After(openUntil.Time) {
		_ = cb.SetState(StateHalfOpen)
		return StateHalfOpen, nil
	}

	return CircuitState(state), nil
}

func (cb *CircuitBreaker) SetState(state CircuitState) error {
	var openUntil interface{}
	if state == StateOpen {
		openUntil = time.Now().Add(cb.cooldownDuration)
	}

	// Ensure row exists for new providers that haven't been seen yet
	cb.db.Exec(`INSERT OR IGNORE INTO circuit_breaker_state (provider, state, failure_count)
		VALUES (?, 'closed', 0)`, cb.provider)

	_, err := cb.db.Exec(`
		UPDATE circuit_breaker_state
		SET state = ?, open_until = ?
		WHERE provider = ?
	`, string(state), openUntil, cb.provider)

	if err == nil {
		log.Printf("[Circuit] %s circuit breaker: %s", cb.provider, state)
	}

	return err
}

func (cb *CircuitBreaker) IsOpen() bool {
	state, err := cb.GetState()
	if err != nil {
		log.Printf("[Circuit] Error getting state for %s: %v", cb.provider, err)
		return false
	}
	return state == StateOpen
}

func (cb *CircuitBreaker) RecordSuccess() error {
	// Ensure row exists, then reset
	cb.db.Exec(`INSERT OR IGNORE INTO circuit_breaker_state (provider, state, failure_count)
		VALUES (?, 'closed', 0)`, cb.provider)
	_, err := cb.db.Exec(`
		UPDATE circuit_breaker_state
		SET state = ?, failure_count = 0, open_until = NULL
		WHERE provider = ?
	`, string(StateClosed), cb.provider)

	if err == nil {
		log.Printf("[Circuit] %s circuit breaker: SUCCESS (reset)", cb.provider)
	}

	return err
}

func (cb *CircuitBreaker) RecordFailure() error {
	tx, err := cb.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Ensure row exists (auto-create on first failure)
	tx.Exec(`INSERT OR IGNORE INTO circuit_breaker_state (provider, state, failure_count)
		VALUES (?, 'closed', 0)`, cb.provider)

	// Increment failure count
	var failureCount int
	err = tx.QueryRow(`
		UPDATE circuit_breaker_state
		SET failure_count = failure_count + 1,
		    last_failure_time = ?
		WHERE provider = ?
		RETURNING failure_count
	`, time.Now(), cb.provider).Scan(&failureCount)

	if err != nil {
		return err
	}

	log.Printf("[Circuit] %s circuit breaker: FAILURE (count=%d)", cb.provider, failureCount)

	// Open circuit if threshold reached
	if failureCount >= cb.failureThreshold {
		openUntil := time.Now().Add(cb.cooldownDuration)
		_, err = tx.Exec(`
			UPDATE circuit_breaker_state
			SET state = ?, open_until = ?
			WHERE provider = ?
		`, string(StateOpen), openUntil, cb.provider)

		if err != nil {
			return err
		}

		log.Printf("[Circuit] %s circuit breaker: OPENED (threshold reached, cooldown %v)",
			cb.provider, cb.cooldownDuration)
	}

	return tx.Commit()
}

func (cb *CircuitBreaker) Open(duration time.Duration) error {
	openUntil := time.Now().Add(duration)

	// Ensure row exists for new providers that haven't been seen yet
	cb.db.Exec(`INSERT OR IGNORE INTO circuit_breaker_state (provider, state, failure_count)
		VALUES (?, 'closed', 0)`, cb.provider)

	_, err := cb.db.Exec(`
		UPDATE circuit_breaker_state
		SET state = ?, open_until = ?
		WHERE provider = ?
	`, string(StateOpen), openUntil, cb.provider)

	if err == nil {
		log.Printf("[Circuit] %s circuit breaker: MANUALLY OPENED (cooldown %v)", cb.provider, duration)
	}

	return err
}

// Reset closes the circuit, zeroing failure count and clearing open_until.
func (cb *CircuitBreaker) Reset() error {
	// Ensure row exists for new providers that haven't been seen yet
	cb.db.Exec(`INSERT OR IGNORE INTO circuit_breaker_state (provider, state, failure_count)
		VALUES (?, 'closed', 0)`, cb.provider)

	_, err := cb.db.Exec(`
		UPDATE circuit_breaker_state
		SET state = ?, failure_count = 0, open_until = NULL
		WHERE provider = ?
	`, string(StateClosed), cb.provider)

	if err == nil {
		log.Printf("[Circuit] %s circuit breaker: RESET", cb.provider)
	}

	return err
}

// ResetCircuitBreaker resets a single provider's circuit breaker by name.
func (r *Router) ResetCircuitBreaker(provider string) error {
	cb, ok := r.circuitBreakers[provider]
	if !ok {
		return fmt.Errorf("no circuit breaker for provider %q", provider)
	}
	return cb.Reset()
}

// ResetAllCircuitBreakers resets all circuit breakers.
func (r *Router) ResetAllCircuitBreakers() ([]string, error) {
	var reset []string
	for name, cb := range r.circuitBreakers {
		if err := cb.Reset(); err != nil {
			return reset, fmt.Errorf("failed to reset %s: %w", name, err)
		}
		reset = append(reset, name)
	}
	return reset, nil
}

// ResetAllCircuitStates resets all circuit breaker states in the DB directly.
func ResetAllCircuitStates(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT provider FROM circuit_breaker_state`)
	if err != nil {
		return nil, err
	}

	var providers []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			continue
		}
		providers = append(providers, p)
	}
	rows.Close()

	_, err = db.Exec(`
		UPDATE circuit_breaker_state
		SET state = ?, failure_count = 0, open_until = NULL
	`, string(StateClosed))
	if err != nil {
		return nil, err
	}

	return providers, nil
}

// GetAllCircuitStates returns states for all providers
func GetAllCircuitStates(db *sql.DB) (map[string]CircuitState, error) {
	rows, err := db.Query(`
		SELECT provider, state, open_until
		FROM circuit_breaker_state
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	states := make(map[string]CircuitState)
	for rows.Next() {
		var provider, state string
		var openUntil sql.NullTime

		if err := rows.Scan(&provider, &state, &openUntil); err != nil {
			continue
		}

		// Check if should transition to half-open
		if state == string(StateOpen) && openUntil.Valid && time.Now().After(openUntil.Time) {
			cb := NewCircuitBreaker(db, provider)
			_ = cb.SetState(StateHalfOpen)
			states[provider] = StateHalfOpen
		} else {
			states[provider] = CircuitState(state)
		}
	}

	return states, nil
}
