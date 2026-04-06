package agent

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

// Verbosity levels for event rendering.
const (
	VerbosityCompact = 0 // Phase transitions and errors only
	VerbosityNormal  = 1 // + tool calls, LLM summaries, sub-agent info
	VerbosityVerbose = 2 // + full output, skill dumps, budget updates
)

// LogRenderer subscribes to an EventBus and writes structured log lines to
// an io.Writer (typically stderr). Designed for --message mode where stdout
// must stay clean for the final response.
type LogRenderer struct {
	out       io.Writer
	verbosity int
	jsonMode  bool
	startTime time.Time
}

// NewLogRenderer creates a renderer that writes structured events to out.
func NewLogRenderer(out io.Writer, verbosity int, jsonMode bool) *LogRenderer {
	return &LogRenderer{
		out:       out,
		verbosity: verbosity,
		jsonMode:  jsonMode,
		startTime: time.Now(),
	}
}

// Run consumes events from the channel and renders them. Blocks until the
// channel is closed. Should be called in a goroutine.
func (lr *LogRenderer) Run(events <-chan AgentEvent) {
	for event := range events {
		if !lr.shouldRender(event) {
			continue
		}
		if lr.jsonMode {
			lr.renderJSON(event)
		} else {
			lr.renderText(event)
		}
	}
}

func (lr *LogRenderer) shouldRender(e AgentEvent) bool {
	switch lr.verbosity {
	case VerbosityCompact:
		switch e.Type {
		case EventPipelineStart, EventPhaseStart, EventPhaseComplete,
			EventEscalation, EventError, EventSubAgentSpawn, EventSubAgentComplete:
			return true
		case EventQualityGate:
			rejected, _ := e.Data["rejected"].(bool)
			return rejected
		}
		return false
	case VerbosityNormal:
		return e.Type != EventBudgetUpdate
	default: // Verbose
		return true
	}
}

func (lr *LogRenderer) renderJSON(e AgentEvent) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	fmt.Fprintf(lr.out, "%s\n", data)
}

