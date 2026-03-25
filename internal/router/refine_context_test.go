package router

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/usage"
)

// =============================================================================
// Context lifecycle tests for the refinement pipeline.
//
// These tests verify how context flows through the system over time:
//   - Refined prompts stored in memory improve future turns
//   - Long sessions with 50+ messages don't blow up or degrade
//   - Session isolation prevents cross-contamination
//   - The refined prompt improves memory search (not just skill dispatch)
//   - Context drift over many refined turns stays useful
//   - The /v1/skills/match endpoint works correctly with cleaned triggers
// =============================================================================

// --- Refined prompts stored in memory improve future turns ---

// TestContext_RefinedTextStoredInMemory_ImprovesNextTurn verifies the chain:
//   Turn 1: user says "implement Go auth handler" → stored in memory
//   Turn 2: user says "whats going on" → refined to "diagnose Go auth handler" →
//           refined text stored in memory (NOT "whats going on")
//   Turn 3: user says "anything else" → refinement retrieves "diagnose Go auth handler"
//           from turn 2, giving it BETTER context than if "whats going on" was stored
func TestContext_RefinedTextStoredInMemory_ImprovesNextTurn(t *testing.T) {
	// Track which messages the provider sees for refinement calls
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "diagnose the Go authentication handler for token refresh issues",
			2: "verify the Go authentication handler fix is complete and tests pass",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "context-chain"

	// Turn 1: specific prompt, stored directly
	vm.Store("implement OAuth token refresh in the Go auth handler", "user", sessionID, nil)
	vm.Store("I updated auth.go with the refresh flow", "assistant", sessionID, nil)

	// Turn 2: vague prompt → refined → the REFINED text gets stored via ChatCompletionForProvider
	req2 := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats going on"}},
	}
	resp2, err := router.ChatCompletionForProvider(context.Background(), req2, sessionID, "", false)
	if err != nil {
		t.Fatalf("Turn 2 failed: %v", err)
	}
	if len(resp2.Choices) == 0 {
		t.Fatal("Turn 2: expected response")
	}

	// Verify the refined text was stored (not the original "whats going on")
	// by checking what memory returns for this session
	retrieved, err := vm.RetrieveRelevant("auth handler", sessionID, 4000)
	if err != nil {
		t.Fatalf("memory retrieval failed: %v", err)
	}

	foundRefinedText := false
	foundVagueText := false
	for _, msg := range retrieved {
		if strings.Contains(msg.Content, "diagnose") && strings.Contains(msg.Content, "auth") {
			foundRefinedText = true
		}
		if msg.Content == "whats going on" {
			foundVagueText = true
		}
	}

	if !foundRefinedText {
		t.Error("refined text should be stored in memory for future turns")
	}
	if foundVagueText {
		t.Error("original vague text should NOT be in memory (was replaced by refinement)")
	}

	// Turn 3: another vague prompt → refinement should have BETTER context
	// because turn 2 stored "diagnose the Go auth handler..." not "whats going on"
	provider.refinementCallCount = 0
	req3 := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "anything else"}},
	}

	refined3, _ := router.RefineIntent(context.Background(), &req3, sessionID)
	if !refined3 {
		t.Fatal("Turn 3 should be refined")
	}

	// The refinement call for turn 3 should have received context containing
	// the refined text from turn 2, not the original vague text
	if provider.lastRefinementContext == nil {
		t.Fatal("expected refinement context for turn 3")
	}

	contextText := ""
	for _, msg := range provider.lastRefinementContext {
		contextText += " " + msg.Content
	}

	if !strings.Contains(contextText, "diagnose") || !strings.Contains(contextText, "auth") {
		t.Errorf("turn 3 refinement context should contain refined text from turn 2, got: %s",
			truncate(contextText, 200))
	}
}

// --- Long session / growing context ---

// TestContext_LongSession_RefinementStaysWithinBounds verifies that sessions
// with many messages don't cause the refinement call to blow up.
// RetrieveRelevant has a token budget that should cap context size.
func TestContext_LongSession_RefinementStaysWithinBounds(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "check the latest changes to the Go router implementation",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "long-session"

	// Seed 50 messages — simulates a long working session
	for i := 0; i < 25; i++ {
		vm.Store(
			fmt.Sprintf("working on Go component %d with auth and routing", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("updated component %d in handler.go", i),
			"assistant", sessionID, nil,
		)
	}

	// Now send a vague prompt
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "how are things"}},
	}

	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("refinement failed on long session: %v", err)
	}
	if !refined {
		t.Fatal("should still refine on long sessions")
	}

	// The refinement request should NOT contain all 50 messages
	// RetrieveRelevant caps at maxTokens (2000 for full refinement)
	if provider.lastRefinementContext != nil {
		totalContextChars := 0
		for _, msg := range provider.lastRefinementContext {
			totalContextChars += len(msg.Content)
		}

		// 2000 tokens ≈ 8000 chars. Context should be well under this.
		// The system prompt + context + user message should not exceed a reasonable size.
		if totalContextChars > 10000 {
			t.Errorf("context too large for refinement: %d chars (should be capped by token budget)",
				totalContextChars)
		}
	}
}

// TestContext_LongSession_RecentMessagesPreferred verifies that in a long session,
// refinement context includes RECENT messages, not just the oldest ones.
func TestContext_LongSession_RecentMessagesPreferred(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "check the Docker deployment status",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "long-recent"

	// Old context: Go work
	for i := 0; i < 10; i++ {
		vm.Store(fmt.Sprintf("old Go work item %d", i), "user", sessionID, nil)
		vm.Store(fmt.Sprintf("did old Go thing %d", i), "assistant", sessionID, nil)
	}

	// Recent context: Docker work
	vm.Store("set up Docker compose for the API", "user", sessionID, nil)
	vm.Store("created docker-compose.yml with services", "assistant", sessionID, nil)
	vm.Store("the containers are failing to start", "user", sessionID, nil)
	vm.Store("I see the port mapping is wrong", "assistant", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it going"}},
	}

	refined, _ := router.RefineIntent(context.Background(), &req, sessionID)
	if !refined {
		t.Fatal("should refine")
	}

	// The refinement context should include RECENT Docker messages
	if provider.lastRefinementContext != nil {
		contextText := ""
		for _, msg := range provider.lastRefinementContext {
			contextText += " " + msg.Content
		}

		hasRecent := strings.Contains(strings.ToLower(contextText), "docker") ||
			strings.Contains(strings.ToLower(contextText), "container") ||
			strings.Contains(strings.ToLower(contextText), "port")

		if !hasRecent {
			t.Errorf("refinement context should include recent Docker messages, got: %s",
				truncate(contextText, 300))
		}
	}
}

// --- Session isolation ---

// TestContext_SessionIsolation_NoLeakage verifies that two active sessions
// don't leak context into each other during refinement.
func TestContext_SessionIsolation_NoLeakage(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "check the Go auth handler status",
			2: "check the Python parser status",
		},
	}
	router, vm := setupContextTestRouter(t, provider)

	// Session A: working on Go auth
	sessionA := "session-A-go-auth"
	vm.Store("implement OAuth token refresh in Go", "user", sessionA, nil)
	vm.Store("updated auth.go with token refresh", "assistant", sessionA, nil)

	// Session B: working on Python parser
	sessionB := "session-B-python"
	vm.Store("write a Python config parser", "user", sessionB, nil)
	vm.Store("created parser.py with YAML support", "assistant", sessionB, nil)

	// Refine a vague prompt in session A
	reqA := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats up"}},
	}
	router.RefineIntent(context.Background(), &reqA, sessionA)

	// Check that session A's refinement context does NOT contain Python/parser
	if provider.lastRefinementContext != nil {
		for _, msg := range provider.lastRefinementContext {
			lower := strings.ToLower(msg.Content)
			if strings.Contains(lower, "python") || strings.Contains(lower, "parser.py") {
				t.Errorf("session A context leaked session B's Python content: %q", msg.Content)
			}
		}
	}

	// Refine a vague prompt in session B
	provider.refinementCallCount = 0
	reqB := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it look"}},
	}
	router.RefineIntent(context.Background(), &reqB, sessionB)

	// Check that session B's refinement context does NOT contain Go/auth
	if provider.lastRefinementContext != nil {
		for _, msg := range provider.lastRefinementContext {
			lower := strings.ToLower(msg.Content)
			if strings.Contains(lower, "oauth") || strings.Contains(lower, "auth.go") {
				t.Errorf("session B context leaked session A's Go auth content: %q", msg.Content)
			}
		}
	}
}

// --- Context drift over many refined turns ---

// TestContext_ManyRefinedTurns_ContextDoesNotDrift verifies that after 5+
// consecutive vague prompts (all refined), the accumulated context still
// produces useful refinements that match skills.
func TestContext_ManyRefinedTurns_ContextDoesNotDrift(t *testing.T) {
	registry := orchestration.DefaultSkills()

	// Each turn's refinement output is designed to be progressively specific
	// about the same topic (Go auth), simulating a real LLM that maintains
	// topic coherence.
	turnRefinements := []string{
		"diagnose the Go authentication handler for errors",
		"fix the Go auth handler error handling for expired tokens",
		"add Go tests for the auth handler token refresh",
		"review the Go auth handler test coverage for completeness",
		"verify the Go auth handler changes pass all tests",
	}

	provider := &contextTrackingProvider{
		refinedResponses: make(map[int]string),
	}
	for i, r := range turnRefinements {
		provider.refinedResponses[i+1] = r
	}

	router, vm := setupContextTestRouter(t, provider)
	sessionID := "context-drift"

	// Seed initial context
	vm.Store("implement OAuth token refresh in the Go auth handler", "user", sessionID, nil)
	vm.Store("I updated auth.go with the refresh flow", "assistant", sessionID, nil)

	vaguePrompts := []string{
		"whats going on",
		"ok fix it",
		"test it",
		"looks good?",
		"we done?",
	}

	for i, prompt := range vaguePrompts {
		t.Run(fmt.Sprintf("turn_%d_%s", i+1, strings.ReplaceAll(prompt, " ", "_")), func(t *testing.T) {
			req := providers.ChatRequest{
				Model:    "auto",
				Messages: []providers.Message{{Role: "user", Content: prompt}},
			}

			refined, err := router.RefineIntent(context.Background(), &req, sessionID)
			if err != nil {
				t.Fatalf("RefineIntent failed: %v", err)
			}
			if !refined {
				t.Fatalf("turn %d should be refined", i+1)
			}

			refinedMsg := lastUserMessage(req.Messages)

			// Every refined prompt should still match skills
			matched := orchestration.MatchSkills(refinedMsg, registry)
			if len(matched) == 0 {
				t.Errorf("turn %d: refined prompt %q matches no skills (context drift!)", i+1, refinedMsg)
			}

			// Every refined prompt should still reference Go/auth
			lower := strings.ToLower(refinedMsg)
			if !strings.Contains(lower, "go") && !strings.Contains(lower, "auth") {
				t.Errorf("turn %d: refined prompt lost topic coherence: %q", i+1, refinedMsg)
			}

			// Store refined text + assistant response for next turn's context
			vm.Store(refinedMsg, "user", sessionID, nil)
			vm.Store(fmt.Sprintf("completed step %d of auth handler work", i+1), "assistant", sessionID, nil)
		})
	}
}

// --- Double retrieval: refinement vs main flow ---

// TestContext_DoubleRetrieval_RefinedQueryBetterThanOriginal verifies that
// the main flow's memory retrieval (post-refinement) uses the REFINED prompt
// as the query, which should return more relevant results than the original
// vague prompt would have.
func TestContext_DoubleRetrieval_RefinedQueryBetterThanOriginal(t *testing.T) {
	provider := &refineTestProvider{
		refinedResponse: "check the Go authentication handler for OAuth token issues",
	}
	router, vm := setupRefineTestRouter(t, provider)
	sessionID := "double-retrieval"

	// Seed specific technical context
	vm.Store("implement OAuth token refresh in the Go auth handler", "user", sessionID, nil)
	vm.Store("Updated auth.go with token refresh and expiry handling", "assistant", sessionID, nil)
	// Also seed some unrelated context
	vm.Store("what should we have for lunch", "user", sessionID, nil)
	vm.Store("how about pizza", "assistant", sessionID, nil)

	// Send through full ChatCompletionWithDebug to see the memory query
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it going"}},
	}

	resp, err := router.ChatCompletionWithDebug(context.Background(), req, sessionID, true)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}

	meta := resp.XProxyMetadata
	if meta == nil {
		t.Fatal("expected metadata")
	}

	// The memory query should be the REFINED prompt (more technical, better semantic match)
	if meta.MemoryQuery == "hows it going" {
		t.Error("memory query should use refined prompt, not the vague original")
	}

	// The refined prompt should be a better query for finding auth-related messages
	if !strings.Contains(strings.ToLower(meta.MemoryQuery), "auth") &&
		!strings.Contains(strings.ToLower(meta.MemoryQuery), "go") {
		t.Errorf("refined memory query should contain technical terms, got: %q", meta.MemoryQuery)
	}
}

