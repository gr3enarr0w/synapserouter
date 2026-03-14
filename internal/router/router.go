package router

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/usage"
)

type Router struct {
	providers       []providers.Provider
	usageTracker    *usage.Tracker
	vectorMemory    *memory.VectorMemory
	db              *sql.DB
	circuitBreakers map[string]*CircuitBreaker
	healthMu        sync.RWMutex
	healthCache     map[string]*cachedHealth
}

type cachedHealth struct {
	healthy   bool
	checkedAt time.Time
}

const healthCacheTTL = 2 * time.Minute

type providerAttempt struct {
	Provider string
	Index    int
	Success  bool
	Error    string
}

type ProviderTrace struct {
	Provider       string       `json:"provider"`
	UsagePercent   float64      `json:"usage_percent,omitempty"`
	CurrentUsage   int64        `json:"current_usage,omitempty"`
	DailyLimit     int64        `json:"daily_limit,omitempty"`
	CircuitState   CircuitState `json:"circuit_state"`
	Healthy        bool         `json:"healthy"`
	Selected       bool         `json:"selected"`
	SelectionCause string       `json:"selection_cause,omitempty"`
}

type DecisionTrace struct {
	SessionID            string          `json:"session_id"`
	MemoryQuery          string          `json:"memory_query"`
	MemoryCandidateCount int             `json:"memory_candidate_count"`
	SelectedProvider     string          `json:"selected_provider,omitempty"`
	Providers            []ProviderTrace `json:"providers"`
}

func NewRouter(
	providerList []providers.Provider,
	tracker *usage.Tracker,
	vm *memory.VectorMemory,
	db *sql.DB,
) *Router {
	// Initialize circuit breakers for each provider
	breakers := make(map[string]*CircuitBreaker)
	for _, p := range providerList {
		breakers[p.Name()] = NewCircuitBreaker(db, p.Name())
	}

	return &Router{
		providers:       providerList,
		usageTracker:    tracker,
		vectorMemory:    vm,
		db:              db,
		circuitBreakers: breakers,
		healthCache:     make(map[string]*cachedHealth),
	}
}

// isHealthyCached returns cached health status to avoid burning provider API quota
// on health checks. A successful request resets the cache; a circuit breaker open
// overrides the cache. Only falls through to a real IsHealthy call if the cache is
// stale (older than healthCacheTTL).
func (r *Router) isHealthyCached(ctx context.Context, p providers.Provider) bool {
	name := p.Name()

	// Circuit breaker open = unhealthy, no API call needed
	if r.circuitBreakers[name].IsOpen() {
		return false
	}

	// Check cache (read lock)
	r.healthMu.RLock()
	if cached, ok := r.healthCache[name]; ok && time.Since(cached.checkedAt) < healthCacheTTL {
		r.healthMu.RUnlock()
		return cached.healthy
	}
	r.healthMu.RUnlock()

	// Cache miss or stale — do real check, then write
	healthy := p.IsHealthy(ctx)
	r.healthMu.Lock()
	r.healthCache[name] = &cachedHealth{healthy: healthy, checkedAt: time.Now()}
	r.healthMu.Unlock()
	return healthy
}

func (r *Router) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	return r.ChatCompletionWithDebug(ctx, req, sessionID, false)
}

