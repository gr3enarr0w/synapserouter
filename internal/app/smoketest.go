package app

import (
	"context"
	"fmt"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

// SmokeTestResult holds the outcome of a single provider smoke test.
type SmokeTestResult struct {
	Provider string `json:"provider"`
	Status   string `json:"status"` // PASS, FAIL
	Healthy  bool   `json:"healthy"`
	Model    string `json:"model,omitempty"`
	Tokens   int    `json:"tokens,omitempty"`
	Latency  int64  `json:"latency_ms"`
	Error    string `json:"error,omitempty"`
}

// SmokeTestOpts configures how smoke tests run.
type SmokeTestOpts struct {
	Provider string        // test only this provider (empty = all)
	Timeout  time.Duration // per-provider timeout
	Verbose  bool
}

// RunSmokeTests tests each provider by calling IsHealthy and sending a minimal completion.
func RunSmokeTests(ctx context.Context, providerList []providers.Provider, opts SmokeTestOpts) []SmokeTestResult {
	var results []SmokeTestResult

	for _, p := range providerList {
		if opts.Provider != "" && p.Name() != opts.Provider {
			continue
		}

		result := testOneProvider(ctx, p, opts.Timeout)
		results = append(results, result)
	}

	return results
}

func testOneProvider(ctx context.Context, p providers.Provider, timeout time.Duration) SmokeTestResult {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	provCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := SmokeTestResult{
		Provider: p.Name(),
	}

	start := time.Now()

	// Step 1: health check
	result.Healthy = p.IsHealthy(provCtx)
	if !result.Healthy {
		result.Status = "FAIL"
		result.Latency = time.Since(start).Milliseconds()
		result.Error = "health check failed"
		return result
	}

	// Step 2: minimal completion
	req := providers.ChatRequest{
		Model: "auto",
		Messages: []providers.Message{
			{Role: "user", Content: "Say ok"},
		},
		MaxTokens:  5,
		SkipMemory: true,
	}

	resp, err := p.ChatCompletion(provCtx, req, fmt.Sprintf("smoketest-%d", time.Now().UnixNano()))
	result.Latency = time.Since(start).Milliseconds()

	if err != nil {
		result.Status = "FAIL"
		result.Error = err.Error()
		return result
	}

	result.Status = "PASS"
	result.Model = resp.Model
	result.Tokens = resp.Usage.TotalTokens
	return result
}
