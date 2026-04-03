package subscriptions

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
)

const (
	codexSubscriptionBaseURL  = "https://chatgpt.com/backend-api/codex"
	codexCompactResponsesPath = "/responses"
	codexClientVersion        = "0.101.0"
	codexUserAgent            = "codex_cli_rs/0.101.0 (Mac OS 26.0.1; arm64) Apple_Terminal/464"

	geminiSubscriptionBaseURL = "https://cloudcode-pa.googleapis.com"
	geminiFetchModelsPath     = "/v1internal:fetchAvailableModels"
	geminiGenerateContentPath      = "/v1internal:generateContent"
	geminiStreamGenerateContentURL = "https://cloudcode-pa.googleapis.com/v1internal:streamGenerateContent?alt=sse"
	geminiCLIVersion          = "0.33.1"

	geminiGenerateContentMaxRetries = 3
	geminiRetryDelayMs              = 1000

	providerModelCacheTTL = 10 * time.Minute
)

type providerModelCacheEntry struct {
	expires time.Time
	models  []ModelInfo
}

var providerModelCache struct {
	mu   sync.RWMutex
	data map[string]providerModelCacheEntry
}

type codexCompactRequest struct {
	Model        string                   `json:"model"`
	Input        []map[string]interface{} `json:"input,omitempty"`
	Instructions string                   `json:"instructions"`
	Tools        []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice   interface{}              `json:"tool_choice,omitempty"`
	Stream       bool                     `json:"stream,omitempty"`
	Store        *bool                    `json:"store,omitempty"`
}

type compositeReadCloser struct {
	io.Reader
	closers []func() error
}

func (c *compositeReadCloser) Close() error {
	var firstErr error
	for _, closer := range c.closers {
		if closer == nil {
			continue
		}
		if err := closer(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type peekableBody struct {
	*bufio.Reader
	closer io.Closer
}

func (p *peekableBody) Close() error {
	if p == nil || p.closer == nil {
		return nil
	}
	return p.closer.Close()
}

func (p *anthropicProvider) liveModels(ctx context.Context) []ModelInfo {
	return cachedProviderModels("anthropic", func() []ModelInfo {
		requestURL := strings.TrimRight(p.baseURL, "/") + "/v1/models"
		for _, credential := range p.credentialSequence(ctx) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
			if err != nil {
				continue
			}
			p.applyAnthropicHeaders(req, credential, "", false)
			resp, err := p.client.Do(req)
			if err != nil {
				continue
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK {
				continue
			}
			if ids := parseModelIDs(body, looksLikeClaudeModel); len(ids) > 0 {
				return buildModelInfos(ids, "claude-code", 200000, "Claude model via subscription gateway", p.Name(), "code")
			}
		}
		return buildModelInfos(p.models, "claude-code", 200000, "Claude model via subscription gateway", p.Name(), "code")
	})
}

func (p *openAIProvider) liveModels(ctx context.Context) []ModelInfo {
	return cachedProviderModels("openai", func() []ModelInfo {
		for _, credential := range p.credentialSequence(ctx) {
			if isCodexOAuthCredential(credential) {
				for _, requestURL := range []string{
					strings.TrimRight(codexBaseURL(p.baseURL), "/") + "/models",
					strings.TrimRight(codexBaseURL(p.baseURL), "/") + "/responses/models",
					"https://chatgpt.com/backend-api/models",
				} {
					req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
					if err != nil {
						continue
					}
					applyCodexHeaders(req, credential, "", false)
					resp, err := p.client.Do(req)
					if err != nil {
						continue
					}
					body, err := io.ReadAll(resp.Body)
					resp.Body.Close()
					if err != nil || resp.StatusCode != http.StatusOK {
						continue
					}
					if ids := parseModelIDs(body, looksLikeCodexModel); len(ids) > 0 {
						return buildModelInfos(codexCatalogAliases(ids), "codex", 128000, "OpenAI / Codex model via subscription gateway", p.Name(), "code")
					}
				}
				continue
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL, "/")+"/models", nil)
			if err != nil {
				continue
			}
			p.applyCredential(req, credential)
			resp, err := p.client.Do(req)
			if err != nil {
				continue
			}
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK {
				continue
			}
			if ids := parseModelIDs(body, looksLikeCodexModel); len(ids) > 0 {
				return buildModelInfos(codexCatalogAliases(ids), "codex", 128000, "OpenAI / Codex model via subscription gateway", p.Name(), "code")
			}
		}
		return buildModelInfos(p.models, "codex", 128000, "OpenAI / Codex model via subscription gateway", p.Name(), "code")
	})
}

func (p *geminiProvider) liveModels(ctx context.Context) []ModelInfo {
	return cachedProviderModels("gemini", func() []ModelInfo {
		for _, credential := range p.credentialSequence(ctx) {
			if isGeminiOAuthCredential(credential) {
				payload := map[string]string{}
				if projectID := strings.TrimSpace(credential.ProjectID); projectID != "" {
					payload["project"] = projectID
				}
				body, _ := json.Marshal(payload)
				req, err := http.NewRequestWithContext(ctx, http.MethodPost, geminiSubscriptionBaseURL+geminiFetchModelsPath, bytes.NewReader(body))
				if err != nil {
					continue
				}
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", bearerPrefix(credential.TokenType)+credential.AccessToken)
				applyGeminiCLIHeaders(req, "")
				resp, err := p.client.Do(req)
				if err != nil {
					continue
				}
				respBody, err := io.ReadAll(resp.Body)
				resp.Body.Close()
				if err != nil || resp.StatusCode != http.StatusOK {
					continue
				}
				if ids := parseModelIDs(respBody, looksLikeGeminiModel); len(ids) > 0 {
					return buildModelInfos(ids, "gemini", 1048576, "Gemini model via subscription gateway", p.Name(), "reasoning")
				}
				continue
			}

			req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL, "/")+"/models", nil)
			if err != nil {
				continue
			}
			p.applyCredential(req, credential)
			resp, err := p.client.Do(req)
			if err != nil {
				continue
			}
			respBody, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil || resp.StatusCode != http.StatusOK {
				continue
			}
			if ids := parseModelIDs(respBody, looksLikeGeminiModel); len(ids) > 0 {
				return buildModelInfos(ids, "gemini", 1048576, "Gemini model via subscription gateway", p.Name(), "reasoning")
			}
		}
		return buildModelInfos(p.models, "gemini", 1048576, "Gemini model via subscription gateway", p.Name(), "reasoning")
	})
}