func (r *Router) ChatCompletionForProvider(
	ctx context.Context,
	req providers.ChatRequest,
	sessionID string,
	preferredProvider string,
	includeMemoryDebug bool,
) (providers.ChatResponse, error) {
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	// Retrieve memory BEFORE storing new messages (to avoid retrieving what we just stored)
	memoryQuery := lastUserMessage(req.Messages)
	var retrievedMessages []memory.Message
	var metadata *providers.ProxyMetadata

	if sessionID != "" && !req.SkipMemory {
		relevant, err := r.vectorMemory.RetrieveRelevant(memoryQuery, sessionID, 4000)
		if err != nil {
			log.Printf("[Router] Warning: failed to retrieve memory: %v", err)
		} else {
			retrievedMessages = relevant

			// Inject retrieved memory into request (prepend to provide context)
			if len(relevant) > 0 {
				priorMessages := make([]providers.Message, 0, len(relevant)+len(req.Messages))
				for _, msg := range relevant {
					priorMessages = append(priorMessages, providers.Message{
						Role:    msg.Role,
						Content: msg.Content,
					})
				}
				priorMessages = append(priorMessages, req.Messages...)
				req.Messages = priorMessages
				log.Printf("[Router] Injected %d memory messages into request for session %s", len(relevant), sessionID)
			}

			// Build debug metadata if requested
			if includeMemoryDebug {
				candidates := make([]providers.Message, len(relevant))
				for i, msg := range relevant {
					candidates[i] = providers.Message{Role: msg.Role, Content: msg.Content}
				}
				metadata = &providers.ProxyMetadata{
					Provider:             "",
					SessionID:            sessionID,
					MemoryQuery:          memoryQuery,
					MemoryCandidateCount: len(relevant),
					MemoryCandidates:     candidates,
				}
			}
		}
	}

	// Store the NEW messages
	if sessionID != "" && !req.SkipMemory {
		// Normal case: store everything after retrieved memory
		originalMsgCount := len(req.Messages) - len(retrievedMessages)
		if originalMsgCount > 0 {
			msgsToStore := req.Messages[len(retrievedMessages):]
			msgs := make([]memory.Message, len(msgsToStore))
			for i, m := range msgsToStore {
				msgs[i] = memory.Message{Role: m.Role, Content: m.Content}
			}
			if err := r.vectorMemory.StoreMessages(msgs, sessionID); err != nil {
				log.Printf("[Router] Warning: failed to store messages: %v", err)
			}
		}
	}

	provider, err := r.selectPreferredProvider(ctx, preferredProvider, req.Model)
	if err != nil {
		return providers.ChatResponse{}, err
	}

	resp, err := r.tryProvider(ctx, provider, req, sessionID)
	attempts := []providerAttempt{{
		Provider: provider.Name(),
		Index:    1,
		Success:  err == nil,
		Error:    stringifyError(err),
	}}
	if err != nil {
		r.persistAudit(requestID, sessionID, provider.Name(), "", "", memoryQuery, metadata, attempts, err)
		return providers.ChatResponse{}, err
	}

	// Store the assistant response in memory for continuity
	if sessionID != "" && !req.SkipMemory && len(resp.Choices) > 0 {
		assistantMsg := resp.Choices[0].Message
		if assistantMsg.Content != "" {
			if err := r.vectorMemory.Store(assistantMsg.Content, "assistant", sessionID, map[string]interface{}{
				"model":     resp.Model,
				"provider":  provider.Name(),
				"timestamp": time.Now().Unix(),
			}); err != nil {
				log.Printf("[Router] Warning: failed to store assistant response: %v", err)
			}
		}
	}

	if metadata != nil {
		metadata.Provider = provider.Name()
		resp.XProxyMetadata = metadata
	}
	r.persistAudit(requestID, sessionID, provider.Name(), provider.Name(), resp.Model, memoryQuery, metadata, attempts, nil)
	return resp, nil
}

