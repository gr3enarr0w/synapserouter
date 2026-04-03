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

	_ "modernc.org/sqlite"

	"github.com/gr3enarr0w/synapserouter/internal/memory"
	"github.com/gr3enarr0w/synapserouter/internal/orchestration"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/usage"
)

// --- Unit tests for needsRefinement ---

func TestNeedsRefinement(t *testing.T) {
	triggers := allSkillTriggers()

	tests := []struct {
		name  string
		text  string
		level refinementLevel
	}{
		// Full refinement — vague, no technical content
		{"short vague", "whats going on", refinementFull},
		{"its broken", "its broken", refinementFull},
		{"help me", "help me", refinementFull},
		{"hmm ok", "hmm ok", refinementFull},
		{"not sure", "im not sure", refinementFull},
		{"whats up", "whats up", refinementFull},
		{"can you help", "can you help", refinementFull},
		{"any thoughts", "any thoughts", refinementFull},
		{"hows it going", "hows it going", refinementFull},
		{"whats wrong with it", "whats wrong with it", refinementFull},
		{"something is off", "something is off", refinementFull},
		{"it stopped", "it stopped", refinementFull},

		// Light refinement — has triggers but sloppy
		{"sloppy auth", "fix the thing in auth", refinementLight},
		{"word vomit", "can you like fix the auth thing or whatever", refinementLight},
		{"fragmented", "auth broken whatever", refinementLight},
		{"vague with trigger", "test stuff", refinementLight},
		{"lazy fix", "fix it", refinementFull}, // "fix" no longer triggers any skill
		{"sloppy docker", "docker thing not right", refinementLight},
		{"sloppy review", "review this", refinementLight},
		{"messy implement", "implement something for the api", refinementLight},
		{"unclear research", "research that", refinementLight},
		{"terse build", "build the thing", refinementLight},

		// No refinement — well-structured
		{"clear action+target", "fix the OAuth token refresh handler in the auth middleware", refinementNone},
		{"long detailed", "implement a new REST endpoint for user management that validates input with JSON schema and returns paginated results with proper error codes", refinementNone},
		{"file path", "update handler.go", refinementNone},
		{"python file", "fix utils.py", refinementNone},
		{"backticks", "what does `fmt.Println` do", refinementNone},
		{"function ref", "func handleAuth is wrong", refinementNone},
		{"code syntax", "the class UserManager has a bug", refinementNone},
		{"clear with verb", "refactor the authentication module to use interfaces", refinementNone},
		{"empty", "", refinementNone},
		{"clear delete", "delete the old backup files from storage", refinementNone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsRefinement(tt.text, triggers)
			if got != tt.level {
				t.Errorf("needsRefinement(%q) = %q, want %q", tt.text, got, tt.level)
			}
		})
	}
}

func TestContainsCodeIndicators(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"update handler.go", true},
		{"fix utils.py", true},
		{"check `config`", true},
		{"func main", true},
		{"def parse", true},
		{"class Foo", true},
		{"just a question", false},
		{"whats going on", false},
		{"fix the auth", false},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := containsCodeIndicators(tt.text)
			if got != tt.want {
				t.Errorf("containsCodeIndicators(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}

func TestHasNounTarget(t *testing.T) {
	tests := []struct {
		words []string
		want  bool
	}{
		{[]string{"fix", "the", "bug"}, true},
		{[]string{"fix", "it"}, false},
		{[]string{"fix", "the", "a"}, false},
		{[]string{"implement", "oauth", "refresh"}, true},
		{[]string{"do"}, false},
		{[]string{}, false},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.words), func(t *testing.T) {
			got := hasNounTarget(tt.words)
			if got != tt.want {
				t.Errorf("hasNounTarget(%v) = %v, want %v", tt.words, got, tt.want)
			}
		})
	}
}

func TestAllSkillTriggers(t *testing.T) {
	triggers := allSkillTriggers()
	if len(triggers) == 0 {
		t.Fatal("expected non-empty skill triggers")
	}

	has := make(map[string]bool)
	for _, tr := range triggers {
		has[tr] = true
	}

	for _, want := range []string{"golang", "python", "docker", "auth", "test"} {
		if !has[want] {
			t.Errorf("expected trigger %q in skill triggers", want)
		}
	}
}

// --- E2E tests: full pipeline from prompt → refinement → skill dispatch ---

// refineTestProvider is a mock that echoes a controlled refinement response.
// When the system prompt contains "Rewrite the user" (the refinement prompt),
// it returns a canned refined response. Otherwise it returns a normal response.
type refineTestProvider struct {
	refinedResponse string // what to return when called for refinement
	lastRequest     *providers.ChatRequest
	callCount       int
}

func (p *refineTestProvider) Name() string { return "refine-test" }

