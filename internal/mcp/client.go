package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// MCPClient connects to external MCP servers and manages tool discovery
type MCPClient struct {
	mu         sync.RWMutex
	servers    map[string]*ServerConnection
	tools      map[string]*Tool
	httpClient *http.Client
}

// ServerConnection represents a connection to an MCP server
type ServerConnection struct {
	Name      string
	URL       string
	APIKey    string
	Connected bool
	Tools     []*Tool
	LastPing  time.Time
	SessionID string
	Protocol  string
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
	Success  bool                   `json:"success"`
	Output   interface{}            `json:"output"`
	Error    string                 `json:"error,omitempty"`
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

	tools, protocol, err := c.connectAndDiscover(ctx, conn)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	conn.Connected = true
	conn.Tools = tools
	conn.LastPing = time.Now()
	conn.Protocol = protocol

	// Register tools globally
	for _, tool := range tools {
		tool.ServerName = serverName
		toolKey := fmt.Sprintf("%s.%s", serverName, tool.Name)
		c.tools[toolKey] = tool
	}

	log.Printf("[MCP] Connected to %s, discovered %d tools", serverName, len(tools))

	return nil
}

func (c *MCPClient) connectAndDiscover(ctx context.Context, conn *ServerConnection) ([]*Tool, string, error) {
	if conn.Protocol != "" {
		tools, err := c.discoverToolsWithProtocol(ctx, conn, conn.Protocol)
		if err != nil {
			return nil, "", fmt.Errorf("failed to discover tools: %w", err)
		}
		return tools, conn.Protocol, nil
	}

	if err := c.pingServerJSONRPC(ctx, conn); err == nil {
		tools, discoverErr := c.discoverToolsJSONRPC(ctx, conn)
		if discoverErr == nil {
			return tools, "jsonrpc", nil
		}
	}

	if err := c.pingServerREST(ctx, conn); err != nil {
		return nil, "", fmt.Errorf("failed to ping server: %w", err)
	}
	tools, err := c.discoverToolsREST(ctx, conn)
	if err != nil {
		return nil, "", fmt.Errorf("failed to discover tools: %w", err)
	}
	return tools, "rest", nil
}

func (c *MCPClient) discoverToolsWithProtocol(ctx context.Context, conn *ServerConnection, protocol string) ([]*Tool, error) {
	switch protocol {
	case "jsonrpc":
		if err := c.pingServerJSONRPC(ctx, conn); err != nil {
			return nil, err
		}
		return c.discoverToolsJSONRPC(ctx, conn)
	case "rest":
		if err := c.pingServerREST(ctx, conn); err != nil {
			return nil, err
		}
		return c.discoverToolsREST(ctx, conn)
	default:
		return nil, fmt.Errorf("unknown MCP protocol %q", protocol)
	}
}

// jsonrpcRequest represents a JSON-RPC 2.0 request
type jsonrpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// jsonrpcResponse represents a JSON-RPC 2.0 response
type jsonrpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *struct {
		Code    int         `json:"code"`
		Message string      `json:"message"`
		Data    interface{} `json:"data,omitempty"`
	} `json:"error,omitempty"`
}

// sendJSONRPC sends a JSON-RPC request to the MCP server and returns the response
func (c *MCPClient) sendJSONRPC(ctx context.Context, conn *ServerConnection, method string, params interface{}) (*jsonrpcResponse, error) {
	reqBody := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      time.Now().UnixNano(),
		Method:  method,
		Params:  params,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON-RPC request: %w", err)
	}

	// API key is passed as query parameter for MCP servers like Tavily
	targetURL := conn.URL
	if conn.APIKey != "" {
		sep := "?"
		if strings.Contains(conn.URL, "?") {
			sep = "&"
		}
		targetURL = conn.URL + sep + "tavilyApiKey=" + conn.APIKey
	}

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if conn.SessionID != "" {
		req.Header.Set("mcp-session-id", conn.SessionID)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Capture session ID from response header
	if sessionID := resp.Header.Get("mcp-session-id"); sessionID != "" {
		conn.SessionID = sessionID
	}

	// Parse SSE format: "event: message\ndata: {...}\n\n"
	// Each SSE event has "data:" lines containing JSON
	bodyStr := string(body)
	var jsonData string
	for _, line := range strings.Split(bodyStr, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			dataLine := strings.TrimPrefix(line, "data:")
			jsonData = strings.TrimSpace(dataLine)
		}
	}

	// If no SSE format, try parsing as plain JSON
	var jsonrpcResp jsonrpcResponse
	if jsonData != "" {
		if err := json.Unmarshal([]byte(jsonData), &jsonrpcResp); err != nil {
			return nil, fmt.Errorf("parse JSON-RPC response from SSE: %w", err)
		}
	} else {
		if err := json.Unmarshal(body, &jsonrpcResp); err != nil {
			return nil, fmt.Errorf("parse JSON-RPC response: %w", err)
		}
	}

	if jsonrpcResp.Error != nil {
		return nil, fmt.Errorf("JSON-RPC error %d: %s", jsonrpcResp.Error.Code, jsonrpcResp.Error.Message)
	}

	return &jsonrpcResp, nil
}

