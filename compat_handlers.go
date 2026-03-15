package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/compat"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/subscriptions"
)

func writeOpenAIError(w http.ResponseWriter, status int, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": message,
			"type":    errType,
			"code":    errType,
		},
	})
}

type responsesRequest struct {
	Model              string                   `json:"model"`
	Input              json.RawMessage          `json:"input"`
	Instructions       string                   `json:"instructions,omitempty"`
	Temperature        float64                  `json:"temperature,omitempty"`
	MaxOutput          int                      `json:"max_output_tokens,omitempty"`
	MaxToolCalls       int                      `json:"max_tool_calls,omitempty"`
	ParallelToolCalls  bool                     `json:"parallel_tool_calls,omitempty"`
	PreviousResponseID string                   `json:"previous_response_id,omitempty"`
	Stream             bool                     `json:"stream,omitempty"`
	Tools              []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice         interface{}              `json:"tool_choice,omitempty"`
	Functions          []map[string]interface{} `json:"functions,omitempty"`
	FunctionCall       interface{}              `json:"function_call,omitempty"`
	ReasoningEffort    string                   `json:"reasoning_effort,omitempty"`
	Thinking           map[string]interface{}   `json:"thinking,omitempty"`
}

var (
	// responsesStore replaced by database persistence
)

