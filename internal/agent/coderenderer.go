package agent

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// ToolEntry records a tool call for the recent tools display.
type ToolEntry struct {
	Name      string
	Summary   string
	Duration  time.Duration
	IsError   bool
	Timestamp time.Time
}

// CodeRenderer provides a pipeline-aware terminal UI for code mode.
// It implements TerminalRenderer and subscribes to EventBus for real-time updates.
type CodeRenderer struct {
	mu sync.Mutex
	out io.Writer

	// Terminal dimensions
	width  int
	height int

	// State tracked from events
	project   string
	phase     string
	phaseIdx  int
	phaseCount int
	model     string
	provider  string
	startTime time.Time
	verbosity int

	// Recent tool calls for ^T display
	recentTools []ToolEntry
	maxTools    int

	// Styling
	noColor bool
}

// NewCodeRenderer creates a renderer for code mode.
func NewCodeRenderer(out io.Writer, width, height int, project string) *CodeRenderer {
	return &CodeRenderer{
		out:       out,
		width:     width,
		height:    height,
		project:   project,
		startTime: time.Now(),
		maxTools:  50,
		noColor:   os.Getenv("NO_COLOR") != "",
	}
}

// ANSI helpers — return empty strings when NO_COLOR is set.

func (cr *CodeRenderer) ansi(code string) string {
	if cr.noColor {
		return ""
	}
	return code
}

func (cr *CodeRenderer) color(code, text string) string {
	if cr.noColor {
		return text
	}
	return code + text + "\033[0m"
}

// --- TerminalRenderer interface ---

func (cr *CodeRenderer) Text(text string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.writeContent(text)
}

func (cr *CodeRenderer) ToolCall(name string, args map[string]interface{}) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	summary := formatToolCallSummary(name, args)
	line := fmt.Sprintf("  %s %s", cr.color("\033[36m", "["+name+"]"), summary)
	cr.writeContent(line)
}

func (cr *CodeRenderer) ToolResult(name string, result string, isError bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if result == "" {
		return
	}
	lines := strings.Split(result, "\n")
	maxLines := 15
	if len(lines) > maxLines {
		for _, line := range lines[:maxLines] {
			cr.writeContent("    " + line)
		}
		cr.writeContent(fmt.Sprintf("    %s", cr.color("\033[2m", fmt.Sprintf("... (%d more lines)", len(lines)-maxLines))))
	} else {
		for _, line := range lines {
			cr.writeContent("    " + line)
		}
	}
}

func (cr *CodeRenderer) ToolDiff(path, oldText, newText string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.writeContent(cr.color("\033[2m", fmt.Sprintf("  -- file_edit: %s --", path)))

	oldLines := strings.Split(oldText, "\n")
	newLines := strings.Split(newText, "\n")
	maxLines := 20

	count := 0
	for _, line := range oldLines {
		if count >= maxLines {
			cr.writeContent(cr.color("\033[2m", fmt.Sprintf("    ... (%d more removed)", len(oldLines)-count)))
			break
		}
		cr.writeContent(cr.color("\033[31m", "  - "+line))
		count++
	}

	count = 0
	for _, line := range newLines {
		if count >= maxLines {
			cr.writeContent(cr.color("\033[2m", fmt.Sprintf("    ... (%d more added)", len(newLines)-count)))
			break
		}
		cr.writeContent(cr.color("\033[32m", "  + "+line))
		count++
	}
}

func (cr *CodeRenderer) Error(msg string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.writeContent(cr.color("\033[31m", "error: ") + msg)
}

func (cr *CodeRenderer) Prompt() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	fmt.Fprint(cr.out, cr.color("\033[32m", "synroute>")+" ")
}

// --- Screen layout ---

// Init sets up the screen: clears, draws status bar and footer, sets scroll region.
func (cr *CodeRenderer) Init() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.drawScreen()
}

// Resize updates dimensions and redraws chrome.
func (cr *CodeRenderer) Resize(width, height int) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.width = width
	cr.height = height
	cr.drawScreen()
}

func (cr *CodeRenderer) drawScreen() {
	// Clear screen
	fmt.Fprint(cr.out, "\033[2J")
	// Draw status bar at row 1
	cr.drawStatusBarLocked()
	// Draw footer at last row
	cr.drawFooterLocked()
	// Set scroll region (rows 2 to h-1)
	if cr.height > 3 {
		fmt.Fprintf(cr.out, "\033[2;%dr", cr.height-1)
	}
	// Position cursor in scroll region
	fmt.Fprint(cr.out, "\033[2;1H")
}