// pingServerJSONRPC checks if server is reachable via MCP initialize handshake.
func (c *MCPClient) pingServerJSONRPC(ctx context.Context, conn *ServerConnection) error {
	_, err := c.sendJSONRPC(ctx, conn, "initialize", map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]interface{}{},
		"clientInfo": map[string]interface{}{
			"name":    "synroute",
			"version": "1.0.0",
		},
	})
	return err
}

// discoverToolsJSONRPC fetches available tools from server via JSON-RPC tools/list.
func (c *MCPClient) discoverToolsJSONRPC(ctx context.Context, conn *ServerConnection) ([]*Tool, error) {
	resp, err := c.sendJSONRPC(ctx, conn, "tools/list", map[string]interface{}{})
	if err != nil {
		return nil, err
	}

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid tools/list response format")
	}

	toolsRaw, ok := resultMap["tools"]
	if !ok {
		return nil, fmt.Errorf("no tools in response")
	}

	toolsBytes, err := json.Marshal(toolsRaw)
	if err != nil {
		return nil, fmt.Errorf("marshal tools: %w", err)
	}

	var tools []*Tool
	if err := json.Unmarshal(toolsBytes, &tools); err != nil {
		return nil, fmt.Errorf("unmarshal tools: %w", err)
	}

	return tools, nil
}

func (c *MCPClient) pingServerREST(ctx context.Context, conn *ServerConnection) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, conn.URL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func (c *MCPClient) discoverToolsREST(ctx context.Context, conn *ServerConnection) ([]*Tool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, conn.URL+"/tools/list", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	var result struct {
		Tools []*Tool `json:"tools"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode tools/list response: %w", err)
	}
	return result.Tools, nil
}

// CallTool invokes a tool on the appropriate MCP server via JSON-RPC
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

	if conn.Protocol == "rest" {
		return c.callToolREST(ctx, conn, tool, call)
	}

	// Send JSON-RPC tools/call request
	resp, err := c.sendJSONRPC(ctx, conn, "tools/call", map[string]interface{}{
		"name":      tool.Name,
		"arguments": call.Arguments,
	})
	if err != nil {
		return nil, err
	}

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid tools/call response format")
	}

	// Parse MCP tool result format: {content: [{type: "text", text: "..."}], isError: bool}
	contentRaw, ok := resultMap["content"]
	if !ok {
		return nil, fmt.Errorf("no content in tools/call response")
	}

	contentArr, ok := contentRaw.([]interface{})
	if !ok || len(contentArr) == 0 {
		return nil, fmt.Errorf("invalid content array in tools/call response")
	}

	var textContent string
	for _, item := range contentArr {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if typ, _ := itemMap["type"].(string); typ == "text" {
			textContent, _ = itemMap["text"].(string)
			break
		}
	}

	isError, _ := resultMap["isError"].(bool)

	result := &ToolResult{
		Success: !isError,
		Output:  textContent,
	}

	if isError {
		result.Error = textContent
		log.Printf("[MCP] Tool call failed: %s - %s", call.ToolName, textContent)
		return result, fmt.Errorf("MCP tool %s failed: %s", call.ToolName, textContent)
	}

	return result, nil
}

func (c *MCPClient) callToolREST(ctx context.Context, conn *ServerConnection, tool *Tool, call ToolCall) (*ToolResult, error) {
	bodyBytes, err := json.Marshal(map[string]interface{}{
		"tool":      tool.Name,
		"arguments": call.Arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal tools/call request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, conn.URL+"/tools/call", bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("MCP server returned HTTP %d: %s", resp.StatusCode, string(body))
	}
	var result ToolResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode tools/call response: %w", err)
	}
	if !result.Success && result.Error != "" {
		return &result, fmt.Errorf("MCP tool %s failed: %s", call.ToolName, result.Error)
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

// ConnectAll connects to all registered servers with retry.
// Failures are logged but don't stop other servers from connecting.
// maxRetries controls how many times to retry each failed server (0 = no retry).
func (c *MCPClient) ConnectAll(ctx context.Context, maxRetries int) {
	c.mu.RLock()
	names := make([]string, 0, len(c.servers))
	for name := range c.servers {
		names = append(names, name)
	}
	c.mu.RUnlock()

	for _, name := range names {
		var lastErr error
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if err := c.Connect(ctx, name); err != nil {
				lastErr = err
				log.Printf("[MCP] Connect to %s failed (attempt %d/%d): %v", name, attempt+1, maxRetries+1, err)
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			log.Printf("[MCP] Giving up on %s after %d attempts", name, maxRetries+1)
		}
	}
}

// ServerNames returns the names of all registered servers.
func (c *MCPClient) ServerNames() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	names := make([]string, 0, len(c.servers))
	for name := range c.servers {
		names = append(names, name)
	}
	return names
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
		_, _, err := c.connectAndDiscover(ctx, conn)
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
