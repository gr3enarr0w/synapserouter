package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

// --- Translation unit tests ---

func TestConvertAnthropicToInternal_StringContent(t *testing.T) {
	req := anthropicMessagesRequest{
		Model:     "claude-sonnet-4-6",
		MaxTokens: 1024,
		Messages: []anthropicInputMessage{
			{Role: "user", Content: json.RawMessage(`"Hello, world!"`)},
		},
	}
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "user" || chatReq.Messages[0].Content != "Hello, world!" {
		t.Errorf("unexpected message: %+v", chatReq.Messages[0])
	}
	if chatReq.Model != "claude-sonnet-4-6" {
		t.Errorf("expected model claude-sonnet-4-6, got %s", chatReq.Model)
	}
	if chatReq.MaxTokens != 1024 {
		t.Errorf("expected max_tokens 1024, got %d", chatReq.MaxTokens)
	}
}

func TestConvertAnthropicToInternal_ArrayContent(t *testing.T) {
	req := anthropicMessagesRequest{
		Model:     "auto",
		MaxTokens: 100,
		Messages: []anthropicInputMessage{
			{Role: "user", Content: json.RawMessage(`[{"type":"text","text":"Hello"},{"type":"text","text":"World"}]`)},
		},
	}
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Content != "Hello\nWorld" {
		t.Errorf("expected joined text, got %q", chatReq.Messages[0].Content)
	}
}

func TestConvertAnthropicToInternal_SystemString(t *testing.T) {
	req := anthropicMessagesRequest{
		Model:     "auto",
		MaxTokens: 100,
		System:    json.RawMessage(`"You are a helpful assistant."`),
		Messages: []anthropicInputMessage{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
	}
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "system" || chatReq.Messages[0].Content != "You are a helpful assistant." {
		t.Errorf("unexpected system message: %+v", chatReq.Messages[0])
	}
}

func TestConvertAnthropicToInternal_SystemArray(t *testing.T) {
	req := anthropicMessagesRequest{
		Model:     "auto",
		MaxTokens: 100,
		System:    json.RawMessage(`[{"type":"text","text":"System part 1"},{"type":"text","text":"System part 2"}]`),
		Messages: []anthropicInputMessage{
			{Role: "user", Content: json.RawMessage(`"Hi"`)},
		},
	}
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chatReq.Messages[0].Content != "System part 1\nSystem part 2" {
		t.Errorf("unexpected system content: %q", chatReq.Messages[0].Content)
	}
}

func TestConvertAnthropicToInternal_ToolUse(t *testing.T) {
	content := `[{"type":"tool_use","id":"toolu_123","name":"get_weather","input":{"city":"London"}}]`
	req := anthropicMessagesRequest{
		Model:     "auto",
		MaxTokens: 100,
		Messages: []anthropicInputMessage{
			{Role: "user", Content: json.RawMessage(`"What's the weather?"`)},
			{Role: "assistant", Content: json.RawMessage(content)},
		},
	}
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// user + assistant with tool_calls
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chatReq.Messages))
	}
	assistantMsg := chatReq.Messages[1]
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(assistantMsg.ToolCalls))
	}
	fn, _ := assistantMsg.ToolCalls[0]["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Errorf("expected tool name get_weather, got %v", fn["name"])
	}
}

func TestConvertAnthropicToInternal_ToolResult(t *testing.T) {
	content := `[{"type":"tool_result","tool_use_id":"toolu_123","content":"72F and sunny"}]`
	req := anthropicMessagesRequest{
		Model:     "auto",
		MaxTokens: 100,
		Messages: []anthropicInputMessage{
			{Role: "user", Content: json.RawMessage(content)},
		},
	}
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "tool" {
		t.Errorf("expected role tool, got %s", chatReq.Messages[0].Role)
	}
	if chatReq.Messages[0].ToolCallID != "toolu_123" {
		t.Errorf("expected tool_call_id toolu_123, got %s", chatReq.Messages[0].ToolCallID)
	}
	if chatReq.Messages[0].Content != "72F and sunny" {
		t.Errorf("expected content '72F and sunny', got %q", chatReq.Messages[0].Content)
	}
}

func TestConvertAnthropicToInternal_Tools(t *testing.T) {
	req := anthropicMessagesRequest{
		Model:     "auto",
		MaxTokens: 100,
		Messages: []anthropicInputMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
		Tools: []map[string]interface{}{
			{
				"name":        "get_weather",
				"description": "Get weather for a city",
				"input_schema": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"city": map[string]interface{}{"type": "string"},
					},
				},
			},
		},
	}
	chatReq, err := convertAnthropicToInternal(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chatReq.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(chatReq.Tools))
	}
	tool := chatReq.Tools[0]
	if tool["type"] != "function" {
		t.Errorf("expected type function, got %v", tool["type"])
	}
	fn, _ := tool["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Errorf("expected tool name get_weather, got %v", fn["name"])
	}
	if fn["parameters"] == nil {
		t.Error("expected parameters to be set from input_schema")
	}
}

