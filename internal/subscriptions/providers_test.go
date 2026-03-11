package subscriptions

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestAnthropicProviderTranslatesToolUseToToolCalls(t *testing.T) {
	p := &anthropicProvider{
		baseURL: "https://anthropic.test",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(`{
					"id":"msg_1",
					"role":"assistant",
					"model":"claude-3-5-sonnet",
					"content":[
						{"type":"text","text":"I will call a tool."},
						{"type":"tool_use","id":"toolu_1","name":"run_tests","input":{"package":"./..."}}
					],
					"usage":{"input_tokens":10,"output_tokens":20}
				}`), nil
			}),
		},
	}

	resp, err := p.ChatCompletion(context.Background(), providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: "test it"}},
	}, "claude-3-5-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %s", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Choices[0].Message.ToolCalls))
	}
	if got := resp.Choices[0].Message.ToolCalls[0]["id"]; got != "toolu_1" {
		t.Fatalf("expected toolu_1 id, got %#v", got)
	}
	function, ok := resp.Choices[0].Message.ToolCalls[0]["function"].(map[string]interface{})
	if !ok || function["name"] != "run_tests" {
		t.Fatalf("unexpected function payload: %#v", resp.Choices[0].Message.ToolCalls[0]["function"])
	}
	if !strings.Contains(resp.Choices[0].Message.Content, "I will call a tool.") {
		t.Fatalf("expected text content to be preserved, got %q", resp.Choices[0].Message.Content)
	}
}

func TestGeminiProviderTranslatesFunctionCallToToolCalls(t *testing.T) {
	p := &geminiProvider{
		baseURL: "https://gemini.test",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				return jsonResponse(`{
					"candidates":[
						{"content":{"parts":[
							{"text":"Need to invoke a function."},
							{"functionCall":{"name":"lookup_docs","args":{"topic":"routing"}}}
						]}}
					],
					"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":7,"totalTokenCount":12}
				}`), nil
			}),
		},
		models: []string{"gemini-2.5-pro"},
	}

	resp, err := p.ChatCompletion(context.Background(), providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: "look it up"}},
	}, "gemini-2.5-pro")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected tool_calls finish reason, got %s", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Choices[0].Message.ToolCalls))
	}
	function, ok := resp.Choices[0].Message.ToolCalls[0]["function"].(map[string]interface{})
	if !ok || function["name"] != "lookup_docs" {
		t.Fatalf("unexpected function payload: %#v", resp.Choices[0].Message.ToolCalls[0]["function"])
	}
	if !strings.Contains(resp.Choices[0].Message.Content, "Need to invoke a function.") {
		t.Fatalf("expected text content to be preserved, got %q", resp.Choices[0].Message.Content)
	}
}

func TestAnthropicProviderTranslatesToolMessagesInRequest(t *testing.T) {
	var captured map[string]interface{}
	p := &anthropicProvider{
		baseURL: "https://anthropic.test",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Fatal(err)
				}
				return jsonResponse(`{"id":"msg_1","role":"assistant","model":"claude","content":[{"type":"text","text":"ok"}],"usage":{"input_tokens":1,"output_tokens":1}}`), nil
			}),
		},
	}

	_, err := p.ChatCompletion(context.Background(), providers.ChatRequest{
		Messages: []providers.Message{
			{Role: "assistant", Content: "Use a tool", ToolCalls: []map[string]interface{}{{"id": "call-1", "function": map[string]interface{}{"name": "lookup", "arguments": `{"topic":"routing"}`}}}},
			{Role: "tool", ToolCallID: "call-1", Content: "routing data"},
		},
	}, "claude-3-5-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	messages, ok := captured["messages"].([]interface{})
	if !ok || len(messages) != 2 {
		t.Fatalf("expected 2 anthropic messages, got %#v", captured["messages"])
	}
	firstContent := messages[0].(map[string]interface{})["content"].([]interface{})
	secondContent := messages[1].(map[string]interface{})["content"].([]interface{})
	if len(firstContent) < 2 || firstContent[1].(map[string]interface{})["type"] != "tool_use" {
		t.Fatalf("expected assistant tool_use content, got %#v", firstContent)
	}
	if secondContent[0].(map[string]interface{})["type"] != "tool_result" {
		t.Fatalf("expected tool_result content, got %#v", secondContent[0])
	}
}

