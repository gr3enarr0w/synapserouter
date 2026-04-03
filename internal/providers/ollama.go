package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		return ChatResponse{}, NewProviderError(p.name, resp, string(respBody))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return ChatResponse{}, fmt.Errorf("failed to parse ollama response: %w", err)
	}

	// Debug: log when model returns text but no tool calls (helps diagnose function calling issues)
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.ToolCalls) == 0 && chatResp.Choices[0].Message.Content != "" {
		if strings.Contains(chatResp.Choices[0].Message.Content, "tool_call") ||
			strings.Contains(chatResp.Choices[0].Message.Content, "function") {
			log.Printf("[Ollama] WARNING: model %s returned text containing 'tool_call'/'function' but no structured tool_calls — model may not support function calling via API", p.model)
		}
	}

	// Normalize tool calls: some Ollama models return tool calls under
	// alternative field names or nested structures that json.Unmarshal
	// into ChatResponse silently drops. Check the raw JSON for tool calls
	// that didn't make it into the parsed response.
	if len(chatResp.Choices) > 0 && len(chatResp.Choices[0].Message.ToolCalls) == 0 {
		normalizeOllamaToolCalls(respBody, &chatResp)
	}

	return chatResp, nil
}

// ChatCompletionStream implements StreamingProvider for Ollama Cloud.
// Sends SSE streaming request and calls onToken for each content delta.
// Returns the accumulated ChatResponse after streaming completes.
func (p *OllamaCloudProvider) ChatCompletionStream(ctx context.Context, req ChatRequest, sessionID string, onToken TokenCallback) (ChatResponse, error) {
	if req.Model == "" || strings.EqualFold(req.Model, "auto") {
		req.Model = p.model
	}
	req.Stream = true

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
		return ChatResponse{}, fmt.Errorf("ollama stream request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return ChatResponse{}, NewProviderError(p.name, resp, string(respBody))
	}

	// Parse SSE stream: each line is "data: {json}\n" or "data: [DONE]\n"
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var content strings.Builder
	var toolCalls []map[string]interface{}
	var lastResp ChatResponse
	var finishReason string

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var chunk struct {
			ID      string `json:"id"`
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content   string                   `json:"content"`
					ToolCalls []map[string]interface{} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage Usage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta
			if delta.Content != "" {
				content.WriteString(delta.Content)
				if onToken != nil {
					onToken(delta.Content)
				}
			}
			if len(delta.ToolCalls) > 0 {
				toolCalls = append(toolCalls, delta.ToolCalls...)
			}
			if chunk.Choices[0].FinishReason != "" {
				finishReason = chunk.Choices[0].FinishReason
			}
		}
		if chunk.Usage.TotalTokens > 0 {
			lastResp.Usage = chunk.Usage
		}
		lastResp.ID = chunk.ID
		lastResp.Model = chunk.Model
	}

	// Build final response
	lastResp.Choices = []Choice{{
		Message: Message{
			Role:      "assistant",
			Content:   content.String(),
			ToolCalls: toolCalls,
		},
		FinishReason: finishReason,
	}}

	return lastResp, nil
}

// normalizeOllamaToolCalls checks raw JSON for tool calls that may have been
// dropped during standard unmarshaling due to format differences.
func normalizeOllamaToolCalls(raw []byte, resp *ChatResponse) {
	// Parse raw response to check for tool_calls under message
	var rawResp struct {
		Choices []struct {
			Message struct {
				ToolCalls []json.RawMessage `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &rawResp); err != nil {
		return
	}
	if len(rawResp.Choices) == 0 || len(rawResp.Choices[0].Message.ToolCalls) == 0 {
		return
	}

	// Tool calls exist in raw JSON but were dropped — try to parse each one
	for _, rawTC := range rawResp.Choices[0].Message.ToolCalls {
		var tc map[string]interface{}
		if err := json.Unmarshal(rawTC, &tc); err != nil {
			continue
		}
		// Normalize: ensure it has the expected structure
		if _, hasFunc := tc["function"]; !hasFunc {
			// Some models put name/arguments at top level instead of nested in "function"
			if name, ok := tc["name"].(string); ok {
				normalized := map[string]interface{}{
					"id":   tc["id"],
					"type": "function",
					"function": map[string]interface{}{
						"name":      name,
						"arguments": tc["arguments"],
					},
				}
				resp.Choices[0].Message.ToolCalls = append(resp.Choices[0].Message.ToolCalls, normalized)
				continue
			}
		}
		resp.Choices[0].Message.ToolCalls = append(resp.Choices[0].Message.ToolCalls, tc)
	}
}

// DefaultModel returns the model this provider was configured with.
func (p *OllamaCloudProvider) DefaultModel() string {
	return p.model
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
