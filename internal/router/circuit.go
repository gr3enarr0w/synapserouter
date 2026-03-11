package router

import (
	"database/sql"
	"log"
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
	return &CircuitBreaker{
		db:                db,
		provider:          provider,
		failureThreshold:  5,
		cooldownDuration:  5 * time.Minute,
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

	if err != nil {
		return StateClosed, err
	}

	// Check if we should transition from open to half-open
	if state == string(StateOpen) && openUntil.Valid && time.Now().After(openUntil.Time) {
		cb.SetState(StateHalfOpen)
		return StateHalfOpen, nil
	}

	return CircuitState(state), nil
}

func (cb *CircuitBreaker) SetState(state CircuitState) error {
	var openUntil interface{}
	if state == StateOpen {
		openUntil = time.Now().Add(cb.cooldownDuration)
	}

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
	// Reset failure count and close circuit
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

// GetAllStates returns states for all providers
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
			cb.SetState(StateHalfOpen)
			states[provider] = StateHalfOpen
		} else {
			states[provider] = CircuitState(state)
		}
	}

	return states, nil
}