func (p *anthropicProvider) applyAnthropicHeaders(req *http.Request, credential ProviderCredential, sessionID string, stream bool) {
	if credential.isSessionCredential() {
		if token := strings.TrimSpace(credential.SessionToken); token != "" {
			req.Header.Set("Cookie", token)
		}
	} else if token := strings.TrimSpace(credential.APIKey); token != "" {
		req.Header.Del("Authorization")
		req.Header.Set("x-api-key", token)
	} else if token := strings.TrimSpace(credential.AccessToken); token != "" {
		req.Header.Del("x-api-key")
		req.Header.Set("Authorization", bearerPrefix(credential.TokenType)+token)
	}
	req.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(sessionID) != "" {
		req.Header.Set("Session_id", sessionID)
	}
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Beta", "claude-code-20250219,oauth-2025-04-20,interleaved-thinking-2025-05-14,context-management-2025-06-27,prompt-caching-scope-2026-01-05")
	req.Header.Set("Anthropic-Dangerous-Direct-Browser-Access", "true")
	req.Header.Set("X-App", "cli")
	req.Header.Set("X-Stainless-Retry-Count", "0")
	req.Header.Set("X-Stainless-Runtime-Version", "v24.3.0")
	req.Header.Set("X-Stainless-Package-Version", "0.74.0")
	req.Header.Set("X-Stainless-Runtime", "node")
	req.Header.Set("X-Stainless-Lang", "js")
	req.Header.Set("X-Stainless-Arch", mapStainlessArch())
	req.Header.Set("X-Stainless-Os", mapStainlessOS())
	req.Header.Set("X-Stainless-Timeout", "600")
	req.Header.Set("User-Agent", "claude-cli/2.1.63 (external, cli)")
	req.Header.Set("Connection", "keep-alive")
	if stream {
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Accept-Encoding", "identity")
	} else {
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Accept-Encoding", "identity")
	}
}

func (p *openAIProvider) codexChatCompletion(ctx context.Context, credential ProviderCredential, req providers.ChatRequest, model, sessionID string) (providers.ChatResponse, error) {
	// Validate tool support - Codex compact API does not reliably support tool calling
	// Return explicit error instead of silently degrading to plain text
	if len(req.Tools) > 0 || len(req.Functions) > 0 {
		return providers.ChatResponse{}, fmt.Errorf("tool calling is not supported on the Codex compact responses path - use standard OpenAI chat completions endpoint for tool support")
	}

	// Note: caller-requested streaming is handled separately by the handler layer.
	// The internal Codex subscription path always uses SSE streaming internally
	// and collects the response into a single ChatResponse.
	if req.Stream {
		return providers.ChatResponse{}, fmt.Errorf("streaming passthrough is not supported on the Codex subscription path - use non-streaming requests")
	}

	model = normalizeCodexModel(model)
	payload := buildCodexCompactRequest(req, model)
	body, err := json.Marshal(payload)
	if err != nil {
		return providers.ChatResponse{}, fmt.Errorf("marshal codex request: %w", err)
	}

	requestURL := strings.TrimRight(codexBaseURL(p.baseURL), "/") + codexCompactResponsesPath
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return providers.ChatResponse{}, err
	}
	applyCodexHeaders(httpReq, credential, sessionID, true)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return providers.ChatResponse{}, fmt.Errorf("codex request failed: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return providers.ChatResponse{}, fmt.Errorf("openai returned %d: %s", httpResp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return collectCodexSSEResponse(httpResp.Body, model)
}