func (p *refineTestProvider) ChatCompletion(_ context.Context, req providers.ChatRequest, _ string) (providers.ChatResponse, error) {
	p.lastRequest = &req
	p.callCount++

	content := "ok"
	// If this is a refinement call (has our system prompt), return the canned response
	if len(req.Messages) > 0 && req.Messages[0].Role == "system" &&
		strings.Contains(req.Messages[0].Content, "Rewrite the user") {
		content = p.refinedResponse
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("resp-%d", p.callCount),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "refine-test-model",
		Choices: []providers.Choice{{
			Index:        0,
			Message:      providers.Message{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: providers.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15},
	}, nil
}

func (p *refineTestProvider) IsHealthy(context.Context) bool { return true }
func (p *refineTestProvider) MaxContextTokens() int          { return 2000000 }
func (p *refineTestProvider) SupportsModel(string) bool      { return true }

func setupRefineTestRouter(t *testing.T, provider *refineTestProvider) (*Router, *memory.VectorMemory) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "refine.db")
	db, err := sql.Open("sqlite", dbPath)
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
	r := NewRouter([]providers.Provider{provider}, tracker, vm, db)
	return r, vm
}

func TestRefineIntent_VagueWithContext_SkillsMatch(t *testing.T) {
	// E2E: vague prompt + conversation context about Go auth →
	// refined prompt → skill dispatch matches go-patterns + security-review
	provider := &refineTestProvider{
		refinedResponse: "diagnose the Go authentication handler and check if the OAuth token refresh is working correctly",
	}
	router, vm := setupRefineTestRouter(t, provider)

	// Seed conversation context about Go auth work
	sessionID := "e2e-session-1"
	vm.Store("fix the Go authentication handler to properly refresh OAuth tokens", "user", sessionID, nil)
	vm.Store("I updated auth.go to handle token refresh with a 5-minute expiry window", "assistant", sessionID, nil)

	// Send vague follow-up
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats going on with it"}},
	}

	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("RefineIntent failed: %v", err)
	}
	if !refined {
		t.Fatal("expected prompt to be refined")
	}

	// The prompt should now be the refined version
	userMsg := lastUserMessage(req.Messages)
	if userMsg != provider.refinedResponse {
		t.Errorf("expected refined message %q, got %q", provider.refinedResponse, userMsg)
	}

	// The refined prompt should match skills via dispatch
	registry := orchestration.DefaultSkills()
	matched := orchestration.MatchSkills(userMsg, registry)
	names := make([]string, len(matched))
	for i, s := range matched {
		names[i] = s.Name
	}

	assertSkillPresent(t, names, "go-patterns", "refined prompt should match go-patterns")
	assertSkillPresent(t, names, "security-review", "refined prompt should match security-review (OAuth/auth)")
}

func TestRefineIntent_SloppyWithContext_SkillsMatch(t *testing.T) {
	// E2E: sloppy prompt with triggers but messy phrasing →
	// refined prompt → better skill dispatch
	provider := &refineTestProvider{
		refinedResponse: "fix the Docker container networking configuration to allow the Go API service to connect to the database",
	}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-session-2"
	vm.Store("set up the Docker compose file for the Go API and database", "user", sessionID, nil)
	vm.Store("I created docker-compose.yml with api and postgres services", "assistant", sessionID, nil)

	// Sloppy prompt — has triggers but messy
	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "docker thing not right"}},
	}

	refined, err := router.RefineIntent(context.Background(), &req, sessionID)
	if err != nil {
		t.Fatalf("RefineIntent failed: %v", err)
	}
	if !refined {
		t.Fatal("expected sloppy prompt to be refined")
	}

	userMsg := lastUserMessage(req.Messages)
	registry := orchestration.DefaultSkills()
	matched := orchestration.MatchSkills(userMsg, registry)
	names := make([]string, len(matched))
	for i, s := range matched {
		names[i] = s.Name
	}

	assertSkillPresent(t, names, "docker-expert", "refined prompt should match docker-expert")
	assertSkillPresent(t, names, "go-patterns", "refined prompt should match go-patterns (Go API)")
	// "fix" no longer triggers code-implement
}

func TestRefineIntent_VagueNoContext_Unchanged(t *testing.T) {
	// E2E: vague prompt + no session context → no refinement
	provider := &refineTestProvider{
		refinedResponse: "should not see this",
	}
	router, _ := setupRefineTestRouter(t, provider)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats going on"}},
	}

	// Empty session — no context available
	refined, err := router.RefineIntent(context.Background(), &req, "empty-session")
	if err != nil {
		t.Fatalf("RefineIntent failed: %v", err)
	}
	if refined {
		t.Error("should not refine when no context is available")
	}

	userMsg := lastUserMessage(req.Messages)
	if userMsg != "whats going on" {
		t.Errorf("prompt should be unchanged, got %q", userMsg)
	}

	// Provider should not have been called at all
	if provider.callCount != 0 {
		t.Errorf("provider should not be called when no context, got %d calls", provider.callCount)
	}
}

