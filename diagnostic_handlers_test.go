package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/app"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/memory"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/router"
)

type diagMockProvider struct {
	name string
}

func (p *diagMockProvider) Name() string { return p.name }
func (p *diagMockProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	return providers.ChatResponse{
		ID:      fmt.Sprintf("%s-resp", p.name),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   "mock-model",
		Choices: []providers.Choice{{Index: 0, Message: providers.Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		Usage:   providers.Usage{TotalTokens: 10},
	}, nil
}
func (p *diagMockProvider) IsHealthy(ctx context.Context) bool { return true }
func (p *diagMockProvider) MaxContextTokens() int              { return 100000 }
func (p *diagMockProvider) SupportsModel(model string) bool    { return true }

func TestSmokeTestHandler(t *testing.T) {
	testDB := newAuditTestDB(t)
	db = testDB
	vectorMemory = memory.NewVectorMemory(testDB)

	providerList = []providers.Provider{&diagMockProvider{name: "mock-test"}}
	proxyRouter = router.NewRouter(providerList, nil, vectorMemory, testDB)

	body := `{"timeout": "5s"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/test/providers", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	smokeTestHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var results []app.SmokeTestResult
	if err := json.Unmarshal(rr.Body.Bytes(), &results); err != nil {
		t.Fatal(err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
}

func TestCircuitBreakerResetHandler(t *testing.T) {
	testDB := newAuditTestDB(t)
	db = testDB
	vectorMemory = memory.NewVectorMemory(testDB)

	providerList = []providers.Provider{&diagMockProvider{name: "mock-cb"}}
	proxyRouter = router.NewRouter(providerList, nil, vectorMemory, testDB)

	req := httptest.NewRequest(http.MethodPost, "/v1/circuit-breakers/reset", bytes.NewBufferString(`{}`))
	rr := httptest.NewRecorder()

	circuitBreakerResetHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Reset  []string                       `json:"reset"`
		States map[string]router.CircuitState `json:"states"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
}

func TestCircuitBreakerResetHandler_SingleProvider(t *testing.T) {
	testDB := newAuditTestDB(t)
	db = testDB
	vectorMemory = memory.NewVectorMemory(testDB)

	providerList = []providers.Provider{&diagMockProvider{name: "mock-single"}}
	proxyRouter = router.NewRouter(providerList, nil, vectorMemory, testDB)

	body := `{"provider": "mock-single"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/circuit-breakers/reset", bytes.NewBufferString(body))
	rr := httptest.NewRecorder()

	circuitBreakerResetHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestProfileHandler(t *testing.T) {
	providerList = []providers.Provider{&diagMockProvider{name: "mock-profile"}}

	req := httptest.NewRequest(http.MethodGet, "/v1/profile", nil)
	rr := httptest.NewRecorder()

	profileHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if _, ok := resp["active"]; !ok {
		t.Error("expected 'active' field in response")
	}
	if _, ok := resp["providers"]; !ok {
		t.Error("expected 'providers' field in response")
	}
}

func TestDoctorHandler(t *testing.T) {
	testDB := newAuditTestDB(t)
	db = testDB
	vectorMemory = memory.NewVectorMemory(testDB)
	usageTracker = nil

	providerList = []providers.Provider{&diagMockProvider{name: "mock-doctor"}}

	req := httptest.NewRequest(http.MethodGet, "/v1/doctor", nil)
	rr := httptest.NewRecorder()

	doctorHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var checks []app.DiagnosticCheck
	if err := json.Unmarshal(rr.Body.Bytes(), &checks); err != nil {
		t.Fatal(err)
	}

	if len(checks) == 0 {
		t.Fatal("expected diagnostic checks")
	}
}