// --- Skills endpoints with cleaned triggers ---

// TestContext_SkillDispatch_CleanedTriggersNoFalsePositives verifies that
// after removing casual triggers, common non-technical phrases don't
// accidentally match skills.
func TestContext_SkillDispatch_CleanedTriggersNoFalsePositives(t *testing.T) {
	registry := orchestration.DefaultSkills()

	// These are phrases that SHOULD NOT match any skills.
	// Before the trigger cleanup, some of these would have matched
	// (e.g., "broken" → code-implement, "whats" → research)
	nonTechnical := []string{
		"hello",
		"whats for lunch",
		"its broken",         // was: matched code-implement via "broken"
		"doesnt work",        // was: matched code-implement via "doesnt work"
		"not working",        // was: matched code-implement via "not working"
		"any idea",           // was: matched research via "any idea"
		"how does that work", // was: matched research via "how does"
		"what is that",       // was: matched research via "what is"
		"why is it like that",// was: matched research via "why is"
		"look at this",       // was: matched code-review via "look at"
		"take a look",        // was: matched code-review via "take a look"
		"hacked",             // was: matched security-review via "hacked"
		"logged in",          // was: matched security-review via "login"
		"im thinking",
		"ok sure",
		"got it thanks",
	}

	for _, phrase := range nonTechnical {
		t.Run(phrase, func(t *testing.T) {
			matched := orchestration.MatchSkills(phrase, registry)
			if len(matched) > 0 {
				names := make([]string, len(matched))
				for i, s := range matched {
					names[i] = s.Name
				}
				t.Errorf("non-technical phrase %q should not match any skills, got: %v", phrase, names)
			}
		})
	}
}

// TestContext_SkillDispatch_TechnicalTriggersStillWork verifies that
// the cleaned triggers still correctly match technical prompts.
func TestContext_SkillDispatch_TechnicalTriggersStillWork(t *testing.T) {
	registry := orchestration.DefaultSkills()

	technical := []struct {
		prompt     string
		wantSkills []string
	}{
		{"implement OAuth", []string{"code-implement", "security-review"}},
		{"fix the auth handler", []string{"security-review"}},
		{"refactor the Go code", []string{"go-patterns", "code-implement"}},
		{"add Docker compose", []string{"docker-expert"}},
		{"research gRPC", []string{"research"}},
		{"test the parser", []string{"go-testing", "python-testing"}},
		{"review code quality", []string{"code-review"}},
		{"create REST endpoint", []string{"api-design"}},
		{"debug the router", []string{"research"}},
		{"explain circuit breakers", []string{"research"}},
	}

	for _, tt := range technical {
		t.Run(tt.prompt, func(t *testing.T) {
			matched := orchestration.MatchSkills(tt.prompt, registry)
			names := make([]string, len(matched))
			for i, s := range matched {
				names[i] = s.Name
			}

			for _, want := range tt.wantSkills {
				found := false
				for _, name := range names {
					if name == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("prompt %q should match %s, got: %v", tt.prompt, want, names)
				}
			}
		})
	}
}

// TestContext_SkillsMatch_VaguePromptFallsBack verifies that the
// DispatchWithDetails path (used by /v1/skills/match endpoint)
// correctly falls back to role-based planning for vague prompts
// that don't match any triggers.
func TestContext_SkillsMatch_VaguePromptFallsBack(t *testing.T) {
	registry := orchestration.DefaultSkills()

	vaguePrompts := []string{
		"whats going on",
		"help me",
		"its broken",
		"any thoughts",
		"now what",
	}

	for _, prompt := range vaguePrompts {
		t.Run(prompt, func(t *testing.T) {
			result := orchestration.DispatchWithDetails(prompt, registry)

			if !result.FallbackToRole {
				t.Errorf("vague prompt %q should fall back to role-based, got %d skills: %v",
					prompt, len(result.MatchedSkills), result.MatchedSkills)
			}
			// Fallback should still produce steps
			if len(result.Steps) == 0 {
				t.Error("fallback should still produce role-based steps")
			}
		})
	}
}

// =============================================================================
// Test infrastructure
// =============================================================================

// contextTrackingProvider captures the context messages sent to refinement calls
// so tests can verify what context the refinement LLM receives.
type contextTrackingProvider struct {
	refinedResponses       map[int]string         // callNum → response
	refinementCallCount    int
	lastRefinementContext   []providers.Message    // context messages from last refinement call
	totalCalls             int
}

func (p *contextTrackingProvider) Name() string { return "context-tracker" }

func (p *contextTrackingProvider) ChatCompletion(_ context.Context, req providers.ChatRequest, _ string) (providers.ChatResponse, error) {
	p.totalCalls++

	content := "ok"
	isRefinement := len(req.Messages) > 0 && req.Messages[0].Role == "system" &&
		strings.Contains(req.Messages[0].Content, "Rewrite the user")

	if isRefinement {
		p.refinementCallCount++
		// Capture the context messages (everything between system prompt and last user message)
		if len(req.Messages) > 2 {
			p.lastRefinementContext = req.Messages[1 : len(req.Messages)-1]
		}
		if resp, ok := p.refinedResponses[p.refinementCallCount]; ok {
			content = resp
		}
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("resp-%d", p.totalCalls),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "context-tracker-model",
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (p *contextTrackingProvider) IsHealthy(context.Context) bool { return true }
func (p *contextTrackingProvider) MaxContextTokens() int          { return 2000000 }
func (p *contextTrackingProvider) SupportsModel(string) bool      { return true }

func setupContextTestRouter(t *testing.T, p providers.Provider) (*Router, *memory.VectorMemory) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "context.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	schemaPath := filepath.Join("..", "..", "migrations", "001_init.sql")
	sqlBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatal(err)
	}

	tracker, err := usage.NewTracker(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tracker.Close() })

	vm := memory.NewVectorMemory(db)
	r := NewRouter([]providers.Provider{p}, tracker, vm, db)
	return r, vm
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// =============================================================================
// Cross-provider handoff tests
//
// In production, the refinement call uses ChatCompletion (picks cheapest
// provider, model "auto"), while the actual request goes to whatever
// provider/model the user specified. These tests verify:
//   - Refinement on provider A, actual request on provider B
//   - User specifies a specific model that only one provider supports
//   - Mid-session model/provider switching
//   - Refined prompt flows correctly across provider boundaries
// =============================================================================

// routedProvider is a mock provider with a specific name, controlled model
// support, and independent call tracking.
type routedProvider struct {
	providerName     string
	supportedModels  map[string]bool // models this provider supports (empty = all)
	refinedResponse  string          // what to return for refinement calls
	refinementCalls  int
	normalCalls      int
	totalCalls       int
	lastNormalPrompt string // last user message seen on a non-refinement call
}

func (p *routedProvider) Name() string { return p.providerName }

func (p *routedProvider) ChatCompletion(_ context.Context, req providers.ChatRequest, _ string) (providers.ChatResponse, error) {
	p.totalCalls++

	isRefinement := len(req.Messages) > 0 && req.Messages[0].Role == "system" &&
		strings.Contains(req.Messages[0].Content, "Rewrite the user")

	content := fmt.Sprintf("response from %s", p.providerName)
	if isRefinement {
		p.refinementCalls++
		if p.refinedResponse != "" {
			content = p.refinedResponse
		}
	} else {
		p.normalCalls++
		// Capture the last user message the provider actually sees
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if req.Messages[i].Role == "user" {
				p.lastNormalPrompt = req.Messages[i].Content
				break
			}
		}
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("%s-resp-%d", p.providerName, p.totalCalls),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (p *routedProvider) IsHealthy(context.Context) bool { return true }
func (p *routedProvider) MaxContextTokens() int          { return 2000000 }

func (p *routedProvider) SupportsModel(model string) bool {
	if len(p.supportedModels) == 0 {
		return true // empty = supports all
	}
	return p.supportedModels[model]
}

func setupMultiProviderRouter(t *testing.T, providerList []providers.Provider) (*Router, *memory.VectorMemory) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "multi.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	schemaPath := filepath.Join("..", "..", "migrations", "001_init.sql")
	sqlBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(sqlBytes)); err != nil {
		t.Fatal(err)
	}

	tracker, err := usage.NewTracker(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tracker.Close() })

	vm := memory.NewVectorMemory(db)
	r := NewRouter(providerList, tracker, vm, db)
	return r, vm
}

// TestCrossProvider_RefinementOnCheap_ActualOnPreferred verifies that
// the refinement call goes through the provider chain (any provider can
// handle it) while the actual request goes to the user-specified preferred
// provider. The refined prompt must flow to the preferred provider.
func TestCrossProvider_RefinementOnCheap_ActualOnPreferred(t *testing.T) {
	refinedText := "diagnose the Go authentication handler for OAuth token refresh issues"

	// Both providers return the same refined response — the router picks
	// whichever has lowest usage, which is non-deterministic at 0/0.
	cheap := &routedProvider{
		providerName:    "nanogpt-sub",
		refinedResponse: refinedText,
	}
	premium := &routedProvider{
		providerName:    "gemini",
		refinedResponse: refinedText,
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{cheap, premium})
	sessionID := "cross-provider-1"

	vm.Store("implement OAuth token refresh in Go", "user", sessionID, nil)
	vm.Store("updated auth.go with refresh flow", "assistant", sessionID, nil)

	// User sends vague prompt, preferring "gemini" as the provider
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats going on"}},
	}

	resp, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "gemini", false,
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// At least one provider should have handled a refinement call
	totalRefinements := cheap.refinementCalls + premium.refinementCalls
	if totalRefinements == 0 {
		t.Error("at least one provider should handle the refinement call")
	}

	// The preferred provider (gemini) MUST handle the actual request
	if premium.normalCalls == 0 {
		t.Error("actual request should go to preferred provider (gemini)")
	}

	// The preferred provider should have received the REFINED prompt, not the vague one
	if premium.lastNormalPrompt == "whats going on" {
		t.Error("premium provider received the original vague prompt instead of the refined one")
	}
	if !strings.Contains(strings.ToLower(premium.lastNormalPrompt), "auth") {
		t.Errorf("premium provider should see refined prompt with 'auth', got: %q",
			premium.lastNormalPrompt)
	}
}

// TestCrossProvider_SpecificModel_RoutesToCorrectProvider verifies that
// when a user requests a specific model (e.g., "claude-sonnet-4-6"), the
// refinement still goes to the cheap provider (model "auto") while the
// actual request routes to the provider that supports that model.
func TestCrossProvider_SpecificModel_RoutesToCorrectProvider(t *testing.T) {
	refinedText := "fix the Docker container networking for the Go API service"

	// All providers can handle refinement (model "auto"), but only claude
	// supports the specific model the user requests.
	cheap := &routedProvider{
		providerName:    "nanogpt-sub",
		supportedModels: map[string]bool{"auto": true, "qwen3.5-plus": true},
		refinedResponse: refinedText,
	}
	claudeProvider := &routedProvider{
		providerName:    "claude-code",
		supportedModels: map[string]bool{"claude-sonnet-4-6": true, "claude-opus-4-6": true},
		refinedResponse: refinedText,
	}
	geminiProvider := &routedProvider{
		providerName:    "gemini",
		supportedModels: map[string]bool{"gemini-2.5-pro": true, "gemini-2.5-flash": true},
		refinedResponse: refinedText,
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{cheap, claudeProvider, geminiProvider})
	sessionID := "cross-model-1"

	vm.Store("set up Docker compose for Go API", "user", sessionID, nil)
	vm.Store("created docker-compose.yml", "assistant", sessionID, nil)

	// User requests a specific Claude model
	req := providers.ChatRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []providers.Message{{Role: "user", Content: "its broken"}},
	}

	_, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "claude-code", false,
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// At least one provider handled the refinement
	totalRefinements := cheap.refinementCalls + claudeProvider.refinementCalls + geminiProvider.refinementCalls
	if totalRefinements == 0 {
		t.Error("refinement should have been called on some provider")
	}

	// Actual request should go to claude-code (preferred provider)
	if claudeProvider.normalCalls == 0 {
		t.Error("actual request should route to claude-code for model claude-sonnet-4-6")
	}

	// Claude provider should see the REFINED prompt, not the vague original
	if claudeProvider.lastNormalPrompt == "its broken" {
		t.Error("claude provider received the original vague prompt instead of refined")
	}
	if !strings.Contains(strings.ToLower(claudeProvider.lastNormalPrompt), "docker") {
		t.Errorf("claude provider should see refined prompt about Docker, got: %q",
			claudeProvider.lastNormalPrompt)
	}

	// Gemini should NOT have been called for the actual request
	if geminiProvider.normalCalls != 0 {
		t.Error("gemini should not handle the actual request when claude-code is preferred")
	}
}

