package agent

import (
	"fmt"
	"log"
	"regexp"
)

// RegressionTracker monitors compilation error counts across build attempts
// to detect when agent changes make things worse instead of better.
type RegressionTracker struct {
	lastErrorCount int
	worseningCount int // consecutive times errors increased
	buildCommand   string
}

// NewRegressionTracker creates a tracker with the given build command.
func NewRegressionTracker(buildCmd string) *RegressionTracker {
	return &RegressionTracker{
		buildCommand:   buildCmd,
		lastErrorCount: -1, // -1 = no baseline yet
	}
}

// compilationErrorRe matches compiler error patterns across languages.
var compilationErrorRe = regexp.MustCompile(`(?im)^.*\.(java|go|rs|ts|js|c|cpp|cs|py|rb|kt|swift):\d+.*(?:error|cannot find symbol|undefined|unresolved)`)

// CountErrors counts compilation error lines in build output.
func CountErrors(output string) int {
	return len(compilationErrorRe.FindAllString(output, -1))
}

// Check compares current error count against previous. Returns a warning
// message if errors increased, empty string otherwise.
func (rt *RegressionTracker) Check(buildOutput string, exitCode int) string {
	if exitCode == 0 {
		// Build succeeded — reset tracker
		rt.lastErrorCount = 0
		rt.worseningCount = 0
		return ""
	}

	current := CountErrors(buildOutput)
	if rt.lastErrorCount < 0 {
		// First build — establish baseline
		rt.lastErrorCount = current
		return ""
	}

	if current > rt.lastErrorCount && rt.lastErrorCount > 0 {
		rt.worseningCount++
		msg := ""
		if rt.worseningCount >= 2 {
			log.Printf("[Agent] REGRESSION: errors increased %d → %d (%d consecutive worsening)",
				rt.lastErrorCount, current, rt.worseningCount)
			msg = fmt.Sprintf("REGRESSION DETECTED: Your recent changes INCREASED compilation errors from %d to %d. "+
				"This has happened %d times in a row. STOP creating new files. Instead:\n"+
				"1. Read the error output carefully\n"+
				"2. Use file_edit to fix the SPECIFIC errors\n"+
				"3. Re-run the build to verify errors decrease\n"+
				"Do NOT create new files or restructure packages.", rt.lastErrorCount, current, rt.worseningCount)
		}
		rt.lastErrorCount = current
		return msg
	}

	if current < rt.lastErrorCount {
		rt.worseningCount = 0 // progress — reset
	}
	rt.lastErrorCount = current
	return ""
}
