package agent

import "os"

// SemanticColor maps purpose to ANSI escape codes.
// All color output goes through this system for consistency and accessibility.
type SemanticColor struct {
	ToolName   string // tool names in brackets
	ToolResult string // tool output text
	Diff       string // diff context
	DiffAdd    string // added lines
	DiffRemove string // removed lines
	Phase      string // pipeline phase indicators
	Model      string // model/provider info
	Error      string // error messages
	Warning    string // warning messages
	Success    string // success indicators
	Dim        string // de-emphasized text
	Bold       string // emphasis
	Reset      string // reset all attributes
}

// DefaultColors returns the standard color scheme.
// Returns empty strings for all colors when NO_COLOR is set.
func DefaultColors() SemanticColor {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return SemanticColor{}
	}
	return SemanticColor{
		ToolName:   "\033[36m",  // cyan
		ToolResult: "\033[37m",  // white
		Diff:       "\033[2m",   // dim
		DiffAdd:    "\033[32m",  // green
		DiffRemove: "\033[31m",  // red
		Phase:      "\033[35m",  // magenta
		Model:      "\033[33m",  // yellow
		Error:      "\033[31m",  // red
		Warning:    "\033[33m",  // yellow
		Success:    "\033[32m",  // green
		Dim:        "\033[2m",   // dim
		Bold:       "\033[1m",   // bold
		Reset:      "\033[0m",   // reset
	}
}

// NoColors returns a color scheme with all codes empty (plain text).
func NoColors() SemanticColor {
	return SemanticColor{}
}

// IsEnabled returns true if colors are active.
func (c SemanticColor) IsEnabled() bool {
	return c.Reset != ""
}

// Wrap applies a color code around text, adding reset at the end.
func (c SemanticColor) Wrap(code, text string) string {
	if code == "" {
		return text
	}
	return code + text + c.Reset
}