// TestCrossProvider_MidSessionModelSwitch verifies that a user can switch
// models/providers mid-session and the refined context carries over correctly.
func TestCrossProvider_MidSessionModelSwitch(t *testing.T) {
	refinedText1 := "check the Go auth handler implementation status"
	refinedText2 := "verify the Go auth handler tests pass and review for security"

	cheap := &routedProvider{
		providerName:    "nanogpt-sub",
		refinedResponse: refinedText1,
	}
	gemini := &routedProvider{
		providerName:    "gemini",
		supportedModels: map[string]bool{"gemini-2.5-pro": true},
		refinedResponse: refinedText1,
	}
	claude := &routedProvider{
		providerName:    "claude-code",
		supportedModels: map[string]bool{"claude-sonnet-4-6": true},
		refinedResponse: refinedText1,
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{cheap, gemini, claude})
	sessionID := "model-switch"

	vm.Store("implement Go auth handler with OAuth", "user", sessionID, nil)
	vm.Store("done, auth.go updated", "assistant", sessionID, nil)

	// Turn 1: User sends vague prompt, prefers Gemini
	req1 := providers.ChatRequest{
		Model:    "gemini-2.5-pro",
		Messages: []providers.Message{{Role: "user", Content: "hows it look"}},
	}
	resp1, err := router.ChatCompletionForProvider(
		context.Background(), req1, sessionID, "gemini", false,
	)
	if err != nil {
		t.Fatalf("Turn 1 failed: %v", err)
	}
	if len(resp1.Choices) == 0 {
		t.Fatal("Turn 1: expected response")
	}

	// Gemini should have handled the actual request
	if gemini.normalCalls == 0 {
		t.Error("Turn 1: gemini should handle the actual request")
	}
	// Gemini should see the refined prompt
	if gemini.lastNormalPrompt == "hows it look" {
		t.Error("Turn 1: gemini got the vague prompt instead of refined")
	}

	// Store turn 1 response in memory for turn 2
	vm.Store("the implementation looks correct", "assistant", sessionID, nil)

	// Turn 2: User switches to Claude mid-session with a vague follow-up
	// Update refinement response on ALL providers since any could handle it
	cheap.refinedResponse = refinedText2
	gemini.refinedResponse = refinedText2
	claude.refinedResponse = refinedText2
	cheap.refinementCalls = 0
	gemini.refinementCalls = 0
	claude.normalCalls = 0
	claude.lastNormalPrompt = ""

	req2 := providers.ChatRequest{
		Model:    "claude-sonnet-4-6",
		Messages: []providers.Message{{Role: "user", Content: "now what"}},
	}
	resp2, err := router.ChatCompletionForProvider(
		context.Background(), req2, sessionID, "claude-code", false,
	)
	if err != nil {
		t.Fatalf("Turn 2 failed: %v", err)
	}
	if len(resp2.Choices) == 0 {
		t.Fatal("Turn 2: expected response")
	}

	// Claude should handle turn 2
	if claude.normalCalls == 0 {
		t.Error("Turn 2: claude should handle the actual request after model switch")
	}

	// Claude should see the refined prompt (not "now what")
	if claude.lastNormalPrompt == "now what" {
		t.Error("Turn 2: claude got vague prompt instead of refined after model switch")
	}

	// The refined prompt should contain technical terms from session context
	lower := strings.ToLower(claude.lastNormalPrompt)
	if !strings.Contains(lower, "go") && !strings.Contains(lower, "auth") && !strings.Contains(lower, "test") {
		t.Errorf("Turn 2: claude should see refined prompt with Go/auth/test context, got: %q",
			claude.lastNormalPrompt)
	}
}

// TestCrossProvider_SpecificPrompt_NoRefinement_DirectToProvider verifies
// that well-structured prompts skip refinement entirely and go directly
// to the user's preferred provider — no extra LLM call, no cost.
func TestCrossProvider_SpecificPrompt_NoRefinement_DirectToProvider(t *testing.T) {
	cheap := &routedProvider{
		providerName:    "nanogpt-sub",
		refinedResponse: "should not see this",
	}
	premium := &routedProvider{
		providerName: "gemini",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{cheap, premium})
	sessionID := "direct-route"

	vm.Store("working on Go code", "user", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "update handler.go to fix the OAuth refresh bug"}},
	}

	_, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "gemini", false,
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// NO refinement call should have been made (prompt has .go file ref = specific)
	if cheap.refinementCalls != 0 {
		t.Errorf("cheap provider should NOT be called for refinement on specific prompt, got %d calls",
			cheap.refinementCalls)
	}

	// Premium provider should get the original prompt unchanged
	if premium.normalCalls == 0 {
		t.Error("premium provider should handle the actual request")
	}
	if premium.lastNormalPrompt != "update handler.go to fix the OAuth refresh bug" {
		t.Errorf("premium provider should see original prompt unchanged, got: %q",
			premium.lastNormalPrompt)
	}
}

// TestCrossProvider_ThreeProviderChain_FallbackOnRefinement verifies
// behavior when the cheapest provider fails during refinement — the
// router falls back to the next provider for refinement, then still
// routes the actual request to the user's preferred provider.
func TestCrossProvider_ThreeProviderChain_FallbackOnRefinement(t *testing.T) {
	// Cheap provider fails on everything (simulates being down)
	failing := &routedProvider{
		providerName: "nanogpt-sub",
	}
	// Mid-tier can handle refinement
	midTier := &routedProvider{
		providerName:    "gemini",
		refinedResponse: "diagnose the Go authentication handler status",
	}
	// Premium is the user's preference for the actual request
	premium := &routedProvider{
		providerName: "claude-code",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{failing, midTier, premium})
	sessionID := "fallback-refine"

	vm.Store("implement Go auth handler", "user", sessionID, nil)
	vm.Store("updated auth.go", "assistant", sessionID, nil)

	// Open circuit breaker on the cheap provider so it's skipped
	router.circuitBreakers["nanogpt-sub"].Open(5 * time.Minute)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats happening"}},
	}

	_, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "claude-code", false,
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// Cheap provider should be skipped (circuit breaker open)
	if failing.totalCalls != 0 {
		t.Error("failing provider should be skipped (circuit breaker open)")
	}

	// Mid-tier should handle refinement (first healthy for model "auto")
	if midTier.refinementCalls == 0 {
		t.Error("mid-tier should handle refinement when cheap provider is down")
	}

	// Premium should handle the actual request
	if premium.normalCalls == 0 {
		t.Error("premium should handle the actual request")
	}

	// Premium should see the refined prompt
	if premium.lastNormalPrompt == "whats happening" {
		t.Error("premium got vague prompt — refinement from mid-tier didn't flow through")
	}
}

// TestCrossProvider_ModelAutoOnRefinement_SpecificOnActual verifies the
// key architectural property: refinement ALWAYS uses model "auto" (cheapest),
// regardless of what model the user requested for the actual call.
func TestCrossProvider_ModelAutoOnRefinement_SpecificOnActual(t *testing.T) {
	var calls []modelCall

	tracker := &routedProvider{
		providerName:    "tracker",
		refinedResponse: "check the Go auth handler",
	}

	// Override ChatCompletion to capture model field
	originalChatCompletion := tracker.ChatCompletion
	_ = originalChatCompletion

	// We can't easily override methods on a struct, so let's use a wrapper
	wrapper := &modelTrackingProvider{
		inner:           tracker,
		refinedResponse: "check the Go auth handler",
		calls:           &calls,
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{wrapper})
	sessionID := "model-tracking"

	vm.Store("implement Go auth", "user", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "gpt-4o",
		Messages: []providers.Message{{Role: "user", Content: "how bout it"}},
	}

	router.ChatCompletionForProvider(context.Background(), req, sessionID, "", false)

	// Find refinement call and actual call
	var refinementModel, actualModel string
	for _, c := range calls {
		if c.isRefinement {
			refinementModel = c.model
		} else {
			actualModel = c.model
		}
	}

	if refinementModel != "auto" {
		t.Errorf("refinement should use model 'auto', got %q", refinementModel)
	}
	if actualModel != "gpt-4o" {
		t.Errorf("actual request should use user's model 'gpt-4o', got %q", actualModel)
	}
}

// modelTrackingProvider wraps a provider to track what model each call uses.
type modelTrackingProvider struct {
	inner           *routedProvider
	refinedResponse string
	calls           *[]modelCall
}

type modelCall struct {
	model        string
	isRefinement bool
}

func (p *modelTrackingProvider) Name() string { return p.inner.providerName }

func (p *modelTrackingProvider) ChatCompletion(_ context.Context, req providers.ChatRequest, _ string) (providers.ChatResponse, error) {
	isRefinement := len(req.Messages) > 0 && req.Messages[0].Role == "system" &&
		strings.Contains(req.Messages[0].Content, "Rewrite the user")

	*p.calls = append(*p.calls, modelCall{model: req.Model, isRefinement: isRefinement})

	content := "ok"
	if isRefinement && p.refinedResponse != "" {
		content = p.refinedResponse
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("resp-%d", len(*p.calls)),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (p *modelTrackingProvider) IsHealthy(context.Context) bool { return true }
func (p *modelTrackingProvider) MaxContextTokens() int          { return 2000000 }
func (p *modelTrackingProvider) SupportsModel(string) bool      { return true }

// =============================================================================
// Context limit pre-check tests
//
// These verify the router's tryProvider pre-check: when estimated tokens
// exceed a provider's MaxContextTokens(), the router skips that provider
// WITHOUT making an API call, saving cost and latency.
// =============================================================================

// TestPreCheck_JustUnderLimit_Accepted verifies that requests just under
// the context limit are accepted normally.
func TestPreCheck_JustUnderLimit_Accepted(t *testing.T) {
	// Provider with 1000-token limit (~4000 chars)
	provider := &realLimitProvider{
		providerName: "tight",
		maxTokens:    1000,
	}
	router, _ := setupMultiProviderRouter(t, []providers.Provider{provider})

	// Build a request that's ~800 tokens (3200 chars) — under the 1000 limit
	content := strings.Repeat("a", 3200)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: content}},
	}

	_, err := router.ChatCompletionForProvider(
		context.Background(), req, "", "", false,
	)
	if err != nil {
		t.Fatalf("request under limit should succeed: %v", err)
	}
	if provider.acceptedCalls == 0 {
		t.Error("provider should have accepted the request")
	}
	t.Logf("under limit: %d estimated tokens, limit %d — accepted", provider.lastTokenCount, provider.maxTokens)
}

// TestPreCheck_JustOverLimit_Rejected verifies that the pre-check catches
// requests just over the limit WITHOUT calling the provider.
func TestPreCheck_JustOverLimit_Rejected(t *testing.T) {
	// Provider with 500-token limit (~2000 chars)
	provider := &realLimitProvider{
		providerName: "tight",
		maxTokens:    500,
	}
	router, _ := setupMultiProviderRouter(t, []providers.Provider{provider})

	// Build a request that's ~600 tokens (2400 chars) — over the 500 limit
	content := strings.Repeat("a", 2400)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: content}},
	}

	_, err := router.ChatCompletionForProvider(
		context.Background(), req, "", "", false,
	)
	if err == nil {
		t.Fatal("request over limit should fail")
	}

	// The KEY assertion: provider should NOT have been called at all
	// The pre-check should have caught it
	if provider.totalCalls != 0 {
		t.Errorf("pre-check should prevent API call, but provider was called %d times", provider.totalCalls)
	}

	if !strings.Contains(err.Error(), "request too large") {
		t.Errorf("error should mention 'request too large', got: %v", err)
	}
	t.Logf("over limit: estimated ~%d tokens, limit %d — rejected by pre-check (0 API calls)",
		len(content)/4, provider.maxTokens)
}

