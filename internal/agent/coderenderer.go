package agent

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
	mu  sync.Mutex
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

	// Tiers tracking
	totalTiers int

	// Token usage
	tokensUsed int

	// Styling
	noColor bool

	// Accessibility
	screenReaderMode bool
	colorblindMode   bool

	// Thinking indicator state
	thinkingActive bool
	thinkingStop   chan struct{}
	thinkingWg     sync.WaitGroup

	// Footer state for fixed bottom status + input
	footerStatus string
	footerNote   string
	inputLine    string
	inputActive  bool
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

func (cr *CodeRenderer) color(code, text string) string {
	if cr.noColor {
		return text
	}
	// Colorblind mode: remap red->orange, green->blue
	if cr.colorblindMode {
		switch code {
		case "\033[31m": // red
			code = "\033[33m" // orange
		case "\033[32m": // green
			code = "\033[34m" // blue
		}
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
	cr.inputActive = true
	cr.renderFooterLocked(true)
}

// --- Screen layout ---

// Init prints a launch banner with project context.
func (cr *CodeRenderer) Init() {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	fmt.Fprintln(cr.out)

	// Brain logo with text — adapts to terminal width
	// Enhanced visual hierarchy with better spacing
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
		cr.setWindowTitle(cr.project + " - synroute")
	}

	// Working directory
	if wd, err := os.Getwd(); err == nil {
		fmt.Fprintln(cr.out, cr.color("\033[2m", "  "+wd))
	}

	fmt.Fprintln(cr.out)
	fmt.Fprintln(cr.out, cr.color("\033[2m", "  /plan  /review  /check  /fix  /help"))
	fmt.Fprintln(cr.out, cr.color("\033[2m", "  Tip: Use @file to reference files"))

	if cr.model == "" {
		fmt.Fprintln(cr.out, cr.color("\033[2m", "  Try: \"Explain this codebase\" or \"Fix the failing tests\" or \"Add error handling\""))
	}
	configPath := filepath.Join(os.Getenv("HOME"), ".synroute", "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Fprintln(cr.out, cr.color("\033[2m", "  First time? Here are some tips:"))
		fmt.Fprintln(cr.out, cr.color("\033[2m", "    1. Create CLAUDE.md files to customize your interactions"))
		fmt.Fprintln(cr.out, cr.color("\033[2m", "    2. Use /help for available commands"))
		fmt.Fprintln(cr.out, cr.color("\033[2m", "    3. Ask coding questions, edit code or run commands"))
		fmt.Fprintln(cr.out, cr.color("\033[2m", "    4. Be specific for the best results"))
	}

	// synroute.md detection handled by CodeREPL.detectProjectFiles()
	fmt.Fprintln(cr.out)
}

func (cr *CodeRenderer) Resize(width, height int) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.width = width
	cr.height = height
	cr.renderFooterLocked(cr.inputActive)
}

// writeContent writes a line of output. Uses \r\n for raw terminal mode
// so the cursor returns to column 0 (raw mode doesn't translate \n to \r\n).
func (cr *CodeRenderer) writeContent(line string) {
	if cr.out == nil {
		return
	}
	if cr.usesFooterLocked() {
		contentRow := maxInt(1, cr.height-2)
		fmt.Fprintf(cr.out, "\033[%d;1H\033[2K%s\r\n", contentRow, line)
		cr.renderFooterLocked(cr.inputActive)
		return
	}
	fmt.Fprint(cr.out, line+"\r\n")
}

func (cr *CodeRenderer) SetInputLine(line string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.inputLine = line
	cr.inputActive = true
	cr.renderFooterLocked(true)
}

func (cr *CodeRenderer) SetInputActive(active bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.inputActive = active
	cr.renderFooterLocked(active)
}

func (cr *CodeRenderer) SetFooterNote(note string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.footerNote = note
	cr.renderFooterLocked(cr.inputActive)
}

func (cr *CodeRenderer) usesFooterLocked() bool {
	return cr.out != nil && cr.height >= 3
}

func (cr *CodeRenderer) renderFooterLocked(focusInput bool) {
	if !cr.usesFooterLocked() {
		return
	}
	statusRow := maxInt(1, cr.height-1)
	inputRow := maxInt(1, cr.height)
	contentRow := maxInt(1, cr.height-2)
	status := cr.footerStatus
	if cr.footerNote != "" {
		status = cr.footerNote
	}
	input := cr.promptPrefixLocked() + cr.inputLine
	fmt.Fprintf(cr.out, "\033[%d;1H\033[2K%s", statusRow, status)
	fmt.Fprintf(cr.out, "\033[%d;1H\033[2K%s", inputRow, input)
	if focusInput {
		fmt.Fprintf(cr.out, "\033[%d;%dH", inputRow, visibleWidth(cr.promptPrefixLocked())+visibleWidth(cr.inputLine)+1)
	} else {
		fmt.Fprintf(cr.out, "\033[%d;1H", contentRow)
	}
}

