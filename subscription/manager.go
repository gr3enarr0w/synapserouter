package subscription

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrNoSubscriptionModels = errors.New("no available subscription models")

const defaultCacheTTL = 2 * time.Minute

// ManagerOption configures the subscription manager during creation.
type ManagerOption func(*Manager)

// WithCacheTTL sets a custom TTL for the subscription cache.
func WithCacheTTL(ttl time.Duration) ManagerOption {
	return func(m *Manager) {
		if ttl > 0 {
			m.ttl = ttl
		}
	}
}

// WithHTTPClient overrides the default HTTP client used to contact the subscription API.
func WithHTTPClient(client *http.Client) ManagerOption {
	return func(m *Manager) {
		if client != nil {
			m.client = client
		}
	}
}

// WithAPIKey sets the API key for authentication with the subscription API.
func WithAPIKey(apiKey string) ManagerOption {
	return func(m *Manager) {
		if apiKey != "" {
			m.apiKey = apiKey
		}
	}
}

// Manager keeps subscription models cached and exposes helpers for exhaustion tracking.
type Manager struct {
	baseURL string
	ttl     time.Duration
	client  *http.Client
	apiKey  string

	cacheMu   sync.RWMutex
	cached    []ModelDefinition
	lastFetch time.Time

	exhaustedMu sync.RWMutex
	exhausted   map[string]struct{}
}

// NewManager creates a subscription manager that targets the given base URL.
func NewManager(baseURL string, opts ...ManagerOption) *Manager {
	cleanURL := strings.TrimRight(baseURL, "/")
	if cleanURL == "" {
		cleanURL = "http://localhost:8091"
	}

	mgr := &Manager{
		baseURL:    cleanURL,
		ttl:        defaultCacheTTL,
		client:     http.DefaultClient,
		exhausted:  make(map[string]struct{}),
		lastFetch:  time.Time{},
		cached:     nil,
	}

	for _, opt := range opts {
		opt(mgr)
	}

	return mgr
}

// GetNextModel selects the next available model for a role using cached or fallback data.
func (m *Manager) GetNextModel(role string) (*ModelSelection, error) {
	return m.getNextModel(context.Background(), role)
}

// GetNextModelWithContext allows callers to provide a context for cache refreshes.
func (m *Manager) GetNextModelWithContext(ctx context.Context, role string) (*ModelSelection, error) {
	return m.getNextModel(ctx, role)
}

// MarkExhausted marks a subscription model as exhausted so it is no longer returned.
func (m *Manager) MarkExhausted(modelID string) {
	if modelID == "" {
		return
	}

	m.exhaustedMu.Lock()
	defer m.exhaustedMu.Unlock()
	m.exhausted[modelID] = struct{}{}
	log.Printf("[SUBSCRIPTION] Model marked exhausted: %s", modelID)
}

// Refresh forces an immediate refresh of cached data from the subscription API.
func (m *Manager) Refresh(ctx context.Context) error {
	log.Println("[SUBSCRIPTION] Manual cache refresh requested")
	return m.fetch(ctx)
}

func (m *Manager) getNextModel(ctx context.Context, role string) (*ModelSelection, error) {
	log.Printf("[DEBUG] getNextModel called for role: %s", role)
	
	if err := m.ensureCache(ctx); err != nil {
		log.Printf("[DEBUG] ensureCache failed: %v", err)
		return nil, err
	}

	m.cacheMu.RLock()
	models := append([]ModelDefinition{}, m.cached...)
	m.cacheMu.RUnlock()

	log.Printf("[DEBUG] Checking %d cached models for role '%s'", len(models), role)
	
	for i, candidate := range models {
		log.Printf("[DEBUG] Model %d: ID=%s, Roles=%v, Status=%s",
			i, candidate.ID, candidate.Roles, candidate.Status)
		
		if !candidate.SupportsRole(role) {
			log.Printf("[DEBUG] Model %s does not support role '%s'", candidate.ID, role)
			continue
		}
		if !candidate.IsAvailable() {
			log.Printf("[DEBUG] Model %s is not available", candidate.ID)
			continue
		}
		if m.isExhausted(candidate.ID) {
			log.Printf("[DEBUG] Model %s is exhausted", candidate.ID)
			continue
		}
		
		log.Printf("[DEBUG] Selected subscription model: %s for role: %s", candidate.ID, role)
		return &ModelSelection{
			Model: candidate,
			Role:  role,
		}, nil
	}

	log.Printf("[DEBUG] No suitable subscription model found for role '%s'. Total models: %d", role, len(models))
	log.Println("[SUBSCRIPTION] All subscription models exhausted or unavailable")
	return nil, ErrNoSubscriptionModels
}