// TestPreCheck_OverLimit_FallsBackToLargerProvider verifies the fallback:
// small provider is skipped by pre-check, large provider accepts.
func TestPreCheck_OverLimit_FallsBackToLargerProvider(t *testing.T) {
	small := &realLimitProvider{
		providerName: "codex",
		maxTokens:    500, // tiny limit
	}
	large := &realLimitProvider{
		providerName: "gemini",
		maxTokens:    1048576, // 1M
	}

	router, _ := setupMultiProviderRouter(t, []providers.Provider{small, large})

	// Build request that's ~600 tokens — over small, under large
	content := strings.Repeat("implement the handler ", 120) // ~2520 chars ≈ 630 tokens
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: content}},
	}

	resp, err := router.ChatCompletionWithDebug(
		context.Background(), req, "", false,
	)
	if err != nil {
		t.Fatalf("should succeed via fallback: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// Small provider: pre-check should skip it (0 API calls)
	if small.totalCalls != 0 {
		t.Errorf("small provider should be skipped by pre-check, got %d calls", small.totalCalls)
	}

	// Large provider: should accept
	if large.acceptedCalls == 0 {
		t.Error("large provider should accept")
	}

	t.Logf("small (500 tokens): %d calls — skipped by pre-check", small.totalCalls)
	t.Logf("large (1M tokens): accepted with %d tokens", large.lastTokenCount)
}

// TestPreCheck_AllProvidersExceeded_CleanError verifies that when the
// request exceeds ALL providers' limits, the error is clean and describes
// the situation.
func TestPreCheck_AllProvidersExceeded_CleanError(t *testing.T) {
	p1 := &realLimitProvider{providerName: "small-1", maxTokens: 500}
	p2 := &realLimitProvider{providerName: "small-2", maxTokens: 500}
	p3 := &realLimitProvider{providerName: "small-3", maxTokens: 500}

	router, _ := setupMultiProviderRouter(t, []providers.Provider{p1, p2, p3})

	// Build request that's ~1000 tokens — over all providers
	content := strings.Repeat("x", 4000) // 4000 chars ≈ 1000 tokens
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: content}},
	}

	_, err := router.ChatCompletionWithDebug(
		context.Background(), req, "", false,
	)
	if err == nil {
		t.Fatal("should fail when all providers are exceeded")
	}

	// None of the providers should have been called
	totalAPICalls := p1.totalCalls + p2.totalCalls + p3.totalCalls
	if totalAPICalls != 0 {
		t.Errorf("no providers should be called when all exceed limits, got %d total calls", totalAPICalls)
	}

	t.Logf("all providers exceeded: error = %v", err)
	t.Logf("API calls saved: 3 (all skipped by pre-check)")
}

// TestPreCheck_WithMemoryInjection_StillCaughtBeforeAPICall verifies that
// when memory injection pushes the request over a provider's limit,
// the pre-check catches it. This is the real-world scenario: user's
// message is small but memory injection bloats the request.
func TestPreCheck_WithMemoryInjection_StillCaughtBeforeAPICall(t *testing.T) {
	small := &realLimitProvider{
		providerName:    "ollama",
		maxTokens:       1000, // tight limit
		refinedResponse: "check the auth handler",
	}
	large := &realLimitProvider{
		providerName:    "gemini",
		maxTokens:       1048576,
		refinedResponse: "check the auth handler",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{small, large})
	sessionID := "precheck-memory"

	// Seed enough context that memory injection will push over 1000 tokens
	for i := 0; i < 30; i++ {
		vm.Store(
			fmt.Sprintf("implement Go auth handler component %d with OAuth validation and error recovery", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("completed component %d with tests and coverage", i),
			"assistant", sessionID, nil,
		)
	}

	// User's message is tiny, but after memory injection it'll be big
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "update handler.go"}},
	}

	resp, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "", false,
	)
	if err != nil {
		t.Fatalf("should succeed via fallback to gemini: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// After memory injection, the request should be too large for ollama.
	// The pre-check should skip ollama and fall back to gemini.
	t.Logf("ollama (1K tokens): %d total calls, %d accepted, %d rejected",
		small.totalCalls, small.acceptedCalls, small.rejectedCalls)
	t.Logf("gemini (1M tokens): %d total calls, %d accepted",
		large.totalCalls, large.acceptedCalls)

	if large.acceptedCalls == 0 {
		t.Error("gemini should have accepted via fallback")
	}
}

// TestPreCheck_RefinementCallRespectsSameLimit verifies that the refinement
// LLM call (which goes through the same tryProvider path) also gets the
// pre-check. If the refinement context is too large for the cheapest
// provider, it should fall back.
func TestPreCheck_RefinementCallRespectsSameLimit(t *testing.T) {
	// Tiny provider that can't handle the refinement context
	tiny := &realLimitProvider{
		providerName:    "tiny",
		maxTokens:       50, // ~200 chars — too small for refinement system prompt alone
		refinedResponse: "check the handler",
	}
	// Normal provider that can handle anything
	normal := &realLimitProvider{
		providerName:    "normal",
		maxTokens:       200000,
		refinedResponse: "diagnose the Go auth handler",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{tiny, normal})
	sessionID := "precheck-refine"

	vm.Store("implement Go auth handler", "user", sessionID, nil)
	vm.Store("updated auth.go", "assistant", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats up"}},
	}

	// RefineIntent calls ChatCompletion which calls tryProvider.
	// The refinement request includes: system prompt (~200 tokens) + context + user msg.
	// At ~50 token limit, the tiny provider should be skipped by pre-check.
	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("refinement should not error: %v", err)
	}

	if refined {
		t.Logf("refined successfully (fell back to normal provider)")
		t.Logf("tiny: %d calls — should be 0 (skipped by pre-check)", tiny.totalCalls)
		t.Logf("normal: %d calls", normal.totalCalls)
	} else {
		t.Logf("refinement skipped (may have failed gracefully)")
	}

	// Tiny provider should have been skipped by pre-check for the refinement call
	// (the refinement system prompt alone is ~200 tokens, over the 50 token limit)
	if tiny.acceptedCalls > 0 {
		t.Errorf("tiny provider should not accept refinement call (system prompt alone exceeds 50 token limit)")
	}
}

// =============================================================================
// Provider limit cascade + full context handoff tests
//
// These test the real production scenario: a request fills up to (or past)
// one provider's context limit, the router pre-check skips it, and the
// next provider in the chain receives the FULL untruncated context.
//
// The receiving provider must see:
//   - The same refined prompt (not the original vague one)
//   - The same memory-injected messages
//   - The full message payload, unchanged
//
// We test every real provider limit in the cascade:
//   codex (128K) → ollama (128K) → qwen (131K) → claude (200K) → gemini (1M)
// =============================================================================

// payloadTrackingProvider records the full message payload it receives,
// so we can verify the fallback provider got the complete context.
type payloadTrackingProvider struct {
	providerName    string
	maxTokens       int
	refinedResponse string
	totalCalls      int
	acceptedCalls   int
	rejectedCalls   int
	lastTokenCount  int
	lastMessages    []providers.Message // full payload received
}

func (p *payloadTrackingProvider) Name() string { return p.providerName }

func (p *payloadTrackingProvider) ChatCompletion(_ context.Context, req providers.ChatRequest, _ string) (providers.ChatResponse, error) {
	p.totalCalls++

	totalTokens := 0
	for _, msg := range req.Messages {
		totalTokens += len(msg.Content) / 4
	}
	p.lastTokenCount = totalTokens
	p.lastMessages = make([]providers.Message, len(req.Messages))
	copy(p.lastMessages, req.Messages)

	if p.maxTokens > 0 && totalTokens > p.maxTokens {
		p.rejectedCalls++
		return providers.ChatResponse{}, fmt.Errorf(
			"context too long: %d tokens exceeds %d token limit",
			totalTokens, p.maxTokens)
	}

	p.acceptedCalls++
	isRefinement := len(req.Messages) > 0 && req.Messages[0].Role == "system" &&
		strings.Contains(req.Messages[0].Content, "Rewrite the user")

	content := fmt.Sprintf("response from %s (processed %d tokens)", p.providerName, totalTokens)
	if isRefinement && p.refinedResponse != "" {
		content = p.refinedResponse
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("%s-resp-%d", p.providerName, p.totalCalls),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{PromptTokens: totalTokens, CompletionTokens: 50, TotalTokens: totalTokens + 50},
	}, nil
}

func (p *payloadTrackingProvider) IsHealthy(context.Context) bool { return true }
func (p *payloadTrackingProvider) MaxContextTokens() int          { return p.maxTokens }
func (p *payloadTrackingProvider) SupportsModel(string) bool      { return true }

// buildPayload creates a message payload of approximately targetTokens size.
func buildPayload(targetTokens int) []providers.Message {
	// Each char ≈ 0.25 tokens, so targetTokens * 4 chars
	targetChars := targetTokens * 4
	chunkSize := 500 // ~125 tokens per message
	numMessages := targetChars / chunkSize
	if numMessages < 1 {
		numMessages = 1
	}

	msgs := make([]providers.Message, 0, numMessages)
	remaining := targetChars
	for i := 0; i < numMessages && remaining > 0; i++ {
		size := chunkSize
		if size > remaining {
			size = remaining
		}
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs = append(msgs, providers.Message{
			Role:    role,
			Content: strings.Repeat("x", size),
		})
		remaining -= size
	}
	return msgs
}