func (cr *CodeRenderer) drawStatusBarLocked() {
	// Save cursor, move to row 1
	fmt.Fprint(cr.out, "\033[s\033[1;1H\033[K")

	elapsed := time.Since(cr.startTime).Truncate(time.Second)

	// Build status components
	left := cr.color("\033[1;37m", " synroute")
	left += cr.color("\033[2m", " -- ")
	left += cr.color("\033[1;36m", "code")

	if cr.project != "" {
		left += cr.color("\033[2m", " -- ")
		left += cr.color("\033[1;37m", cr.project)
	}

	if cr.phase != "" {
		left += cr.color("\033[2m", " -- ")
		phaseText := cr.phase
		if cr.phaseCount > 0 {
			phaseText = fmt.Sprintf("phase %d/%d: %s", cr.phaseIdx+1, cr.phaseCount, cr.phase)
		}
		left += cr.color("\033[1;33m", phaseText)
	}

	if cr.model != "" {
		left += cr.color("\033[2m", " -- ")
		left += cr.color("\033[34m", cr.model)
	}

	right := cr.color("\033[2m", fmt.Sprintf("%s ", elapsed))

	// Write with inverse background
	bar := left
	// Pad to width (approximate — ANSI codes make exact calculation hard)
	// Just write left and right portions
	fmt.Fprint(cr.out, cr.ansi("\033[7m")) // inverse
	fmt.Fprint(cr.out, bar)
	// Fill gap — estimate visible length
	visibleLeft := cr.visibleLen(left)
	visibleRight := cr.visibleLen(right)
	gap := cr.width - visibleLeft - visibleRight
	if gap > 0 {
		fmt.Fprint(cr.out, strings.Repeat(" ", gap))
	}
	fmt.Fprint(cr.out, right)
	fmt.Fprint(cr.out, cr.ansi("\033[0m"))

	// Restore cursor
	fmt.Fprint(cr.out, "\033[u")
}

func (cr *CodeRenderer) drawFooterLocked() {
	// Save cursor, move to last row
	fmt.Fprintf(cr.out, "\033[s\033[%d;1H\033[K", cr.height)

	shortcuts := " ^P pipeline  ^T tools  ^L logs  ^E escalate  ^/ help"
	fmt.Fprint(cr.out, cr.ansi("\033[7m")) // inverse
	fmt.Fprint(cr.out, shortcuts)
	gap := cr.width - len(shortcuts)
	if gap > 0 {
		fmt.Fprint(cr.out, strings.Repeat(" ", gap))
	}
	fmt.Fprint(cr.out, cr.ansi("\033[0m"))

	// Restore cursor
	fmt.Fprint(cr.out, "\033[u")
}

// writeContent writes a line into the scroll region.
func (cr *CodeRenderer) writeContent(line string) {
	// Truncate to terminal width if needed
	if cr.visibleLen(line) > cr.width-1 {
		line = cr.truncate(line, cr.width-4) + "..."
	}
	fmt.Fprintln(cr.out, line)
}

// --- Event handling ---

// Run consumes events from the EventBus and updates the display.
// Call in a goroutine.
func (cr *CodeRenderer) Run(events <-chan AgentEvent) {
	// Start elapsed time ticker
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event, ok := <-events:
			if !ok {
				return
			}
			cr.handleEvent(event)
		case <-ticker.C:
			cr.mu.Lock()
			cr.drawStatusBarLocked()
			cr.mu.Unlock()
		}
	}
}

