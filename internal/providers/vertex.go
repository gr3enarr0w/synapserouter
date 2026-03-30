package providers

import (
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
	"sync"
	"time"
)

// vertexTimeout returns the HTTP client timeout for Vertex AI requests.
// Configurable via VERTEX_TIMEOUT_SECONDS. Default 120s.
func vertexTimeout() time.Duration {
	if v := os.Getenv("VERTEX_TIMEOUT_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Second
		}
	}
	return 120 * time.Second
}

// VertexProvider routes requests to Vertex AI (Claude via rawPredict, Gemini via generateContent).
type VertexProvider struct {
	BaseProvider
	client      *http.Client
	project     string
	location    string
	publisher   string // "anthropic" or "google"
	saKeyFile   string // path to service account JSON (empty = use gcloud)
	modelPrefix string // "claude" or "gemini"
	fixedModel  string // if set, always use this model instead of auto-detecting

	cacheMu          sync.Mutex
	tokenCache       string
	tokenExp         time.Time
	discoveredModels []map[string]interface{}
	modelsExp        time.Time
}

type VertexConfig struct {
	Name      string // provider name e.g. "vertex-claude", "vertex-gemini"
	Project   string
	Location  string // e.g. "us-east5", "global"
	Publisher string // "anthropic" or "google"
	SAKeyFile string // service account JSON path (empty = gcloud ADC)
	Prefix    string // model prefix for SupportsModel: "claude" or "gemini"
	Model     string // specific model to use (e.g. "claude-sonnet-4-6"); empty = auto
}

func NewVertexProvider(cfg VertexConfig) *VertexProvider {
	maxCtx := 200000
	if cfg.Prefix == "gemini" {
		maxCtx = 1048576
	}
	p := &VertexProvider{
		BaseProvider: BaseProvider{
			name:       cfg.Name,
			baseURL:    vertexBaseURL(cfg.Location, cfg.Project),
			apiKey:     "", // unused — auth via token
			maxContext: maxCtx,
			timeout:    vertexTimeout(),
		},
		client:      NewLLMClient(vertexTimeout()),
		project:     cfg.Project,
		location:    cfg.Location,
		publisher:   cfg.Publisher,
		saKeyFile:   cfg.SAKeyFile,
		modelPrefix: cfg.Prefix,
	}
	if cfg.Model != "" {
		p.fixedModel = cfg.Model
	}
	return p
}

func vertexBaseURL(location, project string) string {
	if location == "global" {
		return fmt.Sprintf("https://aiplatform.googleapis.com/v1/projects/%s/locations/global", project)
	}
	return fmt.Sprintf("https://%s-aiplatform.googleapis.com/v1/projects/%s/locations/%s", location, project, location)
}

func (p *VertexProvider) SupportsModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || model == "auto" {
		return true
	}
	return strings.HasPrefix(model, p.modelPrefix)
}

func (p *VertexProvider) IsHealthy(ctx context.Context) bool {
	token, err := p.accessToken()
	if err != nil {
		log.Printf("[Vertex/%s] unhealthy: cannot get access token: %v", p.name, err)
		return false
	}
	return token != ""
}

func (p *VertexProvider) ChatCompletion(ctx context.Context, req ChatRequest, sessionID string) (ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" || strings.EqualFold(model, "auto") {
		model = p.defaultModel()
	}

	token, err := p.accessToken()
	if err != nil {
		return ChatResponse{}, fmt.Errorf("vertex auth: %w", err)
	}

	if p.publisher == "anthropic" {
		return p.claudeRawPredict(ctx, req, model, token, sessionID)
	}
	return p.geminiGenerateContent(ctx, req, model, token)
}

func (p *VertexProvider) defaultModel() string {
	if p.fixedModel != "" {
		return p.fixedModel
	}
	if p.publisher == "anthropic" {
		return "claude-sonnet-4-6"
	}
	return "gemini-3.1-pro-preview"
}

// ── Claude via rawPredict ──

