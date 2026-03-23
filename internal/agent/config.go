package agent

import (
	"database/sql"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/mcp"
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
)

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

	// Skill dispatch — matches user messages against skill triggers and
	// injects matched skill instructions into the system prompt.
	Skills          []orchestration.Skill
	MCPClient       *mcp.MCPClient // MCP client for auto-invoking skill-bound tools
	AutoOrchestrate bool           // Enable auto-mode switching + auto-review + escalation

	// TargetProvider forces all LLM calls to a specific provider by name.
	// Used by sub-agents that need to hit a specific Ollama model.
	TargetProvider string

	// EscalationChain is the ordered list of escalation levels. Built from
	// OLLAMA_CHAIN env var (pipe-separated levels, comma-separated models).
	// Each level's models rotate (cross-review in 2 stages).
	EscalationChain []EscalationLevel

	// Observability
	EventBus  *EventBus // Real-time event bus (nil = no events)
	Verbosity int       // 0=compact, 1=normal, 2=verbose

	// Project context
	ProjectLanguage string // Declared language from spec or detection (overrides Detect())

	// State persistence
	DB        *sql.DB // SQLite database for session persistence
	Resume    bool    // Resume most recent session on startup
	SessionID string  // Resume a specific session ID
}

// EscalationLevel represents one level in the escalation chain.
// Each level has one or more provider names. Currently most levels have 1,
// but the architecture supports X parallel models per level.
type EscalationLevel struct {
	Providers []string
}

// DefaultConfig returns an agent config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:           "auto",
		MaxTurns:        0, // 0 = unlimited; use AgentBudget.MaxTurns for sub-agent limits
		WorkDir:         ".",
		MaxAgents:       3, // match Ollama Cloud Pro concurrent limit
		Skills:          orchestration.DefaultSkills(),
		AutoOrchestrate: false, // set to true in cmdChat for production use
		// EscalationChain is set at startup from router.ProviderNames()
	}
}
