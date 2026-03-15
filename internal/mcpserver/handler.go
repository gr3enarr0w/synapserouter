package mcpserver

import (
	"encoding/json"
	"net/http"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

// Handler implements MCP protocol HTTP handlers.
type Handler struct {
	registry    *tools.Registry
	permissions *tools.PermissionChecker
	workDir     string
}

// HandleInitialize handles the MCP initialize handshake.
func (h *Handler) HandleInitialize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, -32600, "method not allowed")
		return
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, -32700, "parse error")
		return
	}

	writeJSON(w, MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result: map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{},
			},
			"serverInfo": map[string]interface{}{
				"name":    "synroute",
				"version": "1.0.0",
			},
		},
	})
}

// HandleToolsList returns all available tool definitions in MCP format.
func (h *Handler) HandleToolsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, -32600, "method not allowed")
		return
	}

	var reqID interface{}
	if r.Method == http.MethodPost {
		var req MCPRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			reqID = req.ID
		}
	}

	toolNames := h.registry.List()
	defs := make([]ToolDefinition, 0, len(toolNames))
	for _, name := range toolNames {
		tool, ok := h.registry.Get(name)
		if !ok {
			continue
		}
		defs = append(defs, ToolDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}

	writeJSON(w, MCPResponse{
		JSONRPC: "2.0",
		ID:      reqID,
		Result: map[string]interface{}{
			"tools": defs,
		},
	})
}

// HandleToolsCall executes a tool and returns the result.
func (h *Handler) HandleToolsCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, -32600, "method not allowed")
		return
	}

	var req MCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, -32700, "parse error")
		return
	}

	// Parse params
	paramsJSON, _ := json.Marshal(req.Params)
	var params ToolCallParams
	if err := json.Unmarshal(paramsJSON, &params); err != nil {
		writeRPCError(w, req.ID, -32602, "invalid params")
		return
	}

	if params.Name == "" {
		writeRPCError(w, req.ID, -32602, "tool name is required")
		return
	}

	result, err := h.registry.ExecuteChecked(r.Context(), params.Name, params.Arguments, h.workDir, h.permissions)
	if err != nil {
		writeRPCError(w, req.ID, -32603, err.Error())
		return
	}

	callResult := ToolCallResult{
		Content: []ContentBlock{{
			Type: "text",
			Text: result.Output,
		}},
		IsError: result.Error != "",
	}

	if result.Error != "" {
		callResult.Content = []ContentBlock{{
			Type: "text",
			Text: result.Error + "\n" + result.Output,
		}}
	}

	writeJSON(w, MCPResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  callResult,
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(MCPResponse{
		JSONRPC: "2.0",
		Error:   &MCPError{Code: code, Message: msg},
	})
}

func writeRPCError(w http.ResponseWriter, id interface{}, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MCPResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &MCPError{Code: code, Message: msg},
	})
}
