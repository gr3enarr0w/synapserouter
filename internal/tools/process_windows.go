//go:build windows

package tools

import (
	"os/exec"
)

// setupProcessGroup sets up process management for the command (Windows-only)
// Windows doesn't support process groups in the same way as Unix,
// so this is a no-op
func setupProcessGroup(cmd *exec.Cmd) {
	// Windows doesn't support Setpgid, so this is a no-op
}

// killProcessGroup kills the process (Windows-only)
// Returns true if the kill was attempted, false if process was nil
func killProcessGroup(cmd *exec.Cmd) bool {
	if cmd.Process == nil {
		return false
	}
	// Windows doesn't support process groups, so we just kill the process directly
	_ = cmd.Process.Kill()
	return true
}