func (cr *CodeRenderer) promptPrefixLocked() string {
	return cr.color("\033[32m", "synroute>") + " "
}

func visibleWidth(s string) int {
	plain := s
	for {
		start := strings.Index(plain, "\033[")
		if start == -1 {
			break
		}
		end := strings.IndexByte(plain[start:], 'm')
		if end == -1 {
			break
		}
		plain = plain[:start] + plain[start+end+1:]
	}
	return len(plain)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
		cr.setWindowTitle(fmt.Sprintf("Phase %d/%d: %s - synroute", cr.phaseIdx+1, cr.phaseCount, cr.phase))

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
		cr.setWindowTitle("Working... - synroute")
		cr.StartThinking()

	case EventLLMComplete:
		// Update model name from LLM response for accurate status bar display
		if model := str(e.Data, "model"); model != "" {
			cr.model = model
		}
		cr.StopThinking()

		if cr.verbosity >= VerbosityVerbose {
			duration := str(e.Data, "duration")
			tokens := intVal(e.Data, "tokens_used")
			cr.writeContent(cr.color("\033[34m", fmt.Sprintf("  llm <- %s | %d tokens", duration, tokens)))
		}

	case EventToolStart:
		cr.footerNote = ""
		if cr.verbosity >= VerbosityNormal {
			name := str(e.Data, "tool_name")
			summary := str(e.Data, "args_summary")
			cr.writeContent(fmt.Sprintf("  %s %s", cr.color("\033[36m", "["+name+"]"), summary))
		}

	case EventPermissionRequest:
		name := str(e.Data, "tool_name")
		category := str(e.Data, "category")
		cr.footerNote = cr.color("\033[33m", fmt.Sprintf("  permission: %s (%s)", name, category))
		cr.renderFooterLocked(cr.inputActive)

	case EventToolComplete:
		cr.footerNote = ""
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

		statusIcon := "✓"
		statusColor := "\033[32m"
		if isErr {
			statusIcon = "✗"
			statusColor = "\033[31m"
		}

		// Compact mode (level 0): single-line summary based on tool type
		if cr.verbosity == VerbosityCompact {
			var summary string
			switch name {
			case "file_read":
				lines := intVal(e.Data, "lines_read")
				path := str(e.Data, "path")
				summary = fmt.Sprintf("%s (%d lines)", path, lines)
			case "bash":
				cmd := str(e.Data, "command")
				summary = fmt.Sprintf("%s", cmd)
			case "grep":
				pattern := str(e.Data, "pattern")
				matches := intVal(e.Data, "matches")
				summary = fmt.Sprintf("%s (%d matches)", pattern, matches)
			case "glob":
				pattern := str(e.Data, "pattern")
				files := intVal(e.Data, "files_found")
				summary = fmt.Sprintf("%s (%d files)", pattern, files)
			default:
				summary = str(e.Data, "args_summary")
			}
			formatted := fmt.Sprintf("  %s %s %s %s",
				cr.color("\033[36m", "["+name+"]"),
				summary,
				cr.color("\033[2m", "("+duration+")"),
				cr.color(statusColor, statusIcon))
			cr.writeContent(formatted)
		} else if cr.verbosity >= VerbosityNormal {
			// Normal/Verbose mode: show full summary as before
			summary := str(e.Data, "args_summary")
			formatted := fmt.Sprintf("  %s %s %s %s",
				cr.color("\033[36m", "["+name+"]"),
				summary,
				cr.color("\033[2m", "("+duration+")"),
				cr.color(statusColor, statusIcon))
			cr.writeContent(formatted)
		}

	case EventEscalation:
		total := intVal(e.Data, "total_levels")
		if total == 0 {
			total = 6
		}
		cr.totalTiers = total

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
		cr.footerNote = ""
		source := str(e.Data, "source")
		msg := str(e.Data, "message")
		cr.writeContent(cr.color("\033[31m", fmt.Sprintf("  error [%s] %s", source, msg)))

	case EventTokenStream:
		// Print streamed tokens inline as they arrive — no newline
		if token, ok := e.Data["token"].(string); ok {
			fmt.Fprint(cr.out, token)
		}

	case EventBudgetUpdate:
		cr.tokensUsed = intVal(e.Data, "tokens")
		if cr.verbosity >= VerbosityVerbose {
			cr.writeContent(cr.color("\033[2m", fmt.Sprintf("  budget turns=%d tokens=%d elapsed=%s",
				intVal(e.Data, "turns"), intVal(e.Data, "tokens"), str(e.Data, "elapsed"))))
		}

	case EventTaskComplete:
		filesCreated, _ := e.Data["files_created"].([]string)
		filesModified, _ := e.Data["files_modified"].([]string)
		cmdsTotal := intVal(e.Data, "commands_total")
		cmdsPassed := intVal(e.Data, "commands_passed")
		cmdsFailed := intVal(e.Data, "commands_failed")

		cr.writeContent(cr.color("\033[36m", "── Done ──────────────────────────"))
		for _, f := range filesCreated {
			cr.writeContent(cr.color("\033[32m", fmt.Sprintf("  Created: %s", f)))
		}
		for _, f := range filesModified {
			cr.writeContent(cr.color("\033[33m", fmt.Sprintf("  Modified: %s", f)))
		}
		cr.writeContent(cr.color("\033[36m", fmt.Sprintf("  Commands: %d (%d passed, %d failed)", cmdsTotal, cmdsPassed, cmdsFailed)))
		cr.writeContent(cr.color("\033[36m", "──────────────────────────────────"))
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
			var marker string
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

// SetVerbosity updates the verbosity level (internal state only — no visual feedback).
func (cr *CodeRenderer) SetVerbosity(level int) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.verbosity = level
}

