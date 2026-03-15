package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// GitTool provides git operations: status, diff, log, add, commit, branch, push, stash.
type GitTool struct{}

func (t *GitTool) Name() string        { return "git" }
func (t *GitTool) Description() string { return "Run git operations (status, diff, log, add, commit, branch, push, stash)" }
func (t *GitTool) Category() ToolCategory { return CategoryWrite }

func (t *GitTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"subcommand": map[string]interface{}{
				"type":        "string",
				"description": "Git subcommand: status, diff, log, add, commit, branch, checkout, push, pull, stash, show, rev-parse",
				"enum":        []string{"status", "diff", "log", "add", "commit", "branch", "checkout", "push", "pull", "stash", "show", "rev-parse"},
			},
			"args": map[string]interface{}{
				"type":        "string",
				"description": "Additional arguments for the git command",
			},
		},
		"required": []string{"subcommand"},
	}
}

// allowedGitSubcommands is the set of git subcommands the tool will execute.
var allowedGitSubcommands = map[string]bool{
	"status": true, "diff": true, "log": true, "add": true,
	"commit": true, "branch": true, "checkout": true, "push": true,
	"pull": true, "stash": true, "show": true, "rev-parse": true,
}

// dangerousGitFlags lists flags that require explicit approval via the bash tool.
var dangerousGitFlags = map[string][]string{
	"push":     {"--force", "-f", "--force-with-lease", "--force-if-includes"},
	"checkout": {"--force", "-f"},
	"branch":   {"-D"},
	"reset":    {"--hard"},
	"clean":    {"-f", "-fd", "-fdx"},
	"stash":    {"drop", "clear"},
}

func (t *GitTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	subcommand := stringArg(args, "subcommand")
	if subcommand == "" {
		return &ToolResult{Error: "subcommand is required"}, nil
	}

	if !allowedGitSubcommands[subcommand] {
		return &ToolResult{
			Error: fmt.Sprintf("git subcommand %q is not allowed — use bash tool for advanced git operations", subcommand),
		}, nil
	}

	extraArgs := stringArg(args, "args")
	tokens := strings.Fields(extraArgs)

	// Block dangerous flags (check each token individually)
	if dangerous, ok := dangerousGitFlags[subcommand]; ok {
		for _, token := range tokens {
			for _, flag := range dangerous {
				if token == flag {
					return &ToolResult{
						Error: fmt.Sprintf("dangerous flag %q blocked for git %s — use bash tool with explicit approval", flag, subcommand),
					}, nil
				}
			}
		}
	}

	// Block force-push via refspec (args starting with +)
	if subcommand == "push" {
		for _, token := range tokens {
			if strings.HasPrefix(token, "+") && !strings.HasPrefix(token, "+-") {
				return &ToolResult{
					Error: fmt.Sprintf("force-push refspec %q blocked — use bash tool with explicit approval", token),
				}, nil
			}
		}
	}

	gitArgs := []string{subcommand}
	gitArgs = append(gitArgs, tokens...)

	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitWriter{w: &stdout, max: maxOutputBytes}
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := strings.TrimRight(stdout.String(), "\n")

	if err != nil {
		errMsg := strings.TrimRight(stderr.String(), "\n")
		if errMsg == "" {
			errMsg = err.Error()
		}
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		return &ToolResult{
			Output:   output,
			Error:    errMsg,
			ExitCode: exitCode,
		}, nil
	}

	if errStr := strings.TrimRight(stderr.String(), "\n"); errStr != "" && output != "" {
		output += "\n" + errStr
	} else if output == "" {
		output = strings.TrimRight(stderr.String(), "\n")
	}

	if output == "" {
		output = "(no output)"
	}

	return &ToolResult{Output: output}, nil
}
