//go:build windows

package tools

import (
	"os/exec"
)

// wrapCommandPlatform wraps a command with Windows-native sandboxing
// Windows doesn't have native sandboxing like macOS Seatbelt or Linux Bubblewrap,
// so this returns the original command unchanged
func wrapCommandPlatform(cmd *exec.Cmd, cfg SandboxConfig) *exec.Cmd {
	// No sandboxing available on Windows, return cmd unchanged
	return cmd
}