func collectCodexSSEResponse(body io.Reader, model string) (providers.ChatResponse, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	var responseID string
	var contentBuilder strings.Builder
	toolCalls := make([]map[string]interface{}, 0)
	var totalUsage providers.Usage

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			break
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		eventType := stringValue(event["type"])
		switch eventType {
		case "response.created":
			if resp, ok := event["response"].(map[string]interface{}); ok {
				responseID = stringValue(resp["id"])
			}

		case "response.output_text.delta":
			// Do NOT use stringValue here — it trims spaces, which corrupts token boundaries
			if delta, ok := event["delta"].(string); ok {
				contentBuilder.WriteString(delta)
			}

		case "response.output_text.done":
			// Final text for this output item — use if we haven't accumulated via deltas
			if contentBuilder.Len() == 0 {
				if text, ok := event["text"].(string); ok {
					contentBuilder.WriteString(text)
				}
			}

		case "response.function_call_arguments.done":
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   firstNonEmptyString(stringValue(event["call_id"]), stringValue(event["item_id"])),
				"type": "function",
				"function": map[string]interface{}{
					"name":      stringValue(event["name"]),
					"arguments": firstNonEmptyString(stringValue(event["arguments"]), "{}"),
				},
			})

		case "response.completed":
			if resp, ok := event["response"].(map[string]interface{}); ok {
				if responseID == "" {
					responseID = stringValue(resp["id"])
				}
				// Extract output_text if available at top level
				if outputText := stringValue(resp["output_text"]); outputText != "" && contentBuilder.Len() == 0 {
					contentBuilder.WriteString(outputText)
				}
				// Extract usage
				if usageRaw, ok := resp["usage"].(map[string]interface{}); ok {
					if v, ok := usageRaw["input_tokens"].(float64); ok {
						totalUsage.PromptTokens = int(v)
					}
					if v, ok := usageRaw["output_tokens"].(float64); ok {
						totalUsage.CompletionTokens = int(v)
					}
					if v, ok := usageRaw["total_tokens"].(float64); ok {
						totalUsage.TotalTokens = int(v)
					}
				}
			}
		}
	}

	content := contentBuilder.String()
	if totalUsage.TotalTokens == 0 {
		totalUsage.TotalTokens = totalUsage.PromptTokens + totalUsage.CompletionTokens
	}

	if strings.TrimSpace(content) == "" && len(toolCalls) == 0 {
		log.Printf("[Codex] WARNING: SSE stream completed but no content or tool calls extracted")
	}

	finishReason := "stop"
	if len(toolCalls) > 0 {
		finishReason = "tool_calls"
	}

	return providers.ChatResponse{
		ID:     firstNonEmptyString(responseID, "codex-"+strconv.FormatInt(time.Now().UnixNano(), 10)),
		Object: "chat.completion",
		Model:  model,
		Choices: []providers.Choice{{
			Index: 0,
			Message: providers.Message{
				Role:      "assistant",
				Content:   strings.TrimSpace(content),
				ToolCalls: toolCalls,
			},
			FinishReason: finishReason,
		}},
		Usage: normalizeUsage(totalUsage),
	}, nil
}

func codexMessageInput(role, content string) map[string]interface{} {
	partType := "input_text"
	if role == "assistant" {
		partType = "output_text"
	}
	
	msg := map[string]interface{}{
		"type": "message",
		"role": role,
		"content": []map[string]interface{}{
			{
				"type": partType,
				"text": content,
			},
		},
	}
	if role == "system" {
		msg["role"] = "developer"
	}
	return msg
}

func buildCodexCompactRequest(req providers.ChatRequest, model string) codexCompactRequest {
	storeFalse := false
	payload := codexCompactRequest{
		Model:        model,
		Instructions: "You are Codex. Answer the user's request concisely and accurately.",
		Stream:       true,
		Store:        &storeFalse,
	}

	// 1. Map tools (flatten function fields)
	tools := req.Tools
	if len(tools) == 0 && len(req.Functions) > 0 {
		tools = make([]map[string]interface{}, 0, len(req.Functions))
		for _, fn := range req.Functions {
			tools = append(tools, map[string]interface{}{
				"type":     "function",
				"function": fn,
			})
		}
	}

	if len(tools) > 0 {
		payload.Tools = make([]map[string]interface{}, 0, len(tools))
		for _, t := range tools {
			toolType := stringValue(t["type"])
			if toolType == "function" {
				fn, _ := t["function"].(map[string]interface{})
				item := map[string]interface{}{
					"type":        "function",
					"name":        stringValue(fn["name"]),
					"description": stringValue(fn["description"]),
					"parameters":  fn["parameters"],
				}
				if strict, ok := fn["strict"].(bool); ok {
					item["strict"] = strict
				}
				payload.Tools = append(payload.Tools, item)
			} else {
				payload.Tools = append(payload.Tools, t)
			}
		}

		if req.ToolChoice != nil {
			if tc, ok := req.ToolChoice.(map[string]interface{}); ok {
				if stringValue(tc["type"]) == "function" {
					if fn, ok := tc["function"].(map[string]interface{}); ok {
						payload.ToolChoice = map[string]interface{}{
							"type": "function",
							"name": stringValue(fn["name"]),
						}
					}
				}
			} else {
				payload.ToolChoice = req.ToolChoice
			}
		}
	}

	// 2. Build input from messages, handling all message types including tool calls
	for _, m := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(m.Role))

		switch role {
		case "tool":
			// Handle tool response messages as top-level function_call_output objects
			payload.Input = append(payload.Input, map[string]interface{}{
				"type":    "function_call_output",
				"call_id": m.ToolCallID,
				"output":  m.Content,
			})

		default:
			// Handle regular messages
			payload.Input = append(payload.Input, codexMessageInput(role, m.Content))

			// Handle tool calls for assistant messages as separate top-level objects
			if role == "assistant" && len(m.ToolCalls) > 0 {
				for _, tc := range m.ToolCalls {
					fn, _ := tc["function"].(map[string]interface{})
					callID := firstNonEmptyString(stringValue(tc["id"]), stringValue(tc["call_id"]))
					payload.Input = append(payload.Input, map[string]interface{}{
						"type":      "function_call",
						"call_id":   callID,
						"name":      stringValue(fn["name"]),
						"arguments": stringValue(fn["arguments"]),
					})
				}
			}
		}
	}

	return payload
}

