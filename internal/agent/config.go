package agent

// Config holds configuration for an agent session.
type Config struct {
	Model        string // LLM model to use (default: "auto")
	SystemPrompt string // Custom system prompt
	MaxTurns     int    // Max tool-call rounds per message (default 25)
	WorkDir      string // Working directory for tool execution
	Streaming    bool   // Enable streaming output
}

// DefaultConfig returns an agent config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Model:    "auto",
		MaxTurns: 25,
		WorkDir:  ".",
	}
}
