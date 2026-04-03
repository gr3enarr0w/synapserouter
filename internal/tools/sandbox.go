package tools

import (
	"os"
	"os/exec"
	"strings"
)

// SandboxConfig holds configuration for command sandboxing
type SandboxConfig struct {
	Enabled       bool
	WorkDir       string
	AllowedPaths  []string
	NetworkEnabled bool
}

// DefaultSandboxConfig returns a default sandbox configuration
// Sandbox is enabled by default, can be disabled via SYNROUTE_SANDBOX=false
func DefaultSandboxConfig(workDir string) SandboxConfig {
	enabled := true
	if v := os.Getenv("SYNROUTE_SANDBOX"); strings.ToLower(v) == "false" {
		enabled = false
	}

	return SandboxConfig{
		Enabled:        enabled,
		WorkDir:        workDir,
		AllowedPaths:   []string{workDir, "/tmp"},
		NetworkEnabled: false,
	}
}

// WrapCommand wraps a command with OS-native sandboxing if enabled
// On macOS: uses sandbox-exec with Seatbelt profile
// On Linux: uses bwrap (Bubblewrap)
// Returns the original cmd if sandboxing is disabled or unavailable
func WrapCommand(cmd *exec.Cmd, cfg SandboxConfig) *exec.Cmd {
	if !cfg.Enabled {
		return cmd
	}

	return wrapCommandPlatform(cmd, cfg)
}