func TestRefineIntent_NoSession_Unchanged(t *testing.T) {
	provider := &refineTestProvider{refinedResponse: "should not see this"}
	router, _ := setupRefineTestRouter(t, provider)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats going on"}},
	}

	refined, _ := router.RefineIntent(context.Background(), &req, "")
	if refined {
		t.Error("should not refine with empty session ID")
	}
	if provider.callCount != 0 {
		t.Errorf("provider should not be called, got %d calls", provider.callCount)
	}
}

func TestRefineIntent_WellStructured_Skipped(t *testing.T) {
	// E2E: well-structured prompt → no refinement, no LLM call
	provider := &refineTestProvider{refinedResponse: "should not see this"}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-well-structured"
	vm.Store("working on the auth module", "user", sessionID, nil)

	tests := []struct {
		name   string
		prompt string
	}{
		{"file reference", "update handler.go to fix the error handling"},
		{"backtick code", "what does `fmt.Sprintf` return"},
		{"clear action", "refactor the authentication module to use interfaces"},
		{"long detailed", "implement a new REST endpoint for user management that validates input with JSON schema and returns paginated results with proper error codes"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider.callCount = 0
			req := providers.ChatRequest{
				Model:    "auto",
				Messages: []providers.Message{{Role: "user", Content: tt.prompt}},
			}

			refined, _ := router.RefineIntent(context.Background(), &req, sessionID)
			if refined {
				t.Errorf("should not refine well-structured prompt %q", tt.prompt)
			}
			if provider.callCount != 0 {
				t.Errorf("provider should not be called for well-structured prompt, got %d calls", provider.callCount)
			}

			// Original prompt should be unchanged
			userMsg := lastUserMessage(req.Messages)
			if userMsg != tt.prompt {
				t.Errorf("prompt should be unchanged, got %q", userMsg)
			}
		})
	}
}

func TestRefineIntent_LLMReturnsOriginal_NotRefined(t *testing.T) {
	// When the LLM returns the original text unchanged, don't mark as refined
	provider := &refineTestProvider{refinedResponse: "whats going on"}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-echo"
	vm.Store("working on something", "user", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats going on"}},
	}

	refined, _ := router.RefineIntent(context.Background(), &req, sessionID)
	if refined {
		t.Error("should not count as refined when LLM returns original text")
	}
}

func TestRefineIntent_RefinementRequest_HasCorrectParams(t *testing.T) {
	// Verify the refinement LLM call has expected parameters
	provider := &refineTestProvider{
		refinedResponse: "diagnose the authentication issue",
	}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-params"
	vm.Store("fix the auth handler", "user", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats happening"}},
	}

	router.RefineIntent(context.Background(), &req, sessionID)

	if provider.lastRequest == nil {
		t.Fatal("expected a refinement call")
	}

	r := provider.lastRequest
	if r.MaxTokens != 80 {
		t.Errorf("expected MaxTokens=80, got %d", r.MaxTokens)
	}
	if r.Temperature != 0 {
		t.Errorf("expected Temperature=0, got %f", r.Temperature)
	}
	if !r.SkipMemory {
		t.Error("expected SkipMemory=true to prevent recursion")
	}
	if r.Model != "auto" {
		t.Errorf("expected Model=auto, got %s", r.Model)
	}

	// Should have system prompt + context messages + user message
	if len(r.Messages) < 3 {
		t.Errorf("expected at least 3 messages (system + context + user), got %d", len(r.Messages))
	}
	if r.Messages[0].Role != "system" {
		t.Errorf("first message should be system prompt, got role=%s", r.Messages[0].Role)
	}
	if !strings.Contains(r.Messages[0].Content, "Rewrite the user") {
		t.Error("system prompt should contain refinement instructions")
	}
	// Last message should be the user's vague prompt
	lastMsg := r.Messages[len(r.Messages)-1]
	if lastMsg.Role != "user" || lastMsg.Content != "whats happening" {
		t.Errorf("last message should be the vague user prompt, got: %+v", lastMsg)
	}
}