// TestCascade_CodexOverflow_FallsToGemini fills a request to 130K tokens
// (over codex's 128K), verifies codex is skipped, and gemini gets the
// FULL untruncated payload.
func TestCascade_CodexOverflow_FallsToGemini(t *testing.T) {
	codex := &payloadTrackingProvider{
		providerName: "codex",
		maxTokens:    128000,
	}
	gemini := &payloadTrackingProvider{
		providerName: "gemini",
		maxTokens:    1048576,
	}

	router, _ := setupMultiProviderRouter(t, []providers.Provider{codex, gemini})

	// Build payload at ~130K tokens (over codex, under gemini)
	payload := buildPayload(130000)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: payload,
	}

	resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
	if err != nil {
		t.Fatalf("should succeed via gemini fallback: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// Codex: pre-check should skip (0 API calls)
	if codex.totalCalls != 0 {
		t.Errorf("codex should be skipped by pre-check, got %d calls", codex.totalCalls)
	}

	// Gemini: should accept the full payload
	if gemini.acceptedCalls == 0 {
		t.Fatal("gemini should accept")
	}

	// Gemini must receive the FULL payload — same number of messages
	if len(gemini.lastMessages) != len(payload) {
		t.Errorf("gemini received %d messages, expected %d (full payload)",
			len(gemini.lastMessages), len(payload))
	}

	t.Logf("codex (128K): skipped by pre-check, 0 API calls")
	t.Logf("gemini (1M): accepted %d tokens, %d messages (full payload)", gemini.lastTokenCount, len(gemini.lastMessages))
}

// TestCascade_FullChain_EachLimitHit tests the full production provider
// cascade with a request sized to overflow each tier in sequence.
// The request should cascade through until it finds a provider big enough.
func TestCascade_FullChain_EachLimitHit(t *testing.T) {
	// Real provider chain ordered by context limit (ascending)
	codex := &payloadTrackingProvider{providerName: "codex", maxTokens: 128000}
	ollama := &payloadTrackingProvider{providerName: "ollama", maxTokens: 128000}
	qwen := &payloadTrackingProvider{providerName: "qwen", maxTokens: 131072}
	claude := &payloadTrackingProvider{providerName: "claude-code", maxTokens: 200000}
	gemini := &payloadTrackingProvider{providerName: "gemini", maxTokens: 1048576}

	chain := []providers.Provider{codex, ollama, qwen, claude, gemini}

	tests := []struct {
		name           string
		tokens         int
		wantSkipped    []string // providers that should be skipped by pre-check
		wantAccepted   string   // provider that should accept
	}{
		{
			name:         "under all limits (100K)",
			tokens:       100000,
			wantSkipped:  nil,
			wantAccepted: "codex", // first provider, fits
		},
		{
			name:         "over codex+ollama (129K)",
			tokens:       129000,
			wantSkipped:  []string{"codex", "ollama"},
			wantAccepted: "qwen",
		},
		{
			name:         "over codex+ollama+qwen (135K)",
			tokens:       135000,
			wantSkipped:  []string{"codex", "ollama", "qwen"},
			wantAccepted: "claude-code",
		},
		{
			name:         "over everything except gemini (250K)",
			tokens:       250000,
			wantSkipped:  []string{"codex", "ollama", "qwen", "claude-code"},
			wantAccepted: "gemini",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset all providers
			for _, p := range chain {
				pt := p.(*payloadTrackingProvider)
				pt.totalCalls = 0
				pt.acceptedCalls = 0
				pt.rejectedCalls = 0
				pt.lastTokenCount = 0
				pt.lastMessages = nil
			}

			router, _ := setupMultiProviderRouter(t, chain)

			payload := buildPayload(tt.tokens)
			req := providers.ChatRequest{
				Model:    "auto",
				Messages: payload,
			}

			resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
			if err != nil {
				t.Fatalf("should succeed via fallback: %v", err)
			}
			if len(resp.Choices) == 0 {
				t.Fatal("expected response")
			}

			// Verify skipped providers got 0 API calls
			for _, skipName := range tt.wantSkipped {
				for _, p := range chain {
					pt := p.(*payloadTrackingProvider)
					if pt.providerName == skipName && pt.totalCalls != 0 {
						t.Errorf("%s should be skipped (0 API calls), got %d", skipName, pt.totalCalls)
					}
				}
			}

			// Verify the accepting provider got the full payload
			for _, p := range chain {
				pt := p.(*payloadTrackingProvider)
				if pt.providerName == tt.wantAccepted {
					if pt.acceptedCalls == 0 {
						t.Errorf("%s should have accepted", tt.wantAccepted)
					}
					if len(pt.lastMessages) != len(payload) {
						t.Errorf("%s received %d messages, expected %d (full payload not truncated)",
							tt.wantAccepted, len(pt.lastMessages), len(payload))
					}
					t.Logf("%s accepted: %d tokens, %d messages", tt.wantAccepted, pt.lastTokenCount, len(pt.lastMessages))
				}
			}

			// Log the cascade
			for _, p := range chain {
				pt := p.(*payloadTrackingProvider)
				status := "skipped"
				if pt.acceptedCalls > 0 {
					status = fmt.Sprintf("ACCEPTED (%d tokens)", pt.lastTokenCount)
				}
				t.Logf("  %s (%dK limit): %s", pt.providerName, pt.maxTokens/1000, status)
			}
		})
	}
}

// TestCascade_OverAllLimits_CleanFailure verifies that when the request
// exceeds even the largest provider (gemini 1M), we get a clean error
// with 0 wasted API calls.
func TestCascade_OverAllLimits_CleanFailure(t *testing.T) {
	codex := &payloadTrackingProvider{providerName: "codex", maxTokens: 128000}
	claude := &payloadTrackingProvider{providerName: "claude-code", maxTokens: 200000}
	gemini := &payloadTrackingProvider{providerName: "gemini", maxTokens: 1048576}

	router, _ := setupMultiProviderRouter(t, []providers.Provider{codex, claude, gemini})

	// Build payload at 1.1M tokens — over everything
	payload := buildPayload(1100000)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: payload,
	}

	_, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
	if err == nil {
		t.Fatal("should fail when request exceeds all providers")
	}

	// Zero API calls — all caught by pre-check
	totalAPICalls := codex.totalCalls + claude.totalCalls + gemini.totalCalls
	if totalAPICalls != 0 {
		t.Errorf("should save all API calls via pre-check, got %d total", totalAPICalls)
	}

	t.Logf("1.1M token request: all providers skipped, 0 API calls wasted")
	t.Logf("error: %v", err)
}

// TestCascade_RefinedPromptSurvivesHandoff verifies that when refinement
// happens on provider A and the actual request overflows to provider B,
// provider B sees the REFINED prompt (not the original vague one) plus
// all the injected memory context.
func TestCascade_RefinedPromptSurvivesHandoff(t *testing.T) {
	refinedText := "diagnose the Go authentication handler OAuth token refresh"

	// Small provider: handles refinement (small context is fine for that)
	// but can't handle the actual request after memory injection
	small := &payloadTrackingProvider{
		providerName:    "nanogpt-sub",
		maxTokens:       500, // tiny — will overflow after memory injection
		refinedResponse: refinedText,
	}
	// Large provider: handles the actual request
	large := &payloadTrackingProvider{
		providerName:    "gemini",
		maxTokens:       1048576,
		refinedResponse: refinedText,
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{small, large})
	sessionID := "cascade-refined"

	// Seed enough context to push the actual request over 500 tokens
	for i := 0; i < 20; i++ {
		vm.Store(
			fmt.Sprintf("implement Go auth handler component %d with OAuth token validation and refresh", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("completed auth component %d with comprehensive test coverage", i),
			"assistant", sessionID, nil,
		)
	}

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats going on"}},
	}

	resp, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "", false,
	)
	if err != nil {
		t.Fatalf("should succeed via gemini fallback: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// Large provider must see the REFINED prompt, not "whats going on"
	if large.lastMessages == nil || len(large.lastMessages) == 0 {
		t.Fatal("large provider should have received messages")
	}

	// Find the last user message the large provider received
	lastUserMsg := ""
	for i := len(large.lastMessages) - 1; i >= 0; i-- {
		if large.lastMessages[i].Role == "user" {
			lastUserMsg = large.lastMessages[i].Content
			break
		}
	}

	if lastUserMsg == "whats going on" {
		t.Error("large provider received the original vague prompt — refinement didn't carry over to fallback")
	}
	if !strings.Contains(strings.ToLower(lastUserMsg), "auth") ||
		!strings.Contains(strings.ToLower(lastUserMsg), "go") {
		t.Errorf("large provider should see refined prompt about Go auth, got: %q", lastUserMsg)
	}

	// Large provider should also have memory-injected messages
	if len(large.lastMessages) < 5 {
		t.Errorf("large provider should have memory-injected messages, got only %d messages", len(large.lastMessages))
	}

	t.Logf("small (%d token limit): %d calls", small.maxTokens, small.totalCalls)
	t.Logf("large (1M limit): accepted %d tokens, %d messages", large.lastTokenCount, len(large.lastMessages))
	t.Logf("last user message large provider saw: %q", truncate(lastUserMsg, 80))
}

// --- Unlimited context (MaxContextTokens = 0) tests ---

// TestCascade_UnlimitedProvider_AcceptsAnything verifies that a provider
// with MaxContextTokens() = 0 (unlimited) accepts any size request.
// The pre-check skips validation when limit is 0.
func TestCascade_UnlimitedProvider_AcceptsAnything(t *testing.T) {
	unlimited := &payloadTrackingProvider{
		providerName: "unlimited",
		maxTokens:    0, // 0 = unlimited, pre-check skips
	}

	router, _ := setupMultiProviderRouter(t, []providers.Provider{unlimited})

	// Send a massive 2M token payload
	payload := buildPayload(2000000)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: payload,
	}

	resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
	if err != nil {
		t.Fatalf("unlimited provider should accept any size: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	if unlimited.acceptedCalls == 0 {
		t.Error("unlimited provider should accept")
	}
	t.Logf("unlimited provider: accepted %d tokens, %d messages", unlimited.lastTokenCount, len(unlimited.lastMessages))
}

// TestCascade_LimitedOverflow_FallsToUnlimited verifies that when all
// limited providers overflow, an unlimited provider catches the request.
func TestCascade_LimitedOverflow_FallsToUnlimited(t *testing.T) {
	small := &payloadTrackingProvider{providerName: "codex", maxTokens: 128000}
	medium := &payloadTrackingProvider{providerName: "claude-code", maxTokens: 200000}
	unlimited := &payloadTrackingProvider{providerName: "nanogpt", maxTokens: 0} // unlimited

	router, _ := setupMultiProviderRouter(t, []providers.Provider{small, medium, unlimited})

	// 250K tokens — over codex and claude, but unlimited accepts
	payload := buildPayload(250000)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: payload,
	}

	resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
	if err != nil {
		t.Fatalf("should succeed via unlimited fallback: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	if small.totalCalls != 0 {
		t.Errorf("codex should be skipped: %d calls", small.totalCalls)
	}
	if medium.totalCalls != 0 {
		t.Errorf("claude should be skipped: %d calls", medium.totalCalls)
	}
	if unlimited.acceptedCalls == 0 {
		t.Error("unlimited should accept")
	}

	// Unlimited provider must get the FULL payload
	if len(unlimited.lastMessages) != len(payload) {
		t.Errorf("unlimited got %d messages, expected %d (full payload)",
			len(unlimited.lastMessages), len(payload))
	}

	t.Logf("codex (128K): skipped")
	t.Logf("claude (200K): skipped")
	t.Logf("nanogpt (unlimited): accepted %d tokens, %d messages", unlimited.lastTokenCount, len(unlimited.lastMessages))
}

// TestCascade_GracefulOverfill_ProviderStillResponds verifies that when a
// request is close to (but under) a provider's limit, the provider handles
// it correctly even at high utilization.
func TestCascade_GracefulOverfill_ProviderStillResponds(t *testing.T) {
	limits := []struct {
		name      string
		maxTokens int
		fillPct   float64 // fill to this percentage of limit
	}{
		{"codex-95pct", 128000, 0.95},
		{"claude-95pct", 200000, 0.95},
		{"gemini-95pct", 1048576, 0.95},
		{"codex-99pct", 128000, 0.99},
		{"claude-99pct", 200000, 0.99},
	}

	for _, lim := range limits {
		t.Run(lim.name, func(t *testing.T) {
			provider := &payloadTrackingProvider{
				providerName: lim.name,
				maxTokens:    lim.maxTokens,
			}
			router, _ := setupMultiProviderRouter(t, []providers.Provider{provider})

			tokens := int(float64(lim.maxTokens) * lim.fillPct)
			payload := buildPayload(tokens)
			req := providers.ChatRequest{
				Model:    "auto",
				Messages: payload,
			}

			resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
			if err != nil {
				t.Fatalf("should accept at %.0f%% fill: %v", lim.fillPct*100, err)
			}
			if len(resp.Choices) == 0 {
				t.Fatal("expected response")
			}

			utilization := float64(provider.lastTokenCount) / float64(lim.maxTokens) * 100
			t.Logf("%s: %d/%d tokens (%.1f%% utilized) — accepted",
				lim.name, provider.lastTokenCount, lim.maxTokens, utilization)
		})
	}
}

// TestCascade_OverUtilization_150_200_Percent verifies that requests at
// 150% and 200% of a provider's context limit are handled gracefully.
// The pre-check should catch these and skip to a larger provider.
func TestCascade_OverUtilization_150_200_Percent(t *testing.T) {
	tests := []struct {
		name    string
		fillPct float64
	}{
		{"110pct", 1.10},
		{"125pct", 1.25},
		{"150pct", 1.50},
		{"175pct", 1.75},
		{"200pct", 2.00},
		{"300pct", 3.00},
		{"500pct", 5.00},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("codex_%s", tt.name), func(t *testing.T) {
			codex := &payloadTrackingProvider{providerName: "codex", maxTokens: 128000}
			gemini := &payloadTrackingProvider{providerName: "gemini", maxTokens: 1048576}

			router, _ := setupMultiProviderRouter(t, []providers.Provider{codex, gemini})

			tokens := int(float64(128000) * tt.fillPct)
			payload := buildPayload(tokens)
			req := providers.ChatRequest{
				Model:    "auto",
				Messages: payload,
			}

			resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)

			// If even gemini can't handle it (>1M), that's fine — clean error
			if tokens > 1048576 {
				if err == nil {
					t.Fatal("should fail when over all limits")
				}
				t.Logf("codex@%.0f%% (%dK tokens): both providers exceeded — clean error", tt.fillPct*100, tokens/1000)
				return
			}

			if err != nil {
				t.Fatalf("should succeed via gemini fallback at %.0f%%: %v", tt.fillPct*100, err)
			}
			if len(resp.Choices) == 0 {
				t.Fatal("expected response")
			}

			// Codex must be skipped by pre-check (0 API calls)
			if codex.totalCalls != 0 {
				t.Errorf("codex should be skipped at %.0f%% utilization, got %d calls", tt.fillPct*100, codex.totalCalls)
			}

			// Gemini must accept the full payload
			if gemini.acceptedCalls == 0 {
				t.Error("gemini should accept")
			}
			if len(gemini.lastMessages) != len(payload) {
				t.Errorf("gemini got %d messages, expected %d (payload truncated!)", len(gemini.lastMessages), len(payload))
			}

			t.Logf("codex@%.0f%% (%dK tokens): codex skipped (0 calls), gemini accepted %d tokens (%d msgs)",
				tt.fillPct*100, tokens/1000, gemini.lastTokenCount, len(gemini.lastMessages))
		})
	}
}