func (cr *CodeRenderer) handleEvent(e AgentEvent) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	switch e.Type {
	case EventPipelineStart:
		cr.phaseCount = intVal(e.Data, "phase_count")
		pipelineName := str(e.Data, "pipeline_name")
		skills, _ := e.Data["matched_skills"].([]string)
		cr.writeContent("")
		cr.writeContent(cr.color("\033[35m", fmt.Sprintf("  pipeline %s | %d phases", pipelineName, cr.phaseCount)))
		if len(skills) > 0 {
			cr.writeContent(cr.color("\033[2m", fmt.Sprintf("  skills: %s", strings.Join(skills, ", "))))
		}
		cr.writeContent("")
		cr.drawStatusBarLocked()

	case EventPhaseStart:
		cr.phase = str(e.Data, "phase_name")
		cr.phaseIdx = intVal(e.Data, "phase_index")
		cr.writeContent("")
		cr.writeContent(cr.color("\033[1;33m", fmt.Sprintf("  -- phase %d/%d: %s --", cr.phaseIdx+1, cr.phaseCount, cr.phase)))
		cr.writeContent("")
		cr.drawStatusBarLocked()

	case EventPhaseComplete:
		passed, _ := e.Data["passed"].(bool)
		phaseName := str(e.Data, "phase_name")
		status := cr.color("\033[32m", "PASS")
		if !passed {
			status = cr.color("\033[31m", "FAIL")
		}
		cr.writeContent(fmt.Sprintf("  phase %s %s", phaseName, status))
		cr.writeContent("")

	case EventLLMStart:
		cr.model = str(e.Data, "model")
		cr.provider = e.Provider
		cr.drawStatusBarLocked()
		if cr.verbosity >= VerbosityNormal {
			provider := e.Provider
			if provider == "" {
				provider = "router"
			}
			cr.writeContent(cr.color("\033[34m", fmt.Sprintf("  llm -> %s [%s]", provider, cr.model)))
		}

	case EventLLMComplete:
		cr.model = ""
		cr.drawStatusBarLocked()
		if cr.verbosity >= VerbosityNormal {
			duration := str(e.Data, "duration")
			tokens := intVal(e.Data, "tokens_used")
			cr.writeContent(cr.color("\033[34m", fmt.Sprintf("  llm <- %s | %d tokens", duration, tokens)))
		}

	case EventToolStart:
		if cr.verbosity >= VerbosityNormal {
			name := str(e.Data, "tool_name")
			summary := str(e.Data, "args_summary")
			cr.writeContent(fmt.Sprintf("  %s %s", cr.color("\033[36m", "["+name+"]"), summary))
		}

	case EventToolComplete:
		name := str(e.Data, "tool_name")
		duration := str(e.Data, "duration")
		isErr, _ := e.Data["is_error"].(bool)
		entry := ToolEntry{
			Name:      name,
			Summary:   str(e.Data, "args_summary"),
			IsError:   isErr,
			Timestamp: e.Timestamp,
		}
		if d, err := time.ParseDuration(duration); err == nil {
			entry.Duration = d
		}
		cr.recentTools = append(cr.recentTools, entry)
		if len(cr.recentTools) > cr.maxTools {
			cr.recentTools = cr.recentTools[1:]
		}
		if cr.verbosity >= VerbosityNormal {
			status := ""
			if isErr {
				status = cr.color("\033[31m", " (error)")
			}
			cr.writeContent(cr.color("\033[2m", fmt.Sprintf("    <- %s %s%s", name, duration, status)))
		}

	case EventEscalation:
		from := intVal(e.Data, "from_level")
		to := intVal(e.Data, "to_level")
		providers := str(e.Data, "providers")
		cr.writeContent("")
		cr.writeContent(cr.color("\033[1;33m", fmt.Sprintf("  escalate level %d -> %d | %s", from, to, providers)))
		cr.writeContent("")
		cr.drawStatusBarLocked()

	case EventSubAgentSpawn:
		role := str(e.Data, "role")
		cr.writeContent(cr.color("\033[35m", fmt.Sprintf("  spawn %s -> %s", role, e.Provider)))

	case EventSubAgentComplete:
		role := str(e.Data, "role")
		status := str(e.Data, "status")
		duration := str(e.Data, "duration")
		statusColor := "\033[32m"
		if status == "failed" {
			statusColor = "\033[31m"
		}
		cr.writeContent(fmt.Sprintf("  %s %s %s", cr.color(statusColor, role), status, duration))

	case EventQualityGate:
		rejected, _ := e.Data["rejected"].(bool)
		phaseName := str(e.Data, "phase")
		if rejected {
			cr.writeContent(cr.color("\033[31m", fmt.Sprintf("  quality REJECTED %s (need %d tool calls, got %d)",
				phaseName, intVal(e.Data, "required"), intVal(e.Data, "actual"))))
		} else if cr.verbosity >= VerbosityNormal {
			cr.writeContent(cr.color("\033[32m", fmt.Sprintf("  quality PASSED %s", phaseName)))
		}

	case EventError:
		source := str(e.Data, "source")
		msg := str(e.Data, "message")
		cr.writeContent(cr.color("\033[31m", fmt.Sprintf("  error [%s] %s", source, msg)))

	case EventBudgetUpdate:
		if cr.verbosity >= VerbosityVerbose {
			cr.writeContent(cr.color("\033[2m", fmt.Sprintf("  budget turns=%d tokens=%d elapsed=%s",
				intVal(e.Data, "turns"), intVal(e.Data, "tokens"), str(e.Data, "elapsed"))))
		}
	}
}

