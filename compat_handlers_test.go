package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/compat"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

type compatRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn compatRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestAmpUpstreamURLHandlerUpdatesConfig(t *testing.T) {
	ampConfig = compat.AmpCodeConfig{}

	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-url", strings.NewReader(`{"value":"https://example.com"}`))
	rr := httptest.NewRecorder()

	ampUpstreamURLHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if ampConfig.UpstreamURL != "https://example.com" {
		t.Fatalf("unexpected upstream url: %s", ampConfig.UpstreamURL)
	}
}

func TestProviderModelsHandlerFiltersProvider(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/provider/gemini/v1/models", nil)
	req = muxSetVars(req, map[string]string{"provider": "gemini"})
	rr := httptest.NewRecorder()

	providerModelsHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}

	var payload struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Data) != 1 {
		t.Fatalf("expected 1 model, got %d", len(payload.Data))
	}
	if payload.Data[0]["owned_by"] != "gemini" {
		t.Fatalf("unexpected provider: %v", payload.Data[0]["owned_by"])
	}
}

func TestWriteResponsesStreamEmitsSSE(t *testing.T) {
	rr := httptest.NewRecorder()

	writeResponsesStream(rr, providers.ChatResponse{
		ID:     "resp-test",
		Object: "chat.completion",
		Model:  "nanogpt",
		Choices: []providers.Choice{
			{Index: 0, Message: providers.Message{Role: "assistant", Content: "streamed text"}, FinishReason: "stop"},
		},
	}, "streamed text", responsesRequest{})

	body := rr.Body.String()
	if !strings.Contains(body, "response.created") || !strings.Contains(body, "response.completed") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("unexpected stream body: %s", body)
	}
}

func TestWriteResponsesStreamIncludesToolCalls(t *testing.T) {
	rr := httptest.NewRecorder()

	writeResponsesStream(rr, providers.ChatResponse{
		ID:     "resp-tools",
		Object: "chat.completion",
		Model:  "gpt-5-codex",
		Choices: []providers.Choice{
			{
				Index: 0,
				Message: providers.Message{
					Role:      "assistant",
					Content:   "",
					ToolCalls: []map[string]interface{}{{"id": "call-1", "type": "function", "function": map[string]interface{}{"name": "run_tests"}}},
				},
				FinishReason: "tool_calls",
			},
		},
	}, "", responsesRequest{})

	body := rr.Body.String()
	if !strings.Contains(body, "response.output_item.added") ||
		!strings.Contains(body, "response.function_call_arguments.delta") ||
		!strings.Contains(body, "response.function_call_arguments.done") ||
		!strings.Contains(body, "response.output_item.done") ||
		!strings.Contains(body, "run_tests") {
		t.Fatalf("expected tool call events in stream, got: %s", body)
	}
}

func TestAmpUpstreamAPIKeysHandlerSanitizesEntries(t *testing.T) {
	ampConfig = compat.AmpCodeConfig{}

	req := httptest.NewRequest(http.MethodPut, "/v0/management/ampcode/upstream-api-keys", strings.NewReader(`{"value":[{"upstream-api-key":"  u1  ","api-keys":["  k1  ","","k2"]}]}`))
	rr := httptest.NewRecorder()

	ampUpstreamAPIKeysHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if len(ampConfig.UpstreamAPIKeys) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(ampConfig.UpstreamAPIKeys))
	}
	entry := ampConfig.UpstreamAPIKeys[0]
	if entry.UpstreamAPIKey != "u1" {
		t.Fatalf("expected sanitized upstream key, got %q", entry.UpstreamAPIKey)
	}
	if len(entry.APIKeys) != 2 || entry.APIKeys[0] != "k1" || entry.APIKeys[1] != "k2" {
		t.Fatalf("unexpected api key list: %#v", entry.APIKeys)
	}
	if entry.Name == "" {
		t.Fatal("expected generated entry name")
	}
}

