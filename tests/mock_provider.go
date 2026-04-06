// mock_provider.go — Lightweight OpenAI-compatible server for adversarial testing.
// Responds instantly with canned responses so tests complete in <1s each.
// Usage: go run tests/mock_provider.go &
//        OLLAMA_CHAIN=mock-model OLLAMA_API_KEYS=test OLLAMA_BASE_URL=http://localhost:19876 ./synroute code --message "test"

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	port := "19876"
	if p := os.Getenv("MOCK_PORT"); p != "" {
		port = p
	}

	http.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Stream bool `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		lastMsg := ""
		for _, m := range req.Messages {
			if m.Role == "user" {
				lastMsg = m.Content
			}
		}

		response := "Hello! I'm a mock provider for testing."
		toolCall := ""
		toolName := ""
		toolArgs := ""

		// Return actual tool calls for tool-related prompts
		if strings.Contains(lastMsg, "bash") || strings.Contains(lastMsg, "ls") || strings.Contains(lastMsg, "run ") {
			toolName = "bash"
			toolArgs = `{"command": "echo mock"}`
		} else if strings.Contains(lastMsg, "grep") || strings.Contains(lastMsg, "search for") {
			toolName = "grep"
			toolArgs = `{"pattern": "func main", "path": "."}`
		} else if strings.Contains(lastMsg, "glob") || strings.Contains(lastMsg, "find all") {
			toolName = "glob"
			toolArgs = `{"pattern": "**/*.go"}`
		} else if strings.Contains(lastMsg, "file_read") || strings.Contains(lastMsg, "read ") {
			toolName = "file_read"
			toolArgs = `{"path": "go.mod"}`
		} else if strings.Contains(lastMsg, "git") {
			toolName = "git"
			toolArgs = `{"subcommand": "status"}`
		}

		if toolName != "" {
			toolCall = fmt.Sprintf(`[{"id":"call-mock-1","type":"function","function":{"name":"%s","arguments":"%s"}}]`,
				toolName, strings.ReplaceAll(toolArgs, `"`, `\"`))
			response = ""
		}

		// Check if this is a follow-up after tool execution (tool role in messages)
		hasToolResult := false
		for _, m := range req.Messages {
			if m.Role == "tool" {
				hasToolResult = true
			}
		}
		if hasToolResult {
			toolCall = ""
			response = "Done. The operation completed successfully."
		}

		if req.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			chunk := map[string]interface{}{
				"id":      "mock-1",
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   "mock-model",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"delta":         map[string]string{"content": response},
						"finish_reason": "stop",
					},
				},
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			fmt.Fprintf(w, "data: [DONE]\n\n")
			w.(http.Flusher).Flush()
		} else {
			msg := map[string]interface{}{"role": "assistant", "content": response}
			if toolCall != "" {
				msg["tool_calls"] = json.RawMessage(toolCall)
				msg["content"] = nil
			}
			resp := map[string]interface{}{
				"id":      "mock-1",
				"object":  "chat.completion",
				"created": time.Now().Unix(),
				"model":   "mock-model",
				"choices": []map[string]interface{}{
					{
						"index":         0,
						"message":       msg,
						"finish_reason": "stop",
					},
				},
				"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30},
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}
	})

	http.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{
			"data": []map[string]string{{"id": "mock-model", "object": "model"}},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	// Health check
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	fmt.Printf("Mock provider listening on :%s\n", port)
	http.ListenAndServe(":"+port, nil)
}