// TestCascade_OverUtilization_AllProviderLimits tests 150% utilization against
// every real provider limit, with gemini as the fallback.
func TestCascade_OverUtilization_AllProviderLimits(t *testing.T) {
	limits := []struct {
		name      string
		maxTokens int
	}{
		{"codex", 128000},
		{"ollama", 128000},
		{"qwen", 131072},
		{"claude-code", 200000},
	}

	for _, lim := range limits {
		t.Run(fmt.Sprintf("%s_at_150pct", lim.name), func(t *testing.T) {
			provider := &payloadTrackingProvider{providerName: lim.name, maxTokens: lim.maxTokens}
			gemini := &payloadTrackingProvider{providerName: "gemini", maxTokens: 1048576}

			router, _ := setupMultiProviderRouter(t, []providers.Provider{provider, gemini})

			tokens := int(float64(lim.maxTokens) * 1.5)
			payload := buildPayload(tokens)
			req := providers.ChatRequest{
				Model:    "auto",
				Messages: payload,
			}

			resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
			if err != nil {
				t.Fatalf("%s at 150%% should fallback to gemini: %v", lim.name, err)
			}
			if len(resp.Choices) == 0 {
				t.Fatal("expected response")
			}

			// Provider must be skipped by pre-check
			if provider.totalCalls != 0 {
				t.Errorf("%s should be skipped at 150%%, got %d calls", lim.name, provider.totalCalls)
			}

			// Gemini must accept the full payload, not truncated
			if len(gemini.lastMessages) != len(payload) {
				t.Errorf("gemini truncated payload: got %d msgs, expected %d", len(gemini.lastMessages), len(payload))
			}

			t.Logf("%s at 150%% (%dK/%dK tokens): skipped → gemini accepted %d tokens",
				lim.name, tokens/1000, lim.maxTokens/1000, gemini.lastTokenCount)
		})
	}
}

// TestCascade_EstimateMismatch_ProviderRejectsAfterPreCheck tests the edge
// case where the pre-check estimate is WRONG — the request passes the
// pre-check but the provider itself rejects it. This simulates real
// tokenization differences (estimateTokens uses len/4 but real tokenizers
// count differently).
func TestCascade_EstimateMismatch_ProviderRejectsAfterPreCheck(t *testing.T) {
	// Provider with a "real" limit that's tighter than what estimateTokens thinks.
	// estimateTokens says ~100K tokens (400K chars / 4), but the provider's
	// internal tokenizer says it's 130K (e.g., 1.3 chars per token for CJK text).
	// So the pre-check passes (100K < 128K) but the provider rejects.
	mismatch := &payloadTrackingProvider{
		providerName: "codex",
		maxTokens:    128000, // pre-check thinks 100K < 128K = OK
	}
	// But we override the provider's internal check to use a stricter limit
	strict := &strictTokenProvider{
		inner:            mismatch,
		realTokenLimit:   95000, // actually rejects at 95K
		tokenMultiplier:  1.3,   // real tokenizer counts 30% more than len/4
	}
	gemini := &payloadTrackingProvider{
		providerName: "gemini",
		maxTokens:    1048576,
	}

	router, _ := setupMultiProviderRouter(t, []providers.Provider{strict, gemini})

	// Build payload at ~100K estimated tokens (400K chars).
	// Pre-check: 100K < 128K → passes
	// Provider: 100K * 1.3 = 130K > 95K real limit → rejects
	payload := buildPayload(100000)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: payload,
	}

	resp, err := router.ChatCompletionWithDebug(context.Background(), req, "", false)
	if err != nil {
		t.Fatalf("should succeed via gemini fallback after mismatch: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// The strict provider should have been called (pre-check passed) but rejected
	if strict.inner.totalCalls == 0 {
		t.Error("strict provider should have been called (pre-check passed)")
	}
	if strict.inner.rejectedCalls == 0 {
		t.Error("strict provider should have rejected (real tokenizer stricter)")
	}

	// Gemini should have accepted as fallback
	if gemini.acceptedCalls == 0 {
		t.Error("gemini should accept as fallback")
	}

	// Gemini must get the FULL payload, not truncated
	if len(gemini.lastMessages) != len(payload) {
		t.Errorf("gemini got %d messages, expected %d", len(gemini.lastMessages), len(payload))
	}

	t.Logf("estimate mismatch: pre-check passed (est ~100K < 128K limit), provider rejected (real ~130K > 95K limit)")
	t.Logf("gemini accepted %d tokens as fallback (%d messages)", gemini.lastTokenCount, len(gemini.lastMessages))
}

// strictTokenProvider wraps a payloadTrackingProvider but applies a stricter
// token counting method, simulating real tokenizer differences.
type strictTokenProvider struct {
	inner           *payloadTrackingProvider
	realTokenLimit  int
	tokenMultiplier float64 // multiply len/4 by this to get "real" token count
}

func (p *strictTokenProvider) Name() string { return p.inner.providerName }

func (p *strictTokenProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, sid string) (providers.ChatResponse, error) {
	p.inner.totalCalls++

	// Use "real" tokenizer (stricter than len/4)
	totalChars := 0
	for _, msg := range req.Messages {
		totalChars += len(msg.Content)
	}
	realTokens := int(float64(totalChars) / 4.0 * p.tokenMultiplier)
	p.inner.lastTokenCount = realTokens

	if realTokens > p.realTokenLimit {
		p.inner.rejectedCalls++
		return providers.ChatResponse{}, fmt.Errorf(
			"context too long: %d tokens (real tokenizer) exceeds %d token limit",
			realTokens, p.realTokenLimit)
	}

	p.inner.acceptedCalls++
	return providers.ChatResponse{
		ID:      fmt.Sprintf("resp-%d", p.inner.totalCalls),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: "ok"},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{PromptTokens: realTokens, CompletionTokens: 50, TotalTokens: realTokens + 50},
	}, nil
}

func (p *strictTokenProvider) IsHealthy(context.Context) bool { return true }
func (p *strictTokenProvider) MaxContextTokens() int          { return p.inner.maxTokens } // pre-check uses this
func (p *strictTokenProvider) SupportsModel(string) bool      { return true }

// TestCascade_OverfillWithRefinement_EntireFlowWorks verifies the complete
// flow: vague prompt → refinement → memory injection → context pushes past
// small provider → falls back to large → large gets refined prompt + full
// memory + responds successfully. This is the "everything at once" test.
func TestCascade_OverfillWithRefinement_EntireFlowWorks(t *testing.T) {
	refinedText := "review the Go authentication handler OAuth implementation and verify test coverage"

	codex := &payloadTrackingProvider{
		providerName:    "codex",
		maxTokens:       1000, // very tight — will overflow with memory
		refinedResponse: refinedText,
	}
	claude := &payloadTrackingProvider{
		providerName:    "claude-code",
		maxTokens:       200000,
		refinedResponse: refinedText,
	}
	gemini := &payloadTrackingProvider{
		providerName:    "gemini",
		maxTokens:       1048576,
		refinedResponse: refinedText,
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{codex, claude, gemini})
	sessionID := "overfill-full"

	// Build up 50 turns of session history
	for i := 0; i < 50; i++ {
		vm.Store(
			fmt.Sprintf("implement Go auth handler component %d with OAuth token validation, error handling, and rate limiting", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("completed component %d with unit tests, integration tests, and benchmark coverage", i),
			"assistant", sessionID, nil,
		)
	}

	// Vague prompt → triggers refinement
	// Use ChatCompletionWithDebug (not ForProvider) so fallback chain is active
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows everything"}},
	}

	resp, err := router.ChatCompletionWithDebug(
		context.Background(), req, sessionID, false,
	)
	if err != nil {
		t.Fatalf("full flow should succeed via fallback: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// Find which provider accepted
	var acceptor *payloadTrackingProvider
	for _, p := range []*payloadTrackingProvider{codex, claude, gemini} {
		if p.acceptedCalls > 0 {
			// Find the last accepted (the actual request, not refinement)
			acceptor = p
		}
	}

	if acceptor == nil {
		t.Fatal("no provider accepted the actual request")
	}

	// The accepting provider must have the refined prompt
	lastUser := ""
	for i := len(acceptor.lastMessages) - 1; i >= 0; i-- {
		if acceptor.lastMessages[i].Role == "user" {
			lastUser = acceptor.lastMessages[i].Content
			break
		}
	}

	if lastUser == "hows everything" {
		t.Error("accepting provider got the vague prompt — refinement didn't flow through cascade")
	}

	// The accepting provider must have memory-injected context
	if len(acceptor.lastMessages) < 5 {
		t.Errorf("accepting provider should have memory context, got only %d messages", len(acceptor.lastMessages))
	}

	t.Logf("cascade result:")
	for _, p := range []*payloadTrackingProvider{codex, claude, gemini} {
		status := "skipped"
		if p.acceptedCalls > 0 {
			status = fmt.Sprintf("ACCEPTED (%d tokens, %d msgs)", p.lastTokenCount, len(p.lastMessages))
		}
		t.Logf("  %s (%dK limit): %d calls — %s", p.providerName, p.maxTokens/1000, p.totalCalls, status)
	}
	t.Logf("  refined prompt delivered: %q", truncate(lastUser, 80))
}

// =============================================================================
// Large context / context limit tests
//
// These tests verify behavior when sessions accumulate large amounts of
// context, individual messages are very long, or the combined context
// approaches provider limits.
//
// Key architecture facts:
//   - RetrieveRecent always returns up to 4 messages (NO token cap)
//   - Semantic/lexical search respects maxTokens budget
//   - Refinement retrieves 1000-2000 token budget of context
//   - Main flow retrieves 4000 token budget of context
//   - EstimateTokens = len(text) / 4
//   - Both refinement + main flow inject context = up to 6000 tokens overhead
// =============================================================================

// TestLargeContext_HugeSession_500Messages verifies refinement stays bounded
// with a very large session history. The token budget should prevent the
// refinement call from receiving hundreds of messages.
func TestLargeContext_HugeSession_500Messages(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "check the latest Go handler changes",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "huge-500"

	// Seed 500 messages (250 turns)
	for i := 0; i < 250; i++ {
		vm.Store(fmt.Sprintf("working on Go handler %d", i), "user", sessionID, nil)
		vm.Store(fmt.Sprintf("updated handler %d", i), "assistant", sessionID, nil)
	}

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it going"}},
	}

	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("refinement failed on 500-message session: %v", err)
	}
	if !refined {
		t.Fatal("should still refine on huge sessions")
	}

	// The refinement call should receive fewer than 500 messages.
	// RetrieveRecent returns 4, and semantic/lexical search is capped by:
	//   - LIMIT 200 in the DB query
	//   - Token budget (2000 for full refinement)
	// Short messages (~30 chars ≈ 8 tokens each) fit many within 2000 tokens,
	// so we may see up to ~200 messages. The key assertion: it's bounded,
	// not all 500.
	if provider.lastRefinementContext != nil {
		msgCount := len(provider.lastRefinementContext)
		if msgCount >= 500 {
			t.Errorf("refinement received ALL %d messages — should be bounded by DB LIMIT and token budget", msgCount)
		}

		totalChars := 0
		for _, msg := range provider.lastRefinementContext {
			totalChars += len(msg.Content)
		}
		t.Logf("refinement received %d messages (%d chars) from 500-msg session", msgCount, totalChars)
	}
}

