package subscriptions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

type provider interface {
	Name() string
	ChatCompletion(ctx context.Context, req providers.ChatRequest, model, sessionID string) (providers.ChatResponse, error)
	IsHealthy(ctx context.Context) bool
	ListModels() []ModelInfo
	SupportsModel(model string) bool
}

type anthropicProvider struct {
	baseURL      string
	apiKey       string
	sessionToken string
	credentials  []ProviderCredential
	timeout      time.Duration
	models       []string
	client       *http.Client
	mu           sync.Mutex
	cursor       int
}

type openAIProvider struct {
	baseURL      string
	apiKey       string
	sessionToken string
	credentials  []ProviderCredential
	timeout      time.Duration
	models       []string
	client       *http.Client
	mu           sync.Mutex
	cursor       int
}

type geminiProvider struct {
	baseURL      string
	apiKey       string
	sessionToken string
	credentials  []ProviderCredential
	timeout      time.Duration
	models       []string
	client       *http.Client
	mu           sync.Mutex
	cursor       int
}

func buildProviders(cfg Config) ([]provider, error) {
	providers := make([]provider, 0, len(cfg.Providers))
	for _, spec := range cfg.Providers {
		switch spec.Name {
		case "anthropic":
			providers = append(providers, newAnthropicProvider(spec, cfg.ServerTimeout))
		case "openai":
			providers = append(providers, newOpenAIProvider(spec, cfg.ServerTimeout))
		case "gemini":
			providers = append(providers, newGeminiProvider(spec, cfg.ServerTimeout))
		case "qwen":
			providers = append(providers, newOpenAIProvider(spec, cfg.ServerTimeout))
		default:
			return nil, fmt.Errorf("unsupported provider: %s", spec.Name)
		}
	}
	return providers, nil
}

func newAnthropicProvider(spec ProviderSpec, timeout time.Duration) provider {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &anthropicProvider{
		baseURL:      strings.TrimRight(spec.BaseURL, "/"),
		apiKey:       spec.APIKey,
		sessionToken: spec.SessionToken,
		credentials:  ensureCredentials(spec),
		timeout:      timeout,
		models:       spec.Models,
		client:       providers.NewLLMClient(timeout),
	}
}

func newOpenAIProvider(spec ProviderSpec, timeout time.Duration) provider {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &openAIProvider{
		baseURL:      strings.TrimRight(spec.BaseURL, "/"),
		apiKey:       spec.APIKey,
		sessionToken: spec.SessionToken,
		credentials:  ensureCredentials(spec),
		timeout:      timeout,
		models:       spec.Models,
		client:       providers.NewLLMClient(timeout),
	}
}

func newGeminiProvider(spec ProviderSpec, timeout time.Duration) provider {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &geminiProvider{
		baseURL:      strings.TrimRight(spec.BaseURL, "/"),
		apiKey:       spec.APIKey,
		sessionToken: spec.SessionToken,
		credentials:  ensureCredentials(spec),
		timeout:      timeout,
		models:       spec.Models,
		client:       providers.NewLLMClient(timeout),
	}
}

func (p *anthropicProvider) Name() string {
	return "anthropic"
}

func (p *anthropicProvider) SupportsModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, candidate := range p.models {
		if model == candidate || strings.HasPrefix(model, "claude") {
			return true
		}
	}
	return false
}

func (p *anthropicProvider) ListModels() []ModelInfo {
	return p.liveModels(context.Background())
}

