package router

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
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
	sessionTracker  *memory.SessionTracker
	db              *sql.DB
	circuitBreakers map[string]*CircuitBreaker
	healthMu        sync.RWMutex
	healthCache     map[string]*cachedHealth
}

type cachedHealth struct {
	healthy   bool
	checkedAt time.Time
}

// healthCacheTTLSeconds returns the health cache TTL from env or default (120s).
func healthCacheTTLSeconds() time.Duration {
	if v := os.Getenv("HEALTH_CACHE_TTL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 2 * time.Minute
}

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

// ProviderNames returns the ordered list of provider names in the chain.
func (r *Router) ProviderNames() []string {
	names := make([]string, len(r.providers))
	for i, p := range r.providers {
		names[i] = p.Name()
	}
	return names
}

// SetSessionTracker sets the session tracker for cross-session memory continuity.
func (r *Router) SetSessionTracker(st *memory.SessionTracker) {
	r.sessionTracker = st
}

// SessionTracker returns the router's session tracker (may be nil).
func (r *Router) SessionTracker() *memory.SessionTracker {
	return r.sessionTracker
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
	if cached, ok := r.healthCache[name]; ok && time.Since(cached.checkedAt) < healthCacheTTLSeconds() {
		r.healthMu.RUnlock()
		return cached.healthy
	}
	r.healthMu.RUnlock()

	// Cache miss or stale — do real check, then write
	healthy := p.IsHealthy(ctx)
	if !healthy {
		retryCtx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		healthy = p.IsHealthy(retryCtx)
		cancel()
		if healthy {
			log.Printf("[Router] %s health check recovered on retry", name)
		}
	}
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

	// Intent refinement: rewrite vague prompts using conversation context
	if !req.SkipMemory {
		if refined, _ := r.RefineIntent(ctx, &req, sessionID); refined {
			log.Printf("[Router] Refined vague prompt for session %s", sessionID)
		}
	}

	// Touch session as active for cross-session tracking
	if sessionID != "" && r.sessionTracker != nil {
		if err := r.sessionTracker.Touch(sessionID); err != nil {
			log.Printf("[Router] Warning: failed to touch session: %v", err)
		}
	}

	// Retrieve memory BEFORE storing new messages (to avoid retrieving what we just stored)
	injectedCount := 0
	memoryQuery := lastUserMessage(req.Messages)
	var retrievedMessages []memory.Message
	var metadata *providers.ProxyMetadata

	if sessionID != "" && !req.SkipMemory {
		relevant, err := r.vectorMemory.RetrieveRelevant(memoryQuery, sessionID, 4000)
		if err != nil {
			log.Printf("[Router] Warning: failed to retrieve memory: %v", err)
		} else {
			retrievedMessages = relevant

			// Cross-session fallback: carry over context from recently ended session
			if len(relevant) == 0 && r.sessionTracker != nil {
				endedSession, findErr := r.sessionTracker.FindRecentEnded(sessionID, 30*time.Minute)
				if findErr == nil && endedSession != "" {
					crossMsgs, crossErr := r.vectorMemory.RetrieveRecentFromSession(endedSession, 8)
					if crossErr == nil && len(crossMsgs) > 0 {
						retrievedMessages = crossMsgs
						log.Printf("[Router] Cross-session continuity: carried %d messages from ended session %s",
							len(crossMsgs), endedSession)
					}
				}
			}

			// Inject retrieved memory into request (prepend to provide context)
			// Filter out: tool-role messages (orphaned without tool_call_id),
			// empty messages (corrupted assistant messages that lost tool_calls).
			// Track actual injected count (not len(retrievedMessages)) for correct slice offset later.
			injectedCount = 0
			if len(retrievedMessages) > 0 {
				priorMessages := make([]providers.Message, 0, len(retrievedMessages)+len(req.Messages))
				for _, msg := range retrievedMessages {
					if msg.Role == "tool" || msg.Content == "" {
						continue
					}
					priorMessages = append(priorMessages, providers.Message{
						Role:    msg.Role,
						Content: msg.Content,
					})
					injectedCount++
				}
				priorMessages = append(priorMessages, req.Messages...)
				req.Messages = priorMessages
				log.Printf("[Router] Injected %d memory messages into request for session %s", injectedCount, sessionID)
			}

			// Build debug metadata if requested
			if includeMemoryDebug {
				candidates := make([]providers.Message, len(retrievedMessages))
				for i, msg := range retrievedMessages {
					candidates[i] = providers.Message{Role: msg.Role, Content: msg.Content}
				}
				metadata = &providers.ProxyMetadata{
					Provider:             "",
					SessionID:            sessionID,
					MemoryQuery:          memoryQuery,
					MemoryCandidateCount: len(retrievedMessages),
					MemoryCandidates:     candidates,
				}
			}
		}
	}

	// Skill-aware preprocessing: analyze conversation and inject skill context
	r.PreprocessRequest(&req)

	// Store the NEW messages — synthesize content for tool-call-only assistant messages
	// so memory preserves what the agent did without storing empty messages that corrupt conversations.
	if sessionID != "" && !req.SkipMemory {
		originalMsgCount := len(req.Messages) - injectedCount
		if originalMsgCount > 0 {
			msgsToStore := req.Messages[injectedCount:]
			var msgs []memory.Message
			for _, m := range msgsToStore {
				if m.Role == "tool" {
					continue // orphaned without tool_call_id context
				}
				content := m.Content
				if content == "" && len(m.ToolCalls) > 0 {
					content = summarizeToolCalls(m.ToolCalls)
				}
				if content == "" {
					continue
				}
				msgs = append(msgs, memory.Message{Role: m.Role, Content: content})
			}
			if len(msgs) > 0 {
				if err := r.vectorMemory.StoreMessages(msgs, sessionID); err != nil {
					log.Printf("[Router] Warning: failed to store messages: %v", err)
				}
			}
		}
	}

	provider, err := r.selectPreferredProvider(ctx, preferredProvider, req.Model)
	if err != nil {
		return providers.ChatResponse{}, err
	}

	startTime := time.Now()
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

	// Stall detection: if response took too long with minimal output, auto-continue
	resp, _ = r.handleStall(ctx, req, resp, provider, sessionID, time.Since(startTime), metadata)

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

	if metadata == nil {
		metadata = &providers.ProxyMetadata{}
	}
	metadata.Provider = provider.Name()
	resp.XProxyMetadata = metadata
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

	// Intent refinement: rewrite vague prompts using conversation context
	if !req.SkipMemory {
		if refined, _ := r.RefineIntent(ctx, &req, sessionID); refined {
			log.Printf("[Router] Refined vague prompt for session %s", sessionID)
		}
	}

	// Touch session as active for cross-session tracking
	if sessionID != "" && r.sessionTracker != nil {
		if err := r.sessionTracker.Touch(sessionID); err != nil {
			log.Printf("[Router] Warning: failed to touch session: %v", err)
		}
	}

	// 1. Retrieve memory BEFORE storing new messages
	injectedCount2 := 0
	memoryQuery := lastUserMessage(req.Messages)
	var retrievedMessages []memory.Message
	var metadata *providers.ProxyMetadata

	if sessionID != "" && !req.SkipMemory {
		relevant, err := r.vectorMemory.RetrieveRelevant(memoryQuery, sessionID, 4000)
		if err != nil {
			log.Printf("[Router] Warning: failed to retrieve memory: %v", err)
		} else {
			retrievedMessages = relevant

			// Cross-session fallback: carry over context from recently ended session
			if len(relevant) == 0 && r.sessionTracker != nil {
				endedSession, findErr := r.sessionTracker.FindRecentEnded(sessionID, 30*time.Minute)
				if findErr == nil && endedSession != "" {
					crossMsgs, crossErr := r.vectorMemory.RetrieveRecentFromSession(endedSession, 8)
					if crossErr == nil && len(crossMsgs) > 0 {
						retrievedMessages = crossMsgs
						log.Printf("[Router] Cross-session continuity: carried %d messages from ended session %s",
							len(crossMsgs), endedSession)
					}
				}
			}

			// Inject retrieved memory into request
			// Track actual injected count for correct slice offset in storage.
			if len(retrievedMessages) > 0 {
				priorMessages := make([]providers.Message, 0, len(retrievedMessages)+len(req.Messages))
				for _, msg := range retrievedMessages {
					if msg.Role == "tool" || msg.Content == "" {
						continue
					}
					priorMessages = append(priorMessages, providers.Message{
						Role:    msg.Role,
						Content: msg.Content,
					})
					injectedCount2++
				}
				priorMessages = append(priorMessages, req.Messages...)
				req.Messages = priorMessages
				log.Printf("[Router] Injected %d memory messages into request for session %s", injectedCount2, sessionID)
			}

			// Build debug metadata if requested
			if includeMemoryDebug {
				candidates := make([]providers.Message, len(retrievedMessages))
				for i, msg := range retrievedMessages {
					candidates[i] = providers.Message{Role: msg.Role, Content: msg.Content}
				}
				metadata = &providers.ProxyMetadata{
					Provider:             "",
					SessionID:            sessionID,
					MemoryQuery:          memoryQuery,
					MemoryCandidateCount: len(retrievedMessages),
					MemoryCandidates:     candidates,
				}
			}
		}
	}

	// Skill-aware preprocessing: analyze conversation and inject skill context
	r.PreprocessRequest(&req)

	// 2. Store the NEW messages — synthesize content for tool-call-only messages
	if sessionID != "" && !req.SkipMemory {
		originalMsgCount := len(req.Messages) - injectedCount2
		if originalMsgCount > 0 {
			msgsToStore := req.Messages[injectedCount2:]
			var msgs []memory.Message
			for _, m := range msgsToStore {
				if m.Role == "tool" {
					continue
				}
				content := m.Content
				if content == "" && len(m.ToolCalls) > 0 {
					content = summarizeToolCalls(m.ToolCalls)
				}
				if content == "" {
					continue
				}
				msgs = append(msgs, memory.Message{Role: m.Role, Content: content})
			}
			if len(msgs) > 0 {
				if err := r.vectorMemory.StoreMessages(msgs, sessionID); err != nil {
					log.Printf("[Router] Warning: failed to store messages: %v", err)
				}
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
	startTime := time.Now()
	resp, finalProvider, attempts, err := r.tryProvidersWithFallback(ctx, req, provider, sessionID)
	if err != nil {
		r.persistAudit(requestID, sessionID, provider.Name(), "", "", memoryQuery, metadata, attempts, err)
		return providers.ChatResponse{}, err
	}

	// Stall detection: if response took too long with minimal output, auto-continue
	if finalProv := r.findProviderByName(finalProvider); finalProv != nil {
		resp, _ = r.handleStall(ctx, req, resp, finalProv, sessionID, time.Since(startTime), metadata)
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

// ChatCompletionStreamForProvider routes a streaming request to the named provider.
// If the provider supports StreamingProvider, uses SSE streaming.
// Otherwise falls back to non-streaming ChatCompletion.
func (r *Router) ChatCompletionStreamForProvider(
	ctx context.Context,
	req providers.ChatRequest,
	sessionID string,
	provider string,
	onToken providers.TokenCallback,
) (providers.ChatResponse, error) {
	p, err := r.selectPreferredProvider(ctx, provider, req.Model)
	if err != nil {
		return providers.ChatResponse{}, err
	}

	// Check if provider supports streaming
	if sp, ok := p.(providers.StreamingProvider); ok {
		resp, err := sp.ChatCompletionStream(ctx, req, sessionID, onToken)
		if err == nil {
			r.circuitBreakers[p.Name()].RecordSuccess()
			return resp, nil
		}
		log.Printf("[Router] streaming failed for %s, falling back: %v", p.Name(), err)
	}

	// Fallback to non-streaming
	return p.ChatCompletion(ctx, req, sessionID)
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

	// Pre-check: skip if request would exceed provider's context limit
	maxCtx := provider.MaxContextTokens()
	if maxCtx > 0 {
		estimated := estimateTokens(req.Messages)
		if estimated > maxCtx {
			return providers.ChatResponse{}, fmt.Errorf(
				"request too large for %s: ~%d tokens exceeds %d token context limit",
				provider.Name(), estimated, maxCtx)
		}
	}

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

	// Track usage — estimate if provider doesn't report tokens
	tokens := resp.Usage.TotalTokens
	estimated := false
	if tokens == 0 {
		tokens = estimateTokens(req.Messages)
		if len(resp.Choices) > 0 {
			tokens += len(resp.Choices[0].Message.Content) / 4
		}
		estimated = true
	}
	if tokens > 0 {
		if err := r.usageTracker.RecordUsage(
			provider.Name(),
			int64(tokens),
			resp.ID,
			resp.Model,
		); err != nil {
			log.Printf("[Router] Warning: failed to record usage: %v", err)
		}
	}

	suffix := ""
	if estimated {
		suffix = " estimated"
	}
	log.Printf("[Router] ✓ Success with %s (tokens: %d%s)", provider.Name(), tokens, suffix)
	return resp, nil
}

const stallContinuePrompt = "Continue where you left off. If you were waiting on a long-running operation, report what you have so far and suggest next steps."

func stallTimeoutSeconds() int {
	v := os.Getenv("STALL_TIMEOUT_SECONDS")
	if v == "" {
		return 180
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 180
	}
	return n
}

// handleStall checks if a response looks stalled (took too long, very short output)
// and retries once with a continuation prompt. Returns the original or retried response
// and updates metadata flags.
func (r *Router) handleStall(
	ctx context.Context,
	req providers.ChatRequest,
	resp providers.ChatResponse,
	provider providers.Provider,
	sessionID string,
	elapsed time.Duration,
	metadata *providers.ProxyMetadata,
) (providers.ChatResponse, bool) {
	threshold := time.Duration(stallTimeoutSeconds()) * time.Second
	if elapsed < threshold {
		return resp, false
	}

	// Check if the response is substantive
	responseLen := 0
	if len(resp.Choices) > 0 {
		responseLen = len(resp.Choices[0].Message.Content)
	}
	if responseLen >= 50 {
		return resp, false
	}

	log.Printf("[Router] Stall detected: %s took %s with only %d chars output, retrying with continuation prompt",
		provider.Name(), elapsed, responseLen)

	if metadata != nil {
		metadata.StallDetected = true
	}

	// Append continuation prompt and retry
	retryReq := req
	retryReq.Messages = append(append([]providers.Message{}, req.Messages...), providers.Message{
		Role:    "user",
		Content: stallContinuePrompt,
	})
	retryReq.SkipSkillPreprocess = true
	retryReq.SkipMemory = true

	retryResp, err := provider.ChatCompletion(ctx, retryReq, sessionID)
	if err != nil {
		log.Printf("[Router] Stall retry failed: %v, returning original response", err)
		return resp, true
	}

	if metadata != nil {
		metadata.StallRetried = true
	}

	if len(retryResp.Choices) > 0 {
		log.Printf("[Router] Stall retry succeeded with %d chars", len(retryResp.Choices[0].Message.Content))
	} else {
		log.Printf("[Router] Stall retry returned empty choices")
	}
	return retryResp, true
}

func (r *Router) findProviderByName(name string) providers.Provider {
	for _, p := range r.providers {
		if p.Name() == name {
			return p
		}
	}
	return nil
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

// summarizeToolCalls creates a text summary of tool calls for memory storage.
// When an assistant message has empty content but tool calls, this preserves
// what the agent did (e.g., "[Tool calls: bash(npm install), file_write(src/app.ts)]").
func summarizeToolCalls(toolCalls []map[string]interface{}) string {
	var parts []string
	for _, tc := range toolCalls {
		fn, _ := tc["function"].(map[string]interface{})
		name := ""
		if fn != nil {
			name, _ = fn["name"].(string)
		}
		if name == "" {
			name, _ = tc["name"].(string)
		}
		if name == "" {
			name = "unknown"
		}

		// Extract a brief args summary
		argsSummary := ""
		var argsRaw interface{}
		if fn != nil {
			argsRaw = fn["arguments"]
		}
		if argsRaw == nil {
			argsRaw = tc["arguments"]
		}
		switch v := argsRaw.(type) {
		case string:
			if len(v) > 100 {
				argsSummary = v[:100]
			} else {
				argsSummary = v
			}
		case map[string]interface{}:
			// Extract first meaningful arg
			for k, val := range v {
				s := fmt.Sprintf("%v", val)
				if len(s) > 80 {
					s = s[:80]
				}
				argsSummary = k + "=" + s
				break
			}
		}

		if argsSummary != "" {
			parts = append(parts, fmt.Sprintf("%s(%s)", name, argsSummary))
		} else {
			parts = append(parts, name)
		}
	}
	return "[Tool calls: " + strings.Join(parts, ", ") + "]"
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

// StartHealthMonitor launches a background goroutine that periodically checks
// circuit-open providers and resets them if they become healthy again.
func (r *Router) StartHealthMonitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				for _, p := range r.providers {
					name := p.Name()
					cb := r.circuitBreakers[name]
					if !cb.IsOpen() {
						continue
					}
					checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
					if p.IsHealthy(checkCtx) {
						if err := cb.Reset(); err == nil {
							log.Printf("[Router] health monitor: %s recovered, circuit reset", name)
							r.healthMu.Lock()
							r.healthCache[name] = &cachedHealth{healthy: true, checkedAt: time.Now()}
							r.healthMu.Unlock()
						}
					}
					cancel()
				}
			}
		}
	}()
	log.Printf("[Router] health monitor started (30s interval)")
}
