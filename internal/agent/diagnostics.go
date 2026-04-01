package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DiagnosticsReport captures what happened in an agent session for learning.
type DiagnosticsReport struct {
	Timestamp       time.Time
	SessionID       string
	WorkDir         string
	PhasesCompleted []string
	FailedPhase     string // empty if all passed
	VerifyResults   []string
	ErrorMessages   []string
	ProvidersUsed   []string
	TotalDuration   time.Duration
	TotalTurns      int
	ToolCallCount   int
}

const diagnosticsFile = "synroute.md"

// writeDiagnostics appends a session diagnostics report to synroute.md in the working directory.
func (a *Agent) writeDiagnostics(startTime time.Time) {
	if a.config.WorkDir == "" {
		return
	}

	report := a.buildDiagnosticsReport(startTime)
	content := formatDiagnosticsReport(report)

	// Append-only write — avoids read-modify-write race between concurrent agents.
	// Each agent appends its report independently; no data is lost.
	path := filepath.Join(a.config.WorkDir, diagnosticsFile)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return // best-effort
	}
	f.Write([]byte(content))
	f.Close()
}

func (a *Agent) buildDiagnosticsReport(startTime time.Time) DiagnosticsReport {
	report := DiagnosticsReport{
		Timestamp:     time.Now(),
		SessionID:     a.sessionID,
		WorkDir:       a.config.WorkDir,
		TotalDuration: time.Since(startTime),
		ToolCallCount: a.toolCallCount,
	}

	// Collect completed phases
	if a.pipeline != nil {
		for i, phase := range a.pipeline.Phases {
			if i < a.pipelinePhase {
				report.PhasesCompleted = append(report.PhasesCompleted, phase.Name)
			} else if i == a.pipelinePhase {
				report.FailedPhase = phase.Name
			}
		}
		// If we completed all phases, clear the failed phase
		if a.pipelinePhase >= len(a.pipeline.Phases) {
			report.FailedPhase = ""
		}
	}

	// Collect provider info from escalation chain
	if a.providerIdx < len(a.config.EscalationChain) {
		for i := 0; i <= a.providerIdx; i++ {
			level := a.config.EscalationChain[i]
			for _, p := range level.Providers {
				report.ProvidersUsed = append(report.ProvidersUsed, p)
			}
		}
	}

	return report
}

func formatDiagnosticsReport(r DiagnosticsReport) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\n## Session %s\n", r.SessionID))
	b.WriteString(fmt.Sprintf("- **Timestamp:** %s\n", r.Timestamp.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- **Duration:** %s\n", r.TotalDuration.Truncate(time.Second)))
	b.WriteString(fmt.Sprintf("- **Tool calls:** %d\n", r.ToolCallCount))

	if len(r.PhasesCompleted) > 0 {
		b.WriteString(fmt.Sprintf("- **Phases completed:** %s\n", strings.Join(r.PhasesCompleted, " → ")))
	}
	if r.FailedPhase != "" {
		b.WriteString(fmt.Sprintf("- **Failed at:** %s\n", r.FailedPhase))
	} else {
		b.WriteString("- **Result:** all phases passed\n")
	}
	if len(r.ProvidersUsed) > 0 {
		b.WriteString(fmt.Sprintf("- **Providers used:** %s\n", strings.Join(r.ProvidersUsed, ", ")))
	}
	if len(r.ErrorMessages) > 0 {
		b.WriteString("- **Errors:**\n")
		for _, e := range r.ErrorMessages {
			b.WriteString(fmt.Sprintf("  - %s\n", e))
		}
	}
	if len(r.VerifyResults) > 0 {
		b.WriteString("- **Verify results:**\n")
		for _, v := range r.VerifyResults {
			b.WriteString(fmt.Sprintf("  - %s\n", v))
		}
	}
	b.WriteString("\n---\n")
	return b.String()
}

// readPreviousDiagnostics reads the most recent session block from synroute.md.
// Returns empty string if no previous diagnostics exist.
func readPreviousDiagnostics(workDir string) string {
	if workDir == "" {
		return ""
	}
	path := filepath.Join(workDir, diagnosticsFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	content := string(data)
	// Find the last "## Session" block
	lastIdx := strings.LastIndex(content, "## Session")
	if lastIdx < 0 {
		return ""
	}
	return strings.TrimSpace(content[lastIdx:])
}
