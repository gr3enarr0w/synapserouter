package agent

import "database/sql"

// Config holds configuration for an agent session.
type Config struct {
	Model        string // LLM model to use (default: "auto")
	SystemPrompt string // Custom system prompt
	MaxTurns     int    // Max tool-call rounds per message (default 25)
	WorkDir      string // Working directory for tool execution
	Streaming    bool   // Enable streaming output

	// Sub-agent configuration
	MaxAgents int          // Max concurrent sub-agents (default 5)
	Budget    *AgentBudget // Resource limits for this agent

	// State persistence
	DB        *sql.DB // SQLite database for session persistence
	Resume    bool    // Resume most recent session on startup
	SessionID string  // Resume a specific session ID
}

// DefaultConfig returns an agent config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:     "auto",
		MaxTurns:  25,
		WorkDir:   ".",
		MaxAgents: 5,
	}
}