func applyCodexHeaders(req *http.Request, credential ProviderCredential, sessionID string, stream bool) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+credential.authToken())
	req.Header.Set("Version", codexClientVersion)

	if strings.TrimSpace(sessionID) != "" {
		req.Header.Set("Session_id", sessionID)
	} else {
		req.Header.Set("Session_id", "synroute-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	}

	req.Header.Set("User-Agent", codexUserAgent)
	if stream {
		req.Header.Set("Accept", "text/event-stream")
	} else {
		req.Header.Set("Accept", "application/json")
	}
	req.Header.Set("Connection", "Keep-Alive")
	if !credential.isSessionCredential() && strings.TrimSpace(credential.AccessToken) != "" {
		req.Header.Set("Originator", "codex_cli_rs")
	}
}

func codexBaseURL(baseURL string) string {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" || strings.EqualFold(baseURL, defaultOpenAIBaseURL) || strings.Contains(baseURL, "api.openai.com") {
		return codexSubscriptionBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func isCodexOAuthCredential(credential ProviderCredential) bool {
	return strings.TrimSpace(credential.AccessToken) != "" && credential.isBearerCredential()
}

func isGeminiOAuthCredential(credential ProviderCredential) bool {
	return strings.TrimSpace(credential.AccessToken) != "" && credential.isBearerCredential()
}

func (p *geminiProvider) geminiCLIChatCompletion(ctx context.Context, credential ProviderCredential, request providers.ChatRequest, model, sessionID string) (providers.ChatResponse, error) {
	resolvedCredential, err := p.ensureGeminiCLIProject(ctx, credential)
	if err != nil {
		return providers.ChatResponse{}, err
	}

	encoded, err := buildGeminiCLIRequest(request, model, resolvedCredential.ProjectID, sessionID)
	if err != nil {
		return providers.ChatResponse{}, fmt.Errorf("marshal gemini cli request: %w", err)
	}

	// Use streamGenerateContent with SSE to match real Gemini CLI behavior.
	// The streaming endpoint has separate capacity from generateContent.
	requestURL := geminiStreamGenerateContentURL
	auth := bearerPrefix(resolvedCredential.TokenType) + resolvedCredential.AccessToken

	// Retry on 429/5xx matching the real Gemini CLI behavior (3 retries, 1s delay)
	var lastErr error
	for attempt := 0; attempt <= geminiGenerateContentMaxRetries; attempt++ {
		if attempt > 0 {
			log.Printf("[Gemini] Retry %d/%d after %dms", attempt, geminiGenerateContentMaxRetries, geminiRetryDelayMs)
			select {
			case <-ctx.Done():
				return providers.ChatResponse{}, ctx.Err()
			case <-time.After(time.Duration(geminiRetryDelayMs) * time.Millisecond):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(encoded))
		if err != nil {
			return providers.ChatResponse{}, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", auth)
		applyGeminiCLIHeaders(httpReq, model)

		httpResp, err := p.client.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("gemini request failed: %w", err)
			continue
		}

		if httpResp.StatusCode == 429 || httpResp.StatusCode >= 500 {
			body, _ := io.ReadAll(httpResp.Body)
			httpResp.Body.Close()
			lastErr = fmt.Errorf("gemini returned %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
			log.Printf("[Gemini] Got %d on attempt %d: %s", httpResp.StatusCode, attempt+1, truncateLogBody(body, 200))
			continue
		}

		upstreamResp, err := decodeGeminiSSEResponse(httpResp)
		httpResp.Body.Close()
		if err != nil {
			return providers.ChatResponse{}, err
		}
		return upstreamResp.asChatCompletion(model)
	}
	return providers.ChatResponse{}, lastErr
}

// decodeGeminiSSEResponse reads an SSE stream from streamGenerateContent and
// assembles the chunks into a single geminiChatResponse (last chunk wins for
// metadata; text parts are concatenated).
func decodeGeminiSSEResponse(resp *http.Response) (geminiChatResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return geminiChatResponse{}, fmt.Errorf("gemini SSE read failure: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return geminiChatResponse{}, fmt.Errorf("gemini returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	log.Printf("[Gemini] Raw CLI SSE response (%d bytes)", len(body))

	// Parse SSE: each "data: {...}" line is a JSON chunk
	var chunks [][]byte
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			chunks = append(chunks, []byte(strings.TrimPrefix(line, "data: ")))
		}
	}

	if len(chunks) == 0 {
		// Fallback: try parsing the whole body as a single JSON response
		// (in case server doesn't send SSE format)
		return decodeGeminiChatResponseFromBytes(body)
	}

	// Assemble: concatenate text parts, use last chunk's metadata
	var assembled geminiChatResponse
	var textParts []string
	for _, chunk := range chunks {
		parsed, err := decodeGeminiChatResponseFromBytes(chunk)
		if err != nil || len(parsed.Candidates) == 0 {
			continue
		}
		assembled.UsageMetadata = parsed.UsageMetadata
		for _, c := range parsed.Candidates {
			for _, p := range c.Content.Parts {
				if p.Text != "" {
					textParts = append(textParts, p.Text)
				}
			}
		}
	}

	if len(textParts) == 0 && len(chunks) > 0 {
		// Last resort: try to decode any format from the last chunk
		return decodeGeminiChatResponseFromBytes(chunks[len(chunks)-1])
	}

	// Build a properly typed candidate with concatenated text
	var candidate struct {
		Content struct {
			Parts []struct {
				Text             string                 `json:"text,omitempty"`
				FunctionCall     map[string]interface{} `json:"functionCall,omitempty"`
				Thought          bool                   `json:"thought,omitempty"`
				ThoughtSignature string                 `json:"thoughtSignature,omitempty"`
			} `json:"parts"`
		} `json:"content"`
	}
	candidate.Content.Parts = append(candidate.Content.Parts, struct {
		Text             string                 `json:"text,omitempty"`
		FunctionCall     map[string]interface{} `json:"functionCall,omitempty"`
		Thought          bool                   `json:"thought,omitempty"`
		ThoughtSignature string                 `json:"thoughtSignature,omitempty"`
	}{Text: strings.Join(textParts, "")})
	assembled.Candidates = append(assembled.Candidates, candidate)

	return assembled, nil
}

func decodeGeminiChatResponseFromBytes(body []byte) (geminiChatResponse, error) {
	var resp geminiChatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return geminiChatResponse{}, fmt.Errorf("gemini decode failure: %w", err)
	}
	if len(resp.Candidates) == 0 {
		var wrapped struct {
			Response geminiChatResponse `json:"response"`
		}
		if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Response.Candidates) > 0 {
			return wrapped.Response, nil
		}
	}
	return resp, nil
}

func (p *geminiProvider) ensureGeminiCLIProject(ctx context.Context, credential ProviderCredential) (ProviderCredential, error) {
	if strings.TrimSpace(credential.ProjectID) != "" {
		return credential, nil
	}

	projectID, err := discoverGeminiCLIProject(ctx, p.client, credential)
	if err != nil {
		return credential, fmt.Errorf("gemini project discovery failed: %w", err)
	}
	credential.ProjectID = projectID
	_ = StoreCredential("gemini", credential)
	return credential, nil
}

func applyGeminiCLIHeaders(req *http.Request, model string) {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "gemini"
	}
	req.Header.Set("User-Agent", fmt.Sprintf("GeminiCLI/%s/%s (%s; %s; CLI)", geminiCLIVersion, model, runtime.GOOS, runtime.GOARCH))
	req.Header.Set("Accept", "application/json")
}