func (r *Router) TraceDecision(ctx context.Context, req providers.ChatRequest, sessionID string) (*DecisionTrace, error) {
	memoryQuery := lastUserMessage(req.Messages)
	relevant, err := r.vectorMemory.RetrieveRelevant(memoryQuery, sessionID, 4000)
	if err != nil {
		return nil, err
	}

	quotas, err := r.usageTracker.GetAllQuotas()
	if err != nil {
		return nil, err
	}

	trace := &DecisionTrace{
		SessionID:            sessionID,
		MemoryQuery:          memoryQuery,
		MemoryCandidateCount: len(relevant),
		Providers:            make([]ProviderTrace, 0, len(r.providers)),
	}

	selectedName := ""
	lowestUsage := 2.0
	fallbackName := ""
	for _, provider := range r.providers {
		entry := ProviderTrace{
			Provider: provider.Name(),
			Healthy:  provider.IsHealthy(ctx),
		}

		if cb, ok := r.circuitBreakers[provider.Name()]; ok {
			if state, err := cb.GetState(); err == nil {
				entry.CircuitState = state
			}
		}

		if quota, ok := quotas[provider.Name()]; ok {
			entry.UsagePercent = quota.UsagePercent
			entry.CurrentUsage = quota.CurrentUsage
			entry.DailyLimit = quota.DailyLimit

			if quota.UsagePercent < lowestUsage {
				lowestUsage = quota.UsagePercent
				fallbackName = provider.Name()
			}
			if selectedName == "" && quota.UsagePercent < r.usageTracker.WarningThreshold() {
				selectedName = provider.Name()
			}
		}

		trace.Providers = append(trace.Providers, entry)
	}

	if selectedName == "" {
		selectedName = fallbackName
	}

	for i := range trace.Providers {
		entry := &trace.Providers[i]
		if entry.Provider != selectedName {
			continue
		}
		if entry.CircuitState == StateOpen {
			entry.SelectionCause = "lowest usage but circuit open"
			break
		}
		if !entry.Healthy {
			entry.SelectionCause = "lowest usage but unhealthy"
			break
		}
		entry.Selected = true
		if entry.UsagePercent < r.usageTracker.WarningThreshold() {
			entry.SelectionCause = "under warning threshold with lowest eligible usage"
		} else {
			entry.SelectionCause = "lowest usage fallback"
		}
		trace.SelectedProvider = entry.Provider
		break
	}

	if trace.SelectedProvider == "" {
		for i := range trace.Providers {
			entry := &trace.Providers[i]
			if entry.CircuitState == StateOpen || !entry.Healthy {
				continue
			}
			entry.Selected = true
			entry.SelectionCause = "first healthy fallback after selection filter"
			trace.SelectedProvider = entry.Provider
			break
		}
	}

	return trace, nil
}

