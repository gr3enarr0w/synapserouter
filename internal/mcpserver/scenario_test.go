package mcpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// TestScenarioFullMCPWorkflow tests a complete MCP client interaction:
// initialize -> tools/list -> tools/call (multiple tools) -> verify results
func TestScenarioFullMCPWorkflow(t *testing.T) {
	dir := t.TempDir()
	registry := tools.DefaultRegistry()
	srv := NewServer(registry, dir)
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	// Step 1: Initialize
	t.Run("initialize", func(t *testing.T) {
		resp := mcpPost(t, ts.URL+"/mcp/initialize", MCPRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "initialize",
			Params:  map[string]interface{}{},
		})
		if resp.Error != nil {
			t.Fatalf("init error: %v", resp.Error)
		}
		result, _ := resp.Result.(map[string]interface{})
		if result["protocolVersion"] != "2024-11-05" {
			t.Errorf("bad protocol version: %v", result["protocolVersion"])
		}
		info, _ := result["serverInfo"].(map[string]interface{})
		if info["name"] != "synroute" {
			t.Errorf("expected server name 'synroute', got %v", info["name"])
		}
	})

	// Step 2: List tools
	var toolNames []string
	t.Run("tools/list", func(t *testing.T) {
		resp := mcpPost(t, ts.URL+"/mcp/tools/list", MCPRequest{
			JSONRPC: "2.0",
			ID:      2,
			Method:  "tools/list",
		})
		if resp.Error != nil {
			t.Fatalf("list error: %v", resp.Error)
		}
		result, _ := resp.Result.(map[string]interface{})
		toolsList, _ := result["tools"].([]interface{})
		if len(toolsList) != 10 {
			t.Fatalf("expected 10 tools, got %d", len(toolsList))
		}
		for _, td := range toolsList {
			toolDef, _ := td.(map[string]interface{})
			name, _ := toolDef["name"].(string)
			toolNames = append(toolNames, name)
		}
	})

	// Verify expected tools are present
	t.Run("expected tools present", func(t *testing.T) {
		expected := map[string]bool{
			"bash": false, "file_read": false, "file_write": false,
			"file_edit": false, "grep": false, "glob": false, "git": false,
			"web_search": false, "web_fetch": false, "notebook_edit": false,
		}
		for _, name := range toolNames {
			expected[name] = true
		}
		for name, found := range expected {
			if !found {
				t.Errorf("missing tool: %s", name)
			}
		}
	})

	// Step 3: Call tools
	t.Run("tools/call bash", func(t *testing.T) {
		resp := mcpPost(t, ts.URL+"/mcp/tools/call", MCPRequest{
			JSONRPC: "2.0",
			ID:      3,
			Method:  "tools/call",
			Params: ToolCallParams{
				Name:      "bash",
				Arguments: map[string]interface{}{"command": "echo mcp-e2e-test"},
			},
		})
		if resp.Error != nil {
			t.Fatalf("call error: %v", resp.Error)
		}
		result := extractCallResult(t, resp)
		if !strings.Contains(result, "mcp-e2e-test") {
			t.Errorf("expected 'mcp-e2e-test' in output, got %q", result)
		}
	})

	t.Run("tools/call file_write + file_read roundtrip", func(t *testing.T) {
		// Write a file
		resp := mcpPost(t, ts.URL+"/mcp/tools/call", MCPRequest{
			JSONRPC: "2.0",
			ID:      4,
			Method:  "tools/call",
			Params: ToolCallParams{
				Name:      "file_write",
				Arguments: map[string]interface{}{"path": "mcp-test.txt", "content": "mcp roundtrip"},
			},
		})
		if resp.Error != nil {
			t.Fatalf("write error: %v", resp.Error)
		}

		// Read it back
		resp = mcpPost(t, ts.URL+"/mcp/tools/call", MCPRequest{
			JSONRPC: "2.0",
			ID:      5,
			Method:  "tools/call",
			Params: ToolCallParams{
				Name:      "file_read",
				Arguments: map[string]interface{}{"path": "mcp-test.txt"},
			},
		})
		if resp.Error != nil {
			t.Fatalf("read error: %v", resp.Error)
		}
		result := extractCallResult(t, resp)
		if !strings.Contains(result, "mcp roundtrip") {
			t.Errorf("expected 'mcp roundtrip' in read result, got %q", result)
		}
	})

	t.Run("tools/call file_edit", func(t *testing.T) {
		resp := mcpPost(t, ts.URL+"/mcp/tools/call", MCPRequest{
			JSONRPC: "2.0",
			ID:      6,
			Method:  "tools/call",
			Params: ToolCallParams{
				Name: "file_edit",
				Arguments: map[string]interface{}{
					"path": "mcp-test.txt", "old_string": "roundtrip", "new_string": "success",
				},
			},
		})
		if resp.Error != nil {
			t.Fatalf("edit error: %v", resp.Error)
		}

		// Verify on disk
		data, _ := os.ReadFile(filepath.Join(dir, "mcp-test.txt"))
		if string(data) != "mcp success" {
			t.Errorf("expected 'mcp success', got %q", string(data))
		}
	})

	t.Run("tools/call grep", func(t *testing.T) {
		resp := mcpPost(t, ts.URL+"/mcp/tools/call", MCPRequest{
			JSONRPC: "2.0",
			ID:      7,
			Method:  "tools/call",
			Params: ToolCallParams{
				Name:      "grep",
				Arguments: map[string]interface{}{"pattern": "mcp", "path": dir},
			},
		})
		if resp.Error != nil {
			t.Fatalf("grep error: %v", resp.Error)
		}
		result := extractCallResult(t, resp)
		if !strings.Contains(result, "mcp-test.txt") {
			t.Errorf("expected grep to find mcp-test.txt, got %q", result)
		}
	})

	t.Run("tools/call unknown returns error", func(t *testing.T) {
		resp := mcpPost(t, ts.URL+"/mcp/tools/call", MCPRequest{
			JSONRPC: "2.0",
			ID:      8,
			Method:  "tools/call",
			Params: ToolCallParams{
				Name:      "nonexistent",
				Arguments: map[string]interface{}{},
			},
		})
		if resp.Error == nil {
			t.Error("expected error for unknown tool")
		}
	})
}