func truncateLogBody(body []byte, maxLen int) string {
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "...[truncated]"
}

func discoverGeminiCLIProject(ctx context.Context, client *http.Client, credential ProviderCredential) (string, error) {
	metadata := map[string]string{
		"ideType":    "IDE_UNSPECIFIED",
		"platform":   "PLATFORM_UNSPECIFIED",
		"pluginType": "GEMINI",
	}

	var loadResp map[string]interface{}
	if err := callGeminiCLIEndpoint(ctx, client, credential, "loadCodeAssist", map[string]interface{}{
		"metadata": metadata,
	}, &loadResp); err != nil {
		return "", fmt.Errorf("load code assist: %w", err)
	}

	tierID := "legacy-tier"
	if tiers, ok := loadResp["allowedTiers"].([]interface{}); ok {
		for _, rawTier := range tiers {
			tier, ok := rawTier.(map[string]interface{})
			if !ok {
				continue
			}
			if isDefault, _ := tier["isDefault"].(bool); isDefault {
				if id := strings.TrimSpace(stringValue(tier["id"])); id != "" {
					tierID = id
					break
				}
			}
		}
	}

	projectID := geminiProjectIDFromResponse(loadResp["cloudaicompanionProject"])
	if projectID == "" {
		autoCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		autoReq := map[string]interface{}{
			"tierId":   tierID,
			"metadata": metadata,
		}
		for {
			var onboardResp map[string]interface{}
			if err := callGeminiCLIEndpoint(autoCtx, client, credential, "onboardUser", autoReq, &onboardResp); err != nil {
				return "", fmt.Errorf("auto-discovery onboard user: %w", err)
			}
			if done, _ := onboardResp["done"].(bool); done {
				if response, ok := onboardResp["response"].(map[string]interface{}); ok {
					projectID = geminiProjectIDFromResponse(response["cloudaicompanionProject"])
				}
				break
			}
			select {
			case <-autoCtx.Done():
				return "", fmt.Errorf("timed out waiting for project onboarding")
			case <-time.After(2 * time.Second):
			}
		}
	}
	if projectID == "" {
		return "", fmt.Errorf("project onboarding did not return a project id")
	}

	for {
		var onboardResp map[string]interface{}
		if err := callGeminiCLIEndpoint(ctx, client, credential, "onboardUser", map[string]interface{}{
			"tierId":                  tierID,
			"metadata":                metadata,
			"cloudaicompanionProject": projectID,
		}, &onboardResp); err != nil {
			return "", fmt.Errorf("onboard user: %w", err)
		}
		if done, _ := onboardResp["done"].(bool); done {
			if response, ok := onboardResp["response"].(map[string]interface{}); ok {
				if backendProject := geminiProjectIDFromResponse(response["cloudaicompanionProject"]); backendProject != "" {
					projectID = backendProject
				}
			}
			break
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return projectID, nil
}

func callGeminiCLIEndpoint(ctx context.Context, client *http.Client, credential ProviderCredential, endpoint string, body interface{}, result interface{}) error {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}
	url := fmt.Sprintf("%s%s:%s", geminiSubscriptionBaseURL, "/v1internal", endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(rawBody))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", bearerPrefix(credential.TokenType)+credential.AccessToken)
	applyGeminiCLIHeaders(req, "")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("api request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if result == nil {
		return nil
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decode response body: %w", err)
	}
	return nil
}

func geminiProjectIDFromResponse(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		return strings.TrimSpace(firstNonEmptyString(
			stringValue(typed["id"]),
			stringValue(typed["projectId"]),
			stringValue(typed["project_id"]),
		))
	default:
		return ""
	}
}