// SetScreenReaderMode enables or disables screen reader mode.
// When enabled, spinners and box drawing chars are replaced with plain text.
func (cr *CodeRenderer) SetScreenReaderMode(enabled bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.screenReaderMode = enabled
}

// SetColorblindMode enables or disables colorblind-friendly color remapping.
// When enabled, red->orange, green->blue.
func (cr *CodeRenderer) SetColorblindMode(enabled bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.colorblindMode = enabled
}

// setWindowTitle sets the terminal window title using ANSI escape sequence.
func (cr *CodeRenderer) setWindowTitle(title string) {
	if cr.noColor {
		return
	}
	fmt.Fprintf(cr.out, "\033]0;%s\007", title)
}

// StartThinking launches a goroutine with a braille spinner and elapsed timer.
// StartThinking starts the thinking spinner indicator.
// MUST be called with cr.mu already held (called from handleEvent which holds mu).
func (cr *CodeRenderer) StartThinking() {
	// mu is already held by caller — do NOT lock again (sync.Mutex is not reentrant)
	if cr.thinkingActive || cr.screenReaderMode {
		return
	}
	cr.thinkingActive = true
	cr.thinkingStop = make(chan struct{})
	cr.thinkingWg.Add(1)

	startTime := time.Now()
	spinners := []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}
	i := 0

	go func() {
		defer cr.thinkingWg.Done()
		ticker := time.NewTicker(80 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-cr.thinkingStop:
				return
			case <-ticker.C:
				cr.mu.Lock()
				if !cr.thinkingActive {
					cr.mu.Unlock()
					return
				}
				elapsed := time.Since(startTime).Seconds()
				spinner := spinners[i%len(spinners)]
				i++
				cr.footerNote = fmt.Sprintf("  %s Thinking... (%.1fs)", cr.color("\033[36m", string(spinner)), elapsed)
				cr.renderFooterLocked(cr.inputActive)
				cr.mu.Unlock()
			}
		}
	}()
}

// StopThinking stops the thinking spinner indicator.
// MUST be called with cr.mu already held (called from handleEvent which holds mu).
func (cr *CodeRenderer) StopThinking() {
	// mu is already held by caller — do NOT lock again (sync.Mutex is not reentrant)
	if !cr.thinkingActive {
		return
	}
	cr.thinkingActive = false
	close(cr.thinkingStop)

	// Must release mu before Wait() — the spinner goroutine needs mu to exit
	cr.mu.Unlock()
	cr.thinkingWg.Wait()
	cr.mu.Lock()
	cr.footerNote = ""
	cr.renderFooterLocked(cr.inputActive)
}

// RenderStatusBar prints a bottom status line with workspace, git branch, model, and tier.
func (cr *CodeRenderer) RenderStatusBar() {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	// Get workspace basename
	workspace := "unknown"
	if wd, err := os.Getwd(); err == nil {
		workspace = filepath.Base(wd)
	}

	// Get git branch
	branch := "no-git"
	if out, err := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD").Output(); err == nil {
		branch = strings.TrimSpace(string(out))
	}

	// Build status line with improved visual hierarchy
	var status string
	if cr.screenReaderMode {
		// Plain language for screen readers and non-technical users
		status = fmt.Sprintf("  Workspace: %s | Branch: %s | Model: %s | Provider: %s | Tokens: %d", workspace, branch, cr.model, cr.provider, cr.tokensUsed)
	} else {
		// Enhanced visual format with better spacing and icons
		status = fmt.Sprintf("  📁 %s  •  🌿 %s  •  🤖 %s  •  ⚡ %s  •  🪙 %d", workspace, branch, cr.model, cr.provider, cr.tokensUsed)
	}
	cr.footerStatus = cr.color("\033[2m", status)
	cr.renderFooterLocked(cr.inputActive)
}

func (cr *CodeRenderer) Cleanup() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	if !cr.usesFooterLocked() {
		return
	}
	statusRow := maxInt(1, cr.height-1)
	inputRow := maxInt(1, cr.height)
	fmt.Fprintf(cr.out, "\033[%d;1H\033[2K\033[%d;1H\033[2K", statusRow, inputRow)
}
