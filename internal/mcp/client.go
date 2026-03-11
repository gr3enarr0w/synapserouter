package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"
)

// MCPClient connects to external MCP servers and manages tool discovery
type MCPClient struct {
	mu          sync.RWMutex
	servers     map[string]*ServerConnection
	tools       map[string]*Tool
	httpClient  *http.Client
}

// ServerConnection represents a connection to an MCP server
type ServerConnection struct {
	Name      string
	URL       string
	APIKey    string
	Connected bool
	Tools     []*Tool
	LastPing  time.Time
}

// Tool represents a discovered MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
	ServerName  string                 `json:"serverName"`
}

// ToolCall represents a tool invocation request
type ToolCall struct {
	ToolName  string                 `json:"tool_name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResult represents a tool invocation result
type ToolResult struct {
	Success bool                   `json:"success"`
	Output  interface{}            `json:"output"`
	Error   string                 `json:"error,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// NewMCPClient creates a new MCP client
func NewMCPClient() *MCPClient {
	return &MCPClient{
		servers:    make(map[string]*ServerConnection),
		tools:      make(map[string]*Tool),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// AddServer registers a new MCP server
func (c *MCPClient) AddServer(name, url, apiKey string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.servers[name]; exists {
		return fmt.Errorf("server %s already registered", name)
	}

	conn := &ServerConnection{
		Name:   name,
		URL:    url,
		APIKey: apiKey,
	}

	c.servers[name] = conn
	log.Printf("[MCP] Registered server: %s at %s", name, url)

	return nil
}

// Connect establishes connection and discovers tools from an MCP server
func (c *MCPClient) Connect(ctx context.Context, serverName string) error {
	c.mu.RLock()
	conn, exists := c.servers[serverName]
	c.mu.RUnlock()

	if !exists {
		return fmt.Errorf("server %s not registered", serverName)
	}

	// Ping server
	if err := c.pingServer(ctx, conn); err != nil {
		return fmt.Errorf("failed to ping server: %w", err)
	}

	// Discover tools
	tools, err := c.discoverTools(ctx, conn)
	if err != nil {
		return fmt.Errorf("failed to discover tools: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	conn.Connected = true
	conn.Tools = tools
	conn.LastPing = time.Now()

	// Register tools globally
	for _, tool := range tools {
		tool.ServerName = serverName
		toolKey := fmt.Sprintf("%s.%s", serverName, tool.Name)
		c.tools[toolKey] = tool
	}

	log.Printf("[MCP] Connected to %s, discovered %d tools", serverName, len(tools))

	return nil
}

// pingServer checks if server is reachable
func (c *MCPClient) pingServer(ctx context.Context, conn *ServerConnection) error {
	req, err := http.NewRequestWithContext(ctx, "GET", conn.URL+"/health", nil)
	if err != nil {
		return err
	}

	if conn.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+conn.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ping failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// discoverTools fetches available tools from server
func (c *MCPClient) discoverTools(ctx context.Context, conn *ServerConnection) ([]*Tool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", conn.URL+"/tools/list", nil)
	if err != nil {
		return nil, err
	}

	if conn.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+conn.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tool discovery failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Tools []*Tool `json:"tools"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Tools, nil
}

// CallTool invokes a tool on the appropriate MCP server
func (c *MCPClient) CallTool(ctx context.Context, call ToolCall) (*ToolResult, error) {
	c.mu.RLock()
	tool, exists := c.tools[call.ToolName]
	c.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("tool not found: %s", call.ToolName)
	}

	c.mu.RLock()
	conn, exists := c.servers[tool.ServerName]
	c.mu.RUnlock()

	if !exists || !conn.Connected {
		return nil, fmt.Errorf("server %s not connected", tool.ServerName)
	}

	// Prepare request
	reqBody, err := json.Marshal(map[string]interface{}{
		"tool": tool.Name,
		"arguments": call.Arguments,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", conn.URL+"/tools/call", bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if conn.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+conn.APIKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result ToolResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if !result.Success && result.Error != "" {
		log.Printf("[MCP] Tool call failed: %s - %s", call.ToolName, result.Error)
	}

	return &result, nil
}

// ListTools returns all available tools
func (c *MCPClient) ListTools() []*Tool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tools := make([]*Tool, 0, len(c.tools))
	for _, tool := range c.tools {
		tools = append(tools, tool)
	}

	return tools
}

// GetTool retrieves a specific tool
func (c *MCPClient) GetTool(toolName string) (*Tool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	tool, exists := c.tools[toolName]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", toolName)
	}

	return tool, nil
}

// Disconnect disconnects from an MCP server
func (c *MCPClient) Disconnect(serverName string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	conn, exists := c.servers[serverName]
	if !exists {
		return fmt.Errorf("server %s not registered", serverName)
	}

	// Remove tools from global registry
	for toolKey := range c.tools {
		if c.tools[toolKey].ServerName == serverName {
			delete(c.tools, toolKey)
		}
	}

	conn.Connected = false
	conn.Tools = nil

	log.Printf("[MCP] Disconnected from %s", serverName)

	return nil
}

// HealthCheck checks connectivity to all registered servers
func (c *MCPClient) HealthCheck(ctx context.Context) map[string]bool {
	c.mu.RLock()
	servers := make([]*ServerConnection, 0, len(c.servers))
	for _, conn := range c.servers {
		servers = append(servers, conn)
	}
	c.mu.RUnlock()

	health := make(map[string]bool)

	for _, conn := range servers {
		err := c.pingServer(ctx, conn)
		health[conn.Name] = (err == nil)

		if err == nil {
			c.mu.Lock()
			conn.LastPing = time.Now()
			c.mu.Unlock()
		}
	}

	return health
}

// GetServerStatus returns status of all registered servers
func (c *MCPClient) GetServerStatus() map[string]*ServerConnection {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := make(map[string]*ServerConnection)
	for name, conn := range c.servers {
		status[name] = &ServerConnection{
			Name:      conn.Name,
			URL:       conn.URL,
			Connected: conn.Connected,
			LastPing:  conn.LastPing,
			Tools:     append([]*Tool(nil), conn.Tools...),
		}
	}

	return status
}
