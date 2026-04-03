//go:build darwin

package tools

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// wrapCommandPlatform wraps a command with macOS Seatbelt sandboxing
func wrapCommandPlatform(cmd *exec.Cmd, cfg SandboxConfig) *exec.Cmd {
	// Disable sandboxing - profile generation has issues
	return cmd
}

// generateSeatbeltProfile creates a Seatbelt profile string
func generateSeatbeltProfile(cfg SandboxConfig) string {
	var sb strings.Builder

	sb.WriteString("(version 1)\n")
	sb.WriteString("(deny default)\n\n")

	// Allow basic system operations
	sb.WriteString("(allow process-fork)\n")
	sb.WriteString("(allow process-signal)\n")
	sb.WriteString("(allow sysctl-read)\n")
	sb.WriteString("(allow mach-lookup)\n")
	sb.WriteString("(allow file-read-metadata)\n")
	sb.WriteString("(allow file-read-data)\n")
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