func knownModelIDs() []string {
	all := availableModels()
	ids := make([]string, 0, len(all))
	for _, model := range all {
		if id := stringValue(model["id"]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func isKnownModel(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	for _, candidate := range knownModelIDs() {
		if strings.EqualFold(candidate, model) {
			return true
		}
	}
	return false
}

func shouldUseAmpFallback(model string) bool {
	return strings.TrimSpace(ampConfig.UpstreamURL) != "" && !isKnownModel(model)
}

func isPrivateIP(host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	ip := net.ParseIP(hostname)
	if ip == nil {
		// Try DNS resolution
		addrs, err := net.LookupHost(hostname)
		if err != nil || len(addrs) == 0 {
			return false
		}
		ip = net.ParseIP(addrs[0])
		if ip == nil {
			return false
		}
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func forwardToAmpUpstream(w http.ResponseWriter, r *http.Request, rawBody []byte) bool {
	upstreamURL := strings.TrimSpace(ampConfig.UpstreamURL)
	if upstreamURL == "" {
		return false
	}
	base, err := url.Parse(upstreamURL)
	if err != nil {
		http.Error(w, "Invalid Amp upstream URL", http.StatusInternalServerError)
		return true
	}
	// SSRF protection: only allow http/https and block private IPs
	if base.Scheme != "http" && base.Scheme != "https" {
		log.Printf("[Amp] Blocked non-HTTP scheme: %s", base.Scheme)
		http.Error(w, "Invalid upstream URL scheme", http.StatusBadRequest)
		return true
	}
	if isPrivateIP(base.Host) {
		log.Printf("[Amp] Blocked private IP in upstream: %s", base.Host)
		http.Error(w, "Upstream URL must not target private networks", http.StatusBadRequest)
		return true
	}
	target, err := base.Parse(r.URL.Path)
	if err != nil {
		http.Error(w, "Invalid Amp upstream target", http.StatusInternalServerError)
		return true
	}
	target.RawQuery = r.URL.RawQuery
	req, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bytes.NewReader(rawBody))
	if err != nil {
		http.Error(w, "Failed to build Amp upstream request", http.StatusInternalServerError)
		return true
	}
	req.Header = r.Header.Clone()
	if req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey := compat.ResolveAmpSecret(ampConfig); apiKey != "" {
		if strings.TrimSpace(req.Header.Get("Authorization")) == "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		if strings.TrimSpace(req.Header.Get("X-Api-Key")) == "" {
			req.Header.Set("X-Api-Key", apiKey)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, "Amp upstream unavailable", http.StatusServiceUnavailable)
		return true
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
	return true
}

func responsesHandler(w http.ResponseWriter, r *http.Request) {
	handleResponsesRequest(w, r, "")
}

func responsesCompactHandler(w http.ResponseWriter, r *http.Request) {
	var req responsesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if req.Stream {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Streaming not supported for compact responses")
		return
	}
	chatReq, err := convertResponsesRequest(req)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	sessionID := requestSessionID(r)
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		if previousSessionID := responseSessionID(req.PreviousResponseID); previousSessionID != "" {
			sessionID = previousSessionID
		}

		// DON'T manually inject conversation history - let the provider's server-side
		// session management handle continuity via the session ID.
		log.Printf("[Responses] Using session %s for continuity via previous_response_id (relying on server-side session)", sessionID)
		chatReq.SkipMemory = true
	}
	resp, err := routeChatRequest(r, chatReq, sessionID, "")
	if err != nil {
		if strings.Contains(err.Error(), "invalid request:") || strings.Contains(err.Error(), "unknown model") || strings.Contains(err.Error(), "not compatible") {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		writeOpenAIError(w, http.StatusServiceUnavailable, "server_error", err.Error())
		return
	}
	outputText := ""
	if len(resp.Choices) > 0 {
		outputText = resp.Choices[0].Message.Content
	}
	payload := map[string]interface{}{
		"id":                   resp.ID,
		"object":               "response",
		"model":                resp.Model,
		"session_id":           sessionID,
		"output_text":          outputText,
		"status":               "completed",
		"previous_response_id": strings.TrimSpace(req.PreviousResponseID),
	}
	if len(resp.Choices) > 0 {
		payload["finish_reason"] = resp.Choices[0].FinishReason
	}
	storeResponsePayload(resp.ID, payload)

	// Explicitly store only the new turn in vectorMemory for future non-Responses-API calls
	if sessionID != "" {
		if chatReq.SkipMemory {
			if len(chatReq.Messages) > 0 {
				lastMsg := chatReq.Messages[len(chatReq.Messages)-1]
				if lastMsg.Role == "user" {
					_ = vectorMemory.Store(lastMsg.Content, "user", sessionID, map[string]interface{}{"source": "responses-api-compact"})
				}
			}
		} else {
			for _, m := range chatReq.Messages {
				_ = vectorMemory.Store(m.Content, m.Role, sessionID, map[string]interface{}{"source": "responses-api-compact"})
			}
		}
		if outputText != "" {
			providerName := ""
			if resp.XProxyMetadata != nil {
				providerName = resp.XProxyMetadata.Provider
			}
			_ = vectorMemory.Store(outputText, "assistant", sessionID, map[string]interface{}{
				"model":    resp.Model,
				"provider": providerName,
				"source":   "responses-api-compact",
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func providerResponsesHandler(w http.ResponseWriter, r *http.Request) {
	handleResponsesRequest(w, r, mux.Vars(r)["provider"])
}

func responseGetHandler(w http.ResponseWriter, r *http.Request) {
	responseID := strings.TrimSpace(mux.Vars(r)["response_id"])
	payload, ok := getResponsePayload(responseID)
	if !ok {
		http.Error(w, "response not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func responseDeleteHandler(w http.ResponseWriter, r *http.Request) {
	responseID := strings.TrimSpace(mux.Vars(r)["response_id"])
	res, err := db.Exec("DELETE FROM responses WHERE id = ?", responseID)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		http.Error(w, "response not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func handleResponsesRequest(w http.ResponseWriter, r *http.Request, preferredProvider string) {
	var req responsesRequest
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}

	chatReq, err := convertResponsesRequest(req)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	sessionID := requestSessionID(r)
	// Handle previous_response_id chaining
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		// Get session ID from previous response
		if previousSessionID := responseSessionID(req.PreviousResponseID); previousSessionID != "" {
			sessionID = previousSessionID
		}

		// DON'T manually inject conversation history - let the provider's server-side
		// session management handle continuity. Manual injection causes Codex and other
		// session-based providers to echo the conversation as literal text.
		// The session ID is sufficient for server-side continuity.
		log.Printf("[Responses] Using session %s for continuity via previous_response_id (relying on server-side session)", sessionID)

		// Still skip memory since we're using session-based continuity
		chatReq.SkipMemory = true
	}
	if shouldUseAmpFallback(chatReq.Model) {
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
	}
	resp, err := routeChatRequest(r, chatReq, sessionID, preferredProvider)
	if err != nil {
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
		if req.Stream {
			writeResponsesStreamError(w, err)
			return
		}
		if strings.Contains(err.Error(), "invalid request:") || strings.Contains(err.Error(), "unknown model") || strings.Contains(err.Error(), "not compatible") {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		writeOpenAIError(w, http.StatusServiceUnavailable, "server_error", err.Error())
		return
	}

	outputText := ""
	if len(resp.Choices) > 0 {
		outputText = resp.Choices[0].Message.Content
	}
	output := responsesOutput(resp, outputText)
	var decodedInput interface{}
	if len(req.Input) > 0 {
		_ = json.Unmarshal(req.Input, &decodedInput)
	}

	payload := map[string]interface{}{
		"id":          resp.ID,
		"object":      "response",
		"created_at":  time.Now().Unix(),
		"model":       resp.Model,
		"session_id":  sessionID,
		"output":      output,
		"output_text": outputText,
		"usage":       resp.Usage,
		"input":       decodedInput, // Store unmarshaled input
	}
	if len(resp.Choices) > 0 {
		payload["finish_reason"] = resp.Choices[0].FinishReason
	}
	if req.MaxToolCalls > 0 {
		payload["max_tool_calls"] = req.MaxToolCalls
	}
	if req.ParallelToolCalls {
		payload["parallel_tool_calls"] = req.ParallelToolCalls
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		payload["previous_response_id"] = strings.TrimSpace(req.PreviousResponseID)
	}
	storeResponsePayload(resp.ID, payload)

	// Explicitly store only the new turn in vectorMemory for future non-Responses-API calls
	if sessionID != "" {
		// Store the new user message(s) from this request
		// (We use chatReq.Messages since it contains the new input, but we must
		// be careful not to store the reconstructed prior history)
		if chatReq.SkipMemory {
			// If we had prior history injected, the new message is the last user message
			if len(chatReq.Messages) > 0 {
				lastMsg := chatReq.Messages[len(chatReq.Messages)-1]
				if lastMsg.Role == "user" {
					_ = vectorMemory.Store(lastMsg.Content, "user", sessionID, map[string]interface{}{"source": "responses-api"})
				}
			}
		} else {
			// Normal case: store everything in chatReq.Messages
			for _, m := range chatReq.Messages {
				_ = vectorMemory.Store(m.Content, m.Role, sessionID, map[string]interface{}{"source": "responses-api"})
			}
		}

		// Store the assistant response
		if outputText != "" {
			providerName := ""
			if resp.XProxyMetadata != nil {
				providerName = resp.XProxyMetadata.Provider
			}
			_ = vectorMemory.Store(outputText, "assistant", sessionID, map[string]interface{}{
				"model":    resp.Model,
				"provider": providerName,
				"source":   "responses-api",
			})
		}
	}

	if req.Stream {
		writeResponsesStream(w, resp, outputText, req)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(payload)
}

func storeResponsePayload(responseID string, payload map[string]interface{}) {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" || payload == nil {
		return
	}

	sessionID := stringValue(payload["session_id"])
	model := stringValue(payload["model"])
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[Responses] Warning: failed to marshal payload for %s: %v", responseID, err)
		return
	}

	_, err = db.Exec(`
		INSERT INTO responses (id, session_id, model, payload)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			payload = excluded.payload,
			model = excluded.model,
			session_id = excluded.session_id
	`, responseID, sessionID, model, string(payloadJSON))

	if err != nil {
		log.Printf("[Responses] Warning: failed to persist response %s: %v", responseID, err)
	}
}

func getResponsePayload(responseID string) (map[string]interface{}, bool) {
	var payloadJSON string
	err := db.QueryRow("SELECT payload FROM responses WHERE id = ?", responseID).Scan(&payloadJSON)
	if err != nil {
		if err != sql.ErrNoRows {
			log.Printf("[Responses] Warning: database error fetching %s: %v", responseID, err)
		}
		return nil, false
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		log.Printf("[Responses] Warning: failed to unmarshal payload for %s: %v", responseID, err)
		return nil, false
	}

	return payload, true
}

func responseSessionID(responseID string) string {
	payload, ok := getResponsePayload(responseID)
	if !ok {
		return ""
	}
	return strings.TrimSpace(stringValue(payload["session_id"]))
}

// reconstructConversationChain walks backwards through the previous_response_id chain
// and reconstructs the full conversation history
func reconstructConversationChain(previousResponseID string) []providers.Message {
	if previousResponseID == "" {
		return nil
	}

	var allMessages [][]providers.Message // Collect messages from each response
	visited := make(map[string]bool)      // Prevent infinite loops
	currentID := previousResponseID
	maxDepth := 100 // Safety limit

	// Walk backwards through the chain, collecting messages from each turn
	for depth := 0; depth < maxDepth && currentID != ""; depth++ {
		if visited[currentID] {
			log.Printf("[Responses] Detected cycle in response chain at %s", currentID)
			break
		}
		visited[currentID] = true

		payload, ok := getResponsePayload(currentID)

		if !ok {
			log.Printf("[Responses] Response %s not found in store", currentID)
			break
		}

		var turnMessages []providers.Message

		// Extract input messages (user messages from this turn)
		if inputRaw, ok := payload["input"]; ok && inputRaw != nil {
			// Try to parse as string (simple input)
			if inputStr, ok := inputRaw.(string); ok && inputStr != "" {
				turnMessages = append(turnMessages, providers.Message{
					Role:    "user",
					Content: inputStr,
				})
			} else {
				// Try to parse as JSON
				var inputBytes []byte
				if rawMsg, ok := inputRaw.(json.RawMessage); ok {
					inputBytes = rawMsg
				} else {
					// Convert to JSON if it's another type
					inputBytes, _ = json.Marshal(inputRaw)
				}

				// Try parsing as a string
				var stringInput string
				if json.Unmarshal(inputBytes, &stringInput) == nil && stringInput != "" {
					turnMessages = append(turnMessages, providers.Message{
						Role:    "user",
						Content: stringInput,
					})
				} else {
					// Try parsing as structured messages
					var structuredMsgs []providers.Message
					if json.Unmarshal(inputBytes, &structuredMsgs) == nil {
						turnMessages = append(turnMessages, structuredMsgs...)
					}
				}
			}
		}

		// Extract output (assistant response from this turn)
		outputText := stringValue(payload["output_text"])
		if outputText != "" {
			turnMessages = append(turnMessages, providers.Message{
				Role:    "assistant",
				Content: outputText,
			})
		}

		// Prepend this turn's messages (to maintain chronological order)
		if len(turnMessages) > 0 {
			allMessages = append([][]providers.Message{turnMessages}, allMessages...)
		}

		// Get the previous response ID from this payload
		currentID = strings.TrimSpace(stringValue(payload["previous_response_id"]))
	}

	// Flatten all messages into a single array
	var messages []providers.Message
	for _, turnMsgs := range allMessages {
		messages = append(messages, turnMsgs...)
	}

	log.Printf("[Responses] Reconstructed %d messages from previous_response_id chain (%d turns)", len(messages), len(allMessages))
	return messages
}

func providerModelsHandler(w http.ResponseWriter, r *http.Request) {
	providerName := strings.TrimSpace(mux.Vars(r)["provider"])
	all := availableModels()
	filtered := make([]map[string]interface{}, 0, len(all))
	for _, model := range all {
		if strings.EqualFold(stringValue(model["owned_by"]), providerName) {
			filtered = append(filtered, model)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   filtered,
	})
}

func providerChatHandler(w http.ResponseWriter, r *http.Request) {
	providerName := strings.TrimSpace(mux.Vars(r)["provider"])
	var req providers.ChatRequest
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}
	if err := json.NewDecoder(bytes.NewReader(rawBody)).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "Invalid request body")
		return
	}

	if resolved := compat.ResolveModel(ampConfig, req.Model, knownModelIDs()); resolved != "" {
		req.Model = resolved
	}
	if shouldUseAmpFallback(req.Model) {
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
	}
	resp, err := routeChatRequest(r, req, requestSessionID(r), providerName)
	if err != nil {
		if strings.Contains(err.Error(), "invalid request:") || strings.Contains(err.Error(), "unknown model") || strings.Contains(err.Error(), "not compatible") {
			writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
			return
		}
		if forwardToAmpUpstream(w, r, rawBody) {
			return
		}
		writeOpenAIError(w, http.StatusServiceUnavailable, "server_error", err.Error())
		return
	}

	// Handle streaming response format
	if req.Stream {
		writeChatCompletionStream(w, resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func routeChatRequest(r *http.Request, req providers.ChatRequest, sessionID, preferredProvider string) (providers.ChatResponse, error) {
	// Validate model compatibility with provider
	if err := validateModelForProvider(req.Model, preferredProvider); err != nil {
		return providers.ChatResponse{}, fmt.Errorf("invalid request: %w", err)
	}

	includeMemoryDebug := r.Header.Get("X-Debug-Memory") == "true"
	ctx := subscriptions.WithPreferredUpstreamAPIKey(r.Context(), resolveAmpUpstreamAPIKeyForRequest(r))
	if preferredProvider == "" {
		preferredProvider = preferredProviderForModel(req.Model)
	}
	if preferredProvider != "" {
		if providerExists(preferredProvider) {
			return proxyRouter.ChatCompletionForProvider(ctx, req, sessionID, preferredProvider, includeMemoryDebug)
		}
		// Provider not loaded (e.g. work profile) — fall through to generic routing
	}
	return proxyRouter.ChatCompletionWithDebug(ctx, req, sessionID, includeMemoryDebug)
}

func requestSessionID(r *http.Request) string {
	sessionID := strings.TrimSpace(r.Header.Get("X-Session-ID"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.URL.Query().Get("session_id"))
	}
	if sessionID == "" {
		sessionID = strings.TrimSpace(r.URL.Query().Get("resume"))
	}
	if sessionID == "" {
		sessionID = "session-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	return sessionID
}

func convertResponsesRequest(req responsesRequest) (providers.ChatRequest, error) {
	chatReq := providers.ChatRequest{
		Model:           compat.ResolveModel(ampConfig, req.Model, knownModelIDs()),
		Temperature:     req.Temperature,
		MaxTokens:       req.MaxOutput,
		Tools:           req.Tools,
		ToolChoice:      req.ToolChoice,
		Functions:       req.Functions,
		FunctionCall:    req.FunctionCall,
		ReasoningEffort: req.ReasoningEffort,
		Thinking:        req.Thinking,
	}
	if chatReq.Model == "" {
		chatReq.Model = "auto"
	}
	if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
		chatReq.Messages = append(chatReq.Messages, providers.Message{
			Role:    "user",
			Content: instructions,
		})
	}

	if len(req.Input) == 0 {
		return chatReq, nil
	}

	var stringInput string
	if err := json.Unmarshal(req.Input, &stringInput); err == nil {
		chatReq.Messages = []providers.Message{{Role: "user", Content: stringInput}}
		if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
			chatReq.Messages = append([]providers.Message{{
				Role:    "user",
				Content: instructions,
			}}, chatReq.Messages...)
		}
		return chatReq, nil
	}

	var messages []providers.Message
	if err := json.Unmarshal(req.Input, &messages); err == nil {
		if hasStructuredChatMessages(messages) {
			chatReq.Messages = messages
			if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
				chatReq.Messages = append([]providers.Message{{
					Role:    "user",
					Content: instructions,
				}}, chatReq.Messages...)
			}
			return chatReq, nil
		}
	}
	messages = nil

	var items []map[string]interface{}
	if err := json.Unmarshal(req.Input, &items); err == nil {
		for _, item := range items {
			itemType := stringValue(item["type"])
			role := stringValue(item["role"])
			name := stringValue(item["name"])
			content := stringValue(item["content"])
			toolCallID := stringValue(item["call_id"])
			if toolCallID == "" {
				toolCallID = stringValue(item["tool_call_id"])
			}
			var toolCalls []map[string]interface{}
			if rawToolCalls, ok := item["tool_calls"].([]interface{}); ok {
				for _, rawToolCall := range rawToolCalls {
					if toolCallMap, ok := rawToolCall.(map[string]interface{}); ok {
						toolCalls = append(toolCalls, toolCallMap)
					}
				}
			}
			content += extractItemText(item["content"])

			switch itemType {
			case "function_call":
				functionName := stringValue(item["name"])
				arguments := "{}"
				if rawArgs, ok := item["arguments"]; ok {
					if encodedArgs, err := json.Marshal(rawArgs); err == nil && string(encodedArgs) != "null" {
						arguments = string(encodedArgs)
					}
				}
				if toolCallID == "" {
					toolCallID = stringValue(item["id"])
				}
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   firstNonEmpty(stringValue(item["id"]), toolCallID),
					"type": "function",
					"function": map[string]interface{}{
						"name":      functionName,
						"arguments": arguments,
					},
				})
				role = "assistant"
			case "function_call_output", "tool_result":
				role = "tool"
				if content == "" {
					content = firstNonEmpty(stringValue(item["output"]), stringValue(item["text"]))
				}
			}

			if content != "" || len(toolCalls) > 0 || role == "tool" {
				messages = append(messages, providers.Message{
					Role:       defaultRole(role),
					Name:       name,
					Content:    content,
					ToolCallID: toolCallID,
					ToolCalls:  toolCalls,
				})
			}
		}
		chatReq.Messages = messages
		if instructions := strings.TrimSpace(req.Instructions); instructions != "" {
			chatReq.Messages = append([]providers.Message{{
				Role:    "user",
				Content: instructions,
			}}, chatReq.Messages...)
		}
		return chatReq, nil
	}

	return providers.ChatRequest{}, http.ErrNotSupported
}

func extractItemText(value interface{}) string {
	var content string
	switch typed := value.(type) {
	case string:
		content += typed
	case []interface{}:
		for _, contentItem := range typed {
			if contentMap, ok := contentItem.(map[string]interface{}); ok {
				if text := stringValue(contentMap["text"]); text != "" {
					content += text
				}
				if outputText := stringValue(contentMap["output_text"]); outputText != "" {
					content += outputText
				}
			}
		}
	}
	return content
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func hasStructuredChatMessages(messages []providers.Message) bool {
	if len(messages) == 0 {
		return false
	}
	for _, message := range messages {
		if strings.TrimSpace(message.Role) != "" ||
			strings.TrimSpace(message.Content) != "" ||
			strings.TrimSpace(message.ToolCallID) != "" ||
			len(message.ToolCalls) > 0 {
			return true
		}
	}
	return false
}

func writeResponsesStream(w http.ResponseWriter, resp providers.ChatResponse, outputText string, req responsesRequest) {
	sse := newSSEWriter(w)
	if sse == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "server_error", "Streaming not supported")
		return
	}
	flusher := sse.flusher
	_ = flusher // used by existing event loop below

	events := []map[string]interface{}{
		{"type": "response.created", "response": map[string]interface{}{"id": resp.ID, "model": resp.Model}},
		{"type": "response.output_item.added", "item": map[string]interface{}{"type": "message", "role": "assistant"}},
		{"type": "response.content_part.added", "part": map[string]interface{}{"type": "output_text", "text": ""}},
	}
	if outputText != "" {
		events = append(events,
			map[string]interface{}{"type": "response.output_text.delta", "delta": outputText},
			map[string]interface{}{"type": "response.output_text.done", "text": outputText},
		)
	}
	for _, item := range responsesOutput(resp, outputText) {
		if toolCalls, ok := item["tool_calls"].([]map[string]interface{}); ok && len(toolCalls) > 0 {
			for index, toolCall := range toolCalls {
				callID := firstNonEmpty(stringValue(toolCall["id"]), "call")
				function, _ := toolCall["function"].(map[string]interface{})
				functionName := stringValue(function["name"])
				functionArgs := firstNonEmpty(stringValue(function["arguments"]), "{}")
				events = append(events, map[string]interface{}{
					"type": "response.output_item.added",
					"item": map[string]interface{}{
						"id":           "fc_" + callID,
						"type":         "function_call",
						"status":       "in_progress",
						"call_id":      callID,
						"name":         functionName,
						"arguments":    "",
						"output_index": index,
					},
				}, map[string]interface{}{
					"type":         "response.function_call_arguments.delta",
					"item_id":      "fc_" + callID,
					"output_index": index,
					"delta":        functionArgs,
				}, map[string]interface{}{
					"type":         "response.function_call_arguments.done",
					"item_id":      "fc_" + callID,
					"output_index": index,
					"arguments":    functionArgs,
				}, map[string]interface{}{
					"type": "response.output_item.done",
					"item": map[string]interface{}{
						"id":           "fc_" + callID,
						"type":         "function_call",
						"status":       "completed",
						"call_id":      callID,
						"name":         functionName,
						"arguments":    functionArgs,
						"output_index": index,
					},
				})
			}
		}
	}
	completed := map[string]interface{}{"id": resp.ID, "model": resp.Model, "output_text": outputText, "usage": resp.Usage}
	if len(resp.Choices) > 0 {
		completed["finish_reason"] = resp.Choices[0].FinishReason
	}
	if req.MaxToolCalls > 0 {
		completed["max_tool_calls"] = req.MaxToolCalls
	}
	if req.ParallelToolCalls {
		completed["parallel_tool_calls"] = req.ParallelToolCalls
	}
	if strings.TrimSpace(req.PreviousResponseID) != "" {
		completed["previous_response_id"] = strings.TrimSpace(req.PreviousResponseID)
	}
	events = append(events,
		map[string]interface{}{"type": "response.completed", "response": completed},
	)
	for _, event := range events {
		payload, _ := json.Marshal(event)
		_, _ = w.Write([]byte("data: "))
		_, _ = w.Write(payload)
		_, _ = w.Write([]byte("\n\n"))
		flusher.Flush()
	}
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func responsesOutput(resp providers.ChatResponse, outputText string) []map[string]interface{} {
	content := []map[string]interface{}{}
	if outputText != "" {
		content = append(content, map[string]interface{}{"type": "output_text", "text": outputText})
	}

	item := map[string]interface{}{
		"type":    "message",
		"role":    "assistant",
		"content": content,
	}
	if len(resp.Choices) > 0 {
		if toolCalls := resp.Choices[0].Message.ToolCalls; len(toolCalls) > 0 {
			item["tool_calls"] = toolCalls
		}
		if name := strings.TrimSpace(resp.Choices[0].Message.Name); name != "" {
			item["name"] = name
		}
	}
	return []map[string]interface{}{item}
}

func writeResponsesStreamError(w http.ResponseWriter, err error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	payload, _ := json.Marshal(map[string]interface{}{
		"type": "response.error",
		"error": map[string]interface{}{
			"message": err.Error(),
			"type":    "service_unavailable",
		},
	})
	_, _ = w.Write([]byte("data: "))
	_, _ = w.Write(payload)
	_, _ = w.Write([]byte("\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

func defaultRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" {
		return "user"
	}
	return role
}

func resolveAmpUpstreamAPIKeyForRequest(r *http.Request) string {
	if named := resolveAmpUpstreamAPIKeyByAccountName(r); named != "" {
		return named
	}
	clientKey := clientAPIKeyFromRequest(r)
	if clientKey == "" {
		return ""
	}
	for _, entry := range ampConfig.UpstreamAPIKeys {
		for _, mappedClientKey := range entry.APIKeys {
			if strings.TrimSpace(mappedClientKey) == clientKey {
				return strings.TrimSpace(entry.UpstreamAPIKey)
			}
		}
	}
	return ""
}

func resolveAmpUpstreamAPIKeyByAccountName(r *http.Request) string {
	if r == nil {
		return ""
	}
	accountName := strings.TrimSpace(r.Header.Get("X-Upstream-Account"))
	if accountName == "" {
		accountName = strings.TrimSpace(r.URL.Query().Get("upstream_account"))
	}
	if accountName == "" {
		accountName = strings.TrimSpace(r.URL.Query().Get("upstream-account"))
	}
	if accountName == "" {
		return ""
	}
	for _, entry := range ampConfig.UpstreamAPIKeys {
		if strings.EqualFold(strings.TrimSpace(entry.Name), accountName) {
			return strings.TrimSpace(entry.UpstreamAPIKey)
		}
	}
	return ""
}

func clientAPIKeyFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return strings.TrimSpace(r.Header.Get("X-API-Key"))
}

func providerExists(name string) bool {
	for _, p := range providerList {
		if strings.EqualFold(p.Name(), name) {
			return true
		}
	}
	return false
}

var cachedActiveProfile string

func activeProfile() string {
	if cachedActiveProfile == "" {
		cachedActiveProfile = strings.ToLower(strings.TrimSpace(os.Getenv("ACTIVE_PROFILE")))
	}
	return cachedActiveProfile
}

func availableModels() []map[string]interface{} {
	profile := activeProfile()
	seen := make(map[string]struct{})
	out := make([]map[string]interface{}, 0)

	// For personal profile (or default), start with subscription-based model catalog
	if profile != "work" {
		models, err := subscriptions.AvailableModels(context.Background())
		if err == nil {
			for _, model := range models {
				if _, dup := seen[model.ID]; !dup {
					seen[model.ID] = struct{}{}
					out = append(out, map[string]interface{}{
						"id":       model.ID,
						"object":   model.Object,
						"owned_by": model.OwnedBy,
						"context":  model.Context,
					})
				}
			}
		}
	}

	// Merge in models from all registered providers (Vertex, NanoGPT, etc.)
	for _, p := range providerList {
		if lm, ok := p.(interface{ ListModels() []map[string]interface{} }); ok {
			for _, m := range lm.ListModels() {
				id := stringValue(m["id"])
				if _, dup := seen[id]; !dup && id != "" {
					seen[id] = struct{}{}
					out = append(out, m)
				}
			}
		}
	}
	if len(out) > 0 {
		return out
	}

	return []map[string]interface{}{
		{"id": "claude-sonnet-4-5-20250929", "object": "model", "owned_by": "claude-code", "context": 200000},
		{"id": "gpt-5.3-codex", "object": "model", "owned_by": "codex", "context": 128000},
		{"id": "gemini-3.1-pro-preview", "object": "model", "owned_by": "gemini", "context": 1048576},
		{"id": "qwen-max", "object": "model", "owned_by": "qwen", "context": 131072},
	}
}

func modelOwnerAndContext(providerName string) (string, int) {
	switch strings.ToLower(strings.TrimSpace(providerName)) {
	case "anthropic":
		return "claude-code", 200000
	case "openai":
		return "codex", 128000
	case "gemini":
		return "gemini", 1048576
	case "qwen":
		return "qwen", 131072
	case "ollama-cloud":
		return "ollama-cloud", 128000
	case "nanogpt":
		return "nanogpt", 2000000
	default:
		return providerName, 128000
	}
}

// validateModelForProvider checks if a model is compatible with a provider
func validateModelForProvider(model, provider string) error {
	model = strings.ToLower(strings.TrimSpace(model))
	provider = strings.ToLower(strings.TrimSpace(provider))

	if model == "" || model == "auto" {
		return nil // auto model is always valid
	}

	// Check if model exists in our registry
	modelProvider := preferredProviderForModel(model)
	if modelProvider == "" {
		// Allow unknown models to fall through to AMP upstream if configured
		if ampConfig.UpstreamURL != "" {
			return nil
		}
		return fmt.Errorf("unknown model: %s", model)
	}

	// If provider is pinned, validate model compatibility
	if provider != "" {
		// Normalize provider names
		normalizedProvider := provider
		switch provider {
		case "anthropic":
			normalizedProvider = "claude-code"
		case "openai":
			normalizedProvider = "codex"
		}

		if modelProvider != normalizedProvider {
			return fmt.Errorf("model %s is not compatible with provider %s (expected %s provider)", model, provider, modelProvider)
		}
	}

	return nil
}

func preferredProviderForModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" || model == "auto" {
		return ""
	}

	// First check registered models from the live provider list
	for _, item := range availableModels() {
		if strings.EqualFold(stringValue(item["id"]), model) {
			ownedBy := strings.ToLower(strings.TrimSpace(stringValue(item["owned_by"])))
			if ownedBy != "" {
				return ownedBy
			}
		}
	}

	// Determine model family from prefix
	var canonicalProvider string
	switch {
	case strings.HasPrefix(model, "claude"), strings.HasPrefix(model, "opus"), strings.HasPrefix(model, "sonnet"), strings.HasPrefix(model, "haiku"):
		canonicalProvider = "claude-code"
	case strings.HasPrefix(model, "gpt"), strings.Contains(model, "codex"), strings.HasPrefix(model, "o1"), strings.HasPrefix(model, "o3"):
		canonicalProvider = "codex"
	case strings.HasPrefix(model, "gemini"):
		canonicalProvider = "gemini"
	case strings.HasPrefix(model, "qwen"):
		canonicalProvider = "qwen"
	default:
		return ""
	}

	// Check if the canonical provider is actually registered
	if providerExists(canonicalProvider) {
		return canonicalProvider
	}

	// Canonical provider not loaded — find a registered provider whose name matches
	// the model family (e.g. work profile has vertex-claude instead of claude-code)
	for _, p := range providerList {
		pname := strings.ToLower(p.Name())
		familyMatch := false
		switch canonicalProvider {
		case "claude-code":
			familyMatch = strings.Contains(pname, "claude") || strings.Contains(pname, "anthropic")
		case "codex":
			familyMatch = strings.Contains(pname, "codex") || strings.Contains(pname, "openai")
		case "gemini":
			familyMatch = strings.Contains(pname, "gemini") || strings.Contains(pname, "google")
		case "qwen":
			familyMatch = strings.Contains(pname, "qwen")
		}
		if familyMatch && p.SupportsModel(model) {
			return p.Name()
		}
	}

	return canonicalProvider
}

func ampConfigHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"ampcode": ampConfig})
}

func ampUpstreamURLHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]string{"upstream-url": ampConfig.UpstreamURL})
	case http.MethodPut:
		var payload struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		ampConfig.UpstreamURL = strings.TrimSpace(payload.Value)
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]string{"upstream-url": ampConfig.UpstreamURL})
	case http.MethodDelete:
		ampConfig.UpstreamURL = ""
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func ampUpstreamAPIKeyHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]string{"upstream-api-key": compat.ResolveAmpSecret(ampConfig)})
	case http.MethodPut:
		var payload struct {
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		ampConfig.UpstreamAPIKey = strings.TrimSpace(payload.Value)
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]string{"upstream-api-key": ampConfig.UpstreamAPIKey})
	case http.MethodDelete:
		ampConfig.UpstreamAPIKey = ""
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func ampUpstreamAPIKeysHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]interface{}{"upstream-api-keys": ampConfig.UpstreamAPIKeys})
	case http.MethodPut:
		var payload struct {
			Value []compat.AmpUpstreamAPIKeyEntry `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		ampConfig.UpstreamAPIKeys = sanitizeAmpKeyEntries(payload.Value)
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]interface{}{"upstream-api-keys": ampConfig.UpstreamAPIKeys})
	case http.MethodPatch:
		var payload struct {
			Value []compat.AmpUpstreamAPIKeyEntry `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		byName := make(map[string]compat.AmpUpstreamAPIKeyEntry)
		for _, entry := range sanitizeAmpKeyEntries(ampConfig.UpstreamAPIKeys) {
			byName[entry.Name] = entry
		}
		for _, entry := range sanitizeAmpKeyEntries(payload.Value) {
			byName[entry.Name] = entry
		}
		ampConfig.UpstreamAPIKeys = mapAmpKeyEntries(byName)
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]interface{}{"upstream-api-keys": ampConfig.UpstreamAPIKeys})
	case http.MethodDelete:
		ampConfig.UpstreamAPIKeys = nil
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func ampModelMappingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]interface{}{"model-mappings": ampConfig.ModelMappings})
	case http.MethodPut:
		var payload struct {
			Value []compat.AmpModelMapping `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		ampConfig.ModelMappings = payload.Value
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]interface{}{"model-mappings": ampConfig.ModelMappings})
	case http.MethodPatch:
		var payload struct {
			Value []compat.AmpModelMapping `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		byFrom := make(map[string]compat.AmpModelMapping)
		for _, entry := range ampConfig.ModelMappings {
			byFrom[strings.ToLower(entry.From)] = entry
		}
		for _, entry := range payload.Value {
			byFrom[strings.ToLower(entry.From)] = entry
		}
		ampConfig.ModelMappings = mapAmpModelMappings(byFrom)
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]interface{}{"model-mappings": ampConfig.ModelMappings})
	case http.MethodDelete:
		ampConfig.ModelMappings = nil
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func ampForceModelMappingsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]bool{"force-model-mappings": ampConfig.ForceModelMappings})
	case http.MethodPut:
		var payload struct {
			Value bool `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		ampConfig.ForceModelMappings = payload.Value
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]bool{"force-model-mappings": ampConfig.ForceModelMappings})
	}
}

func ampRestrictManagementToLocalhostHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, map[string]bool{"restrict-management-to-localhost": ampConfig.RestrictManagementToLocalhost})
	case http.MethodPut:
		var payload struct {
			Value bool `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		ampConfig.RestrictManagementToLocalhost = payload.Value
		if err := compat.SaveAmpCodeConfig(db, ampConfig); err != nil {
			log.Printf("[Amp] Warning: failed to save config: %v", err)
		}
		writeJSON(w, map[string]bool{"restrict-management-to-localhost": ampConfig.RestrictManagementToLocalhost})
	}
}

