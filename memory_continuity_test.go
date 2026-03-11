package main

import (
	"context"
	"database/sql"
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/usage"

	_ "github.com/mattn/go-sqlite3"
)

// TestMemoryContinuity tests the fix for BUG-MEMORY-001
// Verifies that same-session requests retrieve and include prior context
func TestMemoryContinuity(t *testing.T) {
	// Setup in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create memory table
	schema := `
	CREATE TABLE memory (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		content TEXT NOT NULL,
		embedding BLOB,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		session_id TEXT,
		role TEXT,
		metadata TEXT
	);
	CREATE TABLE usage (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		provider TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		tokens INTEGER,
		response_id TEXT,
		model TEXT
	);
	CREATE TABLE request_audit (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		request_id TEXT,
		session_id TEXT,
		first_provider TEXT,
		final_provider TEXT,
		model TEXT,
		memory_query TEXT,
		proxy_metadata TEXT,
		provider_attempts TEXT,
		error_message TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE circuit_breaker_state (
		provider TEXT PRIMARY KEY,
		state TEXT,
		failures INTEGER,
		last_failure DATETIME
	);`
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}

	// Create vector memory
	vm := memory.NewVectorMemory(db)

	// Create usage tracker with db connection
	// Note: usage.Tracker is not exported, so we'll call NewTracker with a temp file
	tempDB := ":memory:"
	ut, err := usage.NewTracker(tempDB)
	if err != nil {
		t.Fatal(err)
	}

	// Create mock provider
	mockProvider := &mockChatProvider{
		name: "test-provider",
		responses: map[string]providers.ChatResponse{
			"first": {
				ID:    "msg_001",
				Model: "test-model",
				Choices: []providers.Choice{
					{
						Index: 0,
						Message: providers.Message{
							Role:    "assistant",
							Content: "Secret code: XYZZY123",
						},
						FinishReason: "stop",
					},
				},
				Usage: providers.Usage{
					PromptTokens:     10,
					CompletionTokens: 5,
					TotalTokens:      15,
				},
			},
			"second": {
				ID:    "msg_002",
				Model: "test-model",
				Choices: []providers.Choice{
					{
						Index: 0,
						Message: providers.Message{
							Role:    "assistant",
							Content: "The code is XYZZY123",
						},
						FinishReason: "stop",
					},
				},
				Usage: providers.Usage{
					PromptTokens:     30,
					CompletionTokens: 5,
					TotalTokens:      35,
				},
			},
		},
		callCount: 0,
	}

	// Create router
	r := router.NewRouter([]providers.Provider{mockProvider}, ut, vm, db)

	sessionID := "test-session-001"

	// First request: Store a secret
	req1 := providers.ChatRequest{
		Model: "test-model",
		Messages: []providers.Message{
			{Role: "user", Content: "Remember this secret code: XYZZY123"},
		},
	}

	resp1, err := r.ChatCompletionWithDebug(context.Background(), req1, sessionID, false)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	if resp1.ID != "msg_001" {
		t.Errorf("Expected response ID msg_001, got %s", resp1.ID)
	}

	// Second request: Ask about the secret (should retrieve from memory)
	req2 := providers.ChatRequest{
		Model: "test-model",
		Messages: []providers.Message{
			{Role: "user", Content: "What was the secret code?"},
		},
	}

	_, err = r.ChatCompletionWithDebug(context.Background(), req2, sessionID, false)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}

	// Verify that the mock provider received the prior context
	// The mock provider should have received 2 messages:
	// 1. "Remember this secret code: XYZZY123" (from memory - previous user message)
	// 2. "What was the secret code?" (current request)
	// Note: Assistant responses are not currently stored in memory, only user messages
	if mockProvider.lastRequest == nil {
		t.Fatal("Mock provider never received a request")
	}

	if len(mockProvider.lastRequest.Messages) < 2 {
		t.Errorf("Expected at least 2 messages in second request (with memory), got %d: %+v",
			len(mockProvider.lastRequest.Messages), mockProvider.lastRequest.Messages)
	}

	if len(mockProvider.lastRequest.Messages) == 1 {
		t.Fatal("Memory was not injected - only got current request message")
	}

	// Verify the first message is from memory
	if len(mockProvider.lastRequest.Messages) >= 1 {
		firstMsg := mockProvider.lastRequest.Messages[0]
		if firstMsg.Content != "Remember this secret code: XYZZY123" {
			t.Errorf("Expected first message from memory, got: %s", firstMsg.Content)
		}
	}

	// Verify the last message is the current request
	if len(mockProvider.lastRequest.Messages) >= 1 {
		lastMsg := mockProvider.lastRequest.Messages[len(mockProvider.lastRequest.Messages)-1]
		if lastMsg.Content != "What was the secret code?" {
			t.Errorf("Expected last message to be current request, got: %s", lastMsg.Content)
		}
	}

	t.Logf("✅ Memory continuity test passed - prior messages were injected")
	t.Logf("   Session %s: Retrieved %d prior messages", sessionID, len(mockProvider.lastRequest.Messages)-1)
}

// mockChatProvider is a mock provider for testing
type mockChatProvider struct {
	name        string
	responses   map[string]providers.ChatResponse
	callCount   int
	lastRequest *providers.ChatRequest
}

func (m *mockChatProvider) Name() string {
	return m.name
}

func (m *mockChatProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	m.lastRequest = &req
	m.callCount++

	responseKey := "first"
	if m.callCount > 1 {
		responseKey = "second"
	}

	resp := m.responses[responseKey]
	return resp, nil
}

func (m *mockChatProvider) IsHealthy(ctx context.Context) bool {
	return true
}

func (m *mockChatProvider) MaxContextTokens() int {
	return 128000
}
