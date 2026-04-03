//go:build linux

package tools

import (
	"os/exec"
	"strings"
)

// wrapCommandPlatform wraps a command with Linux Bubblewrap sandboxing
func wrapCommandPlatform(cmd *exec.Cmd, cfg SandboxConfig) *exec.Cmd {
	// Check if bwrap is available
	bwrapPath, err := exec.LookPath("bwrap")
	if err != nil {
		// bwrap not available, run unsandboxed
		return cmd
	}

	// Build bwrap arguments
	args := []string{}

	// Bind root filesystem as read-only
	args = append(args, "--ro-bind", "/", "/")

	// Bind work directory as read-write
	if cfg.WorkDir != "" {
		args = append(args, "--bind", cfg.WorkDir, cfg.WorkDir)
	}

	// Bind /tmp as read-write
	args = append(args, "--bind", "/tmp", "/tmp")

	// Bind /dev for device access
	args = append(args, "--dev", "/dev")

	// Bind /proc for process information
	args = append(args, "--proc", "/proc")

	// Network isolation
	if !cfg.NetworkEnabled {
		args = append(args, "--unshare-net")
	}

	// Allow access to additional paths
	for _, path := range cfg.AllowedPaths {
		if path != cfg.WorkDir && path != "/tmp" {
			args = append(args, "--bind", path, path)
		}
	}

	// Add the original command
	args = append(args, cmd.Path)
	args = append(args, cmd.Args[1:]...)

	// Create wrapped command
	wrappedCmd := exec.Command(bwrapPath, args...)

	// Copy environment
	wrappedCmd.Env = cmd.Env
	wrappedCmd.Dir = cmd.Dir
	wrappedCmd.Stdin = cmd.Stdin
	wrappedCmd.Stdout = cmd.Stdout
	wrappedCmd.Stderr = cmd.Stderr

	return wrappedCmd
}

// isBwrapAvailable checks if bubblewrap is installed and functional
func isBwrapAvailable() bool {
	_, err := exec.LookPath("bwrap")
	return err == nil
}

// getBwrapVersion returns the bubblewrap version string
func getBwrapVersion() (string, error) {
	cmd := exec.Command("bwrap", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}
