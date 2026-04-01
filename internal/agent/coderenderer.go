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
	project    string
	language   string
	phase      string
	phaseIdx   int
	phaseCount int
	model      string
	provider   string
	startTime  time.Time
	verbosity  int

	// Recent tool calls for ^T display
	recentTools []ToolEntry
	maxTools    int

	// Version for display
	version string

	// Profile for banner display
	providerLabel string

	// Styling
	noColor bool
}

// NewCodeRenderer creates a renderer for code mode.
func NewCodeRenderer(out io.Writer, width, height int, project, model, language string) *CodeRenderer {
	return &CodeRenderer{
		out:       out,
		width:     width,
		height:    height,
		project:   project,
		model:     model,
		language:  language,
		startTime: time.Now(),
		maxTools:  50,
		noColor:   os.Getenv("NO_COLOR") != "",
	}
}

// SetProviderLabel sets the provider label shown in the banner (e.g. "Ollama Cloud", "Vertex AI").
func (cr *CodeRenderer) SetProviderLabel(label string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.providerLabel = label
}

// SetVersion sets the version string for the launch banner.
func (cr *CodeRenderer) SetVersion(v string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.version = v
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

// Init prints a launch banner with project context.
func (cr *CodeRenderer) Init() {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	fmt.Fprintln(cr.out)

	// Brain logo with text — adapts to terminal width
	fmt.Fprint(cr.out, BannerForWidth(cr.width, cr.noColor))

	// Version + mode
	ver := cr.version
	if ver == "" {
		ver = "dev"
	}
	profileName := cr.providerLabel
	if profileName == "" {
		profileName = "personal"
	}
	fmt.Fprintln(cr.out, cr.color("\033[2m", "  "+ver+" · code mode · "+profileName))

	// Project info
	if cr.project != "" {
		projectLine := "  " + cr.color("\033[1;37m", cr.project)
		if cr.language != "" {
			projectLine += cr.color("\033[2m", " ("+cr.language+")")
		}
		fmt.Fprintln(cr.out, projectLine)
	}

	// Working directory
	if wd, err := os.Getwd(); err == nil {
		fmt.Fprintln(cr.out, cr.color("\033[2m", "  "+wd))
	}

	fmt.Fprintln(cr.out)
	fmt.Fprintln(cr.out, cr.color("\033[2m", "  /plan  /review  /check  /fix  /help"))

	// synroute.md detection handled by CodeREPL.detectProjectFiles()
	fmt.Fprintln(cr.out)
}

// Resize is a no-op without scroll regions.
func (cr *CodeRenderer) Resize(width, height int) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.width = width
	cr.height = height
}

// writeContent writes a line of output. Uses \r\n for raw terminal mode
// so the cursor returns to column 0 (raw mode doesn't translate \n to \r\n).
func (cr *CodeRenderer) writeContent(line string) {
	fmt.Fprint(cr.out, line+"\r\n")
}

// --- Event handling ---

// Run consumes events from the EventBus and updates the display.
// Call in a goroutine.
func (cr *CodeRenderer) Run(events <-chan AgentEvent) {
	for event := range events {
		cr.handleEvent(event)
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


	case EventPhaseStart:
		cr.phase = str(e.Data, "phase_name")
		cr.phaseIdx = intVal(e.Data, "phase_index")
		cr.writeContent("")
		cr.writeContent(cr.color("\033[1;33m", fmt.Sprintf("  -- phase %d/%d: %s --", cr.phaseIdx+1, cr.phaseCount, cr.phase)))
		cr.writeContent("")


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

		if cr.verbosity >= VerbosityVerbose {
			provider := e.Provider
			if provider == "" {
				provider = "router"
			}
			cr.writeContent(cr.color("\033[34m", fmt.Sprintf("  llm -> %s [%s]", provider, cr.model)))
		}

	case EventLLMComplete:
		cr.model = ""

		if cr.verbosity >= VerbosityVerbose {
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
		total := intVal(e.Data, "total_levels")
		if total == 0 {
			total = 6
		}
		cr.writeContent(cr.color("\033[2m", fmt.Sprintf("  %d tiers engaged", total)))


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

	case EventTokenStream:
		// Print streamed tokens inline as they arrive — no newline
		if token, ok := e.Data["token"].(string); ok {
			fmt.Fprint(cr.out, token)
		}

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

// ShowHelp prints command help.
func (cr *CodeRenderer) ShowHelp() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.writeContent("")
	cr.writeContent(cr.color("\033[1;37m", "  Commands"))
	cr.writeContent(cr.color("\033[2m", "  "+strings.Repeat("-", 40)))
	cr.writeContent("")
	cr.writeContent("  Phase commands:")
	cr.writeContent("  /plan [msg]     Generate plan + acceptance criteria")
	cr.writeContent("  /review [msg]   Independent code review")
	cr.writeContent("  /check [msg]    Build, test, verify against criteria")
	cr.writeContent("  /fix <msg>      Targeted fix (requires description)")
	cr.writeContent("")
	cr.writeContent("  Session:")
	cr.writeContent("  /model [name]   Show or set model")
	cr.writeContent("  /tools          List available tools")
	cr.writeContent("  /history        Show conversation history")
	cr.writeContent("  /agents         Show sub-agents")
	cr.writeContent("  /budget         Show budget usage")
	cr.writeContent("  /clear          Clear conversation")
	cr.writeContent("  /exit           Exit code mode")
	cr.writeContent("")
	cr.writeContent("  Keyboard shortcuts:")
	cr.writeContent("  Ctrl-C  Cancel current request (2x to exit)")
	cr.writeContent("  Ctrl-D  Exit")
	cr.writeContent("  Ctrl-P  Pipeline status")
	cr.writeContent("  Ctrl-T  Recent tool calls")
	cr.writeContent("  Ctrl-L  Cycle verbosity (compact/normal/verbose)")
	cr.writeContent("  Ctrl-E  Force escalation to next tier")
	cr.writeContent("  Ctrl-/  Show this help")
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

// Cleanup is a no-op — no scroll regions to restore.
func (cr *CodeRenderer) Cleanup() {
	// Intentionally empty: readline handles terminal restore.
}
