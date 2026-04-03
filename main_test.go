package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"

	"github.com/gr3enarr0w/synapserouter/internal/memory"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/router"
	"github.com/gr3enarr0w/synapserouter/internal/usage"
)

func TestMemorySearchHandler(t *testing.T) {
	db := newFallbackTestDB(t)
	vectorMemory = memory.NewVectorMemory(db)

	if err := vectorMemory.Store("Go router fallback logic for provider availability", "user", "session-1", nil); err != nil {
		t.Fatal(err)
	}
	if err := vectorMemory.Store("Python notebook discussion", "assistant", "session-1", nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/memory/search?session_id=session-1&q=go+fallback", nil)
	rr := httptest.NewRecorder()

	memorySearchHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		ResultCount int              `json:"result_count"`
		Messages    []memory.Message `json:"messages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	if payload.ResultCount == 0 {
		t.Fatal("expected at least one search result")
	}
	if payload.Messages[0].Content != "Go router fallback logic for provider availability" {
		t.Fatalf("unexpected top result: %q", payload.Messages[0].Content)
	}
}

func TestMemorySessionHandler(t *testing.T) {
	db := newFallbackTestDB(t)
	vectorMemory = memory.NewVectorMemory(db)

	if err := vectorMemory.Store("first", "user", "session-2", nil); err != nil {
		t.Fatal(err)
	}
	if err := vectorMemory.Store("second", "assistant", "session-2", nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/memory/session/session-2", nil)
	req.SetPathValue("session_id", "session-2")
	rr := httptest.NewRecorder()

	memorySessionHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		MessageCount int              `json:"message_count"`
		Messages     []memory.Message `json:"messages"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}

	if payload.MessageCount != 2 {
		t.Fatalf("expected 2 messages, got %d", payload.MessageCount)
	}
	if payload.Messages[0].Content != "first" || payload.Messages[1].Content != "second" {
		t.Fatalf("unexpected session history: %+v", payload.Messages)
	}
}

func TestMemorySearchHandlerRejectsMissingSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/memory/search?q=go", nil)
	rr := httptest.NewRecorder()

	memorySearchHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestWithAdminAuthRejectsUnauthorizedRequest(t *testing.T) {
	t.Setenv("SYNROUTE_ADMIN_TOKEN", "secret")

	handler := withAdminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d", rr.Code)
	}
}