func TestGeminiProviderTranslatesToolMessagesInRequest(t *testing.T) {
	var captured map[string]interface{}
	p := &geminiProvider{
		baseURL: "https://gemini.test",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
					t.Fatal(err)
				}
				return jsonResponse(`{"candidates":[{"content":{"parts":[{"text":"ok"}]}}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":1,"totalTokenCount":2}}`), nil
			}),
		},
		models: []string{"gemini-2.5-pro"},
	}

	_, err := p.ChatCompletion(context.Background(), providers.ChatRequest{
		Messages: []providers.Message{
			{Role: "assistant", Content: "Use a tool", ToolCalls: []map[string]interface{}{{"id": "call-1", "function": map[string]interface{}{"name": "lookup", "arguments": `{"topic":"routing"}`}}}},
			{Role: "tool", Name: "lookup", ToolCallID: "call-1", Content: "routing data"},
		},
	}, "gemini-2.5-pro")
	if err != nil {
		t.Fatal(err)
	}
	contents, ok := captured["contents"].([]interface{})
	if !ok || len(contents) != 2 {
		t.Fatalf("expected 2 gemini contents, got %#v", captured["contents"])
	}
	firstParts := contents[0].(map[string]interface{})["parts"].([]interface{})
	secondParts := contents[1].(map[string]interface{})["parts"].([]interface{})
	if _, ok := firstParts[1].(map[string]interface{})["functionCall"]; !ok {
		t.Fatalf("expected functionCall part, got %#v", firstParts[1])
	}
	if _, ok := secondParts[0].(map[string]interface{})["functionResponse"]; !ok {
		t.Fatalf("expected functionResponse part, got %#v", secondParts[0])
	}
}

func TestOpenAIProviderRetriesAcrossCredentials(t *testing.T) {
	var authHeaders []string
	p := &openAIProvider{
		baseURL: "https://openai.test",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				authHeaders = append(authHeaders, r.Header.Get("Authorization"))
				if len(authHeaders) == 1 {
					return &http.Response{
						StatusCode: http.StatusTooManyRequests,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`rate limited`)),
					}, nil
				}
				return jsonResponse(`{"id":"resp_1","object":"chat.completion","model":"gpt-5-codex","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`), nil
			}),
		},
		credentials: []ProviderCredential{
			{APIKey: "key-1"},
			{APIKey: "key-2"},
		},
	}

	resp, err := p.ChatCompletion(context.Background(), providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: "hello"}},
	}, "gpt-5-codex")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.Content != "ok" {
		t.Fatalf("expected successful fallback response, got %#v", resp)
	}
	if len(authHeaders) != 2 {
		t.Fatalf("expected 2 upstream attempts, got %d", len(authHeaders))
	}
	if authHeaders[0] != "Bearer key-1" || authHeaders[1] != "Bearer key-2" {
		t.Fatalf("unexpected auth header sequence: %#v", authHeaders)
	}
}

func TestAnthropicProviderRetriesAcrossCredentials(t *testing.T) {
	var apiKeys []string
	p := &anthropicProvider{
		baseURL: "https://anthropic.test",
		client: &http.Client{
			Timeout: 2 * time.Second,
			Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				apiKeys = append(apiKeys, r.Header.Get("x-api-key"))
				if len(apiKeys) == 1 {
					return &http.Response{
						StatusCode: http.StatusUnauthorized,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(`unauthorized`)),
					}, nil
				}
				return jsonResponse(`{"id":"msg_2","role":"assistant","model":"claude","content":[{"type":"text","text":"retried"}],"usage":{"input_tokens":1,"output_tokens":1}}`), nil
			}),
		},
		credentials: []ProviderCredential{
			{APIKey: "claude-1"},
			{APIKey: "claude-2"},
		},
	}

	resp, err := p.ChatCompletion(context.Background(), providers.ChatRequest{
		Messages: []providers.Message{{Role: "user", Content: "retry"}},
	}, "claude-3-5-sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Choices[0].Message.Content != "retried" {
		t.Fatalf("expected retried response, got %#v", resp)
	}
	if fmt.Sprint(apiKeys) != "[claude-1 claude-2]" {
		t.Fatalf("unexpected anthropic key sequence: %#v", apiKeys)
	}
}