func (p *anthropicProvider) IsHealthy(ctx context.Context) bool {
	hasUsableCredential := false
	for _, credential := range p.credentialSequence(ctx) {
		if credentialLooksUsable(credential) {
			hasUsableCredential = true
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/v1/models", nil)
		if err != nil {
			return hasUsableCredential
		}
		p.applyCredential(req, credential)
		resp, err := p.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return hasUsableCredential
}

// TransformToAnthropicTools converts OpenAI-format tools to Anthropic-format tools
// OpenAI format: {"type": "function", "function": {"name": "...", "parameters": {...}}}
// Anthropic format: {"name": "...", "description": "...", "input_schema": {...}}
func TransformToAnthropicTools(openaiTools []map[string]interface{}) []map[string]interface{} {
	if len(openaiTools) == 0 {
		return nil
	}

	anthropicTools := make([]map[string]interface{}, 0, len(openaiTools))
	for _, tool := range openaiTools {
		// Check if this is OpenAI format (has "type" and "function" fields)
		if toolType, ok := tool["type"].(string); ok && toolType == "function" {
			if function, ok := tool["function"].(map[string]interface{}); ok {
				anthropicTool := make(map[string]interface{})

				// Copy name and description
				if name, ok := function["name"]; ok {
					anthropicTool["name"] = name
				}
				if description, ok := function["description"]; ok {
					anthropicTool["description"] = description
				}

				// Rename "parameters" to "input_schema"
				if parameters, ok := function["parameters"]; ok {
					anthropicTool["input_schema"] = parameters
				}

				anthropicTools = append(anthropicTools, anthropicTool)
			}
		} else {
			// Already in Anthropic format or unknown format, pass through
			anthropicTools = append(anthropicTools, tool)
		}
	}

	return anthropicTools
}

type anthropicChatRequest struct {
	Model       string                   `json:"model"`
	MaxTokens   int                      `json:"max_tokens"`
	Temperature float64                  `json:"temperature,omitempty"`
	System      string                   `json:"system,omitempty"`
	Messages    []anthropicChatMessage   `json:"messages"`
	Tools       []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice  interface{}              `json:"tool_choice,omitempty"`
	Thinking    map[string]interface{}   `json:"thinking,omitempty"`
	Stream      bool                     `json:"stream,omitempty"`
}

type anthropicChatMessage struct {
	Role    string        `json:"role"`
	Content []interface{} `json:"content"`
}

type anthropicChatResponse struct {
	ID      string                     `json:"id"`
	Role    string                     `json:"role"`
	Model   string                     `json:"model"`
	Content []anthropicResponseMessage `json:"content"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicResponseMessage struct {
	Type  string                 `json:"type"`
	Text  string                 `json:"text"`
	ID    string                 `json:"id,omitempty"`
	Name  string                 `json:"name,omitempty"`
	Input map[string]interface{} `json:"input,omitempty"`
}

func (p *anthropicProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, model, sessionID string) (providers.ChatResponse, error) {
	if strings.TrimSpace(model) == "" {
		model = preferredFallbackModel(p.models, "claude-sonnet-4-5-20250929")
	}

	payload := anthropicChatRequest{
		Model:     model,
		MaxTokens: req.MaxTokens,
		Stream:    req.Stream,
	}
	if req.MaxTokens == 0 {
		payload.MaxTokens = 1024
	}
	if req.Temperature > 0 {
		payload.Temperature = req.Temperature
	}

	var systemMessage strings.Builder
	messages := make([]anthropicChatMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := strings.TrimSpace(strings.ToLower(msg.Role))
		if role == "system" {
			if systemMessage.Len() > 0 {
				systemMessage.WriteString("\n")
			}
			systemMessage.WriteString(msg.Content)
			continue
		}
		if role == "tool" {
			messages = append(messages, anthropicChatMessage{
				Role: "user",
				Content: []interface{}{
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
		for _, toolCall := range msg.ToolCalls {
			function, _ := toolCall["function"].(map[string]interface{})
			content = append(content, map[string]interface{}{
				"type":  "tool_use",
				"id":    firstNonEmptyString(stringValue(toolCall["id"]), stringValue(toolCall["call_id"])),
				"name":  stringValue(function["name"]),
				"input": parseArgumentsMap(function["arguments"]),
			})
		}
		messages = append(messages, anthropicChatMessage{
			Role:    role,
			Content: content,
		})
	}
	if systemMessage.Len() > 0 {
		payload.System = systemMessage.String()
	}
	payload.Messages = messages
	if len(req.Tools) > 0 {
		// Transform OpenAI-format tools to Anthropic format
		payload.Tools = TransformToAnthropicTools(req.Tools)
	}
	if req.ToolChoice != nil {
		payload.ToolChoice = req.ToolChoice
	}
	if len(req.Thinking) > 0 {
		payload.Thinking = req.Thinking
	} else if req.ReasoningEffort != "" {
		payload.Thinking = map[string]interface{}{
			"type":          "enabled",
			"budget_tokens": reasoningBudgetForEffort(req.ReasoningEffort),
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return providers.ChatResponse{}, fmt.Errorf("marshal anthropic request: %w", err)
	}

	var lastErr error
	for _, credential := range p.credentialSequence(ctx) {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/messages", bytes.NewBuffer(body))
		if err != nil {
			return providers.ChatResponse{}, err
		}
		p.applyAnthropicHeaders(request, credential, sessionID, req.Stream)

		resp, err := p.client.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("anthropic request failed: %w", err)
			continue
		}

		var upstreamResp anthropicChatResponse
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			lastErr = fmt.Errorf("anthropic returned %d", resp.StatusCode)
			continue
		}
		decodedBody, err := decodeResponseBody(resp.Body, resp.Header.Get("Content-Encoding"))
		if err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("anthropic decode setup failure: %w", err)
			continue
		}
		if err := json.NewDecoder(decodedBody).Decode(&upstreamResp); err != nil {
			decodedBody.Close()
			lastErr = fmt.Errorf("anthropic decode failure: %w", err)
			continue
		}
		decodedBody.Close()

		content := ""
		toolCalls := make([]map[string]interface{}, 0)
		for _, block := range upstreamResp.Content {
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
				content += block.Text
			}
		}
		finishReason := "stop"
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		}
		return providers.ChatResponse{
			ID:     upstreamResp.ID,
			Object: "chat.completion",
			Model:  model,
			Choices: []providers.Choice{
				{
					Index: 0,
					Message: providers.Message{
						Role:      "assistant",
						Content:   content,
						ToolCalls: toolCalls,
					},
					FinishReason: finishReason,
				},
			},
			Usage: providers.Usage{
				PromptTokens:     upstreamResp.Usage.InputTokens,
				CompletionTokens: upstreamResp.Usage.OutputTokens,
				TotalTokens:      upstreamResp.Usage.InputTokens + upstreamResp.Usage.OutputTokens,
			},
		}, nil
	}
	return providers.ChatResponse{}, lastErr
}

func (p *openAIProvider) Name() string { return "openai" }

func (p *openAIProvider) SupportsModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || model == "auto" {
		return true
	}
	for _, candidate := range p.models {
		if candidate == model {
			return true
		}
	}
	return strings.HasPrefix(model, "gpt") || strings.Contains(model, "codex") || strings.HasPrefix(model, "qwen")
}

func (p *openAIProvider) ListModels() []ModelInfo {
	return p.liveModels(context.Background())
}

func (p *openAIProvider) IsHealthy(ctx context.Context) bool {
	hasUsableCredential := false
	for _, credential := range p.credentialSequence(ctx) {
		if credentialLooksUsable(credential) {
			hasUsableCredential = true
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/models", nil)
		if err != nil {
			return hasUsableCredential
		}
		p.applyCredential(req, credential)
		resp, err := p.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return hasUsableCredential
}

type openAIChatRequest struct {
	Model           string                   `json:"model"`
	Messages        []providers.Message      `json:"messages"`
	Temperature     float64                  `json:"temperature,omitempty"`
	MaxTokens       int                      `json:"max_tokens,omitempty"`
	Stream          bool                     `json:"stream,omitempty"`
	Functions       []map[string]interface{} `json:"functions,omitempty"`
	FunctionCall    interface{}              `json:"function_call,omitempty"`
	ReasoningEffort string                   `json:"reasoning_effort,omitempty"`
	Thinking        map[string]interface{}   `json:"thinking,omitempty"`
}

func (p *openAIProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, model, sessionID string) (providers.ChatResponse, error) {
	if model == "" {
		model = preferredFallbackModel(p.models, "gpt-5.3-codex")
	}

	payload := openAIChatRequest{
		Model:           model,
		Messages:        req.Messages,
		Temperature:     req.Temperature,
		MaxTokens:       req.MaxTokens,
		Stream:          req.Stream,
		ReasoningEffort: req.ReasoningEffort,
		Thinking:        req.Thinking,
	}

	// Translate tools to functions for Codex/older backends
	if len(req.Tools) > 0 {
		payload.Functions = make([]map[string]interface{}, 0, len(req.Tools))
		for _, tool := range req.Tools {
			if stringValue(tool["type"]) == "function" {
				if fn, ok := tool["function"].(map[string]interface{}); ok {
					payload.Functions = append(payload.Functions, fn)
				}
			}
		}
		if req.ToolChoice != nil {
			if choice, ok := req.ToolChoice.(map[string]interface{}); ok {
				if fn, ok := choice["function"].(map[string]interface{}); ok {
					payload.FunctionCall = stringValue(fn["name"])
				}
			} else {
				payload.FunctionCall = req.ToolChoice
			}
		}
	} else if len(req.Functions) > 0 {
		payload.Functions = req.Functions
		payload.FunctionCall = req.FunctionCall
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return providers.ChatResponse{}, fmt.Errorf("marshal openai request: %w", err)
	}

	var lastErr error
	for _, credential := range p.credentialSequence(ctx) {
		if isCodexOAuthCredential(credential) {
			upstreamResp, err := p.codexChatCompletion(ctx, credential, req, model, sessionID)
			if err != nil {
				lastErr = err
				continue
			}
			return upstreamResp, nil
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewBuffer(body))
		if err != nil {
			return providers.ChatResponse{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(sessionID) != "" {
			request.Header.Set("Session_id", sessionID)
		}
		p.applyCredential(request, credential)

		resp, err := p.client.Do(request)
		if err != nil {
			lastErr = fmt.Errorf("openai request failed: %w", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respErrBody, _ := io.ReadAll(resp.Body)
			lastErr = fmt.Errorf("openai returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respErrBody)))
			continue
		}
		var upstreamResp providers.ChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&upstreamResp); err != nil {
			lastErr = fmt.Errorf("openai decode failure: %w", err)
			continue
		}
		return upstreamResp, nil
	}
	return providers.ChatResponse{}, lastErr
}

func (p *geminiProvider) Name() string { return "gemini" }

func (p *geminiProvider) SupportsModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	for _, candidate := range p.models {
		if candidate == model {
			return true
		}
	}
	return strings.HasPrefix(model, "gemini") || strings.Contains(model, "gemini")
}

func (p *geminiProvider) ListModels() []ModelInfo {
	return p.liveModels(context.Background())
}

func (p *geminiProvider) IsHealthy(ctx context.Context) bool {
	requestURL := fmt.Sprintf("%s/models", p.baseURL)
	hasUsableCredential := false
	for _, credential := range p.credentialSequence(ctx) {
		if credentialLooksUsable(credential) {
			hasUsableCredential = true
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
		if err != nil {
			return hasUsableCredential
		}
		p.applyCredential(req, credential)
		resp, err := p.client.Do(req)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return hasUsableCredential
}

type geminiChatRequest struct {
	Contents         []geminiContent        `json:"contents"`
	GenerationConfig geminiGenerationConfig `json:"generationConfig,omitempty"`
	Stream           bool                   `json:"stream,omitempty"`
}

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text             string                 `json:"text,omitempty"`
	FunctionCall     map[string]interface{} `json:"functionCall,omitempty"`
	FunctionResponse map[string]interface{} `json:"functionResponse,omitempty"`
}

type geminiGenerationConfig struct {
	Temperature     float64                `json:"temperature,omitempty"`
	MaxOutputTokens int                    `json:"maxOutputTokens,omitempty"`
	ThinkingConfig  map[string]interface{} `json:"thinkingConfig,omitempty"`
}

type geminiChatResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text         string                 `json:"text,omitempty"`
				FunctionCall map[string]interface{} `json:"functionCall,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

func (p *geminiProvider) ChatCompletion(ctx context.Context, req providers.ChatRequest, model, sessionID string) (providers.ChatResponse, error) {
	if model == "" {
		model = preferredFallbackModel(p.models, "gemini-3-flash-preview")
	}

	// Always prioritize the CLI-mirroring subscription path if possible
	var lastErr error
	credentials := p.credentialSequence(ctx)
	log.Printf("[Gemini] Trying %d credentials for model %s", len(credentials), model)
	for i, credential := range credentials {
		if isGeminiOAuthCredential(credential) {
			log.Printf("[Gemini] Attempting OAuth credential %d/%d", i+1, len(credentials))
			upstreamResp, err := p.geminiCLIChatCompletion(ctx, credential, req, model, sessionID)
			if err != nil {
				log.Printf("[Gemini] OAuth credential %d failed: %v", i+1, err)
				lastErr = err
				continue
			}
			log.Printf("[Gemini] OAuth credential %d succeeded", i+1)
			return upstreamResp, nil
		}
	}

	contents := make([]geminiContent, 0, len(req.Messages))
	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "system" {
			role = "user"
		}
		if role != "assistant" && role != "user" && role != "tool" {
			role = "user"
		}
		parts := make([]geminiPart, 0, 1+len(msg.ToolCalls))
		if strings.TrimSpace(msg.Content) != "" {
			parts = append(parts, geminiPart{Text: msg.Content})
		}
		for _, toolCall := range msg.ToolCalls {
			function, _ := toolCall["function"].(map[string]interface{})
			parts = append(parts, geminiPart{
				FunctionCall: map[string]interface{}{
					"name": stringValue(function["name"]),
					"args": parseArgumentsMap(function["arguments"]),
				},
			})
		}
		if role == "tool" {
			role = "user"
			parts = []geminiPart{{
				FunctionResponse: map[string]interface{}{
					"name":     msg.Name,
					"response": map[string]interface{}{"tool_call_id": msg.ToolCallID, "content": msg.Content},
				},
			}}
		}
		contents = append(contents, geminiContent{
			Role:  role,
			Parts: parts,
		})
	}
	if len(contents) == 0 {
		return providers.ChatResponse{}, fmt.Errorf("no valid message content")
	}

	payload := geminiChatRequest{
		Contents: contents,
		Stream:   req.Stream,
	}
	if req.MaxTokens > 0 {
		maxTokens := req.MaxTokens
		// Gemini 2.5 models use thinking tokens from the output budget.
		// Ensure enough headroom so thinking doesn't consume all output tokens.
		if maxTokens < 1024 {
			maxTokens = 1024
		}
		payload.GenerationConfig.MaxOutputTokens = maxTokens
	}
	if req.Temperature > 0 {
		payload.GenerationConfig.Temperature = req.Temperature
	}
	if len(req.Thinking) > 0 {
		payload.GenerationConfig.ThinkingConfig = req.Thinking
	} else if req.ReasoningEffort != "" {
		payload.GenerationConfig.ThinkingConfig = map[string]interface{}{
			"thinkingBudget": reasoningBudgetForEffort(req.ReasoningEffort),
		}
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return providers.ChatResponse{}, fmt.Errorf("marshal gemini request: %w", err)
	}

	requestURL := fmt.Sprintf("%s/models/%s:generateContent", p.baseURL, model)
	apiKeyAttempts := 0
	for i, credential := range p.credentialSequence(ctx) {
		// Non-OAuth credentials (API keys) will hit the manual path below
		if isGeminiOAuthCredential(credential) {
			continue // Already tried above
		}

		apiKeyAttempts++
		log.Printf("[Gemini] Attempting API key credential %d (overall credential %d/%d)", apiKeyAttempts, i+1, len(credentials))

		request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewBuffer(body))
		if err != nil {
			return providers.ChatResponse{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		p.applyCredential(request, credential)

		resp, err := p.client.Do(request)
		if err != nil {
			log.Printf("[Gemini] API key credential %d failed with network error: %v", apiKeyAttempts, err)
			lastErr = fmt.Errorf("gemini request failed: %w", err)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			log.Printf("[Gemini] API key credential %d returned HTTP %d", apiKeyAttempts, resp.StatusCode)
			resp.Body.Close()
			lastErr = fmt.Errorf("gemini returned %d", resp.StatusCode)
			continue
		}

		var upstreamResp geminiChatResponse
		if err := json.NewDecoder(resp.Body).Decode(&upstreamResp); err != nil {
			resp.Body.Close()
			lastErr = fmt.Errorf("gemini decode failure: %w", err)
			continue
		}
		resp.Body.Close()
		if len(upstreamResp.Candidates) == 0 {
			lastErr = fmt.Errorf("gemini returned empty completion (no candidates)")
			continue
		}
		responseText := ""
		toolCalls := make([]map[string]interface{}, 0)
		for _, part := range upstreamResp.Candidates[0].Content.Parts {
			if len(part.FunctionCall) > 0 {
				name := stringValue(part.FunctionCall["name"])
				args := "{}"
				if rawArgs, ok := part.FunctionCall["args"]; ok {
					if encodedArgs, err := json.Marshal(rawArgs); err == nil {
						args = string(encodedArgs)
					}
				}
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   fmt.Sprintf("gemini-call-%d", len(toolCalls)+1),
					"type": "function",
					"function": map[string]interface{}{
						"name":      name,
						"arguments": args,
					},
				})
				continue
			}
			responseText += part.Text
		}
		finishReason := "stop"
		if len(toolCalls) > 0 {
			finishReason = "tool_calls"
		}
		return providers.ChatResponse{
			ID:     fmt.Sprintf("gemini-%d", time.Now().UnixNano()),
			Object: "chat.completion",
			Model:  model,
			Choices: []providers.Choice{
				{
					Index: 0,
					Message: providers.Message{
						Role:      "assistant",
						Content:   responseText,
						ToolCalls: toolCalls,
					},
					FinishReason: finishReason,
				},
			},
			Usage: providers.Usage{
				PromptTokens:     upstreamResp.UsageMetadata.PromptTokenCount,
				CompletionTokens: upstreamResp.UsageMetadata.CandidatesTokenCount,
				TotalTokens:      upstreamResp.UsageMetadata.TotalTokenCount,
			},
		}, nil
	}
	return providers.ChatResponse{}, lastErr
}

func ensureCredentials(spec ProviderSpec) []ProviderCredential {
	if len(spec.Credentials) > 0 {
		return append([]ProviderCredential(nil), spec.Credentials...)
	}
	if spec.APIKey != "" || spec.SessionToken != "" {
		return []ProviderCredential{{APIKey: spec.APIKey, SessionToken: spec.SessionToken}}
	}
	return nil
}

func reasoningBudgetForEffort(effort string) int {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return 4096
	case "medium":
		return 8192
	case "high":
		return 16384
	case "xhigh":
		return 32768
	case "none":
		return 0
	default:
		return 8192
	}
}

