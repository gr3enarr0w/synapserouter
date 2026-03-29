package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ServerConfig represents a persisted MCP server registration.
type ServerConfig struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	APIKey string `json:"api_key,omitempty"`
}

// MCPConfig holds all registered MCP servers.
type MCPConfig struct {
	Servers []ServerConfig `json:"servers"`
}

// DefaultConfigPath returns ~/.synroute/mcp-servers.json
func DefaultConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".synroute", "mcp-servers.json")
}

// LoadConfig reads MCP server config from the given path.
// Returns an empty config if the file doesn't exist.
func LoadConfig(path string) (*MCPConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &MCPConfig{}, nil
		}
		return nil, fmt.Errorf("read mcp config: %w", err)
	}

	var cfg MCPConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse mcp config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig writes MCP server config to the given path.
func SaveConfig(path string, cfg *MCPConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal mcp config: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}

// AddServerConfig adds a server to the config, replacing if name exists.
func (c *MCPConfig) AddServerConfig(name, url, apiKey string) {
	for i, s := range c.Servers {
		if s.Name == name {
			c.Servers[i] = ServerConfig{Name: name, URL: url, APIKey: apiKey}
			return
		}
	}
	c.Servers = append(c.Servers, ServerConfig{Name: name, URL: url, APIKey: apiKey})
}

// RemoveServerConfig removes a server from the config by name.
func (c *MCPConfig) RemoveServerConfig(name string) bool {
	for i, s := range c.Servers {
		if s.Name == name {
			c.Servers = append(c.Servers[:i], c.Servers[i+1:]...)
			return true
		}
	}
	return false
}

// NewClientFromConfig creates an MCPClient and registers all configured servers.
// Does NOT connect — call ConnectAll separately.
func NewClientFromConfig(cfg *MCPConfig) *MCPClient {
	client := NewMCPClient()
	for _, s := range cfg.Servers {
		client.AddServer(s.Name, s.URL, s.APIKey)
	}
	return client
}
