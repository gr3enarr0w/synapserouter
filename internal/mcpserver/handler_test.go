package mcpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
)

func testHandler() *Handler {
	return &Handler{
		registry: tools.DefaultRegistry(),
		workDir:  "/tmp",
	}
}

func TestHandleInitialize(t *testing.T) {
	h := testHandler()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/initialize", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleInitialize(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp MCPResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	result, _ := resp.Result.(map[string]interface{})
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("expected protocol version 2024-11-05, got %v", result["protocolVersion"])
	}
}

func TestHandleToolsList(t *testing.T) {
	h := testHandler()
	body := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/list", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleToolsList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp MCPResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}

	result, _ := resp.Result.(map[string]interface{})
	toolsList, _ := result["tools"].([]interface{})
	if len(toolsList) != 10 {
		t.Errorf("expected 10 tools, got %d", len(toolsList))
	}
}

func TestHandleToolsListGET(t *testing.T) {
	h := testHandler()
	req := httptest.NewRequest(http.MethodGet, "/mcp/tools/list", nil)
	w := httptest.NewRecorder()

	h.HandleToolsList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleToolsCall(t *testing.T) {
	h := testHandler()
	body := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"bash","arguments":{"command":"echo hello"}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleToolsCall(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp MCPResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
}

func TestHandleToolsCallUnknownTool(t *testing.T) {
	h := testHandler()
	body := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"nonexistent","arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleToolsCall(w, req)

	var resp MCPResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestHandleToolsCallMissingName(t *testing.T) {
	h := testHandler()
	body := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"arguments":{}}}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/tools/call", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleToolsCall(w, req)

	var resp MCPResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Error == nil {
		t.Error("expected error for missing tool name")
	}
}

func TestHandleMethodNotAllowed(t *testing.T) {
	h := testHandler()
	req := httptest.NewRequest(http.MethodGet, "/mcp/tools/call", nil)
	w := httptest.NewRecorder()

	h.HandleToolsCall(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestServer(t *testing.T) {
	registry := tools.DefaultRegistry()
	srv := NewServer(registry, "/tmp")
	handler := srv.Routes()

	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Test tools/list
	resp, err := http.Get(ts.URL + "/mcp/tools/list")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