func stringValue(value interface{}) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func parseArgumentsMap(raw interface{}) map[string]interface{} {
	switch typed := raw.(type) {
	case map[string]interface{}:
		return typed
	case string:
		if strings.TrimSpace(typed) == "" {
			return map[string]interface{}{}
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal([]byte(typed), &decoded); err == nil {
			return decoded
		}
	}
	return map[string]interface{}{}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (p *anthropicProvider) nextCredential() ProviderCredential {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.credentials) == 0 {
		return p.fallbackCredential()
	}
	credential := p.credentials[p.cursor%len(p.credentials)]
	p.cursor = (p.cursor + 1) % len(p.credentials)
	return credential
}

func (p *anthropicProvider) credentialSequence(ctx context.Context) []ProviderCredential {
	if override := preferredUpstreamAPIKey(ctx); override != "" {
		return []ProviderCredential{{APIKey: override}}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.credentials) == 0 {
		return []ProviderCredential{p.fallbackCredential()}
	}

	refreshed := refreshConfiguredCredentials(ctx, p.Name(), p.credentials)
	p.credentials = append(p.credentials[:0], refreshed...)
	return append([]ProviderCredential(nil), refreshed...)
}

func (p *anthropicProvider) applyCredential(req *http.Request, credential ProviderCredential) {
	p.applyAnthropicHeaders(req, credential, "", false)
}

func (p *anthropicProvider) fallbackCredential() ProviderCredential {
	return ProviderCredential{
		APIKey:         strings.TrimSpace(p.apiKey),
		SessionToken:   strings.TrimSpace(p.sessionToken),
		CredentialType: resolveCredentialType(p.apiKey, p.sessionToken, credentialTypeUnknown),
	}
}

func (p *openAIProvider) nextCredential() ProviderCredential {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.credentials) == 0 {
		return p.fallbackCredential()
	}
	credential := p.credentials[p.cursor%len(p.credentials)]
	p.cursor = (p.cursor + 1) % len(p.credentials)
	return credential
}