func decodeResponseBody(body io.ReadCloser, contentEncoding string) (io.ReadCloser, error) {
	if body == nil {
		return nil, fmt.Errorf("response body is nil")
	}
	if strings.TrimSpace(contentEncoding) == "" {
		peekable := &peekableBody{Reader: bufio.NewReader(body), closer: body}
		magic, peekErr := peekable.Peek(2)
		if peekErr == nil || (peekErr == io.EOF && len(magic) >= 2) {
			if len(magic) >= 2 && magic[0] == 0x1f && magic[1] == 0x8b {
				reader, err := gzip.NewReader(peekable)
				if err != nil {
					_ = peekable.Close()
					return nil, fmt.Errorf("magic-byte gzip: failed to create reader: %w", err)
				}
				return &compositeReadCloser{
					Reader: reader,
					closers: []func() error{
						reader.Close,
						peekable.Close,
					},
				}, nil
			}
		}
		return peekable, nil
	}

	for _, raw := range strings.Split(contentEncoding, ",") {
		switch strings.TrimSpace(strings.ToLower(raw)) {
		case "", "identity":
			continue
		case "gzip":
			reader, err := gzip.NewReader(body)
			if err != nil {
				_ = body.Close()
				return nil, fmt.Errorf("failed to create gzip reader: %w", err)
			}
			return &compositeReadCloser{
				Reader: reader,
				closers: []func() error{
					reader.Close,
					body.Close,
				},
			}, nil
		case "deflate":
			reader := flate.NewReader(body)
			return &compositeReadCloser{
				Reader: reader,
				closers: []func() error{
					reader.Close,
					body.Close,
				},
			}, nil
		}
	}
	return body, nil
}

func (r geminiChatResponse) asChatCompletion(model string) (providers.ChatResponse, error) {
	responseText := ""
	toolCalls := make([]map[string]interface{}, 0)
	if len(r.Candidates) == 0 {
		return providers.ChatResponse{}, fmt.Errorf("gemini returned empty completion (no candidates)")
	}
	for _, part := range r.Candidates[0].Content.Parts {
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
	// Reject empty responses — return error so router tries next provider.
	// Empty content WITH tool_calls is valid (model delegating to tools).
	if strings.TrimSpace(responseText) == "" && len(toolCalls) == 0 {
		return providers.ChatResponse{}, fmt.Errorf("gemini returned empty completion (no text or tool calls)")
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
			PromptTokens:     r.UsageMetadata.PromptTokenCount,
			CompletionTokens: r.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      r.UsageMetadata.TotalTokenCount,
		},
	}, nil
}

func cachedProviderModels(providerName string, loader func() []ModelInfo) []ModelInfo {
	now := time.Now()

	providerModelCache.mu.RLock()
	entry, ok := providerModelCache.data[providerName]
	providerModelCache.mu.RUnlock()
	if ok && now.Before(entry.expires) && len(entry.models) > 0 {
		return cloneModelInfos(entry.models)
	}

	models := loader()
	if len(models) == 0 {
		return nil
	}

	providerModelCache.mu.Lock()
	if providerModelCache.data == nil {
		providerModelCache.data = make(map[string]providerModelCacheEntry)
	}
	providerModelCache.data[providerName] = providerModelCacheEntry{
		expires: now.Add(providerModelCacheTTL),
		models:  cloneModelInfos(models),
	}
	providerModelCache.mu.Unlock()
	return cloneModelInfos(models)
}

