package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// ollamaTimeout returns the HTTP client timeout for Ollama requests.
// Configurable via OLLAMA_TIMEOUT_SECONDS. Default 300s (5 min) to handle
// large models on cold start (both cloud and local ollama serve).
func ollamaTimeout() time.Duration {
	if v := os.Getenv("OLLAMA_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 600 * time.Second // 10 min — cloud models processing large specs need time
}

// OllamaCloudProvider for Ollama Cloud API
type OllamaCloudProvider struct {
	BaseProvider
	client *http.Client
	model  string
}

func NewOllamaCloudProvider(baseURL, apiKey, model, name string) *OllamaCloudProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "qwen3.5:cloud"
	}
	if name == "" {
		name = "ollama-cloud"
	}

	return &OllamaCloudProvider{
		BaseProvider: BaseProvider{
			name:       name,
			baseURL:    strings.TrimSuffix(baseURL, "/"),
			apiKey:     apiKey,
			maxContext: 256000,
			timeout:    ollamaTimeout(),
		},
		client: NewLLMClient(ollamaTimeout()),
		model:  model,
	}
}

func (p *OllamaCloudProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
	if req.Model == "" || strings.EqualFold(req.Model, "auto") {
		req.Model = p.model
	}

	// Use OpenAI-compatible endpoint — handles tools and reports usage
	body, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.baseURL+"/v1/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("ollama error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse ollama response: %w", err)
	}

	return chatResp, nil
}

func (p *OllamaCloudProvider) SupportsModel(model string) bool {
	if model == "" || strings.EqualFold(model, "auto") {
		return true // Ollama Cloud handles auto routing
	}
	model = strings.ToLower(model)
	return strings.Contains(model, "llama") || strings.EqualFold(model, p.model)
}

func (p *OllamaCloudProvider) IsHealthy(ctx context.Context) bool {
	healthTimeout := 10 * time.Second // large models need more time for cold starts
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/api/tags", nil)
	if err != nil {
		return false
	}

	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
