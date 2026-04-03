package subscription_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/gr3enarr0w/synapserouter/subscription"
)

// TestMockSubscriptionServer validates that the mock server returns correct models and roles
func TestMockSubscriptionServer(t *testing.T) {
	// Create mock server with updated models that match model_routing.json
	mockServer := subscription.NewMockSubscriptionServerWithRealModels()

	req, err := http.NewRequest(http.MethodGet, "/api/subscription/v1/models", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	recorder := newResponseRecorder()
	mockServer.HandleModelsRequest(recorder, req)

	if recorder.statusCode != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", recorder.statusCode)
	}

	var modelResp subscription.ModelListResponse
	if err := json.Unmarshal(recorder.body, &modelResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Validate that we have models
	if len(modelResp.Models) == 0 {
		t.Fatal("No models returned from mock server")
	}

	// Validate that models match the expected roles from model_routing.json
	expectedRoles := map[string][]string{
		"architect":      {"architect"},
		"implementation": {"implementation"},
		"code_review":    {"code_review"},
		"debugging":      {"debugging"},
		"testing":        {"testing"},
		"documentation":  {"documentation"},
		"research":       {"research"},
		"general":        {"general"},
	}

	roleToModels := make(map[string][]string)
	for _, model := range modelResp.Models {
		for _, role := range model.Roles {
			roleToModels[role] = append(roleToModels[role], model.ID)
		}

		// Validate model status
		if !model.IsAvailable() {
			t.Errorf("Model %s should be available", model.ID)
		}

		// Validate required fields
		if model.ID == "" {
			t.Error("Model ID should not be empty")
		}
		if model.Name == "" {
			t.Error("Model name should not be empty")
		}
	}

	// Check that all expected roles have models
	for expectedRole := range expectedRoles {
		models, exists := roleToModels[expectedRole]
		if !exists {
			t.Errorf("Expected role '%s' not found in any model", expectedRole)
			continue
		}

		if len(models) == 0 {
			t.Errorf("No models found for role '%s'", expectedRole)
		}

		t.Logf("Role '%s' has %d models: %v", expectedRole, len(models), models)
	}

	// Log all models for debugging
	t.Logf("Mock server returned %d models:", len(modelResp.Models))
	for i, model := range modelResp.Models {
		t.Logf("  %d. %s (%s) - Roles: %v, Status: %s",
			i+1, model.ID, model.Name, model.Roles, model.Status)
	}
}

// TestMockSubscriptionServerExhaustion tests model exhaustion tracking
func TestMockSubscriptionServerExhaustion(t *testing.T) {
	mockServer := subscription.NewMockSubscriptionServerWithRealModels()

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			recorder := newResponseRecorder()
			mockServer.HandleModelsRequest(recorder, req)
			return recorder.response(req), nil
		}),
	}

	subMgr := subscription.NewManager(
		"http://mock-subscription.test",
		subscription.WithCacheTTL(time.Second),
		subscription.WithHTTPClient(client),
	)

	// Test getting models for a role
	selection, err := subMgr.GetNextModel("architect")
	if err != nil {
		t.Fatalf("Failed to get model: %v", err)
	}

	if selection.Model.ID == "" {
		t.Fatal("Expected model ID to be set")
	}

	// Mark model as exhausted
	subMgr.MarkExhausted(selection.Model.ID)

	// Try to get another model for the same role
	selection2, err := subMgr.GetNextModel("architect")
	if err != nil {
		// This might be expected if all models are exhausted
		t.Logf("No more models available (expected): %v", err)
	} else {
		// If we get a model, it should be different
		if selection2.Model.ID == selection.Model.ID {
			t.Error("Expected different model after marking first as exhausted")
		}
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type responseRecorder struct {
	header     http.Header
	body       []byte
	statusCode int
}

func newResponseRecorder() *responseRecorder {
	return &responseRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.body = append(r.body, data...)
	return len(data), nil
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.statusCode = statusCode
}

func (r *responseRecorder) response(req *http.Request) *http.Response {
	return &http.Response{
		StatusCode: r.statusCode,
		Header:     r.header.Clone(),
		Body:       io.NopCloser(bytes.NewReader(r.body)),
		Request:    req,
	}
}