func buildModelInfos(ids []string, ownedBy string, contextWindow int, description, providerName, category string) []ModelInfo {
	seen := make(map[string]struct{}, len(ids))
	models := make([]ModelInfo, 0, len(ids))
	for _, id := range ids {
		modelID := strings.TrimSpace(id)
		if modelID == "" {
			continue
		}
		if _, ok := seen[modelID]; ok {
			continue
		}
		seen[modelID] = struct{}{}
		models = append(models, ModelInfo{
			ID:          modelID,
			Object:      "model",
			OwnedBy:     ownedBy,
			Context:     contextWindow,
			Description: description,
			Provider:    providerName,
			Category:    category,
		})
	}
	return models
}

func parseModelIDs(body []byte, keep func(string) bool) []string {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	ids := make([]string, 0)
	appendID := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if keep != nil && !keep(raw) {
			return
		}
		ids = append(ids, raw)
	}
	collectArray := func(items []interface{}) {
		for _, item := range items {
			switch typed := item.(type) {
			case string:
				appendID(typed)
			case map[string]interface{}:
				appendID(firstNonEmptyString(
					stringValue(typed["id"]),
					stringValue(typed["name"]),
					stringValue(typed["model"]),
					stringValue(typed["slug"]),
				))
			}
		}
	}

	if data, ok := payload["data"].([]interface{}); ok {
		collectArray(data)
	}
	if models, ok := payload["models"].([]interface{}); ok {
		collectArray(models)
	}
	if modelsMap, ok := payload["models"].(map[string]interface{}); ok {
		keys := make([]string, 0, len(modelsMap))
		for key := range modelsMap {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			appendID(key)
		}
	}
	if categories, ok := payload["categories"].([]interface{}); ok {
		for _, category := range categories {
			categoryMap, ok := category.(map[string]interface{})
			if !ok {
				continue
			}
			if models, ok := categoryMap["models"].([]interface{}); ok {
				collectArray(models)
			}
		}
	}

	return dedupeStrings(ids)
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeUsage(usage providers.Usage) providers.Usage {
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	// If only total_tokens is provided (common in some Codex paths), use a heuristic split
	if usage.TotalTokens > 0 && usage.PromptTokens == 0 && usage.CompletionTokens == 0 {
		// More conservative heuristic: prompt tokens are usually less than completion for typical chat
		usage.PromptTokens = int(float64(usage.TotalTokens) * 0.25)
		usage.CompletionTokens = usage.TotalTokens - usage.PromptTokens
	}
	return usage
}

func preferredFallbackModel(models []string, fallback string) string {
	for _, model := range models {
		if strings.EqualFold(strings.TrimSpace(model), fallback) {
			return fallback
		}
	}
	if len(models) > 0 {
		return strings.TrimSpace(models[0])
	}
	return fallback
}

func looksLikeClaudeModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "claude")
}

func looksLikeCodexModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gpt") || strings.Contains(model, "codex") || strings.HasPrefix(model, "o1") || strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "gpt-5")
}

func looksLikeGeminiModel(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "gemini")
}

func buildGeminiCLIRequest(req providers.ChatRequest, model, projectID, sessionID string) ([]byte, error) {
	envelope := map[string]interface{}{
		"model": model,
		"request": map[string]interface{}{
			"contents": []map[string]interface{}{},
		},
		"user_prompt_id": fmt.Sprintf("synroute-%d", time.Now().UnixNano()),
	}
	if projectID = strings.TrimSpace(projectID); projectID != "" {
		envelope["project"] = projectID
	}
	requestNode := envelope["request"].(map[string]interface{})
	if sessionID = strings.TrimSpace(sessionID); sessionID != "" {
		requestNode["session_id"] = sessionID
	}

	generationConfig := map[string]interface{}{}
	maxTokens := req.MaxTokens
	if maxTokens > 0 {
		// Gemini 2.5 models use thinking tokens from the output budget.
		// Ensure enough headroom so thinking doesn't consume all output tokens.
		if maxTokens < 1024 {
			maxTokens = 1024
		}
		generationConfig["maxOutputTokens"] = maxTokens
	}
	if req.Temperature > 0 {
		generationConfig["temperature"] = req.Temperature
	}
	if len(req.Thinking) > 0 {
		generationConfig["thinkingConfig"] = req.Thinking
	} else if effort := strings.ToLower(strings.TrimSpace(req.ReasoningEffort)); effort != "" {
		thinking := map[string]interface{}{}
		if effort == "auto" {
			thinking["thinkingBudget"] = -1
			thinking["includeThoughts"] = true
		} else {
			thinking["thinkingLevel"] = effort
			thinking["includeThoughts"] = effort != "none"
		}
		generationConfig["thinkingConfig"] = thinking
	}
	if len(generationConfig) > 0 {
		requestNode["generationConfig"] = generationConfig
	}

	systemParts := make([]map[string]interface{}, 0)
	contents := make([]map[string]interface{}, 0, len(req.Messages))
	toolNamesByID := make(map[string]string)
	for _, msg := range req.Messages {
		if strings.EqualFold(strings.TrimSpace(msg.Role), "assistant") {
			for _, toolCall := range msg.ToolCalls {
				function, _ := toolCall["function"].(map[string]interface{})
				callID := firstNonEmptyString(stringValue(toolCall["id"]), stringValue(toolCall["call_id"]))
				if callID != "" {
					toolNamesByID[callID] = stringValue(function["name"])
				}
			}
		}
	}

	for _, msg := range req.Messages {
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		switch role {
		case "system", "developer":
			if text := strings.TrimSpace(msg.Content); text != "" {
				systemParts = append(systemParts, map[string]interface{}{"text": text})
			}
		case "assistant":
			node := map[string]interface{}{
				"role":  "model",
				"parts": buildGeminiAssistantParts(msg),
			}
			if len(node["parts"].([]map[string]interface{})) > 0 {
				contents = append(contents, node)
			}
		case "tool":
			functionName := firstNonEmptyString(strings.TrimSpace(msg.Name), toolNamesByID[msg.ToolCallID], msg.ToolCallID)
			response := map[string]interface{}{"result": decodeStructuredText(msg.Content)}
			node := map[string]interface{}{
				"role": "user",
				"parts": []map[string]interface{}{{
					"functionResponse": map[string]interface{}{
						"name":     functionName,
						"response": response,
					},
				}},
			}
			contents = append(contents, node)
		default:
			node := map[string]interface{}{
				"role":  "user",
				"parts": buildGeminiUserParts(msg),
			}
			if len(node["parts"].([]map[string]interface{})) > 0 {
				contents = append(contents, node)
			}
		}
	}

	if len(systemParts) > 0 {
		requestNode["systemInstruction"] = map[string]interface{}{
			"role":  "user",
			"parts": systemParts,
		}
	}
	requestNode["contents"] = contents

	if tools := buildGeminiCLITools(req); len(tools) > 0 {
		requestNode["tools"] = tools
	}

	return json.Marshal(envelope)
}

