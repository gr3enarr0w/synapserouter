package agent

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/tools"
	"golang.org/x/term"
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

		// Read a single byte from /dev/tty — terminal may be in raw mode
		// so we can't wait for a line ending. Just read y, n, or a.
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			// Can't open TTY — auto-approve as fallback
			log.Printf("[Permission] can't open /dev/tty: %v — auto-approving", err)
			return true
		}

		// Temporarily set cooked mode so we get line-buffered input
		oldState, err := term.MakeRaw(int(tty.Fd()))
		if err == nil {
			defer term.Restore(int(tty.Fd()), oldState)
		}

		// Read one byte
		buf := make([]byte, 1)
		n, err := tty.Read(buf)
		tty.Close()
		if err != nil || n == 0 {
			return true // auto-approve on read error
		}

		fmt.Fprintln(out) // newline after the single char
		return parsePermissionByte(buf[0], &approveAll)
	}
}

func parsePermissionResponse(text string, approveAll *bool) bool {
	response := strings.TrimSpace(strings.ToLower(text))
	switch response {
	case "", "y", "yes":
		return true
	case "a", "all":
		*approveAll = true
		return true
	default:
		return false
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

// formatPermissionSummary returns a human-readable summary of what the tool will do.
func formatPermissionSummary(toolName string, args map[string]interface{}) string {
	switch toolName {
	case "bash":
		if cmd, ok := args["command"].(string); ok {
			if len(cmd) > 80 {
				cmd = cmd[:77] + "..."
			}
			return cmd
		}
	case "file_write":
		if path, ok := args["path"].(string); ok {
			return fmt.Sprintf("create/overwrite %s", path)
		}
	case "file_edit":
		if path, ok := args["path"].(string); ok {
			return fmt.Sprintf("edit %s", path)
		}
	case "notebook_edit":
		if path, ok := args["path"].(string); ok {
			cell, _ := args["cell"].(float64)
			return fmt.Sprintf("edit cell %d in %s", int(cell), path)
		}
	case "git":
		if sub, ok := args["subcommand"].(string); ok {
			return fmt.Sprintf("git %s", sub)
		}
	}
	return ""
}