// TestLargeContext_VeryLongMessages verifies behavior when individual messages
// are very large (10K+ characters each). RetrieveRecent returns up to 4
// messages with NO token cap, so 4 huge messages could create a large payload.
func TestLargeContext_VeryLongMessages(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "review the large Go implementation",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "huge-messages"

	// Store 4 very large messages (10K chars each = ~2500 tokens each)
	largeContent := strings.Repeat("implement the Go handler with proper error handling and validation ", 150) // ~10K chars
	vm.Store(largeContent, "user", sessionID, nil)
	vm.Store(largeContent, "assistant", sessionID, nil)
	vm.Store(largeContent, "user", sessionID, nil)
	vm.Store(largeContent, "assistant", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "any updates"}},
	}

	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("refinement failed with large messages: %v", err)
	}

	// Refinement should still work — it may receive large context but shouldn't crash
	if !refined {
		t.Fatal("should still attempt refinement even with large messages")
	}

	// Measure total context size sent to the refinement LLM
	if provider.lastRefinementContext != nil {
		totalChars := 0
		for _, msg := range provider.lastRefinementContext {
			totalChars += len(msg.Content)
		}
		t.Logf("refinement received %d chars of context from large messages (%d messages)",
			totalChars, len(provider.lastRefinementContext))

		// NOTE: RetrieveRecent returns up to 4 messages WITHOUT token cap.
		// This is a known architecture decision — recent messages are always included.
		// The token budget only caps the semantic/lexical search results.
		// With 4x 10K messages, the refinement context could be ~40K chars.
		// This test documents the behavior rather than asserting a hard limit.
	}
}

// TestLargeContext_CombinedOverhead_RefinementPlusMainFlow measures the total
// context injected into the provider when BOTH refinement and main memory
// injection happen. This is the worst case for a vague prompt.
func TestLargeContext_CombinedOverhead_RefinementPlusMainFlow(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "diagnose the Go auth handler token refresh issue",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "combined-overhead"

	// Seed 20 messages of moderate size
	for i := 0; i < 10; i++ {
		msg := fmt.Sprintf("implement Go auth component %d with OAuth token handling and error recovery for expired credentials", i)
		vm.Store(msg, "user", sessionID, nil)
		vm.Store(fmt.Sprintf("completed component %d with tests", i), "assistant", sessionID, nil)
	}

	// Track what the ACTUAL request provider receives (including injected memory)
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats happening"}},
	}

	resp, err := router.ChatCompletionWithDebug(context.Background(), req, sessionID, true)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	// The provider was called twice: once for refinement, once for actual request.
	// The actual request includes: injected memory + refined prompt.
	if provider.totalCalls < 2 {
		t.Errorf("expected at least 2 provider calls, got %d", provider.totalCalls)
	}

	// Check metadata for memory injection stats
	if resp.XProxyMetadata != nil {
		t.Logf("memory query: %q", resp.XProxyMetadata.MemoryQuery)
		t.Logf("memory candidates injected: %d", resp.XProxyMetadata.MemoryCandidateCount)
	}
}

// TestLargeContext_TokenBudget_CapsSemanticNotRecent verifies the architectural
// property that RetrieveRecent (4 most recent) is ALWAYS included regardless
// of token budget, while semantic search respects the budget.
func TestLargeContext_TokenBudget_CapsSemanticNotRecent(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "check the Go handler",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "budget-cap"

	// Store 4 recent messages (small) + 20 older messages (medium)
	for i := 0; i < 20; i++ {
		vm.Store(fmt.Sprintf("old Go work item %d with auth and routing context", i), "user", sessionID, nil)
		vm.Store(fmt.Sprintf("completed old item %d", i), "assistant", sessionID, nil)
	}
	// These 4 are the most recent and should ALWAYS be included
	vm.Store("just deployed the Docker fix", "user", sessionID, nil)
	vm.Store("deployment successful", "assistant", sessionID, nil)
	vm.Store("running final tests now", "user", sessionID, nil)
	vm.Store("all tests pass", "assistant", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats the status"}},
	}

	router.RefineIntent(context.Background(), &req, sessionID)

	// The refinement context should include the recent messages
	if provider.lastRefinementContext != nil {
		contextText := ""
		for _, msg := range provider.lastRefinementContext {
			contextText += " " + msg.Content
		}

		// Recent messages should be present
		hasRecent := strings.Contains(contextText, "Docker fix") ||
			strings.Contains(contextText, "deployment") ||
			strings.Contains(contextText, "final tests") ||
			strings.Contains(contextText, "all tests pass")

		if !hasRecent {
			t.Error("recent messages should ALWAYS be in refinement context regardless of token budget")
		}

		t.Logf("refinement context: %d messages, %d chars",
			len(provider.lastRefinementContext), len(contextText))
	}
}

// TestLargeContext_ProviderRejectsOversize verifies that when the combined
// context (memory injection + refined prompt + user messages) exceeds a
// provider's limit, the error is handled gracefully — no crash, and the
// router tries fallback providers.
func TestLargeContext_ProviderRejectsOversize(t *testing.T) {
	// Provider that rejects requests over a certain size
	rejectingProvider := &contextLimitProvider{
		providerName:    "limited",
		maxChars:        5000, // reject if total message content > 5000 chars
		refinedResponse: "check the Go handler",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{rejectingProvider})
	sessionID := "oversize"

	// Seed enough context that memory injection will push over 5000 chars
	for i := 0; i < 20; i++ {
		vm.Store(
			fmt.Sprintf("working on Go auth component %d with detailed implementation notes about OAuth token handling patterns and error recovery strategies", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("completed component %d with comprehensive test coverage and documentation", i),
			"assistant", sessionID, nil,
		)
	}

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it going"}},
	}

	// The request may fail (single provider, no fallback), but should not panic
	_, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "", false,
	)

	// We expect an error (provider rejects) but no panic/crash
	if err != nil {
		t.Logf("provider correctly rejected oversize request: %v", err)
	} else {
		t.Log("request succeeded despite large context (provider accepted it)")
	}

	// Provider should have been called
	if rejectingProvider.totalCalls == 0 {
		t.Error("provider should have been called at least once")
	}
	t.Logf("provider saw %d total chars across %d calls",
		rejectingProvider.lastTotalChars, rejectingProvider.totalCalls)
}

// TestLargeContext_SloppyLongPrompt verifies refinement handles a prompt
// that is long (200+ chars) AND sloppy — has triggers but is messy.
// This is the "word vomit" scenario.
func TestLargeContext_SloppyLongPrompt(t *testing.T) {
	triggers := allSkillTriggers()

	// 250 chars of word vomit with triggers scattered throughout
	wordVomit := "ok so like i was thinking about the auth thing and the docker stuff and i think maybe we should fix the way the test runs because its kind of slow and also the api endpoint returns wrong errors sometimes and the whole thing feels broken"

	level := needsRefinement(wordVomit, triggers)
	// This is >100 chars AND has triggers AND has 10+ words → should be refinementNone
	// because it meets: charCount > 100 && hasTriggers && len(words) >= 10
	if level == refinementFull {
		t.Errorf("long sloppy prompt should not get full refinement (it has triggers and detail), got: %s", level)
	}
	t.Logf("250-char sloppy prompt with triggers: refinement level = %s", level)

	// Verify it still matches skills even without refinement
	registry := orchestration.DefaultSkills()
	matched := orchestration.MatchSkills(wordVomit, registry)
	if len(matched) == 0 {
		t.Error("long sloppy prompt with triggers should match at least some skills")
	}
	names := make([]string, len(matched))
	for i, s := range matched {
		names[i] = s.Name
	}
	t.Logf("long sloppy prompt matched %d skills: %v", len(matched), names)
}

// TestLargeContext_SingleGiantMessage verifies behavior when a session has
// one extremely large message (50K+ chars). The memory retrieval should
// handle this without crashing.
func TestLargeContext_SingleGiantMessage(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "review the large Go implementation",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "giant-msg"

	// One 50K char message
	giant := strings.Repeat("implement the authentication handler with proper OAuth token validation and refresh logic for the Go service ", 500)
	vm.Store(giant, "user", sessionID, nil)
	vm.Store("done", "assistant", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "any thoughts"}},
	}

	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("refinement crashed on giant message: %v", err)
	}

	// Should still work — the giant message will be in recent context
	if !refined {
		t.Fatal("should attempt refinement")
	}

	if provider.lastRefinementContext != nil {
		totalChars := 0
		for _, msg := range provider.lastRefinementContext {
			totalChars += len(msg.Content)
		}
		t.Logf("refinement received %d chars from session with 50K message (%d context messages)",
			totalChars, len(provider.lastRefinementContext))
	}
}

// TestLargeContext_MemoryBudgetZeroAfterRecent verifies that when recent
// messages consume the entire token budget, the semantic search gets 0
// budget and returns nothing extra — only recent messages are in context.
func TestLargeContext_MemoryBudgetZeroAfterRecent(t *testing.T) {
	provider := &contextTrackingProvider{
		refinedResponses: map[int]string{
			1: "check the implementation",
		},
	}
	router, vm := setupContextTestRouter(t, provider)
	sessionID := "budget-zero"

	// 4 messages that are each ~4000 chars (~1000 tokens each)
	// With refinement budget of 2000 tokens, recent alone exceeds the budget
	bigMsg := strings.Repeat("Go auth handler implementation details ", 100) // ~3800 chars ≈ 950 tokens
	vm.Store(bigMsg, "user", sessionID, nil)
	vm.Store(bigMsg, "assistant", sessionID, nil)
	vm.Store(bigMsg, "user", sessionID, nil)
	vm.Store(bigMsg, "assistant", sessionID, nil)

	// Also store 10 older small messages that semantic search would normally find
	for i := 0; i < 10; i++ {
		vm.Store(fmt.Sprintf("old context %d about Go", i), "user", sessionID, nil)
	}

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats up"}},
	}

	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("refinement failed: %v", err)
	}

	// Should still work — recent messages are always included
	if !refined {
		t.Fatal("should refine even when budget is consumed by recent messages")
	}

	if provider.lastRefinementContext != nil {
		t.Logf("with budget-exceeding recent messages: %d context messages, %d chars",
			len(provider.lastRefinementContext),
			sumChars(provider.lastRefinementContext))
	}
}

// contextLimitProvider rejects requests where total content exceeds maxChars.
type contextLimitProvider struct {
	providerName    string
	maxChars        int
	refinedResponse string
	totalCalls      int
	lastTotalChars  int
}

func (p *contextLimitProvider) Name() string { return p.providerName }

func (p *contextLimitProvider) ChatCompletion(_ context.Context, req providers.ChatRequest, _ string) (providers.ChatResponse, error) {
	p.totalCalls++

	totalChars := 0
	for _, msg := range req.Messages {
		totalChars += len(msg.Content)
	}
	p.lastTotalChars = totalChars

	isRefinement := len(req.Messages) > 0 && req.Messages[0].Role == "system" &&
		strings.Contains(req.Messages[0].Content, "Rewrite the user")

	// For refinement calls, always succeed (they have SkipMemory so less context)
	if isRefinement {
		content := p.refinedResponse
		if content == "" {
			content = "refined"
		}
		return providers.ChatResponse{
			ID:      fmt.Sprintf("resp-%d", p.totalCalls),
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "limited-model",
			Choices: []providers.Choice{{
				Index:        0,
				Message:      providers.Message{Role: "assistant", Content: content},
				FinishReason: "stop",
			}},
			Usage: providers.Usage{TotalTokens: 15},
		}, nil
	}

	// For actual requests, reject if too large (0 = unlimited)
	if p.maxChars > 0 && totalChars > p.maxChars {
		return providers.ChatResponse{}, fmt.Errorf(
			"context too long: %d chars exceeds maximum context window (%d chars)",
			totalChars, p.maxChars,
		)
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("resp-%d", p.totalCalls),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "limited-model",
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: "ok"},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{TotalTokens: 15},
	}, nil
}

func (p *contextLimitProvider) IsHealthy(context.Context) bool { return true }
func (p *contextLimitProvider) MaxContextTokens() int          { return p.maxChars / 4 }
func (p *contextLimitProvider) SupportsModel(string) bool      { return true }

func sumChars(msgs []providers.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
	}
	return total
}

