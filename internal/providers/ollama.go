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

// OllamaCloudProvider for Ollama Cloud API
type OllamaCloudProvider struct {
	BaseProvider
	client *http.Client
	model  string
}

func NewOllamaCloudProvider(baseURL, apiKey, model string) *OllamaCloudProvider {
	if baseURL == "" {
		baseURL = "https://ollama.com/api"
	}
	if model == "" {
		model = "llama3.1-70b-cloud"
	}

	return &OllamaCloudProvider{
		BaseProvider: BaseProvider{
			name:       "ollama-cloud",
			baseURL:    baseURL,
			apiKey:     apiKey,
			maxContext: 128000,
			timeout:    180 * time.Second,
		},
		client: &http.Client{Timeout: 180 * time.Second},
		model:  model,
	}
}

func (p *OllamaCloudProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
	// Convert to Ollama format
	ollamaReq := map[string]interface{}{
		"model":    p.model,
		"messages": req.Messages,
		"stream":   false,
	}

	if req.Temperature > 0 {
		ollamaReq["temperature"] = req.Temperature
	}

	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return ChatResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		p.baseURL+"/chat", bytes.NewBuffer(body))
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

	var ollamaResp map[string]interface{}
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return ChatResponse{}, err
	}

	// Convert Ollama response to OpenAI format
	return p.convertResponse(ollamaResp), nil
}

func (p *OllamaCloudProvider) convertResponse(resp map[string]interface{}) ChatResponse {
	message, _ := resp["message"].(map[string]interface{})
	content, _ := message["content"].(string)
	role, _ := message["role"].(string)

	return ChatResponse{
		ID:      fmt.Sprintf("ollama-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   p.model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    role,
					Content: content,
				},
				FinishReason: "stop",
			},
		},
	}
}

func (p *OllamaCloudProvider) SupportsModel(model string) bool {
	model = strings.ToLower(model)
	return strings.Contains(model, "llama") || strings.EqualFold(model, p.model)
}

func (p *OllamaCloudProvider) IsHealthy(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", p.baseURL+"/tags", nil)
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