func (p *VertexProvider) claudeRawPredict(ctx context.Context, req ChatRequest, model, token, sessionID string) (ChatResponse, error) {
	url := fmt.Sprintf("%s/publishers/anthropic/models/%s:rawPredict", p.baseURL, model)

	// Build Anthropic messages format
	payload := map[string]interface{}{
		"anthropic_version": "vertex-2023-10-16",
		"max_tokens":        req.MaxTokens,
		"messages":          []interface{}{},
	}
	if req.MaxTokens == 0 {
		payload["max_tokens"] = 4096
	}
	if req.Temperature > 0 {
		payload["temperature"] = req.Temperature
	}

	var systemParts []string
	messages := make([]interface{}, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "system" {
			systemParts = append(systemParts, msg.Content)
			continue
		}
		if role == "tool" {
			messages = append(messages, map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": msg.ToolCallID,
						"content":     msg.Content,
					},
				},
			})
			continue
		}
		if role != "user" && role != "assistant" {
			role = "user"
		}
		content := make([]interface{}, 0, 1+len(msg.ToolCalls))
		if strings.TrimSpace(msg.Content) != "" {
			content = append(content, map[string]interface{}{
				"type": "text",
				"text": msg.Content,
			})
		}
		for _, tc := range msg.ToolCalls {
			fn, _ := tc["function"].(map[string]interface{})
			content = append(content, map[string]interface{}{
				"type":  "tool_use",
				"id":    firstStr(strVal(tc["id"]), strVal(tc["call_id"])),
				"name":  strVal(fn["name"]),
				"input": parseArgs(fn["arguments"]),
			})
		}
		messages = append(messages, map[string]interface{}{
			"role":    role,
			"content": content,
		})
	}
	if len(systemParts) > 0 {
		payload["system"] = strings.Join(systemParts, "\n")
	}
	payload["messages"] = messages

	if len(req.Tools) > 0 {
		tools := make([]interface{}, 0, len(req.Tools))
		for _, t := range req.Tools {
			if strVal(t["type"]) == "function" {
				fn, _ := t["function"].(map[string]interface{})
				tools = append(tools, map[string]interface{}{
					"name":         strVal(fn["name"]),
					"description":  strVal(fn["description"]),
					"input_schema": fn["parameters"],
				})
			}
		}
		if len(tools) > 0 {
			payload["tools"] = tools
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal vertex claude request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("vertex claude request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("vertex claude read failure: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("vertex claude returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var anthResp struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type  string                 `json:"type"`
			Text  string                 `json:"text"`
			ID    string                 `json:"id,omitempty"`
			Name  string                 `json:"name,omitempty"`
			Input map[string]interface{} `json:"input,omitempty"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &anthResp); err != nil {
		return ChatResponse{}, fmt.Errorf("vertex claude decode failure: %w", err)
	}

	text := ""
	toolCalls := make([]map[string]interface{}, 0)
	for _, block := range anthResp.Content {
		switch block.Type {
		case "tool_use":
			args, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   block.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      block.Name,
					"arguments": string(args),
				},
			})
		default:
			text += block.Text
		}
	}
	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}
	return ChatResponse{
		ID:     anthResp.ID,
		Object: "chat.completion",
		Model:  firstStr(anthResp.Model, model),
		Choices: []Choice{{
			Index: 0,
			Message: Message{
				Role:      "assistant",
				Content:   text,
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
		Usage: Usage{
			PromptTokens:     anthResp.Usage.InputTokens,
			CompletionTokens: anthResp.Usage.OutputTokens,
			TotalTokens:      anthResp.Usage.InputTokens + anthResp.Usage.OutputTokens,
		},
	}, nil
}

// ── Gemini via generateContent ──

func (p *VertexProvider) geminiGenerateContent(ctx context.Context, req ChatRequest, model, token string) (ChatResponse, error) {
	url := fmt.Sprintf("%s/publishers/google/models/%s:generateContent", p.baseURL, model)

	contents := make([]map[string]interface{}, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "system" || role == "developer" {
			role = "user"
		}
		if role == "assistant" {
			role = "model"
		}
		if role == "tool" {
			role = "user"
		}
		parts := make([]map[string]interface{}, 0, 1)
		if strings.TrimSpace(msg.Content) != "" {
			parts = append(parts, map[string]interface{}{"text": msg.Content})
		}
		if len(parts) > 0 {
			contents = append(contents, map[string]interface{}{
				"role":  role,
				"parts": parts,
			})
		}
	}

	payload := map[string]interface{}{
		"contents": contents,
	}
	genCfg := map[string]interface{}{}
	maxTokens := req.MaxTokens
	if maxTokens > 0 {
		if maxTokens < 1024 {
			maxTokens = 1024
		}
		genCfg["maxOutputTokens"] = maxTokens
	}
	if req.Temperature > 0 {
		genCfg["temperature"] = req.Temperature
	}
	if len(genCfg) > 0 {
		payload["generationConfig"] = genCfg
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal vertex gemini request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("vertex gemini request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("vertex gemini read failure: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return ChatResponse{}, fmt.Errorf("vertex gemini returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var gemResp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &gemResp); err != nil {
		return ChatResponse{}, fmt.Errorf("vertex gemini decode failure: %w", err)
	}

	text := ""
	if len(gemResp.Candidates) > 0 {
		for _, part := range gemResp.Candidates[0].Content.Parts {
			text += part.Text
		}
	}

	return ChatResponse{
		ID:     fmt.Sprintf("vertex-gemini-%d", time.Now().UnixNano()),
		Object: "chat.completion",
		Model:  model,
		Choices: []Choice{{
			Index:        0,
			Message:      Message{Role: "assistant", Content: text},
			FinishReason: "stop",
		}},
		Usage: Usage{
			PromptTokens:     gemResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: gemResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gemResp.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

// ── Model Discovery ──

// ListModels returns the discovered models for /v1/models endpoint.
func (p *VertexProvider) ListModels() []map[string]interface{} {
	p.cacheMu.Lock()
	if p.discoveredModels != nil && time.Now().Before(p.modelsExp) {
		models := p.discoveredModels
		p.cacheMu.Unlock()
		return models
	}
	p.cacheMu.Unlock()

	models := p.discoverModels()

	p.cacheMu.Lock()
	p.discoveredModels = models
	p.modelsExp = time.Now().Add(10 * time.Minute)
	p.cacheMu.Unlock()

	return models
}

func (p *VertexProvider) discoverModels() []map[string]interface{} {
	token, err := p.accessToken()
	if err != nil {
		log.Printf("[Vertex/%s] model discovery failed (auth): %v", p.name, err)
		return nil
	}

	if p.publisher == "google" {
		return p.discoverGeminiModels(token)
	}
	return p.discoverClaudeModels(token)
}

func (p *VertexProvider) discoverGeminiModels(token string) []map[string]interface{} {
	// The v1beta1 publisher listing works without project scoping
	listURL := "https://aiplatform.googleapis.com/v1beta1/publishers/google/models"
	req, err := http.NewRequest(http.MethodGet, listURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.client.Do(req)
	if err != nil {
		log.Printf("[Vertex/%s] model list request failed: %v", p.name, err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		log.Printf("[Vertex/%s] model list returned %d", p.name, resp.StatusCode)
		return nil
	}

	var listing struct {
		PublisherModels []struct {
			Name string `json:"name"`
		} `json:"publisherModels"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		return nil
	}

	var models []map[string]interface{}
	for _, m := range listing.PublisherModels {
		id := m.Name
		if idx := strings.LastIndex(id, "/"); idx >= 0 {
			id = id[idx+1:]
		}
		if !strings.Contains(strings.ToLower(id), "gemini") {
			continue
		}
		models = append(models, map[string]interface{}{
			"id":       id,
			"object":   "model",
			"owned_by": p.name,
			"context":  p.maxContext,
		})
	}
	log.Printf("[Vertex/%s] discovered %d models", p.name, len(models))
	return models
}

func (p *VertexProvider) discoverClaudeModels(token string) []map[string]interface{} {
	// Anthropic doesn't expose a listing endpoint on Vertex.
	// Probe known model families in parallel to discover what's available.
	candidates := []string{
		"claude-sonnet-4-6",
		"claude-opus-4-6",
		"claude-sonnet-4-5",
		"claude-opus-4-5",
		"claude-haiku-4-5",
		"claude-3-5-haiku",
		"claude-3-5-sonnet",
	}

	type probeResult struct {
		model string
		ok    bool
	}
	results := make(chan probeResult, len(candidates))

	for _, model := range candidates {
		go func(model string) {
			probeURL := fmt.Sprintf("%s/publishers/anthropic/models/%s:rawPredict", p.baseURL, model)
			body := []byte(`{"anthropic_version":"vertex-2023-10-16","max_tokens":1,"messages":[{"role":"user","content":"."}]}`)
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, probeURL, bytes.NewReader(body))
			if err != nil {
				results <- probeResult{model: model}
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := p.client.Do(req)
			if err != nil {
				results <- probeResult{model: model}
				return
			}
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var result struct {
					Model string `json:"model"`
				}
				json.Unmarshal(respBody, &result)
				results <- probeResult{model: firstStr(result.Model, model), ok: true}
			} else {
				results <- probeResult{model: model}
			}
		}(model)
	}

	var models []map[string]interface{}
	for range candidates {
		r := <-results
		if r.ok {
			models = append(models, map[string]interface{}{
				"id":       r.model,
				"object":   "model",
				"owned_by": p.name,
				"context":  p.maxContext,
			})
		}
	}
	log.Printf("[Vertex/%s] discovered %d models (probed %d candidates)", p.name, len(models), len(candidates))
	return models
}

// ── Auth ──

func (p *VertexProvider) accessToken() (string, error) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	if p.tokenCache != "" && time.Now().Before(p.tokenExp) {
		return p.tokenCache, nil
	}

	var token string
	var err error
	if p.saKeyFile != "" {
		token, err = nativeServiceAccountToken(p.saKeyFile)
	} else {
		token, err = nativeADCToken()
	}
	if err != nil {
		return "", err
	}

	p.tokenCache = token
	p.tokenExp = time.Now().Add(50 * time.Minute) // tokens last ~60min
	return token, nil
}

// ── Helpers ──

func strVal(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func firstStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func parseArgs(raw interface{}) map[string]interface{} {
	switch v := raw.(type) {
	case map[string]interface{}:
		return v
	case string:
		var m map[string]interface{}
		if json.Unmarshal([]byte(v), &m) == nil {
			return m
		}
	}
	return map[string]interface{}{}
}