func (m *Manager) ensureCache(ctx context.Context) error {
	m.cacheMu.RLock()
	hasCache := len(m.cached) > 0
	stale := time.Since(m.lastFetch) >= m.ttl
	lastFetch := m.lastFetch
	m.cacheMu.RUnlock()

	log.Printf("[DEBUG] Cache check - Has cache: %t, Stale: %t, Last fetch: %v, TTL: %v",
		hasCache, stale, lastFetch, m.ttl)

	if hasCache && !stale {
		log.Printf("[DEBUG] Cache hit - %d models available", len(m.cached))
		log.Println("[SUBSCRIPTION] Cache hit")
		return nil
	}

	log.Printf("[DEBUG] Cache miss or stale - fetching from subscription API at: %s", m.baseURL)
	log.Println("[SUBSCRIPTION] Cache miss or stale; fetching from subscription API")
	if err := m.fetch(ctx); err != nil {
		m.cacheMu.RLock()
		defer m.cacheMu.RUnlock()
		if len(m.cached) > 0 {
			log.Printf("[SUBSCRIPTION] Fetch error (%v) — falling back to cached data", err)
			return nil
		}
		log.Printf("[DEBUG] Fetch failed and no cached data available: %v", err)
		return err
	}

	return nil
}

func (m *Manager) isExhausted(modelID string) bool {
	m.exhaustedMu.RLock()
	defer m.exhaustedMu.RUnlock()
	_, ok := m.exhausted[modelID]
	return ok
}

func (m *Manager) fetch(ctx context.Context) error {
	endpoint := fmt.Sprintf("%s/api/subscription/v1/models", m.baseURL)
	log.Printf("[DEBUG] Fetching subscription models from: %s", endpoint)
	
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		log.Printf("[DEBUG] Failed to build request: %v", err)
		return fmt.Errorf("failed to build subscription fetch request: %w", err)
	}

	// Add authentication header if API key is provided
	if m.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.apiKey)
		log.Printf("[DEBUG] Added Authorization header with API key")
	}

	log.Printf("[DEBUG] Sending request to subscription API...")
	resp, err := m.client.Do(req)
	if err != nil {
		log.Printf("[DEBUG] Request failed: %v", err)
		return fmt.Errorf("request to subscription API failed: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[DEBUG] Subscription API response status: %d", resp.StatusCode)
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		log.Printf("[DEBUG] Error response body: %s", string(body))
		return fmt.Errorf("subscription API responded with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload ModelListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		log.Printf("[DEBUG] Failed to decode response: %v", err)
		return fmt.Errorf("failed to decode subscription response: %w", err)
	}

	log.Printf("[DEBUG] Successfully decoded %d models from subscription API", len(payload.Models))
	for i, model := range payload.Models {
		log.Printf("[DEBUG] Model %d: ID=%s, Name=%s, Roles=%v, Status=%s",
			i, model.ID, model.Name, model.Roles, model.Status)
	}

	m.cacheMu.Lock()
	m.cached = payload.Models
	m.lastFetch = time.Now()
	m.cacheMu.Unlock()

	log.Printf("[SUBSCRIPTION] Cache refreshed with %d models", len(payload.Models))
	return nil
}