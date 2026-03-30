package agent

import (
	"fmt"
	"io"
	"strings"
)

// TerminalRenderer is the interface for agent output rendering.
// Both the simple Renderer (synroute chat) and CodeRenderer (synroute code)
// implement this interface.
type TerminalRenderer interface {
	Text(text string)
	ToolCall(name string, args map[string]interface{})
	ToolResult(name string, result string, isError bool)
	ToolDiff(path, oldText, newText string)
	Error(msg string)
	Prompt()
}

// Renderer handles terminal output formatting for agent interactions.
type Renderer struct {
	out io.Writer
}

// NewRenderer creates a renderer that writes to the given writer.
func NewRenderer(out io.Writer) *Renderer {
	return &Renderer{out: out}
}

// Text outputs a text response.
func (r *Renderer) Text(text string) {
	fmt.Fprintln(r.out, text)
}

// ToolCall displays a tool call being executed.
func (r *Renderer) ToolCall(name string, args map[string]interface{}) {
	summary := formatToolCallSummary(name, args)
	fmt.Fprintf(r.out, "\033[36m[%s]\033[0m %s\n", name, summary)
}

// ToolResult displays the result of a tool call.
func (r *Renderer) ToolResult(name string, result string, isError bool) {
	if result == "" {
		return
	}
	// Indent tool output
	lines := strings.Split(result, "\n")
	maxLines := 20
	if len(lines) > maxLines {
		for _, line := range lines[:maxLines] {
			fmt.Fprintf(r.out, "  %s\n", line)
		}
		fmt.Fprintf(r.out, "  ... (%d more lines)\n", len(lines)-maxLines)
	} else {
		for _, line := range lines {
			fmt.Fprintf(r.out, "  %s\n", line)
		}
	}
}

// ToolDiff displays a colored unified diff for file edits.
func (r *Renderer) ToolDiff(path, oldText, newText string) {
	fmt.Fprintf(r.out, "\033[2m── file_edit: %s ──\033[0m\n", path)

	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")

	maxLines := 30
	count := 0

	for _, line := range oldLines {
		if count >= maxLines {
			fmt.Fprintf(r.out, "  \033[2m... (%d more removed lines)\033[0m\n", len(oldLines)-count)
			break
		}
		fmt.Fprintf(r.out, "  \033[31m- %s\033[0m\n", line)
		count++
	}

	count = 0
	for _, line := range newLines {
		if count >= maxLines {
			fmt.Fprintf(r.out, "  \033[2m... (%d more added lines)\033[0m\n", len(newLines)-count)
			break
		}
		fmt.Fprintf(r.out, "  \033[32m+ %s\033[0m\n", line)
		count++
	}
}

// Error displays an error message.
func (r *Renderer) Error(msg string) {
	fmt.Fprintf(r.out, "\033[31merror:\033[0m %s\n", msg)
}

// Prompt displays the input prompt.
func (r *Renderer) Prompt() {
	fmt.Fprint(r.out, "\033[32msynroute>\033[0m ")
}

func formatToolCallSummary(name string, args map[string]interface{}) string {
	switch name {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 80 {
				return cmd[:80] + "..."
			}
			return cmd
		}
	case "file_read":
		return fmt.Sprintf("%v", args["path"])
	case "file_write":
		return fmt.Sprintf("%v", args["path"])
	case "file_edit":
		return fmt.Sprintf("%v", args["path"])
	case "grep":
		return fmt.Sprintf("%v in %v", args["pattern"], args["path"])
	case "glob":
		return fmt.Sprintf("%v", args["pattern"])
	case "git":
		sub := args["subcommand"]
		extra := args["args"]
		if extra != nil {
			return fmt.Sprintf("%v %v", sub, extra)
		}
		return fmt.Sprintf("%v", sub)
	}
	return fmt.Sprintf("%v", args)
}