// --- Overlay displays (called from CodeREPL shortcuts) ---

// ShowPipeline prints pipeline status inline.
func (cr *CodeRenderer) ShowPipeline() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.writeContent("")
	cr.writeContent(cr.color("\033[1;35m", "  Pipeline Status"))
	cr.writeContent(cr.color("\033[2m", "  "+strings.Repeat("-", 40)))
	if cr.phaseCount == 0 {
		cr.writeContent("  No pipeline active")
	} else {
		for i := 0; i < cr.phaseCount; i++ {
			marker := "  "
			if i < cr.phaseIdx {
				marker = cr.color("\033[32m", "  done ")
			} else if i == cr.phaseIdx {
				marker = cr.color("\033[33m", "  >>   ")
			} else {
				marker = cr.color("\033[2m", "       ")
			}
			phaseName := cr.phase
			if i != cr.phaseIdx {
				phaseName = fmt.Sprintf("phase %d", i+1)
			}
			cr.writeContent(fmt.Sprintf("%s%d/%d %s", marker, i+1, cr.phaseCount, phaseName))
		}
	}
	cr.writeContent("")
}

// ShowRecentTools prints the last N tool calls.
func (cr *CodeRenderer) ShowRecentTools() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.writeContent("")
	cr.writeContent(cr.color("\033[1;36m", "  Recent Tools"))
	cr.writeContent(cr.color("\033[2m", "  "+strings.Repeat("-", 40)))
	if len(cr.recentTools) == 0 {
		cr.writeContent("  No tool calls yet")
	} else {
		start := 0
		if len(cr.recentTools) > 20 {
			start = len(cr.recentTools) - 20
		}
		for _, t := range cr.recentTools[start:] {
			status := ""
			if t.IsError {
				status = cr.color("\033[31m", " ERR")
			}
			cr.writeContent(fmt.Sprintf("  %s %-12s %s%s",
				cr.color("\033[2m", t.Duration.Truncate(time.Millisecond).String()),
				cr.color("\033[36m", t.Name),
				t.Summary, status))
		}
	}
	cr.writeContent("")
}

// ShowHelp prints keyboard shortcut help.
func (cr *CodeRenderer) ShowHelp() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.writeContent("")
	cr.writeContent(cr.color("\033[1;37m", "  Keyboard Shortcuts"))
	cr.writeContent(cr.color("\033[2m", "  "+strings.Repeat("-", 40)))
	cr.writeContent("  ^P    Show pipeline status")
	cr.writeContent("  ^T    Show recent tool calls")
	cr.writeContent("  ^L    Cycle log verbosity (compact/normal/verbose)")
	cr.writeContent("  ^E    Force provider escalation")
	cr.writeContent("  ^/    Show this help")
	cr.writeContent("  ^C    Cancel current request")
	cr.writeContent("  ^D    Exit")
	cr.writeContent("")
	cr.writeContent("  Slash commands: /exit /clear /model /tools /history /agents /budget")
	cr.writeContent("")
}

// SetVerbosity updates the verbosity level and shows feedback.
func (cr *CodeRenderer) SetVerbosity(level int) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.verbosity = level
	names := []string{"compact", "normal", "verbose"}
	name := names[level%3]
	cr.writeContent(cr.color("\033[33m", fmt.Sprintf("  verbosity: %s", name)))
}

// --- Utility ---

// visibleLen estimates the visible length of a string with ANSI codes.
func (cr *CodeRenderer) visibleLen(s string) int {
	inEscape := false
	visible := 0
	for _, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		visible++
	}
	return visible
}

// truncate cuts a string to maxVisible visible characters, preserving ANSI codes.
func (cr *CodeRenderer) truncate(s string, maxVisible int) string {
	inEscape := false
	visible := 0
	for i, r := range s {
		if r == '\033' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		visible++
		if visible >= maxVisible {
			return s[:i+1]
		}
	}
	return s
}

// Cleanup restores the terminal scroll region to full screen.
func (cr *CodeRenderer) Cleanup() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	// Reset scroll region to full screen
	fmt.Fprintf(cr.out, "\033[1;%dr", cr.height)
	// Move cursor to last line
	fmt.Fprintf(cr.out, "\033[%d;1H", cr.height)
	fmt.Fprintln(cr.out)
}
