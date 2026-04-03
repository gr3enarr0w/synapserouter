//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

// setupProcessGroup sets up a new process group for the command (Unix-only)
// This allows killing the entire process tree on timeout
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the entire process group (Unix-only)
// Returns true if the kill was attempted, false if process was nil
func killProcessGroup(cmd *exec.Cmd) bool {
	if cmd.Process == nil {
		return false
	}
	// Kill the entire process group using negative PID
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	// Also kill the process directly to close stdin and unblock pipe goroutines
	_ = cmd.Process.Kill()
	return true
}