func TestRequestSessionIDUsesQueryFallbacks(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v1/chat/completions?session_id=session-q", nil)
	if got := requestSessionID(req); got != "session-q" {
		t.Fatalf("expected session-q, got %s", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/v1/chat/completions?resume=session-r", nil)
	if got := requestSessionID(req); got != "session-r" {
		t.Fatalf("expected session-r, got %s", got)
	}
}

func TestApplyModelMappingSupportsWildcardSuffix(t *testing.T) {
	cfg := compat.AmpCodeConfig{
		ModelMappings: []compat.AmpModelMapping{
			{From: "*-thinking", To: "gemini-2.5-pro"},
		},
	}

	if got := compat.ApplyModelMapping(cfg, "claude-opus-4-5-thinking"); got != "gemini-2.5-pro" {
		t.Fatalf("expected wildcard mapping, got %s", got)
	}
}

func TestResolveAmpUpstreamAPIKeyForRequestUsesClientKeyMapping(t *testing.T) {
	ampConfig = compat.AmpCodeConfig{
		UpstreamAPIKeys: []compat.AmpUpstreamAPIKeyEntry{
			{UpstreamAPIKey: "upstream-1", APIKeys: []string{"client-1"}},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("Authorization", "Bearer client-1")
	if got := resolveAmpUpstreamAPIKeyForRequest(req); got != "upstream-1" {
		t.Fatalf("expected upstream-1, got %s", got)
	}
}

func TestResolveAmpUpstreamAPIKeyForRequestUsesNamedAccountOverride(t *testing.T) {
	ampConfig = compat.AmpCodeConfig{
		UpstreamAPIKeys: []compat.AmpUpstreamAPIKeyEntry{
			{Name: "claude-main", UpstreamAPIKey: "upstream-claude", APIKeys: []string{"client-1"}},
			{Name: "gemini-lab", UpstreamAPIKey: "upstream-gemini", APIKeys: []string{"client-2"}},
		},
	}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions?upstream_account=gemini-lab", nil)
	req.Header.Set("Authorization", "Bearer client-1")
	if got := resolveAmpUpstreamAPIKeyForRequest(req); got != "upstream-gemini" {
		t.Fatalf("expected named account override, got %s", got)
	}

	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	req.Header.Set("X-Upstream-Account", "claude-main")
	if got := resolveAmpUpstreamAPIKeyForRequest(req); got != "upstream-claude" {
		t.Fatalf("expected header account override, got %s", got)
	}
}

func TestResolveModelHonorsForceFlag(t *testing.T) {
	cfg := compat.AmpCodeConfig{
		ModelMappings: []compat.AmpModelMapping{
			{From: "gpt-5", To: "gemini-2.5-pro"},
		},
	}

	if got := compat.ResolveModel(cfg, "gpt-5", []string{"gpt-5", "gemini-2.5-pro"}); got != "gpt-5" {
		t.Fatalf("expected original available model, got %s", got)
	}

	cfg.ForceModelMappings = true
	if got := compat.ResolveModel(cfg, "gpt-5", []string{"gpt-5", "gemini-2.5-pro"}); got != "gemini-2.5-pro" {
		t.Fatalf("expected forced mapped model, got %s", got)
	}
}

func TestConvertResponsesRequestPreservesNameAndToolCalls(t *testing.T) {
	req := responsesRequest{
		Model: "gpt-5",
		Input: json.RawMessage(`[{"role":"assistant","name":"planner","content":[{"text":"working"}],"tool_calls":[{"id":"call-1","type":"function"}]}]`),
	}

	chatReq, err := convertResponsesRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(chatReq.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Name != "planner" {
		t.Fatalf("expected planner name, got %s", chatReq.Messages[0].Name)
	}
	if len(chatReq.Messages[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(chatReq.Messages[0].ToolCalls))
	}
}

func TestConvertResponsesRequestSupportsFunctionCallAndOutputItems(t *testing.T) {
	req := responsesRequest{
		Model: "gpt-5",
		Input: json.RawMessage(`[
			{"type":"function_call","id":"fc-1","call_id":"call-1","name":"run_tests","arguments":{"package":"./..."}},
			{"type":"function_call_output","call_id":"call-1","output":"tests passed"}
		]`),
	}

	chatReq, err := convertResponsesRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Role != "assistant" {
		t.Fatalf("expected assistant role, got %s", chatReq.Messages[0].Role)
	}
	if len(chatReq.Messages[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 assistant tool call, got %d", len(chatReq.Messages[0].ToolCalls))
	}
	if chatReq.Messages[1].Role != "tool" {
		t.Fatalf("expected tool role, got %s", chatReq.Messages[1].Role)
	}
	if chatReq.Messages[1].ToolCallID != "call-1" {
		t.Fatalf("expected tool_call_id call-1, got %s", chatReq.Messages[1].ToolCallID)
	}
	if chatReq.Messages[1].Content != "tests passed" {
		t.Fatalf("expected tool output content, got %q", chatReq.Messages[1].Content)
	}
}

func TestResponsesOutputPreservesToolCalls(t *testing.T) {
	resp := providers.ChatResponse{
		ID:     "resp-1",
		Object: "chat.completion",
		Model:  "gpt-5-codex",
		Choices: []providers.Choice{
			{
				Index: 0,
				Message: providers.Message{
					Role:      "assistant",
					Name:      "planner",
					Content:   "",
					ToolCalls: []map[string]interface{}{{"id": "call-1", "type": "function"}},
				},
				FinishReason: "tool_calls",
			},
		},
	}

	output := responsesOutput(resp, "")
	if len(output) != 1 {
		t.Fatalf("expected 1 output item, got %d", len(output))
	}
	if output[0]["name"] != "planner" {
		t.Fatalf("expected planner name, got %#v", output[0]["name"])
	}
	toolCalls, ok := output[0]["tool_calls"].([]map[string]interface{})
	if !ok || len(toolCalls) != 1 {
		t.Fatalf("expected tool calls in output, got %#v", output[0]["tool_calls"])
	}
}

func TestWriteResponsesStreamCarriesResponseMetadataFields(t *testing.T) {
	rr := httptest.NewRecorder()

	writeResponsesStream(rr, providers.ChatResponse{
		ID:     "resp-meta",
		Object: "chat.completion",
		Model:  "gpt-5-codex",
		Choices: []providers.Choice{
			{Index: 0, Message: providers.Message{Role: "assistant", Content: "done"}, FinishReason: "stop"},
		},
	}, "done", responsesRequest{
		MaxToolCalls:       3,
		ParallelToolCalls:  true,
		PreviousResponseID: "resp-prev",
	})

	body := rr.Body.String()
	if !strings.Contains(body, `"max_tool_calls":3`) || !strings.Contains(body, `"parallel_tool_calls":true`) || !strings.Contains(body, `"previous_response_id":"resp-prev"`) {
		t.Fatalf("expected response metadata in stream, got: %s", body)
	}
}

func TestResponseGetHandlerReturnsStoredResponse(t *testing.T) {
	responsesStoreMu.Lock()
	responsesStore = map[string]map[string]interface{}{
		"resp-get": {
			"id":          "resp-get",
			"object":      "response",
			"model":       "gpt-5-codex",
			"output_text": "stored output",
		},
	}
	responsesStoreMu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/responses/resp-get", nil)
	req = muxSetVars(req, map[string]string{"response_id": "resp-get"})
	rr := httptest.NewRecorder()

	responseGetHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"output_text":"stored output"`) {
		t.Fatalf("expected stored response body, got %s", rr.Body.String())
	}
}

func TestResponseDeleteHandlerRemovesStoredResponse(t *testing.T) {
	responsesStoreMu.Lock()
	responsesStore = map[string]map[string]interface{}{
		"resp-delete": {"id": "resp-delete"},
	}
	responsesStoreMu.Unlock()

	req := httptest.NewRequest(http.MethodDelete, "/v1/responses/resp-delete", nil)
	req = muxSetVars(req, map[string]string{"response_id": "resp-delete"})
	rr := httptest.NewRecorder()

	responseDeleteHandler(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status 204, got %d", rr.Code)
	}
	if got := responseSessionID("resp-delete"); got != "" {
		t.Fatalf("expected deleted response to be absent, got session %q", got)
	}
}

func TestConvertResponsesRequestPrependsInstructions(t *testing.T) {
	req := responsesRequest{
		Model:        "gpt-5",
		Instructions: "Always respond with a plan first.",
		Input:        json.RawMessage(`"Implement this feature"`),
	}

	chatReq, err := convertResponsesRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(chatReq.Messages))
	}
	if chatReq.Messages[0].Content != "Always respond with a plan first." {
		t.Fatalf("expected instruction prefix, got %q", chatReq.Messages[0].Content)
	}
}

func TestResponseSessionIDReturnsStoredSession(t *testing.T) {
	responsesStoreMu.Lock()
	responsesStore = map[string]map[string]interface{}{
		"resp-prev": {
			"id":         "resp-prev",
			"session_id": "session-prev",
		},
	}
	responsesStoreMu.Unlock()

	if got := responseSessionID("resp-prev"); got != "session-prev" {
		t.Fatalf("expected session-prev, got %q", got)
	}
}

func TestAmpUpstreamAPIKeyHandlerFallsBackToAmpSecretsFile(t *testing.T) {
	oldHome := os.Getenv("HOME")
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("AMP_API_KEY", "")
	t.Setenv("AMP_UPSTREAM_API_KEY", "")
	if oldHome != "" {
		defer os.Setenv("HOME", oldHome)
	}

	secretsDir := filepath.Join(tempHome, ".local", "share", "amp")
	if err := os.MkdirAll(secretsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secretsDir, "secrets.json"), []byte(`{"apiKey@https://ampcode.com/":"file-secret"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ampConfig = compat.AmpCodeConfig{}

	req := httptest.NewRequest(http.MethodGet, "/v0/management/ampcode/upstream-api-key", nil)
	rr := httptest.NewRecorder()
	ampUpstreamAPIKeyHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"upstream-api-key":"file-secret"`) {
		t.Fatalf("expected file secret fallback, got %s", rr.Body.String())
	}
}

func TestResponsesCompactHandlerRejectsStreaming(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/responses/compact", strings.NewReader(`{"model":"gpt-5-codex","stream":true}`))
	rr := httptest.NewRecorder()

	responsesCompactHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rr.Code)
	}
}

func TestForwardToAmpUpstreamAddsFallbackAuthHeaders(t *testing.T) {
	var gotAuth string
	var gotAPIKey string
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: compatRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			gotAuth = r.Header.Get("Authorization")
			gotAPIKey = r.Header.Get("X-Api-Key")
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		}),
	}
	defer func() { http.DefaultClient = oldClient }()

	ampConfig = compat.AmpCodeConfig{UpstreamURL: "https://amp.example", UpstreamAPIKey: "amp-secret"}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"unknown-model"}`))
	rr := httptest.NewRecorder()

	if !forwardToAmpUpstream(rr, req, []byte(`{"model":"unknown-model"}`)) {
		t.Fatal("expected amp fallback to handle request")
	}
	if gotAuth != "Bearer amp-secret" {
		t.Fatalf("expected bearer auth fallback, got %q", gotAuth)
	}
	if gotAPIKey != "amp-secret" {
		t.Fatalf("expected x-api-key fallback, got %q", gotAPIKey)
	}
}

func TestProviderChatHandlerFallsBackToAmpUpstreamForUnknownModel(t *testing.T) {
	oldClient := http.DefaultClient
	http.DefaultClient = &http.Client{
		Transport: compatRoundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/provider/openai/v1/chat/completions" {
				t.Fatalf("unexpected upstream path: %s", r.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"amp-chat","object":"chat.completion","model":"unknown-model","choices":[{"index":0,"message":{"role":"assistant","content":"from amp"},"finish_reason":"stop"}]}`)),
			}, nil
		}),
	}
	defer func() { http.DefaultClient = oldClient }()

	ampConfig = compat.AmpCodeConfig{UpstreamURL: "https://amp.example", UpstreamAPIKey: "amp-secret"}
	req := httptest.NewRequest(http.MethodPost, "/api/provider/openai/v1/chat/completions", strings.NewReader(`{"model":"unknown-model","messages":[{"role":"user","content":"hello"}]}`))
	req = muxSetVars(req, map[string]string{"provider": "openai"})
	rr := httptest.NewRecorder()

	providerChatHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"from amp"`) {
		t.Fatalf("expected amp fallback response, got %s", rr.Body.String())
	}
}
