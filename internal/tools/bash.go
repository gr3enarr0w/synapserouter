package tools

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const (
	defaultBashTimeout = 120 * time.Second
	maxBashTimeout     = 600 * time.Second
	maxOutputBytes     = 1 << 20 // 1MB
	maxConcurrentBash  = 10
)

// bashSemaphore limits concurrent shell executions.
var bashSemaphore = make(chan struct{}, maxConcurrentBash)

// BashTool executes shell commands via sh -c.
type BashTool struct{}

func (t *BashTool) Name() string        { return "bash" }
func (t *BashTool) Description() string { return "Execute a shell command and return its output" }
func (t *BashTool) Category() ToolCategory { return CategoryWrite }

func (t *BashTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"timeout_ms": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in milliseconds (default 120000, max 600000)",
			},
		},
		"required": []string{"command"},
	}
}

func (t *BashTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return &ToolResult{Error: "command is required"}, nil
	}

	timeout := defaultBashTimeout
	if ms, ok := args["timeout_ms"].(float64); ok && ms > 0 {
		timeout = time.Duration(ms) * time.Millisecond
		if timeout > maxBashTimeout {
			timeout = maxBashTimeout
		}
	}

	// Acquire semaphore to limit concurrent executions
	select {
	case bashSemaphore <- struct{}{}:
		defer func() { <-bashSemaphore }()
	case <-ctx.Done():
		return &ToolResult{Error: "too many concurrent commands", ExitCode: -1}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workDir
	cmd.Env = filteredEnv()
	// Set process group so we can kill the entire tree on timeout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdout, max: maxOutputBytes}
	cmd.Stderr = &limitWriter{w: &stderr, max: maxOutputBytes}

	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else if ctx.Err() != nil {
			// Kill the process group on timeout or cancellation
			if cmd.Process != nil {
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			}
			msg := "command cancelled"
			if ctx.Err() == context.DeadlineExceeded {
				msg = fmt.Sprintf("command timed out after %s", timeout)
			}
			return &ToolResult{
				Output:   stdout.String(),
				Error:    msg,
				ExitCode: -1,
			}, nil
		} else {
			return &ToolResult{Error: err.Error(), ExitCode: -1}, nil
		}
	}

	output := stdout.String()
	if errStr := stderr.String(); errStr != "" {
		if output != "" {
			output += "\n"
		}
		output += errStr
	}

	result := &ToolResult{Output: output, ExitCode: exitCode}
	if exitCode != 0 {
		result.Error = fmt.Sprintf("exit code %d", exitCode)
	}
	return result, nil
}

// limitWriter wraps a writer and stops writing after max bytes.
type limitWriter struct {
	w         *bytes.Buffer
	max       int
	written   int
	truncated bool
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	remaining := lw.max - lw.written
	if remaining <= 0 {
		return len(p), nil // discard but don't error
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := lw.w.Write(p)
	lw.written += n
	if lw.written >= lw.max && !lw.truncated && err == nil {
		lw.truncated = true
		lw.w.WriteString("...\n[output truncated at 1MB]")
	}
	return len(p), err
}

// safeEnvPrefixes lists environment variable prefixes that are safe to pass to child processes.
var safeEnvPrefixes = []string{
	"PATH=", "HOME=", "USER=", "LOGNAME=", "SHELL=",
	"LANG=", "LC_", "TERM=", "TMPDIR=", "TMP=", "TEMP=",
	"GOPATH=", "GOROOT=", "GOBIN=", "GOPROXY=", "GOFLAGS=",
	"XDG_", "DISPLAY=", "SSH_AUTH_SOCK=", "EDITOR=", "VISUAL=",
	"COLORTERM=", "CLICOLOR=", "NO_COLOR=",
	"GIT_AUTHOR_", "GIT_COMMITTER_", "GIT_EDITOR=",
	"PWD=", "OLDPWD=", "SHLVL=", "HOSTNAME=",
}

// filteredEnv returns a copy of the environment with secret-bearing variables removed.
func filteredEnv() []string {
	var filtered []string
	for _, env := range os.Environ() {
		safe := false
		for _, prefix := range safeEnvPrefixes {
			if strings.HasPrefix(env, prefix) {
				safe = true
				break
			}
		}
		if safe {
			filtered = append(filtered, env)
		}
	}
	return filtered
}
