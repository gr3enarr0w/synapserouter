package agent

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// DefaultPermissionPrompt creates a permission prompt function that writes
// to the given output and reads from the given input. Suitable for both
// code mode and chat mode interactive sessions.
//
// The prompt shows the tool name and key arguments, and accepts:
//   - y/yes/enter: approve this tool call
//   - n/no: deny this tool call
//   - a/all: approve all remaining tool calls in this session
//
// Note: in code mode, readline owns stdin. The permission prompt uses
// a fresh file descriptor for /dev/tty to avoid conflicting with readline.
func DefaultPermissionPrompt(out io.Writer, in io.Reader) tools.PermissionPromptFunc {
	var mu sync.Mutex
	approveAll := false

	return func(toolName string, category tools.ToolCategory, args map[string]interface{}) bool {
		mu.Lock()
		defer mu.Unlock()

		if approveAll {
			return true
		}

		noColor := os.Getenv("NO_COLOR") != ""
		c := func(code, text string) string {
			if noColor {
				return text
			}
			return code + text + "\033[0m"
		}

		// Format tool call summary
		summary := formatPermissionSummary(toolName, args)
		label := "write"
		if category == tools.CategoryDangerous {
			label = "dangerous"
		}

		fmt.Fprintln(out)
		fmt.Fprintln(out, c("\033[1;33m", fmt.Sprintf("  ⚠ %s tool: %s", label, toolName)))
		if summary != "" {
			fmt.Fprintln(out, c("\033[2m", "  "+summary))
		}
		fmt.Fprint(out, c("\033[33m", "  Allow? [y/n/a] "))

		// Read user response from /dev/tty.
		// Terminal is in raw mode (readline), so we read single bytes.
		// Flush any pending input first (cursor position reports etc).
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			log.Printf("[Permission] can't open /dev/tty: %v — auto-approving", err)
			return true
		}
		defer tty.Close()

		// Drain any pending bytes (readline cursor queries produce \033[...R responses)
		_ = tty.SetDeadline(time.Now().Add(100 * time.Millisecond))
		drain := make([]byte, 64)
		_, _ = tty.Read(drain)

		// Now read the actual user input with a proper timeout
		_ = tty.SetDeadline(time.Now().Add(30 * time.Second))
		buf := make([]byte, 1)
		for {
			n, err := tty.Read(buf)
			if err != nil || n == 0 {
				return true // timeout or error — auto-approve
			}
			// Skip escape sequences and non-printable chars
			if buf[0] < 0x20 && buf[0] != '\r' && buf[0] != '\n' && buf[0] != 0x03 {
				continue // skip control chars (escape sequences from readline)
			}
			break
		}

		fmt.Fprintln(out) // newline after the single char
		return parsePermissionByte(buf[0], &approveAll)
	}
}

// CodeModePermissionPrompt renders permission requests through the code-mode
// renderer instead of using a separate /dev/tty prompt.
func CodeModePermissionPrompt(renderer *CodeRenderer, in io.Reader) tools.PermissionPromptFunc {
	var mu sync.Mutex
	approveAll := false
	reader := bufio.NewReader(in)

	return func(toolName string, category tools.ToolCategory, args map[string]interface{}) bool {
		mu.Lock()
		defer mu.Unlock()

		if approveAll {
			return true
		}

		label := "write"
		if category == tools.CategoryDangerous {
			label = "dangerous"
		}
		summary := formatPermissionSummary(toolName, args)

		renderer.mu.Lock()
		renderer.writeContent("")
		renderer.writeContent(renderer.color("\033[1;33m", fmt.Sprintf("  [permission] %s tool: %s", label, toolName)))
		if summary != "" {
			for _, line := range strings.Split(summary, "\n") {
				renderer.writeContent(renderer.color("\033[2m", "  "+line))
			}
		}
		renderer.footerNote = renderer.color("\033[33m", "  Allow? [y/n/a]")
		renderer.inputLine = ""
		renderer.inputActive = true
		renderer.renderFooterLocked(true)
		renderer.mu.Unlock()

		for {
			resp, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				log.Printf("[Permission] stdin read failed: %v — auto-approving", err)
				renderer.SetFooterNote("")
				return true
			}
			resp = strings.TrimSpace(resp)
			if resp == "" {
				renderer.SetFooterNote("")
				return parsePermissionByte('\n', &approveAll)
			}
			if approved, ok := parsePermissionString(resp, &approveAll); ok {
				renderer.SetFooterNote("")
				return approved
			}
			renderer.SetFooterNote(renderer.color("\033[33m", "  Allow? [y/n/a]"))
			if err == io.EOF {
				return false
			}
		}
	}
}

func parsePermissionByte(b byte, approveAll *bool) bool {
	switch b {
	case 'y', 'Y', '\r', '\n':
		return true
	case 'a', 'A':
		*approveAll = true
		return true
	case 'n', 'N', 0x03: // n or Ctrl-C
		return false
	default:
		return false
	}
}

func parsePermissionString(input string, approveAll *bool) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "y", "yes":
		return true, true
	case "a", "all":
		*approveAll = true
		return true, true
	case "n", "no":
		return false, true
	default:
		return false, false
	}
}

// formatPermissionSummary returns a human-readable summary of what the tool will do.
func formatPermissionSummary(toolName string, args map[string]interface{}) string {
	switch toolName {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			cmd = strings.TrimSpace(cmd)
			return cmd
		}
	case "file_write":
		if path, ok := args["path"].(string); ok {
			content, _ := args["content"].(string)
			preview := previewText(content, 8, 120)
			if preview == "" {
				return fmt.Sprintf("create/overwrite %s", path)
			}
			return fmt.Sprintf("create/overwrite %s\n  --- preview ---\n%s", path, indentLines(preview, "  "))
		}
	case "file_edit":
		if path, ok := args["path"].(string); ok {
			oldStr, _ := args["old_string"].(string)
			newStr, _ := args["new_string"].(string)
			return fmt.Sprintf("edit %s\n  --- replace ---\n%s\n  --- with ---\n%s", path, indentLines(previewText(oldStr, 6, 120), "  "), indentLines(previewText(newStr, 6, 120), "  "))
		}
	case "notebook_edit":
		if path, ok := args["path"].(string); ok {
			cell, _ := args["cell"].(float64)
			source, _ := args["source"].(string)
			preview := previewText(source, 8, 120)
			if preview == "" {
				return fmt.Sprintf("edit cell %d in %s", int(cell), path)
			}
			return fmt.Sprintf("edit cell %d in %s\n  --- cell source ---\n%s", int(cell), path, indentLines(preview, "  "))
		}
	case "git":
		if sub, ok := args["subcommand"].(string); ok {
			extra, _ := args["args"].(string)
			extra = strings.TrimSpace(extra)
			if extra == "" {
				return fmt.Sprintf("git %s", sub)
			}
			return fmt.Sprintf("git %s %s", sub, extra)
		}
	}
	return ""
}

func previewText(text string, maxLines int, maxLineLen int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = append(lines[:maxLines], fmt.Sprintf("... (%d more lines)", len(lines)-maxLines))
	}
	for i, line := range lines {
		line = strings.TrimRight(line, "\r")
		if len(line) > maxLineLen {
			line = line[:maxLineLen-3] + "..."
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func indentLines(text string, prefix string) string {
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