func (r *Router) ChatCompletionWithDebug(
	ctx context.Context,
	req providers.ChatRequest,
	sessionID string,
	includeMemoryDebug bool,
) (providers.ChatResponse, error) {
	requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	// 1. Retrieve memory BEFORE storing new messages
	memoryQuery := lastUserMessage(req.Messages)
	var retrievedMessages []memory.Message
	var metadata *providers.ProxyMetadata

	if sessionID != "" && !req.SkipMemory {
		relevant, err := r.vectorMemory.RetrieveRelevant(memoryQuery, sessionID, 4000)
		if err != nil {
			log.Printf("[Router] Warning: failed to retrieve memory: %v", err)
		} else {
			retrievedMessages = relevant

			// Inject retrieved memory into request
			if len(relevant) > 0 {
				priorMessages := make([]providers.Message, 0, len(relevant)+len(req.Messages))
				for _, msg := range relevant {
					priorMessages = append(priorMessages, providers.Message{
						Role:    msg.Role,
						Content: msg.Content,
					})
				}
				priorMessages = append(priorMessages, req.Messages...)
				req.Messages = priorMessages
				log.Printf("[Router] Injected %d memory messages into request for session %s", len(relevant), sessionID)
			}

			// Build debug metadata if requested
			if includeMemoryDebug {
				candidates := make([]providers.Message, len(relevant))
				for i, msg := range relevant {
					candidates[i] = providers.Message{Role: msg.Role, Content: msg.Content}
				}
				metadata = &providers.ProxyMetadata{
					Provider:             "",
					SessionID:            sessionID,
					MemoryQuery:          memoryQuery,
					MemoryCandidateCount: len(relevant),
					MemoryCandidates:     candidates,
				}
			}
		}
	}

	// 2. Store the NEW messages
	if sessionID != "" && !req.SkipMemory {
		// Normal case: store everything after retrieved memory
		originalMsgCount := len(req.Messages) - len(retrievedMessages)
		if originalMsgCount > 0 {
			msgsToStore := req.Messages[len(retrievedMessages):]
			msgs := make([]memory.Message, len(msgsToStore))
			for i, m := range msgsToStore {
				msgs[i] = memory.Message{Role: m.Role, Content: m.Content}
			}
			if err := r.vectorMemory.StoreMessages(msgs, sessionID); err != nil {
				log.Printf("[Router] Warning: failed to store messages: %v", err)
			}
		}
	}

	// 3. Select provider based on availability (not context size)
	provider, err := r.selectProvider(ctx, req.Model)
	if err != nil {
		return providers.ChatResponse{}, err
	}

	log.Printf("[Router] Selected provider: %s", provider.Name())

	// 3. Try selected provider with fallback using the original request body.
	resp, finalProvider, attempts, err := r.tryProvidersWithFallback(ctx, req, provider, sessionID)
	if err != nil {
		r.persistAudit(requestID, sessionID, provider.Name(), "", "", memoryQuery, metadata, attempts, err)
		return providers.ChatResponse{}, err
	}

	// Store the assistant response in memory for continuity
	if sessionID != "" && !req.SkipMemory && len(resp.Choices) > 0 {
		assistantMsg := resp.Choices[0].Message
		if assistantMsg.Content != "" {
			if err := r.vectorMemory.Store(assistantMsg.Content, "assistant", sessionID, map[string]interface{}{
				"model":     resp.Model,
				"provider":  finalProvider,
				"timestamp": time.Now().Unix(),
			}); err != nil {
				log.Printf("[Router] Warning: failed to store assistant response: %v", err)
			}
		}
	}

	if metadata != nil {
		metadata.Provider = finalProvider
		resp.XProxyMetadata = metadata
	}
	r.persistAudit(requestID, sessionID, provider.Name(), finalProvider, resp.Model, memoryQuery, metadata, attempts, nil)
	return resp, nil
}

