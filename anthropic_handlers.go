package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"


	"github.com/gr3enarr0w/synapserouter/internal/compat"
	"github.com/gr3enarr0w/synapserouter/internal/providers"
)

// --- Anthropic Messages API types ---

type anthropicMessagesRequest struct {
	Model         string                   `json:"model"`
	MaxTokens     int                      `json:"max_tokens"`
	Messages      []anthropicInputMessage  `json:"messages"`
	System        json.RawMessage          `json:"system,omitempty"`
	Temperature   *float64                 `json:"temperature,omitempty"`
	Stream        bool                     `json:"stream,omitempty"`
	Tools         []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice    interface{}              `json:"tool_choice,omitempty"`
	Thinking      map[string]interface{}   `json:"thinking,omitempty"`
	StopSequences []string                 `json:"stop_sequences,omitempty"`
	Metadata      map[string]interface{}   `json:"metadata,omitempty"`
	TopP          *float64                 `json:"top_p,omitempty"`
	TopK          *int                     `json:"top_k,omitempty"`
}

type anthropicInputMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type anthropicContentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   json.RawMessage `json:"content,omitempty"`
}

type anthropicMessagesResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []anthropicOutputBlock  `json:"content"`
	Model        string                  `json:"model"`
	StopReason   *string                 `json:"stop_reason"`
	StopSequence *string                 `json:"stop_sequence"`
	Usage        anthropicUsage          `json:"usage"`
}

type anthropicOutputBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text,omitempty"`
	ID    string          `json:"id,omitempty"`
	Name  string          `json:"name,omitempty"`
	Input json.RawMessage `json:"input,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicError struct {
	Type  string              `json:"type"`
	Error anthropicErrorDetail `json:"error"`
}

type anthropicErrorDetail struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// --- Translation functions ---

func convertAnthropicToInternal(req anthropicMessagesRequest) (providers.ChatRequest, error) {
	chatReq := providers.ChatRequest{
		Model:      req.Model,
		MaxTokens:  req.MaxTokens,
		ToolChoice: req.ToolChoice,
		Thinking:   req.Thinking,
	}
	if req.Temperature != nil {
		chatReq.Temperature = *req.Temperature
	}

	// Parse system message
	if len(req.System) > 0 {
		systemText, err := parseAnthropicSystem(req.System)
		if err != nil {
			return chatReq, fmt.Errorf("invalid system field: %w", err)
		}
		if systemText != "" {
			chatReq.Messages = append(chatReq.Messages, providers.Message{
				Role:    "system",
				Content: systemText,
			})
		}
	}

	// Convert messages
	for _, msg := range req.Messages {
		converted, err := convertAnthropicMessage(msg)
		if err != nil {
			return chatReq, fmt.Errorf("invalid message: %w", err)
		}
		chatReq.Messages = append(chatReq.Messages, converted...)
	}

	// Convert tools: Anthropic input_schema → OpenAI parameters
	if len(req.Tools) > 0 {
		chatReq.Tools = convertAnthropicToolsToOpenAI(req.Tools)
	}

	return chatReq, nil
}

func parseAnthropicSystem(raw json.RawMessage) (string, error) {
	// Try string first
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	// Try array of content blocks
	var blocks []anthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return "", fmt.Errorf("system must be string or array of content blocks")
	}
	var parts []string
	for _, b := range blocks {
		if b.Type == "text" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n"), nil
}

func convertAnthropicMessage(msg anthropicInputMessage) ([]providers.Message, error) {
	// Try string content first
	var strContent string
	if err := json.Unmarshal(msg.Content, &strContent); err == nil {
		return []providers.Message{{Role: msg.Role, Content: strContent}}, nil
	}

	// Parse as array of content blocks
	var blocks []anthropicContentBlock
	if err := json.Unmarshal(msg.Content, &blocks); err != nil {
		return nil, fmt.Errorf("content must be string or array of content blocks")
	}

	var result []providers.Message
	var textParts []string
	var toolCalls []map[string]interface{}

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			toolCalls = append(toolCalls, map[string]interface{}{
				"id":   block.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      block.Name,
					"arguments": string(block.Input),
				},
			})
		case "tool_result":
			// tool_result content can be string or array
			resultText := extractToolResultContent(block.Content)
			result = append(result, providers.Message{
				Role:       "tool",
				Content:    resultText,
				ToolCallID: block.ToolUseID,
			})
		}
	}

	// Build assistant/user message with text + tool_calls
	if len(textParts) > 0 || len(toolCalls) > 0 {
		m := providers.Message{
			Role:    msg.Role,
			Content: strings.Join(textParts, "\n"),
		}
		if len(toolCalls) > 0 {
			m.ToolCalls = toolCalls
		}
		result = append([]providers.Message{m}, result...)
	}

	if len(result) == 0 {
		// Empty content blocks — still produce a message
		result = append(result, providers.Message{Role: msg.Role, Content: ""})
	}

	return result, nil
}

func extractToolResultContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// Try array of content blocks
	var blocks []anthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var parts []string
		for _, b := range blocks {
			if b.Type == "text" {
				parts = append(parts, b.Text)
			}
		}
		return strings.Join(parts, "\n")
	}
	return string(raw)
}

func convertAnthropicToolsToOpenAI(tools []map[string]interface{}) []map[string]interface{} {
	var result []map[string]interface{}
	for _, tool := range tools {
		name, _ := tool["name"].(string)
		desc, _ := tool["description"].(string)
		inputSchema := tool["input_schema"]

		result = append(result, map[string]interface{}{
			"type": "function",
			"function": map[string]interface{}{
				"name":        name,
				"description": desc,
				"parameters":  inputSchema,
			},
		})
	}
	return result
}

func convertInternalToAnthropic(resp providers.ChatResponse) anthropicMessagesResponse {
	out := anthropicMessagesResponse{
		ID:    resp.ID,
		Type:  "message",
		Role:  "assistant",
		Model: resp.Model,
		Usage: anthropicUsage{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
		},
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]

		// Map finish reason
		stopReason := mapFinishReasonToAnthropic(choice.FinishReason)
		out.StopReason = &stopReason

		// Build content blocks
		if choice.Message.Content != "" {
			out.Content = append(out.Content, anthropicOutputBlock{
				Type: "text",
				Text: choice.Message.Content,
			})
		}

		// Convert tool calls
		for _, tc := range choice.Message.ToolCalls {
			fn, ok := tc["function"].(map[string]interface{})
			if !ok || fn == nil {
				continue
			}
			id, _ := tc["id"].(string)
			name, _ := fn["name"].(string)
			args, _ := fn["arguments"].(string)

			out.Content = append(out.Content, anthropicOutputBlock{
				Type:  "tool_use",
				ID:    id,
				Name:  name,
				Input: json.RawMessage(args),
			})
		}
	}

	// Ensure content is never nil
	if out.Content == nil {
		out.Content = []anthropicOutputBlock{}
	}

	return out
}

func mapFinishReasonToAnthropic(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "tool_calls":
		return "tool_use"
	case "length":
		return "max_tokens"
	default:
		if reason == "" {
			return "end_turn"
		}
		return reason
	}
}

func mapFinishReasonFromAnthropic(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "tool_use":
		return "tool_calls"
	case "max_tokens":
		return "length"
	default:
		return reason
	}
}

// --- Handlers ---

func messagesHandler(w http.ResponseWriter, r *http.Request) {
	handleAnthropicMessages(w, r, "")
}

func providerMessagesHandler(w http.ResponseWriter, r *http.Request) {
	providerName := strings.TrimSpace(r.PathValue("provider"))
	handleAnthropicMessages(w, r, providerName)
}

func handleAnthropicMessages(w http.ResponseWriter, r *http.Request, preferredProvider string) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}

	var req anthropicMessagesRequest
	if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&req); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "Invalid JSON in request body")
		return
	}

	if req.MaxTokens == 0 {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "max_tokens is required")
		return
	}

	// Convert to internal format
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	// Always get a complete response — we convert to SSE ourselves if streaming requested
	chatReq.Stream = false

	// Resolve model
	if resolved := compat.ResolveModel(ampConfig, chatReq.Model, knownModelIDs()); resolved != "" {
		chatReq.Model = resolved
	}
	if chatReq.Model == "" {
		chatReq.Model = "auto"
	}

	// Amp fallback
	if shouldUseAmpFallback(chatReq.Model) {
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
	}

	sessionID := requestSessionID(r)
	resp, err := routeChatRequest(r, chatReq, sessionID, preferredProvider)
	if err != nil {
		if strings.Contains(err.Error(), "invalid request:") || strings.Contains(err.Error(), "unknown model") || strings.Contains(err.Error(), "not compatible") {
			writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
		writeAnthropicError(w, http.StatusServiceUnavailable, "api_error", err.Error())
		return
	}

	if req.Stream {
		writeAnthropicStream(w, resp)
		return
	}

	anthropicResp := convertInternalToAnthropic(resp)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(anthropicResp)
}

func writeAnthropicError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(anthropicError{
		Type: "error",
		Error: anthropicErrorDetail{
			Type:    errType,
			Message: message,
		},
	})
}

func writeAnthropicStream(w http.ResponseWriter, resp providers.ChatResponse) {
	sse := newSSEWriter(w)
	if sse == nil {
		writeAnthropicError(w, http.StatusInternalServerError, "api_error", "Streaming not supported")
		return
	}

	msgID := resp.ID
	if msgID == "" {
		msgID = "msg_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	model := resp.Model
	if model == "" {
		model = "auto"
	}

	inputTokens := resp.Usage.PromptTokens
	outputTokens := resp.Usage.CompletionTokens

	// message_start
	messageStart := map[string]interface{}{
		"type": "message_start",
		"message": map[string]interface{}{
			"id":            msgID,
			"type":          "message",
			"role":          "assistant",
			"content":       []interface{}{},
			"model":         model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]interface{}{
				"input_tokens":  inputTokens,
				"output_tokens": 0,
			},
		},
	}
	sse.WriteEvent("message_start", messageStart)

	// Emit content blocks
	blockIndex := 0
	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]

		// Text content
		if choice.Message.Content != "" {
			// content_block_start
			sse.WriteEvent("content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": blockIndex,
				"content_block": map[string]interface{}{
					"type": "text",
					"text": "",
				},
			})

			// content_block_delta
			sse.WriteEvent("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": blockIndex,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": choice.Message.Content,
				},
			})

			// content_block_stop
			sse.WriteEvent("content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": blockIndex,
			})
			blockIndex++
		}

		// Tool use blocks
		for _, tc := range choice.Message.ToolCalls {
			fn, ok := tc["function"].(map[string]interface{})
			if !ok || fn == nil {
				continue
			}
			id, _ := tc["id"].(string)
			name, _ := fn["name"].(string)
			args, _ := fn["arguments"].(string)

			sse.WriteEvent("content_block_start", map[string]interface{}{
				"type":  "content_block_start",
				"index": blockIndex,
				"content_block": map[string]interface{}{
					"type":  "tool_use",
					"id":    id,
					"name":  name,
					"input": map[string]interface{}{},
				},
			})

			sse.WriteEvent("content_block_delta", map[string]interface{}{
				"type":  "content_block_delta",
				"index": blockIndex,
				"delta": map[string]interface{}{
					"type":         "input_json_delta",
					"partial_json": args,
				},
			})

			sse.WriteEvent("content_block_stop", map[string]interface{}{
				"type":  "content_block_stop",
				"index": blockIndex,
			})
			blockIndex++
		}

		// message_delta with stop_reason
		stopReason := mapFinishReasonToAnthropic(choice.FinishReason)
		sse.WriteEvent("message_delta", map[string]interface{}{
			"type": "message_delta",
			"delta": map[string]interface{}{
				"stop_reason":   stopReason,
				"stop_sequence": nil,
			},
			"usage": map[string]interface{}{
				"output_tokens": outputTokens,
			},
		})
	}

	// message_stop
	sse.WriteEvent("message_stop", map[string]interface{}{
		"type": "message_stop",
	})
}

