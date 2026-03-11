package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// NanoGPTProvider for NanoGPT API
type NanoGPTProvider struct {
	BaseProvider
	client *http.Client
}

func NewNanoGPTProvider(baseURL, apiKey string) *NanoGPTProvider {
	if baseURL == "" {
		baseURL = "https://nano-gpt.com/api/v1"
	}
	// Ensure base URL doesn't have trailing slash
	baseURL = strings.TrimSuffix(baseURL, "/")

	return &NanoGPTProvider{
		BaseProvider: BaseProvider{
			name:       "nanogpt",
			baseURL:    baseURL,
			apiKey:     apiKey,
			maxContext: 2000000, // Model-dependent context (check per model)
			timeout:    120 * time.Second,
		},
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (p *NanoGPTProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.baseURL+"/chat/completions", bytes.NewBuffer(body))
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("nanogpt request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("nanogpt error %d: %s", resp.StatusCode, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse nanogpt response: %w", err)
	}

	return chatResp, nil
}

func (p *NanoGPTProvider) SupportsModel(model string) bool {
	return true // NanoGPT is a catch-all
}

func (p *NanoGPTProvider) IsHealthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/models", nil)
	if err != nil {
		return false
	}

	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
