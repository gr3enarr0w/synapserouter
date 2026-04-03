package agent

import (
	"os"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// TenantConfig holds configuration for tenant-scoped operations
// In single-user mode, UserID is always 'local'
// In multi-user mode, each user gets isolated sessions, memory, and tasks
type TenantConfig struct {
	UserID        string
	SandboxConfig tools.SandboxConfig
	MaxAgents     int
	WorkDir       string
}

// DefaultTenantConfig returns configuration for single-user mode
// All operations are scoped to user_id='local'
func DefaultTenantConfig() TenantConfig {
	workDir, _ := os.Getwd()
	if v := os.Getenv("SYNROUTE_WORKDIR"); v != "" {
		workDir = v
	}

	maxAgents := 5
	if v := os.Getenv("SYNROUTE_MAX_AGENTS"); v != "" {
		if n := parseInt(v); n > 0 {
			maxAgents = n
		}
	}

	return TenantConfig{
		UserID:        "local",
		SandboxConfig: tools.DefaultSandboxConfig(workDir),
		MaxAgents:     maxAgents,
		WorkDir:       workDir,
	}
}

// parseInt is a helper to parse environment variables
func parseInt(s string) int {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