func buildGeminiUserParts(msg providers.Message) []map[string]interface{} {
	parts := make([]map[string]interface{}, 0, 1)
	if text := strings.TrimSpace(msg.Content); text != "" {
		parts = append(parts, map[string]interface{}{"text": text})
	}
	return parts
}

func buildGeminiAssistantParts(msg providers.Message) []map[string]interface{} {
	parts := make([]map[string]interface{}, 0, 1+len(msg.ToolCalls))
	if text := strings.TrimSpace(msg.Content); text != "" {
		parts = append(parts, map[string]interface{}{"text": text})
	}
	for _, toolCall := range msg.ToolCalls {
		function, _ := toolCall["function"].(map[string]interface{})
		parts = append(parts, map[string]interface{}{
			"functionCall": map[string]interface{}{
				"name": stringValue(function["name"]),
				"args": parseArgumentsMap(function["arguments"]),
			},
		})
	}
	return parts
}

func buildGeminiCLITools(req providers.ChatRequest) []map[string]interface{} {
	rawTools := req.Tools
	if len(rawTools) == 0 && len(req.Functions) > 0 {
		rawTools = make([]map[string]interface{}, 0, len(req.Functions))
		for _, fn := range req.Functions {
			rawTools = append(rawTools, map[string]interface{}{
				"type":     "function",
				"function": fn,
			})
		}
	}
	if len(rawTools) == 0 {
		return nil
	}

	declarations := make([]map[string]interface{}, 0, len(rawTools))
	for _, tool := range rawTools {
		if strings.TrimSpace(stringValue(tool["type"])) != "function" {
			continue
		}
		function, _ := tool["function"].(map[string]interface{})
		if len(function) == 0 {
			continue
		}
		decl := map[string]interface{}{
			"name":        stringValue(function["name"]),
			"description": stringValue(function["description"]),
		}
		if parameters, ok := function["parameters"]; ok && parameters != nil {
			decl["parametersJsonSchema"] = parameters
		} else {
			decl["parametersJsonSchema"] = map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			}
		}
		declarations = append(declarations, decl)
	}
	if len(declarations) == 0 {
		return nil
	}
	return []map[string]interface{}{{
		"functionDeclarations": declarations,
	}}
}

func decodeStructuredText(raw string) interface{} {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var decoded interface{}
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return decoded
	}
	return raw
}

func normalizeCodexModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	switch model {
	case "gpt-5-3":
		return "gpt-5.3-codex"
	case "gpt-5-3-instant":
		return "gpt-5.3-codex-spark"
	case "gpt-5-4-thinking":
		return "gpt-5.4"
	case "gpt-5-2":
		return "gpt-5.2-codex"
	case "gpt-5-2-thinking":
		return "gpt-5.2"
	case "gpt-5-1-thinking":
		return "gpt-5.1-codex-max"
	case "gpt-5-mini":
		return "gpt-5.1-codex-mini"
	default:
		return model
	}
}

func codexCatalogAliases(models []string) []string {
	aliased := make([]string, 0, len(models))
	for _, model := range models {
		aliased = append(aliased, normalizeCodexModel(model))
	}
	return dedupeStrings(aliased)
}

func mapStainlessOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "MacOS"
	case "linux":
		return "Linux"
	case "windows":
		return "Windows"
	default:
		return runtime.GOOS
	}
}

func mapStainlessArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return runtime.GOARCH
	}
}
