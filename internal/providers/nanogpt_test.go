package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewNanoGPTProvider_Tiers(t *testing.T) {
	// Clear env override so default URLs are used
	t.Setenv("NANOGPT_BASE_URL", "")

	tests := []struct {
		name        string
		tier        string
		wantName    string
		wantURLSub string // substring expected in baseURL
	}{
		{"subscription tier", "subscription", "nanogpt-sub", "/api/subscription/v1"},
		{"paid tier", "paid", "nanogpt-paid", "/api/paid/v1"},
		{"unknown defaults to subscription", "unknown", "nanogpt-sub", "/api/subscription/v1"},
		{"empty defaults to subscription", "", "nanogpt-sub", "/api/subscription/v1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewNanoGPTProvider("test-key", tt.tier)
			if p.Name() != tt.wantName {
				t.Errorf("Name() = %q, want %q", p.Name(), tt.wantName)
			}
			if p.baseURL == "" {
				t.Fatal("baseURL is empty")
			}
			if got := p.baseURL; !contains(got, tt.wantURLSub) {
				t.Errorf("baseURL = %q, want substring %q", got, tt.wantURLSub)
			}
		})
	}
}

func TestNewNanoGPTProvider_EnvOverride(t *testing.T) {
	t.Setenv("NANOGPT_BASE_URL", "http://localhost:9999/custom")
	p := NewNanoGPTProvider("test-key", "subscription")
	if p.baseURL != "http://localhost:9999/custom" {
		t.Errorf("baseURL = %q, want env override", p.baseURL)
	}
}

func TestNanoGPTProvider_SupportsModel(t *testing.T) {
	t.Setenv("NANOGPT_BASE_URL", "")

	sub := NewNanoGPTProvider("key", "subscription")
	paid := NewNanoGPTProvider("key", "paid")

	tests := []struct {
		name     string
		provider *NanoGPTProvider
		model    string
		want     bool
	}{
		// Auto/empty → subscription only
		{"sub supports auto", sub, "auto", true},
		{"sub supports empty", sub, "", true},
		{"paid rejects auto", paid, "auto", false},
		{"paid rejects empty", paid, "", false},

		// Subscription models
		{"sub supports qwen", sub, "qwen/qwen3.5-plus", true},
		{"sub supports deepseek-r1", sub, "deepseek-r1", true},
		{"sub supports glm-5", sub, "glm-5-plus", true},
		{"paid rejects qwen", paid, "qwen/qwen3.5-plus", false},

		// Paid models
		{"paid supports gpt-5", paid, "openai/gpt-5", true},
		{"paid supports claude-opus", paid, "anthropic/claude-opus-4.6", true},
		{"paid supports grok-4", paid, "grok-4", true},
		{"sub rejects gpt-5", sub, "openai/gpt-5", false},

		// Unknown models → neither supports
		{"sub rejects unknown", sub, "some-random-model", false},
		{"paid rejects unknown", paid, "some-random-model", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.provider.SupportsModel(tt.model)
			if got != tt.want {
				t.Errorf("SupportsModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

func TestNanoGPTModelTier(t *testing.T) {
	tests := []struct {
		model string
		want  string
	}{
		{"qwen/qwen3.5-plus", NanoGPTTierSubscription},
		{"deepseek-r1", NanoGPTTierSubscription},
		{"deepseek-chat", NanoGPTTierSubscription},
		{"kimi-k2.5", NanoGPTTierSubscription},
		{"step-3", NanoGPTTierSubscription},
		{"openai/gpt-5", NanoGPTTierAPI},
		{"anthropic/claude-sonnet-4.6", NanoGPTTierAPI},
		{"google/gemini-3", NanoGPTTierAPI},
		{"x-ai/grok-4", NanoGPTTierAPI},
		{"unknown-model", ""},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := nanoGPTModelTier(tt.model)
			if got != tt.want {
				t.Errorf("nanoGPTModelTier(%q) = %q, want %q", tt.model, got, tt.want)
			}
		})
	}
}

func TestNanoGPTProvider_ChatCompletion_DefaultModel(t *testing.T) {
	// Mock server that echoes back the model from the request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := ChatResponse{
			ID:    "test-resp",
			Model: req.Model,
			Choices: []Choice{
				{Index: 0, Message: Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tests := []struct {
		name      string
		tier      string
		reqModel  string
		wantModel string
	}{
		{"sub auto defaults to qwen", "subscription", "auto", "qwen/qwen3.5-plus"},
		{"sub empty defaults to qwen", "subscription", "", "qwen/qwen3.5-plus"},
		{"paid auto defaults to chatgpt", "paid", "auto", "chatgpt-4o-latest"},
		{"paid empty defaults to chatgpt", "paid", "", "chatgpt-4o-latest"},
		{"explicit model preserved", "subscription", "deepseek-r1", "deepseek-r1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("NANOGPT_BASE_URL", srv.URL)
			p := NewNanoGPTProvider("test-key", tt.tier)

			resp, err := p.ChatCompletion(context.Background(), ChatRequest{
				Model:    tt.reqModel,
				Messages: []Message{{Role: "user", Content: "hi"}},
			}, "sess-1")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if resp.Model != tt.wantModel {
				t.Errorf("Model = %q, want %q", resp.Model, tt.wantModel)
			}
		})
	}
}

func TestNanoGPTProvider_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]string{
				{"id": "qwen/qwen3.5-plus"},
				{"id": "deepseek-r1"},
				{"id": "openai/gpt-5"},
				{"id": "unknown-model"},
				{"id": ""},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	t.Setenv("NANOGPT_BASE_URL", srv.URL)
	p := NewNanoGPTProvider("test-key", "subscription")

	models := p.ListModels()
	// Should include qwen, deepseek, gpt-5 but NOT unknown-model or empty
	if len(models) != 3 {
		t.Fatalf("ListModels() returned %d models, want 3", len(models))
	}

	// Verify cache works
	models2 := p.ListModels()
	if len(models2) != 3 {
		t.Fatalf("cached ListModels() returned %d models, want 3", len(models2))
	}
}

func TestNanoGPTProvider_IsHealthy(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"healthy", http.StatusOK, true},
		{"unhealthy 500", http.StatusInternalServerError, false},
		{"unhealthy 401", http.StatusUnauthorized, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			t.Setenv("NANOGPT_BASE_URL", srv.URL)
			p := NewNanoGPTProvider("test-key", "subscription")

			got := p.IsHealthy(context.Background())
			if got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