func (p *openAIProvider) credentialSequence(ctx context.Context) []ProviderCredential {
	if override := preferredUpstreamAPIKey(ctx); override != "" {
		return []ProviderCredential{{APIKey: override}}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.credentials) == 0 {
		return []ProviderCredential{p.fallbackCredential()}
	}

	refreshed := refreshConfiguredCredentials(ctx, p.Name(), p.credentials)
	p.credentials = append(p.credentials[:0], refreshed...)
	return append([]ProviderCredential(nil), refreshed...)
}

func (p *openAIProvider) applyCredential(req *http.Request, credential ProviderCredential) {
	if credential.isSessionCredential() {
		if token := strings.TrimSpace(credential.SessionToken); token != "" {
			req.Header.Set("Cookie", token)
		}
		return
	}
	if token := strings.TrimSpace(credential.authToken()); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

func (p *openAIProvider) fallbackCredential() ProviderCredential {
	return ProviderCredential{
		APIKey:         strings.TrimSpace(p.apiKey),
		SessionToken:   strings.TrimSpace(p.sessionToken),
		CredentialType: resolveCredentialType(p.apiKey, p.sessionToken, credentialTypeUnknown),
	}
}

func (p *geminiProvider) nextCredential() ProviderCredential {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.credentials) == 0 {
		return p.fallbackCredential()
	}
	credential := p.credentials[p.cursor%len(p.credentials)]
	p.cursor = (p.cursor + 1) % len(p.credentials)
	return credential
}