func mapAmpKeyEntries(entries map[string]compat.AmpUpstreamAPIKeyEntry) []compat.AmpUpstreamAPIKeyEntry {
	out := make([]compat.AmpUpstreamAPIKeyEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	return out
}

func sanitizeAmpKeyEntries(entries []compat.AmpUpstreamAPIKeyEntry) []compat.AmpUpstreamAPIKeyEntry {
	out := make([]compat.AmpUpstreamAPIKeyEntry, 0, len(entries))
	for idx, entry := range entries {
		entry.Name = strings.TrimSpace(entry.Name)
		entry.UpstreamAPIKey = strings.TrimSpace(entry.UpstreamAPIKey)
		if entry.Name == "" {
			entry.Name = "entry-" + strconv.Itoa(idx+1)
		}
		filteredKeys := make([]string, 0, len(entry.APIKeys))
		for _, apiKey := range entry.APIKeys {
			apiKey = strings.TrimSpace(apiKey)
			if apiKey == "" {
				continue
			}
			filteredKeys = append(filteredKeys, apiKey)
		}
		entry.APIKeys = filteredKeys
		if entry.UpstreamAPIKey == "" {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func mapAmpModelMappings(entries map[string]compat.AmpModelMapping) []compat.AmpModelMapping {
	out := make([]compat.AmpModelMapping, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry)
	}
	return out
}

func writeJSON(w http.ResponseWriter, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[Response] Warning: failed to encode JSON response: %v", err)
	}
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return s
	}
	return ""
}

// SSEWriter handles Server-Sent Events streaming setup and writing.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// newSSEWriter creates an SSEWriter with proper headers. Returns nil if flushing is unsupported.
func newSSEWriter(w http.ResponseWriter) *SSEWriter {
	flusher, ok := w.(http.Flusher)
	if !ok {
		log.Println("[SSE] ResponseWriter does not support flushing")
		return nil
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	return &SSEWriter{w: w, flusher: flusher}
}

// WriteData writes an SSE data-only line (OpenAI format: "data: ...\n\n").
func (s *SSEWriter) WriteData(data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("[SSE] Failed to marshal data: %v", err)
		return
	}
	fmt.Fprintf(s.w, "data: %s\n\n", jsonData)
	s.flusher.Flush()
}

// WriteEvent writes a named SSE event (Anthropic format: "event: ...\ndata: ...\n\n").
func (s *SSEWriter) WriteEvent(event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		log.Printf("[SSE] Failed to marshal %s event: %v", event, err)
		return
	}
	fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", event, jsonData)
	s.flusher.Flush()
}

// WriteDone writes the OpenAI [DONE] marker.
func (s *SSEWriter) WriteDone() {
	fmt.Fprintf(s.w, "data: [DONE]\n\n")
	s.flusher.Flush()
}

func writeChatCompletionStream(w http.ResponseWriter, resp providers.ChatResponse) {
	sse := newSSEWriter(w)
	if sse == nil {
		writeOpenAIError(w, http.StatusInternalServerError, "server_error", "Streaming not supported")
		return
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]

		sse.WriteData(map[string]interface{}{
			"id":      resp.ID,
			"object":  "chat.completion.chunk",
			"created": resp.Created,
			"model":   resp.Model,
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"delta": map[string]interface{}{
						"role":    choice.Message.Role,
						"content": choice.Message.Content,
					},
					"finish_reason": nil,
				},
			},
		})

		sse.WriteData(map[string]interface{}{
			"id":      resp.ID,
			"object":  "chat.completion.chunk",
			"created": resp.Created,
			"model":   resp.Model,
			"choices": []map[string]interface{}{
				{
					"index":         0,
					"delta":         map[string]interface{}{},
					"finish_reason": choice.FinishReason,
				},
			},
		})
	}

	sse.WriteDone()
}
