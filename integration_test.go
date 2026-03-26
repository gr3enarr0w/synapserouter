// +build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/usage"
)

// Integration tests verify end-to-end functionality of the Synapse Router

type mockProvider struct {
	name  string
	calls int
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	m.calls++
	content := "Mock response from " + m.name
	if len(req.Messages) > 0 {
		content = fmt.Sprintf("Echo from %s: %s", m.name, req.Messages[len(req.Messages)-1].Content)
	}

	return providers.ChatResponse{
		ID:      fmt.Sprintf("%s-response-%d", m.name, m.calls),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []providers.Choice{
			{
				Index: 0,
				Message: providers.Message{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{
			PromptTokens:     100,
			CompletionTokens: 50,
			TotalTokens:      150,
		},
	}, nil
}

func (m *mockProvider) Available(ctx context.Context) bool {
	return true
}

func setupIntegrationTest(t *testing.T) (*httptest.Server, *sql.DB, func()) {
	t.Helper()

	// Create in-memory database
	testDB, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	// Load migration
	migrationSQL, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := testDB.Exec(string(migrationSQL)); err != nil {
		t.Fatal(err)
	}

	// Initialize components
	mockProviders := []providers.Provider{
		&mockProvider{name: "mock-primary"},
		&mockProvider{name: "mock-fallback"},
	}

	usageTracker, err := usage.NewTrackerWithDB(testDB)
	if err != nil {
		t.Fatal(err)
	}

	vectorMemory := memory.NewVectorMemory(testDB)
	proxyRouter = router.NewRouter(mockProviders, usageTracker, vectorMemory, testDB)
	orchestrator = orchestration.NewManagerWithStore(proxyRouter, vectorMemory, testDB)

	// Set global variables for handlers
	db = testDB
	providerList = mockProviders

	// Create HTTP router (Go 1.22+ stdlib routing)
	r := http.NewServeMux()
	r.HandleFunc("GET /health", healthHandler)
	r.HandleFunc("GET /v1/models", modelsHandler)
	r.HandleFunc("POST /v1/chat/completions", chatHandler)
	r.HandleFunc("GET /v1/providers", providersHandler)
	r.Handle("GET /v1/orchestration/tasks", withAdminAuth(http.HandlerFunc(orchestrationTasksHandler)))
	r.Handle("POST /v1/orchestration/tasks", withAdminAuth(http.HandlerFunc(orchestrationTasksHandler)))
	r.Handle("GET /v1/orchestration/tasks/{task_id}", withAdminAuth(http.HandlerFunc(orchestrationTaskHandler)))
	r.Handle("GET /v1/orchestration/agents", withAdminAuth(http.HandlerFunc(orchestrationAgentsHandler)))
	r.Handle("POST /v1/orchestration/agents", withAdminAuth(http.HandlerFunc(orchestrationAgentsHandler)))
	r.Handle("GET /v1/orchestration/swarms", withAdminAuth(http.HandlerFunc(orchestrationSwarmsHandler)))
	r.Handle("POST /v1/orchestration/swarms", withAdminAuth(http.HandlerFunc(orchestrationSwarmsHandler)))

	// Create test server
	server := httptest.NewServer(r)

	cleanup := func() {
		server.Close()
		usageTracker.Close()
		testDB.Close()
	}

	return server, testDB, cleanup
}

func TestIntegration_HealthCheck(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status ok, got %v", result["status"])
	}
}

func TestIntegration_ChatCompletion(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	reqBody := map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": "Hello, world!"},
		},
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result providers.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Choices) == 0 {
		t.Fatal("expected at least one choice in response")
	}

	if result.Choices[0].Message.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", result.Choices[0].Message.Role)
	}

	if result.Choices[0].Message.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestIntegration_ListModels(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatal("expected data array in response")
	}

	if len(data) == 0 {
		t.Error("expected at least one model")
	}
}

func TestIntegration_ProviderStats(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	resp, err := http.Get(server.URL + "/v1/providers")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	providers, ok := result["providers"].([]interface{})
	if !ok {
		t.Fatal("expected providers array in response")
	}

	if len(providers) != 2 {
		t.Errorf("expected 2 providers, got %d", len(providers))
	}
}

func TestIntegration_OrchestrationTaskCreation(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Set admin token for auth
	os.Setenv("ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	reqBody := map[string]interface{}{
		"goal":       "Test task goal",
		"session_id": "test-session",
		"execute":    false,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", server.URL+"/v1/orchestration/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-admin-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["id"] == nil {
		t.Error("expected task ID in response")
	}

	if result["goal"] != "Test task goal" {
		t.Errorf("expected goal 'Test task goal', got %v", result["goal"])
	}
}

func TestIntegration_OrchestrationAgentListing(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	os.Setenv("ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	req, _ := http.NewRequest("GET", server.URL+"/v1/orchestration/agents", nil)
	req.Header.Set("Authorization", "Bearer test-admin-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	agents, ok := result["agents"].([]interface{})
	if !ok {
		t.Fatal("expected agents array in response")
	}

	// Default agents should be initialized
	if len(agents) == 0 {
		t.Error("expected default agents to be initialized")
	}
}

func TestIntegration_OrchestrationSwarmCreation(t *testing.T) {
	server, _, cleanup := setupIntegrationTest(t)
	defer cleanup()

	os.Setenv("ADMIN_TOKEN", "test-admin-token")
	defer os.Unsetenv("ADMIN_TOKEN")

	reqBody := map[string]interface{}{
		"objective":  "Test swarm objective",
		"topology":   "flat",
		"strategy":   "round-robin",
		"max_agents": 3,
	}

	body, _ := json.Marshal(reqBody)
	req, _ := http.NewRequest("POST", server.URL+"/v1/orchestration/swarms", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-admin-token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if result["id"] == nil {
		t.Error("expected swarm ID in response")
	}

	if result["objective"] != "Test swarm objective" {
		t.Errorf("expected objective 'Test swarm objective', got %v", result["objective"])
	}
}

func TestIntegration_ProviderFallback(t *testing.T) {
	server, testDB, cleanup := setupIntegrationTest(t)
	defer cleanup()

	// Create a provider that fails
	failingProvider := &failingMockProvider{name: "failing-provider"}
	workingProvider := &mockProvider{name: "working-fallback"}

	// Replace provider list
	providerList = []providers.Provider{failingProvider, workingProvider}
	usageTracker, _ := usage.NewTrackerWithDB(testDB)
	vectorMemory := memory.NewVectorMemory(testDB)
	proxyRouter = router.NewRouter(providerList, usageTracker, vectorMemory, testDB)

	reqBody := map[string]interface{}{
		"model": "test-model",
		"messages": []map[string]string{
			{"role": "user", "content": "Test fallback"},
		},
	}

	body, _ := json.Marshal(reqBody)
	resp, err := http.Post(server.URL+"/v1/chat/completions", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result providers.ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	// Should have fallen back to working provider
	if result.Choices[0].Message.Content == "" {
		t.Error("expected response from fallback provider")
	}

	if workingProvider.calls == 0 {
		t.Error("expected fallback provider to be called")
	}
}

// Helper types

type failingMockProvider struct {
	name string
}

func (f *failingMockProvider) Name() string {
	return f.name
}

func (f *failingMockProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest) (providers.ChatResponse, error) {
	return providers.ChatResponse{}, fmt.Errorf("provider unavailable")
}

func (f *failingMockProvider) Available(ctx context.Context) bool {
	return false
}