func (lr *LogRenderer) renderText(e AgentEvent) {
	elapsed := e.Timestamp.Sub(lr.startTime).Truncate(time.Millisecond)
	prefix := fmt.Sprintf("[%s] ", elapsed)
	agent := ""
	if e.ParentID != "" {
		agent = fmt.Sprintf(" (%s)", e.AgentID)
	}

	switch e.Type {
	case EventPipelineStart:
		skills, _ := e.Data["matched_skills"].([]string)
		fmt.Fprintf(lr.out, "%s\033[35mpipeline\033[0m %s | %d phases | skills: %s%s\n",
			prefix, str(e.Data, "pipeline_name"), intVal(e.Data, "phase_count"),
			strings.Join(skills, ", "), agent)

	case EventPhaseStart:
		fmt.Fprintf(lr.out, "%s\033[33mphase\033[0m [%d/%d] %s%s\n",
			prefix, intVal(e.Data, "phase_index")+1, intVal(e.Data, "total_phases"),
			str(e.Data, "phase_name"), agent)

	case EventPhaseComplete:
		passed, _ := e.Data["passed"].(bool)
		status := "\033[32mPASS\033[0m"
		if !passed {
			status = "\033[31mFAIL\033[0m"
		}
		fmt.Fprintf(lr.out, "%s\033[33mphase\033[0m %s %s%s\n",
			prefix, str(e.Data, "phase_name"), status, agent)

	case EventLLMStart:
		if lr.verbosity >= VerbosityNormal {
			provider := e.Provider
			if provider == "" {
				provider = "router"
			}
			fmt.Fprintf(lr.out, "%s\033[34mllm\033[0m → %s [%s] (turn %d)%s\n",
				prefix, provider, str(e.Data, "model"), intVal(e.Data, "turn"), agent)
		}

	case EventLLMComplete:
		if lr.verbosity >= VerbosityNormal {
			provider := e.Provider
			if provider == "" {
				provider = "?"
			}
			model := str(e.Data, "model")
			if model == "" {
				model = "?"
			}
			fmt.Fprintf(lr.out, "%s\033[34mllm\033[0m ← %s [%s] | %s | %d tokens%s\n",
				prefix, provider, model, str(e.Data, "duration"),
				intVal(e.Data, "tokens_used"), agent)
		} else {
			fmt.Fprintf(lr.out, "%s\033[34mllm\033[0m %s%s\n",
				prefix, str(e.Data, "duration"), agent)
		}

	case EventToolStart:
		if lr.verbosity >= VerbosityNormal {
			fmt.Fprintf(lr.out, "%s\033[36mtool\033[0m → %s %s%s\n",
				prefix, str(e.Data, "tool_name"), str(e.Data, "args_summary"), agent)
		}

	case EventPermissionRequest:
		if lr.verbosity >= VerbosityNormal {
			fmt.Fprintf(lr.out, "%s\033[33mpermission\033[0m → %s (category: %s)%s\n",
				prefix, str(e.Data, "tool_name"), str(e.Data, "category"), agent)
		}

	case EventToolComplete:
		if lr.verbosity >= VerbosityNormal {
			isErr, _ := e.Data["is_error"].(bool)
			status := ""
			if isErr {
				status = " \033[31m(error)\033[0m"
			}
			fmt.Fprintf(lr.out, "%s\033[36mtool\033[0m ← %s %s%s%s\n",
				prefix, str(e.Data, "tool_name"), str(e.Data, "duration"), status, agent)
			if lr.verbosity >= VerbosityVerbose {
				if output := str(e.Data, "output"); output != "" {
					for _, line := range strings.Split(output, "\n") {
						fmt.Fprintf(lr.out, "         %s\n", line)
					}
				}
			}
		}

	case EventSubAgentSpawn:
		fmt.Fprintf(lr.out, "%s\033[35mspawn\033[0m %s → %s%s\n",
			prefix, str(e.Data, "role"), e.Provider, agent)
		if lr.verbosity >= VerbosityVerbose {
			if preview := str(e.Data, "task_preview"); preview != "" {
				fmt.Fprintf(lr.out, "         task: %s\n", preview)
			}
		}

	case EventSubAgentComplete:
		status := str(e.Data, "status")
		color := "32" // green
		if status == "failed" {
			color = "31" // red
		}
		fmt.Fprintf(lr.out, "%s\033[%smcomplete\033[0m %s %s %s%s\n",
			prefix, color, str(e.Data, "role"), status, str(e.Data, "duration"), agent)

	case EventEscalation:
		fmt.Fprintf(lr.out, "%s\033[33mescalate\033[0m level %d → %d | providers: %s%s\n",
			prefix, intVal(e.Data, "from_level"), intVal(e.Data, "to_level"),
			str(e.Data, "providers"), agent)

	case EventSkillMatch:
		if lr.verbosity >= VerbosityNormal {
			skills, _ := e.Data["skill_names"].([]string)
			fmt.Fprintf(lr.out, "%s\033[35mskills\033[0m matched: %s%s\n",
				prefix, strings.Join(skills, ", "), agent)
		}

	case EventQualityGate:
		rejected, _ := e.Data["rejected"].(bool)
		if rejected {
			fmt.Fprintf(lr.out, "%s\033[31mquality\033[0m REJECTED %s (need %d tool calls, got %d)%s\n",
				prefix, str(e.Data, "phase"), intVal(e.Data, "required"), intVal(e.Data, "actual"), agent)
		} else if lr.verbosity >= VerbosityNormal {
			fmt.Fprintf(lr.out, "%s\033[32mquality\033[0m PASSED %s%s\n",
				prefix, str(e.Data, "phase"), agent)
		}

	case EventParallelStart:
		fmt.Fprintf(lr.out, "%s\033[35mparallel\033[0m %d agents for %s%s\n",
			prefix, intVal(e.Data, "agent_count"), str(e.Data, "phase"), agent)

	case EventCrossReview:
		fmt.Fprintf(lr.out, "%s\033[36mcross-review\033[0m %s reviews %s (step %d)%s\n",
			prefix, str(e.Data, "reviewer"), str(e.Data, "target"), intVal(e.Data, "step"), agent)

	case EventBudgetUpdate:
		fmt.Fprintf(lr.out, "%s\033[33mbudget\033[0m turns=%d tokens=%d elapsed=%s%s\n",
			prefix, intVal(e.Data, "turns"), intVal(e.Data, "tokens"), str(e.Data, "elapsed"), agent)

	case EventError:
		fmt.Fprintf(lr.out, "%s\033[31merror\033[0m [%s] %s%s\n",
			prefix, str(e.Data, "source"), str(e.Data, "message"), agent)
	}
}

// Helper functions for extracting typed data from event maps.
func str(data map[string]any, key string) string {
	if v, ok := data[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

func intVal(data map[string]any, key string) int {
	if v, ok := data[key]; ok {
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	}
	return 0
}