func (r *Router) selectProvider(ctx context.Context, model string) (providers.Provider, error) {
	// Build list of provider names that support this model
	var candidates []string
	var candidateObjects []providers.Provider

	for _, p := range r.providers {
		// If model is empty or 'auto', all providers are candidates
		if model == "" || strings.EqualFold(model, "auto") || p.SupportsModel(model) {
			candidates = append(candidates, p.Name())
			candidateObjects = append(candidateObjects, p)
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no providers support model: %s", model)
	}

	// Get provider with lowest usage among candidates
	bestName, err := r.usageTracker.GetBestProvider(candidates)
	if err != nil {
		// If usage tracking fails, try first healthy candidate
		log.Printf("[Router] Usage tracking failed: %v, trying first healthy candidate", err)
		return r.findFirstHealthyCandidate(ctx, candidateObjects)
	}

	// Find the provider object
	for _, p := range candidateObjects {
		if p.Name() == bestName {
			// Check circuit breaker
			if r.circuitBreakers[bestName].IsOpen() {
				log.Printf("[Router] %s circuit breaker is open, trying fallback", bestName)
				return r.findFirstHealthyCandidate(ctx, candidateObjects)
			}
			return p, nil
		}
	}

	return r.findFirstHealthyCandidate(ctx, candidateObjects)
}

func (r *Router) selectPreferredProvider(ctx context.Context, preferredProvider, model string) (providers.Provider, error) {
	preferredProvider = strings.TrimSpace(strings.ToLower(preferredProvider))
	if preferredProvider == "" {
		return r.selectProvider(ctx, model)
	}

	for _, p := range r.providers {
		if strings.EqualFold(p.Name(), preferredProvider) {
			if r.circuitBreakers[p.Name()].IsOpen() {
				return nil, fmt.Errorf("preferred provider %s is unavailable (circuit open)", preferredProvider)
			}
			if r.isHealthyCached(ctx, p) {
				return p, nil
			}
			return nil, fmt.Errorf("preferred provider %s is unavailable (unhealthy)", preferredProvider)
		}
	}

	return nil, fmt.Errorf("preferred provider %s not found", preferredProvider)
}

func (r *Router) findFirstHealthyCandidate(ctx context.Context, candidates []providers.Provider) (providers.Provider, error) {
	for _, p := range candidates {
		// Skip if circuit breaker is open
		if r.circuitBreakers[p.Name()].IsOpen() {
			log.Printf("[Router] Skipping %s (circuit breaker open)", p.Name())
			continue
		}

		// Check health (cached to avoid burning API quota)
		if r.isHealthyCached(ctx, p) {
			return p, nil
		}
	}
	return nil, errors.New("no healthy providers available for this model")
}

func (r *Router) tryProvidersWithFallback(
	ctx context.Context,
	req providers.ChatRequest,
	primary providers.Provider,
	sessionID string,
) (providers.ChatResponse, string, []providerAttempt, error) {
	attempts := []providerAttempt{}

	// 1. Try primary first
	resp, err := r.tryProvider(ctx, primary, req, sessionID)
	attempts = append(attempts, providerAttempt{
		Provider: primary.Name(),
		Index:    len(attempts) + 1,
		Success:  err == nil,
		Error:    stringifyError(err),
	})
	if err == nil {
		return resp, primary.Name(), attempts, nil
	}

	log.Printf("[Router] Primary provider %s failed: %v, trying fallback", primary.Name(), err)

	// 2. Build list of candidate fallbacks (healthy & circuit closed)
	candidates := make([]string, 0, len(r.providers))
	for _, p := range r.providers {
		if p.Name() == primary.Name() {
			continue
		}
		if r.circuitBreakers[p.Name()].IsOpen() {
			continue
		}
		if !r.isHealthyCached(ctx, p) {
			continue
		}
		candidates = append(candidates, p.Name())
	}

	// 3. Sort candidates by usage if possible
	var orderedCandidates []string
	if bestOrdered, err := r.usageTracker.GetBestProvider(candidates); err == nil && bestOrdered != "" {
		// usageTracker.GetBestProvider currently returns just ONE best name.
		// For true multi-fallback, we should ideally get a sorted list.
		// For now, we'll try the 'best' one first, then the rest.
		orderedCandidates = append(orderedCandidates, bestOrdered)
		for _, name := range candidates {
			if name != bestOrdered {
				orderedCandidates = append(orderedCandidates, name)
			}
		}
	} else {
		orderedCandidates = candidates
	}

	// 4. Try fallbacks in order
	for _, name := range orderedCandidates {
		var provider providers.Provider
		for _, p := range r.providers {
			if p.Name() == name {
				provider = p
				break
			}
		}
		if provider == nil {
			continue
		}

		resp, err := r.tryProvider(ctx, provider, req, sessionID)
		attempts = append(attempts, providerAttempt{
			Provider: provider.Name(),
			Index:    len(attempts) + 1,
			Success:  err == nil,
			Error:    stringifyError(err),
		})
		if err == nil {
			return resp, provider.Name(), attempts, nil
		}

		log.Printf("[Router] Fallback provider %s failed: %v", provider.Name(), err)
	}

	return providers.ChatResponse{}, "", attempts, errors.New("all providers failed")
}

func (r *Router) tryProvider(
	ctx context.Context,
	provider providers.Provider,
	req providers.ChatRequest,
	sessionID string,
) (providers.ChatResponse, error) {
	log.Printf("[Router] Trying %s...", provider.Name())

	resp, err := provider.ChatCompletion(ctx, req, sessionID)
	if err != nil {
		// Record failure
		r.circuitBreakers[provider.Name()].RecordFailure()

		// Check if rate limit error — use shorter cooldown for providers with fast resets
		if isRateLimitError(err) {
			cooldown := rateLimitCooldown(provider.Name(), err)
			log.Printf("[Router] %s rate limited, opening circuit for %s", provider.Name(), cooldown)
			r.circuitBreakers[provider.Name()].Open(cooldown)
		}

		return providers.ChatResponse{}, err
	}

	// Record success and update health cache
	r.circuitBreakers[provider.Name()].RecordSuccess()
	r.healthMu.Lock()
	r.healthCache[provider.Name()] = &cachedHealth{healthy: true, checkedAt: time.Now()}
	r.healthMu.Unlock()

	// Track usage
	if resp.Usage.TotalTokens > 0 {
		if err := r.usageTracker.RecordUsage(
			provider.Name(),
			int64(resp.Usage.TotalTokens),
			resp.ID,
			resp.Model,
		); err != nil {
			log.Printf("[Router] Warning: failed to record usage: %v", err)
		}
	}

	log.Printf("[Router] ✓ Success with %s (tokens: %d)", provider.Name(), resp.Usage.TotalTokens)
	return resp, nil
}

func estimateTokens(messages []providers.Message) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4 // Rough estimate: 1 token ≈ 4 chars
	}
	return total
}