func TestWithAdminAuthAcceptsBearerToken(t *testing.T) {
	t.Setenv("SYNROUTE_ADMIN_TOKEN", "secret")

	handler := withAdminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestWithAdminAuthAcceptsLegacyAdminAPIKeyEnv(t *testing.T) {
	t.Setenv("ADMIN_API_KEY", "legacy-secret")

	handler := withAdminAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	req.Header.Set("Authorization", "Bearer legacy-secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
}

func TestAuditSessionHandler(t *testing.T) {
	db = newAuditTestDB(t)

	_, err := db.Exec(`
		INSERT INTO request_audit (
			request_id, session_id, selected_provider, final_provider, final_model,
			memory_query, memory_candidate_count, success, error_message, created_at
		) VALUES ('req-1', 'session-a', 'claude-code', 'nanogpt', 'nanogpt-test',
		          'go fallback', 2, 1, '', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/audit/session/session-a?limit=10", nil)
	req.SetPathValue("session_id", "session-a")
	rr := httptest.NewRecorder()

	auditSessionHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Count   int                      `json:"count"`
		Entries []map[string]interface{} `json:"entries"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 1 {
		t.Fatalf("expected 1 audit entry, got %d", payload.Count)
	}
}

func TestAuditRequestHandler(t *testing.T) {
	db = newAuditTestDB(t)

	_, err := db.Exec(`
		INSERT INTO request_audit (
			request_id, session_id, selected_provider, final_provider, final_model,
			memory_query, memory_candidate_count, success, error_message, created_at
		) VALUES ('req-2', 'session-b', 'claude-code', 'codex', 'gpt-5-codex',
		          'provider debug', 1, 1, '', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`
		INSERT INTO provider_attempt_audit (
			request_id, provider, attempt_index, success, error_message, created_at
		) VALUES ('req-2', 'claude-code', 1, 0, 'rate limited', CURRENT_TIMESTAMP),
		         ('req-2', 'codex', 2, 1, '', CURRENT_TIMESTAMP)
	`)
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/audit/request/req-2", nil)
	req.SetPathValue("request_id", "req-2")
	rr := httptest.NewRecorder()

	auditRequestHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		RequestID string                   `json:"request_id"`
		Attempts  []map[string]interface{} `json:"attempts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.RequestID != "req-2" {
		t.Fatalf("unexpected request_id: %s", payload.RequestID)
	}
	if len(payload.Attempts) != 2 {
		t.Fatalf("expected 2 attempts, got %d", len(payload.Attempts))
	}
}

func TestTraceHandler(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace-handler.db")
	testDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db = testDB
	t.Cleanup(func() { _ = testDB.Close() })

	sqlBytes, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Exec(string(sqlBytes)); err != nil {
		t.Fatal(err)
	}

	vectorMemory = memory.NewVectorMemory(testDB)
	usageTracker, err = usage.NewTracker(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = usageTracker.Close() })

	proxyRouter = router.NewRouter([]providers.Provider{
		&traceTestProvider{name: "nanogpt"},
	}, usageTracker, vectorMemory, testDB)

	// Seed prior context so trace finds memory candidates
	if err := vectorMemory.Store("go fallback trace query", "user", "session-trace", nil); err != nil {
		t.Fatal(err)
	}
	if err := vectorMemory.Store("Here is how Go fallback routing works", "assistant", "session-trace", nil); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/debug/trace?session_id=session-trace", strings.NewReader(`{
		"model":"gpt-test",
		"messages":[{"role":"user","content":"tell me more about go fallback trace"}]
	}`))
	rr := httptest.NewRecorder()

	traceHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		SelectedProvider     string `json:"selected_provider"`
		MemoryCandidateCount int    `json:"memory_candidate_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SelectedProvider == "" {
		t.Fatal("expected selected provider")
	}
	if payload.MemoryCandidateCount == 0 {
		t.Fatal("expected memory candidates in trace")
	}
}

func TestProvidersHandler(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "providers.db")
	testDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	db = testDB
	t.Cleanup(func() { _ = testDB.Close() })

	sqlBytes, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Exec(string(sqlBytes)); err != nil {
		t.Fatal(err)
	}

	usageTracker, err = usage.NewTracker(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = usageTracker.Close() })

	providerList = []providers.Provider{&traceTestProvider{name: "nanogpt"}}

	req := httptest.NewRequest(http.MethodGet, "/v1/providers", nil)
	rr := httptest.NewRecorder()
	providersHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Count != 1 {
		t.Fatalf("expected 1 provider, got %d", payload.Count)
	}
}

func TestStartupCheckHandler(t *testing.T) {
	startupCheck = map[string]interface{}{
		"provider_count": 1,
		"healthy_count":  1,
		"all_healthy":    true,
		"providers": []map[string]interface{}{
			{"name": "nanogpt", "healthy": true},
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/v1/startup-check", nil)
	rr := httptest.NewRecorder()
	startupCheckHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		ProviderCount int `json:"provider_count"`
		HealthyCount  int `json:"healthy_count"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ProviderCount != 1 || payload.HealthyCount != 1 {
		t.Fatalf("unexpected startup payload: %+v", payload)
	}
}

type traceTestProvider struct {
	name string
}

func (p *traceTestProvider) Name() string { return p.name }

func (p *traceTestProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	return providers.ChatResponse{}, errors.New("not used in trace test")
}

func (p *traceTestProvider) IsHealthy(ctx context.Context) bool { return true }

func (p *traceTestProvider) MaxContextTokens() int { return 2000000 }

func (p *traceTestProvider) SupportsModel(model string) bool { return true }

func newFallbackTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}

	_, err = db.Exec(`
		CREATE TABLE memory (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			content TEXT NOT NULL,
			embedding BLOB,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			session_id TEXT,
			role TEXT,
			metadata TEXT
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = db.Close()
	})

	return db
}

func newAuditTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "audit.db")
	testDB, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	sqlBytes, err := os.ReadFile("migrations/001_init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDB.Exec(string(sqlBytes)); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = testDB.Close()
	})

	return testDB
}