func TestRefineIntent_FullFlow_ChatCompletionForProvider(t *testing.T) {
	// E2E: send a vague prompt through ChatCompletionForProvider and verify
	// the refinement happened (by checking the response came back and provider
	// was called twice — once for refinement, once for the actual request)
	provider := &refineTestProvider{
		refinedResponse: "check the status of the Go authentication handler changes",
	}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-full-flow"
	vm.Store("fix the Go auth handler", "user", sessionID, nil)
	vm.Store("I updated the auth handler in auth.go", "assistant", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it going"}},
	}

	resp, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "", false,
	)
	if err != nil {
		t.Fatalf("ChatCompletionForProvider failed: %v", err)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("expected response choices")
	}

	// Provider should have been called at least twice:
	// 1. Refinement call (via ChatCompletion inside RefineIntent)
	// 2. Actual request (via tryProvider in ChatCompletionForProvider)
	if provider.callCount < 2 {
		t.Errorf("expected at least 2 provider calls (refinement + actual), got %d", provider.callCount)
	}
}

func TestRefineIntent_FullFlow_SpecificSkipped(t *testing.T) {
	// E2E: send a well-structured prompt through ChatCompletionForProvider
	// and verify no extra refinement call was made
	provider := &refineTestProvider{
		refinedResponse: "should not see this",
	}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-specific"
	vm.Store("working on Go code", "user", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "update handler.go to return proper error codes"}},
	}

	_, err := router.ChatCompletionForProvider(
		context.Background(), req, sessionID, "", false,
	)
	if err != nil {
		t.Fatalf("ChatCompletionForProvider failed: %v", err)
	}

	// Provider should have been called exactly once (the actual request only)
	if provider.callCount != 1 {
		t.Errorf("expected 1 provider call (no refinement for specific prompt), got %d", provider.callCount)
	}
}

func TestRefineIntent_MultipleVagueFollowups(t *testing.T) {
	// E2E: simulate a conversation where user sends multiple vague follow-ups
	provider := &refineTestProvider{}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-multi"
	vm.Store("implement the Go REST endpoint for user management", "user", sessionID, nil)
	vm.Store("I created the handler with proper validation", "assistant", sessionID, nil)

	// First vague follow-up
	provider.refinedResponse = "check the status of the Go REST endpoint implementation for user management"
	provider.callCount = 0
	req1 := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "hows it look"}},
	}
	refined1, _ := router.RefineIntent(context.Background(), &req1, sessionID)
	if !refined1 {
		t.Error("first vague follow-up should be refined")
	}

	// Verify the refined prompt matches skills
	msg1 := lastUserMessage(req1.Messages)
	matched1 := orchestration.MatchSkills(msg1, orchestration.DefaultSkills())
	if len(matched1) == 0 {
		t.Errorf("first refined prompt %q should match at least one skill", msg1)
	}

	// Second vague follow-up
	provider.refinedResponse = "test the Go REST endpoint handler to verify it handles edge cases"
	provider.callCount = 0
	req2 := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "now what"}},
	}
	refined2, _ := router.RefineIntent(context.Background(), &req2, sessionID)
	if !refined2 {
		t.Error("second vague follow-up should be refined")
	}

	msg2 := lastUserMessage(req2.Messages)
	matched2 := orchestration.MatchSkills(msg2, orchestration.DefaultSkills())
	if len(matched2) == 0 {
		t.Errorf("second refined prompt %q should match at least one skill", msg2)
	}
}

func TestRefineIntent_SkipMemoryPreventsRefinement(t *testing.T) {
	// Verify that SkipMemory on the outer request prevents refinement
	// (the router checks this before calling RefineIntent)
	provider := &refineTestProvider{refinedResponse: "should not be called"}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-skip"
	vm.Store("working on auth", "user", sessionID, nil)

	req := providers.ChatRequest{
		Model:      "auto",
		Messages:   []providers.Message{{Role: "user", Content: "whats up"}},
		SkipMemory: true,
	}

	// Simulate what the router does — check SkipMemory before calling RefineIntent
	if !req.SkipMemory {
		router.RefineIntent(context.Background(), &req, sessionID)
	}

	if provider.callCount != 0 {
		t.Error("provider should not be called when SkipMemory is true")
	}
}

// --- Regression tests: original prompt preserved on refinement failure ---

func TestRefineIntent_EmptyLLMResponse_Unchanged(t *testing.T) {
	provider := &refineTestProvider{refinedResponse: ""}
	router, vm := setupRefineTestRouter(t, provider)

	sessionID := "e2e-empty-resp"
	vm.Store("working on code", "user", sessionID, nil)

	req := providers.ChatRequest{
		Model:    "auto",
		Messages: []providers.Message{{Role: "user", Content: "whats up"}},
	}

	refined, _ := router.RefineIntent(context.Background(), &req, sessionID)
	if refined {
		t.Error("should not be refined when LLM returns empty")
	}
	if lastUserMessage(req.Messages) != "whats up" {
		t.Error("original prompt should be preserved on empty LLM response")
	}
}

// helpers

func assertSkillPresent(t *testing.T, names []string, target, msg string) {
	t.Helper()
	for _, n := range names {
		if n == target {
			return
		}
	}
	t.Errorf("%s — got skills: %v", msg, names)
}
