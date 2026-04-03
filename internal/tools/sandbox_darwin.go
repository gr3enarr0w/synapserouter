//go:build darwin

package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// wrapCommandPlatform wraps a command with macOS Seatbelt sandboxing
func wrapCommandPlatform(cmd *exec.Cmd, cfg SandboxConfig) *exec.Cmd {
	// Check if sandbox-exec is available
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		// sandbox-exec not available, run unsandboxed
		return cmd
	}

	// Generate Seatbelt profile
	profile := generateSeatbeltProfile(cfg)

	// Write profile to temp file
	tmpFile, err := os.CreateTemp("", "synroute-sandbox-*.sb")
	if err != nil {
		// Can't create temp file, run unsandboxed
		return cmd
	}

	if _, err := tmpFile.WriteString(profile); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return cmd
	}
	tmpFile.Close()

	// Wrap command: sandbox-exec -f <profile> <original command>
	originalCmd := append([]string{cmd.Path}, cmd.Args[1:]...)
	wrappedCmd := exec.Command("sandbox-exec", append([]string{"-f", tmpFile.Name()}, originalCmd...)...)

	// Copy environment
	wrappedCmd.Env = cmd.Env
	wrappedCmd.Dir = cmd.Dir
	wrappedCmd.Stdin = cmd.Stdin
	wrappedCmd.Stdout = cmd.Stdout
	wrappedCmd.Stderr = cmd.Stderr

	// Schedule temp file cleanup after command completes
	go func() {
		// Wait a bit for sandbox-exec to start reading the file
		// Then clean up
		<-time.After(5 * time.Second)
		os.Remove(tmpFile.Name())
	}()

	return wrappedCmd
}

// generateSeatbeltProfile creates a Seatbelt profile string
func generateSeatbeltProfile(cfg SandboxConfig) string {
	var sb strings.Builder

	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n\n")

	// Allow basic system operations
	sb.WriteString("(allow process-fork)\n")
	sb.WriteString("(allow signal)\n")
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow mach-lookup)\n")
	sb.WriteString("(allow file-read-metadata)\n")
	sb.WriteString("(allow file-read-data)\n")
	sb.WriteString("(allow process-signal)\n")
	sb.WriteString("(allow network-outbound)\n")

	// Read-only access to root filesystem
	sb.WriteString("(allow file-read-metadata (subpath \"/\"))\n")
	sb.WriteString("(allow file-read-data (subpath \"/\"))\n")

	// Read-write access to work directory
	if cfg.WorkDir != "" {
		absWorkDir, err := filepath.Abs(cfg.WorkDir)
		if err == nil {
			sb.WriteString(fmt.Sprintf("(allow file-write-data (subpath \"%s\"))\n", absWorkDir))
			sb.WriteString(fmt.Sprintf("(allow file-create (subpath \"%s\"))\n", absWorkDir))
			sb.WriteString(fmt.Sprintf("(allow file-delete (subpath \"%s\"))\n", absWorkDir))
		}
	}

	// Read-write access to /tmp
	sb.WriteString("(allow file-write-data (subpath \"/tmp\"))\n")
	sb.WriteString("(allow file-create (subpath \"/tmp\"))\n")
	sb.WriteString("(allow file-delete (subpath \"/tmp\"))\n")

	// Allow access to allowed paths
	for _, path := range cfg.AllowedPaths {
		if path != cfg.WorkDir && path != "/tmp" {
			sb.WriteString(fmt.Sprintf("(allow file-write-data (subpath \"%s\"))\n", path))
		}
	}

	// Network access
	if !cfg.NetworkEnabled {
		sb.WriteString("(deny network-outbound)\n")
		sb.WriteString("(deny network-inbound)\n")
	}

	// Allow access to essential system paths
	sb.WriteString("(allow file-read-metadata (subpath \"/usr\"))\n")
	sb.WriteString("(allow file-read-data (subpath \"/usr\"))\n")
	sb.WriteString("(allow file-read-metadata (subpath \"/bin\"))\n")
	sb.WriteString("(allow file-read-data (subpath \"/bin\"))\n")
	sb.WriteString("(allow file-read-metadata (subpath \"/private\"))\n")
	sb.WriteString("(allow file-read-data (subpath \"/private\"))\n")
	sb.WriteString("(allow file-read-metadata (subpath \"/var\"))\n")
	sb.WriteString("(allow file-read-data (subpath \"/var\"))\n")

	// Allow execution of binaries in standard paths
	sb.WriteString("(allow file-execute (subpath \"/usr\"))\n")
	sb.WriteString("(allow file-execute (subpath \"/bin\"))\n")
	sb.WriteString("(allow file-execute (subpath \"/private\"))\n")

	return sb.String()
}