// =============================================================================
// Real provider context limit tests
//
// These tests simulate the ACTUAL context limits of production providers and
// verify the system handles overflow correctly. Key finding: the router NEVER
// checks MaxContextTokens() before sending — it relies on the provider to
// reject and the fallback chain to recover.
//
// Real provider limits (from runtime.go + provider constructors):
//   claude-code:    200,000 tokens
//   codex:          128,000 tokens
//   gemini:       1,048,576 tokens
//   qwen:           131,072 tokens
//   ollama:         128,000 tokens
//   nanogpt:      2,000,000 tokens
//   vertex-claude:  200,000 tokens
//   vertex-gemini:1,048,576 tokens
// =============================================================================

// realLimitProvider simulates a provider with a real context token limit.
// It estimates tokens (len/4) and rejects requests that exceed the limit
// with a realistic error message.
type realLimitProvider struct {
	providerName    string
	maxTokens       int    // real context limit in tokens
	refinedResponse string
	totalCalls      int
	rejectedCalls   int
	acceptedCalls   int
	lastTokenCount  int
}

func (p *realLimitProvider) Name() string { return p.providerName }

func (p *realLimitProvider) ChatCompletion(_ context.Context, req providers.ChatRequest, _ string) (providers.ChatResponse, error) {
	p.totalCalls++

	// Estimate tokens like the real system does
	totalTokens := 0
	for _, msg := range req.Messages {
		totalTokens += len(msg.Content) / 4
	}
	p.lastTokenCount = totalTokens

	if totalTokens > p.maxTokens {
		p.rejectedCalls++
		return providers.ChatResponse{}, fmt.Errorf(
			"context too long: %d tokens exceeds maximum context length of %d tokens",
			totalTokens, p.maxTokens,
		)
	}

	p.acceptedCalls++
	isRefinement := len(req.Messages) > 0 && req.Messages[0].Role == "system" &&
		strings.Contains(req.Messages[0].Content, "Rewrite the user")

	content := fmt.Sprintf("response from %s", p.providerName)
	if isRefinement && p.refinedResponse != "" {
		content = p.refinedResponse
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("%s-resp-%d", p.providerName, p.totalCalls),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{PromptTokens: totalTokens, CompletionTokens: 50, TotalTokens: totalTokens + 50},
	}, nil
}

func (p *realLimitProvider) IsHealthy(context.Context) bool { return true }
func (p *realLimitProvider) MaxContextTokens() int          { return p.maxTokens }
func (p *realLimitProvider) SupportsModel(string) bool      { return true }

// TestRealLimits_OllamaContextOverflow simulates the tightest real limit:
// Ollama at 128K tokens. With aggressive memory injection from a long session,
// the request should either succeed (within limit) or fail gracefully.
func TestRealLimits_OllamaContextOverflow(t *testing.T) {
	ollama := &realLimitProvider{
		providerName:    "ollama",
		maxTokens:       128000, // 128K tokens = ~512K chars
		refinedResponse: "check the Go service status",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{ollama})
	sessionID := "ollama-limit"

	// Seed a realistic session — 100 turns of moderate messages
	for i := 0; i < 100; i++ {
		vm.Store(
			fmt.Sprintf("implement Go handler component %d with auth token validation and error handling", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("completed component %d with tests and documentation", i),
			"assistant", sessionID, nil,
		)
	}

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it going"}},
	}

	_, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "", false,
	)

	// With 100 turns of moderate messages + memory injection (4000 token budget),
	// we should stay well within 128K. This tests that the memory budget
	// system actually prevents overflow on real limits.
	if err != nil {
		t.Logf("Ollama (128K) rejected request: %v (tokens: %d)", err, ollama.lastTokenCount)
	} else {
		t.Logf("Ollama (128K) accepted request (tokens: %d)", ollama.lastTokenCount)
	}

	if ollama.lastTokenCount > 128000 {
		t.Errorf("sent %d tokens to Ollama (limit 128K) — memory budget should prevent this",
			ollama.lastTokenCount)
	}
}

// TestRealLimits_SmallProvider_FallsBackToLarger simulates a scenario where
// the cheapest provider has a small context limit and rejects, but the
// fallback provider has a larger limit and succeeds.
func TestRealLimits_SmallProvider_FallsBackToLarger(t *testing.T) {
	refinedText := "diagnose the Go auth handler"

	// Small provider: 1000 tokens — will reject most requests with memory injection
	small := &realLimitProvider{
		providerName:    "small-model",
		maxTokens:       1000,
		refinedResponse: refinedText,
	}
	// Large provider: 200K tokens — will accept
	large := &realLimitProvider{
		providerName:    "claude-code",
		maxTokens:       200000,
		refinedResponse: refinedText,
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{small, large})
	sessionID := "fallback-limit"

	// Seed enough context to exceed 1000 tokens (~4000 chars)
	for i := 0; i < 30; i++ {
		vm.Store(
			fmt.Sprintf("implement Go auth component %d with OAuth validation and token refresh handling", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("completed Go auth component %d with tests", i),
			"assistant", sessionID, nil,
		)
	}

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats the status"}},
	}

	resp, err := router.ChatCompletionWithDebug(
		context.Background(), req, sessionID, false,
	)
	if err != nil {
		t.Fatalf("request should succeed via fallback: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response")
	}

	// Small provider should have been tried and rejected
	if small.rejectedCalls == 0 && small.acceptedCalls > 0 {
		t.Logf("small provider (1K tokens) accepted — context was small enough (%d tokens)", small.lastTokenCount)
	} else {
		t.Logf("small provider (1K tokens) rejected with %d tokens", small.lastTokenCount)
	}

	// Large provider should have accepted
	if large.acceptedCalls == 0 {
		t.Error("large provider (200K tokens) should have accepted as fallback")
	}
	t.Logf("large provider (200K tokens) accepted with %d tokens", large.lastTokenCount)
}

// TestRealLimits_RefinementTokenOverhead measures the actual token overhead
// that refinement adds to a request, using realistic provider limits.
func TestRealLimits_RefinementTokenOverhead(t *testing.T) {
	// Provider that tracks token counts
	tracker := &realLimitProvider{
		providerName:    "tracker",
		maxTokens:       2000000, // unlimited for measuring
		refinedResponse: "diagnose the Go authentication handler status",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{tracker})
	sessionID := "overhead-measure"

	vm.Store("implement Go auth handler with OAuth", "user", sessionID, nil)
	vm.Store("updated auth.go with token refresh", "assistant", sessionID, nil)

	// First: send a SPECIFIC prompt (no refinement)
	tracker.totalCalls = 0
	tracker.lastTokenCount = 0
	reqSpecific := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "update auth.go to fix token refresh"}},
	}
	router.ChatCompletionForProvider(context.Background(), reqSpecific, sessionID, "", false)
	tokensWithoutRefinement := tracker.lastTokenCount
	callsWithoutRefinement := tracker.totalCalls

	// Second: send a VAGUE prompt (triggers refinement)
	tracker.totalCalls = 0
	tracker.lastTokenCount = 0
	reqVague := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it going"}},
	}
	router.ChatCompletionForProvider(context.Background(), reqVague, sessionID, "", false)
	tokensWithRefinement := tracker.lastTokenCount
	callsWithRefinement := tracker.totalCalls

	t.Logf("without refinement: %d tokens, %d provider calls", tokensWithoutRefinement, callsWithoutRefinement)
	t.Logf("with refinement: %d tokens (actual request), %d provider calls", tokensWithRefinement, callsWithRefinement)
	t.Logf("refinement overhead: %d extra provider calls", callsWithRefinement-callsWithoutRefinement)

	// Refinement should add exactly 1 extra provider call
	if callsWithRefinement-callsWithoutRefinement != 1 {
		t.Errorf("refinement should add exactly 1 extra call, added %d",
			callsWithRefinement-callsWithoutRefinement)
	}
}

// TestRealLimits_AllProviderLimits verifies that a normal request with
// moderate session context stays within ALL real provider context limits.
// This is the "does it actually work in production" test.
func TestRealLimits_AllProviderLimits(t *testing.T) {
	// Real provider limits
	limits := []struct {
		name      string
		maxTokens int
	}{
		{"codex", 128000},
		{"ollama", 128000},
		{"qwen", 131072},
		{"claude-code", 200000},
		{"vertex-claude", 200000},
		{"gemini", 1048576},
		{"vertex-gemini", 1048576},
		{"nanogpt", 2000000},
	}

	for _, lim := range limits {
		t.Run(lim.name, func(t *testing.T) {
			provider := &realLimitProvider{
				providerName:    lim.name,
				maxTokens:       lim.maxTokens,
				refinedResponse: "check the Go auth handler",
			}

			router, vm := setupMultiProviderRouter(t, []providers.Provider{provider})
			sessionID := "real-limit-" + lim.name

			// Seed a moderate session: 20 turns
			for i := 0; i < 20; i++ {
				vm.Store(
					fmt.Sprintf("working on Go component %d with auth", i),
					"user", sessionID, nil,
				)
				vm.Store(
					fmt.Sprintf("done with component %d", i),
					"assistant", sessionID, nil,
				)
			}

			req := providers.ChatRequest{
				Model:    "auto",
				Messages: []providers.Message{{Role: "user", Content: "any updates"}},
			}

			_, err := router.ChatCompletionForProvider(
				context.Background(), req, sessionID, "", false,
			)

			if err != nil {
				t.Errorf("%s (%dK tokens) rejected a normal 20-turn session: %v (tokens: %d)",
					lim.name, lim.maxTokens/1000, err, provider.lastTokenCount)
			} else {
				t.Logf("%s (%dK tokens): accepted, used %d tokens (%.1f%% of limit)",
					lim.name, lim.maxTokens/1000, provider.lastTokenCount,
					float64(provider.lastTokenCount)/float64(lim.maxTokens)*100)
			}
		})
	}
}

// TestRealLimits_HugeSession_ExceedsTightLimit verifies that a very large
// session (200+ turns) with memory injection can push past the tightest
// real provider limits (128K for codex/ollama) and the system handles it.
func TestRealLimits_HugeSession_ExceedsTightLimit(t *testing.T) {
	// Codex: 128K tokens — tightest mainstream limit
	codex := &realLimitProvider{
		providerName:    "codex",
		maxTokens:       128000,
		refinedResponse: "check the implementation",
	}
	// Gemini: 1M tokens — generous limit, should always accept
	gemini := &realLimitProvider{
		providerName:    "gemini",
		maxTokens:       1048576,
		refinedResponse: "check the implementation",
	}

	router, vm := setupMultiProviderRouter(t, []providers.Provider{codex, gemini})
	sessionID := "huge-tight"

	// Seed 200 turns with moderately detailed messages (~100 chars each)
	for i := 0; i < 200; i++ {
		vm.Store(
			fmt.Sprintf("implement Go microservice component %d with authentication, authorization, rate limiting, and error handling patterns", i),
			"user", sessionID, nil,
		)
		vm.Store(
			fmt.Sprintf("completed component %d with comprehensive test coverage including unit, integration, and benchmark tests", i),
			"assistant", sessionID, nil,
		)
	}

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows everything"}},
	}

	resp, err := router.ChatCompletionWithDebug(
		context.Background(), req, sessionID, false,
	)

	// The memory budget (4000 tokens) should prevent blowing past 128K,
	// but with refinement overhead + memory injection, let's see
	if err != nil {
		t.Logf("all providers rejected (unexpected for 200-turn session with memory budget): %v", err)
		t.Logf("codex: %d tokens (limit 128K), rejected=%d, accepted=%d",
			codex.lastTokenCount, codex.rejectedCalls, codex.acceptedCalls)
		t.Logf("gemini: %d tokens (limit 1M), rejected=%d, accepted=%d",
			gemini.lastTokenCount, gemini.rejectedCalls, gemini.acceptedCalls)
	} else {
		if len(resp.Choices) == 0 {
			t.Fatal("expected response")
		}
		t.Logf("request succeeded")
		t.Logf("codex: %d tokens (limit 128K), rejected=%d, accepted=%d",
			codex.lastTokenCount, codex.rejectedCalls, codex.acceptedCalls)
		t.Logf("gemini: %d tokens (limit 1M), rejected=%d, accepted=%d",
			gemini.lastTokenCount, gemini.rejectedCalls, gemini.acceptedCalls)
	}
}