func (p *geminiProvider) credentialSequence(ctx context.Context) []ProviderCredential {
	if override := preferredUpstreamAPIKey(ctx); override != "" {
		return []ProviderCredential{{APIKey: override}}
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.credentials) == 0 {
		return []ProviderCredential{p.fallbackCredential()}
	}

	refreshed := refreshConfiguredCredentials(ctx, p.Name(), p.credentials)
	p.credentials = append(p.credentials[:0], refreshed...)
	return append([]ProviderCredential(nil), refreshed...)
}

func (p *geminiProvider) applyCredential(req *http.Request, credential ProviderCredential) {
	if credential.isSessionCredential() {
		if token := strings.TrimSpace(credential.SessionToken); token != "" {
			req.Header.Set("Cookie", token)
		}
		return
	}
	if token := strings.TrimSpace(credential.AccessToken); token != "" {
		req.Header.Set("Authorization", bearerPrefix(credential.TokenType)+token)
		return
	}
	if credential.APIKey != "" {
		q := req.URL.Query()
		q.Set("key", credential.APIKey)
		req.URL.RawQuery = q.Encode()
	}
}

func (p *geminiProvider) fallbackCredential() ProviderCredential {
	return ProviderCredential{
		APIKey:         strings.TrimSpace(p.apiKey),
		SessionToken:   strings.TrimSpace(p.sessionToken),
		CredentialType: resolveCredentialType(p.apiKey, p.sessionToken, credentialTypeUnknown),
	}
}

func credentialLooksUsable(credential ProviderCredential) bool {
	return strings.TrimSpace(credential.APIKey) != "" ||
		strings.TrimSpace(credential.SessionToken) != "" ||
		strings.TrimSpace(credential.AccessToken) != ""
}

func refreshConfiguredCredentials(ctx context.Context, provider string, configured []ProviderCredential) []ProviderCredential {
	refreshed := append([]ProviderCredential(nil), configured...)
	for i, credential := range refreshed {
		if next, ok := refreshCredentialIfNeeded(ctx, provider, credential); ok {
			refreshed[i] = next
		}
	}
	return refreshed
}

func bearerPrefix(tokenType string) string {
	tokenType = strings.TrimSpace(tokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	return tokenType + " "
}
