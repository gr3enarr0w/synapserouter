package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OpenAICompatProviderConfig holds configuration for an OpenAI-compatible provider.
type OpenAICompatProviderConfig struct {
	Name         string `yaml:"name"`
	BaseURL      string `yaml:"base_url"`
	APIKeyEnv    string `yaml:"api_key_env"`
	DefaultModel string `yaml:"default_model"`
}

// OpenAICompatProvider implements Provider for any OpenAI-compatible endpoint.
// Covers: DeepSeek, Groq, xAI/Grok, Together.ai, Fireworks.ai, OpenRouter,
// Mistral, Cohere, LM Studio, LocalAI, vLLM, llama.cpp, etc.
type OpenAICompatProvider struct {
	BaseProvider
	client       *http.Client
	model        string
	apiKeyEnv    string
}

// NewOpenAICompatProvider creates a new OpenAI-compatible provider.
func NewOpenAICompatProvider(cfg OpenAICompatProviderConfig) *OpenAICompatProvider {
	return &OpenAICompatProvider{
		BaseProvider: BaseProvider{
			name:       cfg.Name,
			baseURL:    strings.TrimSuffix(cfg.BaseURL, "/"),
			maxContext: 128000,
			timeout:    120 * time.Second,
		},
		client:    NewLLMClient(120 * time.Second),
		model:     cfg.DefaultModel,
		apiKeyEnv: cfg.APIKeyEnv,
	}
}

// DefaultModel returns the configured default model.
func (p *OpenAICompatProvider) DefaultModel() string {
	return p.model
}

func (p *OpenAICompatProvider) SupportsModel(model string) bool {
	if model == "" || strings.EqualFold(model, "auto") {
		return true
	}
	return strings.EqualFold(model, p.model)
}

func (p *OpenAICompatProvider) IsHealthy(ctx context.Context) bool {
	apiKey := os.Getenv(p.apiKeyEnv)
	if apiKey == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/models", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode < 500
}

func (p *OpenAICompatProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
	if req.Model == "" || strings.EqualFold(req.Model, "auto") {
		req.Model = p.model
	}

	apiKey := os.Getenv(p.apiKeyEnv)
	if apiKey == "" {
		return ChatResponse{}, fmt.Errorf("API key not found in env var %s", p.apiKeyEnv)
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request to %s: %w", p.name, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("%s error %d: %s", p.name, resp.StatusCode, string(respBody))
	}

	var openAIResp struct {
		ID      string `json:"id"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int     `json:"index"`
			Message      Message `json:"message"`
			FinishReason string  `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(respBody, &openAIResp); err != nil {
		return ChatResponse{}, fmt.Errorf("parse response from %s: %w", p.name, err)
	}

	if len(openAIResp.Choices) == 0 {
		return ChatResponse{}, fmt.Errorf("%s returned no choices", p.name)
	}

	var choices []Choice
	for _, c := range openAIResp.Choices {
		choices = append(choices, Choice{
			Index:        c.Index,
			Message:      c.Message,
			FinishReason: c.FinishReason,
		})
	}

	return ChatResponse{
		ID:      openAIResp.ID,
		Created: openAIResp.Created,
		Model:   openAIResp.Model,
		Choices: choices,
		Usage:   Usage(openAIResp.Usage),
	}, nil
}