func rateLimitCooldown(providerName string, err error) time.Duration {
	errStr := err.Error()
	// Try to extract "reset after Ns" from Gemini-style errors
	if idx := strings.Index(errStr, "reset after "); idx >= 0 {
		numStr := ""
		for _, c := range errStr[idx+len("reset after "):] {
			if c >= '0' && c <= '9' {
				numStr += string(c)
			} else {
				break
			}
		}
		if seconds, parseErr := strconv.Atoi(numStr); parseErr == nil && seconds > 0 {
			// Add a small buffer to the provider's stated reset time
			return time.Duration(seconds+5) * time.Second
		}
	}
	// Gemini and subscription providers reset quickly; use shorter cooldown
	switch providerName {
	case "gemini":
		return 30 * time.Second
	case "claude-code":
		return 60 * time.Second
	default:
		return 2 * time.Minute
	}
}

func isRateLimitError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "rate limit") ||
		strings.Contains(errStr, "429") ||
		strings.Contains(errStr, "quota exceeded") ||
		strings.Contains(errStr, "too many requests")
}

func isContextError(err error) bool {
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "context") ||
		strings.Contains(errStr, "too long") ||
		strings.Contains(errStr, "maximum context") ||
		strings.Contains(errStr, "token limit")
}

func lastUserMessage(messages []providers.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	if len(messages) == 0 {
		return ""
	}
	return messages[len(messages)-1].Content
}

func stringifyError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (r *Router) persistAudit(
	requestID string,
	sessionID string,
	selectedProvider string,
	finalProvider string,
	finalModel string,
	memoryQuery string,
	metadata *providers.ProxyMetadata,
	attempts []providerAttempt,
	err error,
) {
	if r.db == nil {
		return
	}

	memoryCandidateCount := 0
	if metadata != nil {
		memoryCandidateCount = metadata.MemoryCandidateCount
	}

	success := err == nil
	if _, execErr := r.db.Exec(`
		INSERT INTO request_audit (
			request_id, session_id, selected_provider, final_provider, final_model,
			memory_query, memory_candidate_count, success, error_message, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, requestID, sessionID, selectedProvider, finalProvider, finalModel,
		memoryQuery, memoryCandidateCount, boolToInt(success), stringifyError(err), time.Now()); execErr != nil {
		log.Printf("[Router] Warning: failed to persist request audit: %v", execErr)
		return
	}

	for _, attempt := range attempts {
		if _, execErr := r.db.Exec(`
			INSERT INTO provider_attempt_audit (
				request_id, provider, attempt_index, success, error_message, created_at
			) VALUES (?, ?, ?, ?, ?, ?)
		`, requestID, attempt.Provider, attempt.Index, boolToInt(attempt.Success), attempt.Error, time.Now()); execErr != nil {
			log.Printf("[Router] Warning: failed to persist provider attempt audit: %v", execErr)
		}
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