func TestConvertInternalToAnthropic_TextResponse(t *testing.T) {
	resp := providers.ChatResponse{
		ID:    "chatcmpl-123",
		Model: "gpt-4",
		Choices: []providers.Choice{
			{
				Message:      providers.Message{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}
	out := convertInternalToAnthropic(resp)
	if out.Type != "message" {
		t.Errorf("expected type message, got %s", out.Type)
	}
	if out.Role != "assistant" {
		t.Errorf("expected role assistant, got %s", out.Role)
	}
	if len(out.Content) != 1 || out.Content[0].Type != "text" || out.Content[0].Text != "Hello!" {
		t.Errorf("unexpected content: %+v", out.Content)
	}
	if *out.StopReason != "end_turn" {
		t.Errorf("expected stop_reason end_turn, got %s", *out.StopReason)
	}
	if out.Usage.InputTokens != 10 || out.Usage.OutputTokens != 5 {
		t.Errorf("unexpected usage: %+v", out.Usage)
	}
}

func TestConvertInternalToAnthropic_ToolCallResponse(t *testing.T) {
	resp := providers.ChatResponse{
		ID:    "chatcmpl-456",
		Model: "gpt-4",
		Choices: []providers.Choice{
			{
				Message: providers.Message{
					Role: "assistant",
					ToolCalls: []map[string]interface{}{
						{
							"id":   "call_123",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"city":"London"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	out := convertInternalToAnthropic(resp)
	if *out.StopReason != "tool_use" {
		t.Errorf("expected stop_reason tool_use, got %s", *out.StopReason)
	}
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(out.Content))
	}
	if out.Content[0].Type != "tool_use" || out.Content[0].Name != "get_weather" {
		t.Errorf("unexpected tool_use block: %+v", out.Content[0])
	}
}

func TestConvertInternalToAnthropic_MalformedToolCalls(t *testing.T) {
	resp := providers.ChatResponse{
		Choices: []providers.Choice{
			{
				Message: providers.Message{
					Role: "assistant",
					ToolCalls: []map[string]interface{}{
						{"id": "call_1", "function": "not_a_map"},          // wrong type
						{"id": "call_2"},                                    // missing function key
						nil,                                                 // nil entry
						{"id": "call_3", "type": "function", "function": map[string]interface{}{"name": "valid", "arguments": "{}"}},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	// Should not panic — malformed entries skipped, valid one kept
	out := convertInternalToAnthropic(resp)
	if len(out.Content) != 1 {
		t.Fatalf("expected 1 valid content block (skipping malformed), got %d", len(out.Content))
	}
	if out.Content[0].Name != "valid" {
		t.Errorf("expected tool name 'valid', got %s", out.Content[0].Name)
	}
}

func TestConvertInternalToAnthropic_EmptyContent(t *testing.T) {
	resp := providers.ChatResponse{
		Choices: []providers.Choice{{FinishReason: "stop"}},
	}
	out := convertInternalToAnthropic(resp)
	if out.Content == nil {
		t.Error("content should not be nil")
	}
}

func TestMapFinishReasonRoundTrip(t *testing.T) {
	tests := []struct{ anthropic, openai string }{
		{"end_turn", "stop"},
		{"tool_use", "tool_calls"},
		{"max_tokens", "length"},
	}
	for _, tt := range tests {
		got := mapFinishReasonToAnthropic(tt.openai)
		if got != tt.anthropic {
			t.Errorf("toAnthropic(%s) = %s, want %s", tt.openai, got, tt.anthropic)
		}
		back := mapFinishReasonFromAnthropic(tt.anthropic)
		if back != tt.openai {
			t.Errorf("fromAnthropic(%s) = %s, want %s", tt.anthropic, back, tt.openai)
		}
	}
}

// --- Handler tests ---

func TestMessagesHandler_ErrorFormat(t *testing.T) {
	body := `{"model":"auto","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Missing max_tokens — should return Anthropic error format
	handleAnthropicMessages(w, req, "")

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}

	var errResp anthropicError
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Type != "error" {
		t.Errorf("expected type error, got %s", errResp.Type)
	}
	if errResp.Error.Type != "invalid_request_error" {
		t.Errorf("expected error type invalid_request_error, got %s", errResp.Error.Type)
	}
}

func TestMessagesHandler_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/v1/messages", bytes.NewBufferString(`{invalid`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handleAnthropicMessages(w, req, "")

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", resp.StatusCode)
	}
}

func TestWriteAnthropicStream_EventSequence(t *testing.T) {
	resp := providers.ChatResponse{
		ID:    "msg_test_123",
		Model: "claude-sonnet-4-6",
		Choices: []providers.Choice{
			{
				Message:      providers.Message{Role: "assistant", Content: "Hello!"},
				FinishReason: "stop",
			},
		},
		Usage: providers.Usage{
			PromptTokens:     5,
			CompletionTokens: 2,
		},
	}

	w := httptest.NewRecorder()
	writeAnthropicStream(w, resp)

	body := w.Body.String()

	// Verify SSE event sequence
	expectedEvents := []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	}
	for _, event := range expectedEvents {
		if !containsString(body, event) {
			t.Errorf("missing expected event: %s", event)
		}
	}

	// Verify content
	if !containsString(body, `"text_delta"`) {
		t.Error("missing text_delta in stream")
	}
	if !containsString(body, `"Hello!"`) {
		t.Error("missing content text in stream")
	}
	if !containsString(body, `"end_turn"`) {
		t.Error("missing end_turn stop reason in stream")
	}
}

func TestWriteAnthropicStream_ToolUse(t *testing.T) {
	resp := providers.ChatResponse{
		ID:    "msg_test_456",
		Model: "auto",
		Choices: []providers.Choice{
			{
				Message: providers.Message{
					Role:    "assistant",
					Content: "Let me check the weather.",
					ToolCalls: []map[string]interface{}{
						{
							"id":   "call_abc",
							"type": "function",
							"function": map[string]interface{}{
								"name":      "get_weather",
								"arguments": `{"city":"London"}`,
							},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	w := httptest.NewRecorder()
	writeAnthropicStream(w, resp)

	body := w.Body.String()
	if !containsString(body, `"tool_use"`) {
		t.Error("missing tool_use in stream")
	}
	if !containsString(body, `"input_json_delta"`) {
		t.Error("missing input_json_delta in stream")
	}
	if !containsString(body, `"get_weather"`) {
		t.Error("missing tool name in stream")
	}
}

func containsString(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