func TestScenarioMCPErrorHandling(t *testing.T) {
	registry := tools.DefaultRegistry()
	srv := NewServer(registry, t.TempDir())
	ts := httptest.NewServer(srv.Routes())
	defer ts.Close()

	t.Run("invalid JSON", func(t *testing.T) {
		resp, err := http.Post(ts.URL+"/mcp/tools/call", "application/json", bytes.NewBufferString("{invalid"))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		var mcpResp MCPResponse
		json.NewDecoder(resp.Body).Decode(&mcpResp)
		if mcpResp.Error == nil {
			t.Error("expected parse error")
		}
	})

	t.Run("missing tool name", func(t *testing.T) {
		resp := mcpPost(t, ts.URL+"/mcp/tools/call", MCPRequest{
			JSONRPC: "2.0",
			ID:      1,
			Method:  "tools/call",
			Params:  ToolCallParams{Arguments: map[string]interface{}{}},
		})
		if resp.Error == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("GET on tools/call returns 405", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/mcp/tools/call")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
	})

	t.Run("GET on initialize returns 405", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/mcp/initialize")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Errorf("expected 405, got %d", resp.StatusCode)
		}
	})
}

func mcpPost(t *testing.T, url string, req MCPRequest) MCPResponse {
	t.Helper()
	body, _ := json.Marshal(req)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body)) //nolint:G107 // test helper, URL is from httptest.Server
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	var mcpResp MCPResponse
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		t.Fatal(err)
	}
	return mcpResp
}

func extractCallResult(t *testing.T, resp MCPResponse) string {
	t.Helper()
	resultJSON, _ := json.Marshal(resp.Result)
	var result ToolCallResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("cannot parse call result: %v (raw: %s)", err, string(resultJSON))
	}
	if len(result.Content) == 0 {
		t.Fatal("empty content in call result")
	}
	return result.Content[0].Text
}
