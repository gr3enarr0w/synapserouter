package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newMockMCPServer(t *testing.T, tools []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/tools/list":
			result := struct {
				Tools []*Tool `json:"tools"`
			}{}
			for _, name := range tools {
				result.Tools = append(result.Tools, &Tool{
					Name:        name,
					Description: "Mock " + name,
					InputSchema: map[string]interface{}{"type": "object"},
				})
			}
			json.NewEncoder(w).Encode(result)
		case "/tools/call":
			var req struct {
				Tool      string                 `json:"tool"`
				Arguments map[string]interface{} `json:"arguments"`
			}
			json.NewDecoder(r.Body).Decode(&req)
			json.NewEncoder(w).Encode(ToolResult{
				Success: true,
				Output:  "result from " + req.Tool,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestNewMCPClient(t *testing.T) {
	client := NewMCPClient()
	if client == nil {
		t.Fatal("NewMCPClient returned nil")
	}
	if len(client.ListTools()) != 0 {
		t.Fatal("new client should have no tools")
	}
}

func TestAddServer(t *testing.T) {
	client := NewMCPClient()
	if err := client.AddServer("test", "http://localhost:1234", ""); err != nil {
		t.Fatal(err)
	}
	// duplicate should error
	if err := client.AddServer("test", "http://localhost:1234", ""); err == nil {
		t.Fatal("expected error on duplicate server")
	}
}

func TestConnectAndDiscoverTools(t *testing.T) {
	server := newMockMCPServer(t, []string{"bash", "file_read"})
	defer server.Close()

	client := NewMCPClient()
	client.AddServer("test-server", server.URL, "")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Connect(ctx, "test-server"); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	tools := client.ListTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
}

func TestCallTool(t *testing.T) {
	server := newMockMCPServer(t, []string{"bash"})
	defer server.Close()

	client := NewMCPClient()
	client.AddServer("test-server", server.URL, "")

	ctx := context.Background()
	client.Connect(ctx, "test-server")

	result, err := client.CallTool(ctx, ToolCall{
		ToolName:  "test-server.bash",
		Arguments: map[string]interface{}{"command": "echo hello"},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
}

func TestCallToolNotFound(t *testing.T) {
	client := NewMCPClient()
	_, err := client.CallTool(context.Background(), ToolCall{ToolName: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent tool")
	}
}

func TestConnectAll(t *testing.T) {
	server := newMockMCPServer(t, []string{"bash"})
	defer server.Close()

	client := NewMCPClient()
	client.AddServer("s1", server.URL, "")
	client.AddServer("s2", "http://localhost:1", "") // will fail

	ctx := context.Background()
	client.ConnectAll(ctx, 0)

	status := client.GetServerStatus()
	if !status["s1"].Connected {
		t.Fatal("s1 should be connected")
	}
	if status["s2"].Connected {
		t.Fatal("s2 should not be connected")
	}
}

func TestDisconnect(t *testing.T) {
	server := newMockMCPServer(t, []string{"bash"})
	defer server.Close()

	client := NewMCPClient()
	client.AddServer("test-server", server.URL, "")
	client.Connect(context.Background(), "test-server")

	if len(client.ListTools()) == 0 {
		t.Fatal("should have tools after connect")
	}

	client.Disconnect("test-server")
	if len(client.ListTools()) != 0 {
		t.Fatal("should have no tools after disconnect")
	}
}

func TestHealthCheck(t *testing.T) {
	server := newMockMCPServer(t, []string{"bash"})
	defer server.Close()

	client := NewMCPClient()
	client.AddServer("healthy", server.URL, "")
	client.AddServer("unhealthy", "http://localhost:1", "")

	health := client.HealthCheck(context.Background())
	if !health["healthy"] {
		t.Fatal("healthy server should be healthy")
	}
	if health["unhealthy"] {
		t.Fatal("unhealthy server should not be healthy")
	}
}

func TestServerNames(t *testing.T) {
	client := NewMCPClient()
	client.AddServer("a", "http://a", "")
	client.AddServer("b", "http://b", "")

	names := client.ServerNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 names, got %d", len(names))
	}
}

func TestConfigLoadSave(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "mcp-servers.json")

	// Load nonexistent — empty config
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 0 {
		t.Fatal("expected empty config")
	}

	// Add and save
	cfg.AddServerConfig("test", "http://localhost:8080", "key123")
	if err := SaveConfig(cfgPath, cfg); err != nil {
		t.Fatal(err)
	}

	// Reload
	cfg2, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg2.Servers) != 1 {
		t.Fatal("expected 1 server")
	}
	if cfg2.Servers[0].Name != "test" || cfg2.Servers[0].APIKey != "key123" {
		t.Fatal("wrong server data")
	}
}

func TestConfigAddReplace(t *testing.T) {
	cfg := &MCPConfig{}
	cfg.AddServerConfig("test", "http://old", "")
	cfg.AddServerConfig("test", "http://new", "key")
	if len(cfg.Servers) != 1 {
		t.Fatal("should replace, not duplicate")
	}
	if cfg.Servers[0].URL != "http://new" {
		t.Fatal("should have updated URL")
	}
}

func TestConfigRemove(t *testing.T) {
	cfg := &MCPConfig{}
	cfg.AddServerConfig("test", "http://test", "")
	if !cfg.RemoveServerConfig("test") {
		t.Fatal("should return true on removal")
	}
	if cfg.RemoveServerConfig("test") {
		t.Fatal("should return false when not found")
	}
}

func TestNewClientFromConfig(t *testing.T) {
	server := newMockMCPServer(t, []string{"bash"})
	defer server.Close()

	cfg := &MCPConfig{
		Servers: []ServerConfig{
			{Name: "test", URL: server.URL},
		},
	}

	client := NewClientFromConfig(cfg)
	names := client.ServerNames()
	if len(names) != 1 {
		t.Fatalf("expected 1 server, got %d", len(names))
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".synroute", "mcp-servers.json")
	if path != expected {
		t.Fatalf("expected %s, got %s", expected, path)
	}
}
