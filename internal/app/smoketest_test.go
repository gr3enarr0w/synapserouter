package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
)

type healthyMockProvider struct{ name string }

func (p *healthyMockProvider) Name() string { return p.name }
func (p *healthyMockProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	return providers.ChatResponse{
		Model:   "test-model",
		Choices: []providers.Choice{{Index: 0, Message: providers.Message{Role: "assistant", Content: "ok"}, FinishReason: "stop"}},
		Usage:   providers.Usage{TotalTokens: 10},
	}, nil
}
func (p *healthyMockProvider) IsHealthy(ctx context.Context) bool { return true }
func (p *healthyMockProvider) MaxContextTokens() int              { return 100000 }
func (p *healthyMockProvider) SupportsModel(model string) bool    { return true }

type failingMockProvider struct{ name string }

func (p *failingMockProvider) Name() string { return p.name }
func (p *failingMockProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, sessionID string) (providers.ChatResponse, error) {
	return providers.ChatResponse{}, errors.New("connection refused")
}
func (p *failingMockProvider) IsHealthy(ctx context.Context) bool { return false }
func (p *failingMockProvider) MaxContextTokens() int              { return 100000 }
func (p *failingMockProvider) SupportsModel(model string) bool    { return true }

func TestRunSmokeTests_HealthyProvider(t *testing.T) {
	results := RunSmokeTests(context.Background(), []providers.Provider{
		&healthyMockProvider{name: "test-healthy"},
	}, SmokeTestOpts{Timeout: 5 * time.Second})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "PASS" {
		t.Errorf("expected PASS, got %s (error: %s)", r.Status, r.Error)
	}
	if r.Provider != "test-healthy" {
		t.Errorf("expected provider test-healthy, got %s", r.Provider)
	}
	if r.Tokens != 10 {
		t.Errorf("expected 10 tokens, got %d", r.Tokens)
	}
	if r.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", r.Model)
	}
}

func TestRunSmokeTests_FailingProvider(t *testing.T) {
	results := RunSmokeTests(context.Background(), []providers.Provider{
		&failingMockProvider{name: "test-failing"},
	}, SmokeTestOpts{Timeout: 5 * time.Second})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.Status != "FAIL" {
		t.Errorf("expected FAIL, got %s", r.Status)
	}
	if r.Error == "" {
		t.Error("expected error message")
	}
}

func TestRunSmokeTests_FilterByProvider(t *testing.T) {
	results := RunSmokeTests(context.Background(), []providers.Provider{
		&healthyMockProvider{name: "alpha"},
		&healthyMockProvider{name: "beta"},
	}, SmokeTestOpts{Provider: "alpha", Timeout: 5 * time.Second})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Provider != "alpha" {
		t.Errorf("expected provider alpha, got %s", results[0].Provider)
	}
}

func TestRunSmokeTests_MultipleProviders(t *testing.T) {
	results := RunSmokeTests(context.Background(), []providers.Provider{
		&healthyMockProvider{name: "good"},
		&failingMockProvider{name: "bad"},
	}, SmokeTestOpts{Timeout: 5 * time.Second})

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	passed, failed := 0, 0
	for _, r := range results {
		if r.Status == "PASS" {
			passed++
		} else {
			failed++
		}
	}
	if passed != 1 || failed != 1 {
		t.Errorf("expected 1 pass + 1 fail, got %d pass + %d fail", passed, failed)
	}
}
