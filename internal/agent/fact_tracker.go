package agent

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// FactProvider is the interface for querying ground-truth facts.
// Implemented by FactTracker. Small interface for testability (go-patterns).
type FactProvider interface {
	KnownPaths() map[string]bool
	LastTestResult() *TestFact
	LastBashResult(prefix string) *BashFact
	RecentBashFacts(n int) []BashFact
}

// TestFact captures the result of a test run.
type TestFact struct {
	Passed    bool
	FailCount int
	PassCount int
	OutputID  int64
	Timestamp time.Time
}

// BashFact captures the result of a bash command.
type BashFact struct {
	Command   string
	ExitCode  int
	LastLines string // last 5 lines of output
	OutputID  int64
	Timestamp time.Time
}

// FactTracker accumulates ground-truth facts from tool outputs as they execute.
// In-memory, zero-latency. NOT goroutine-safe for writes — only called from agent loop.
// Reads are protected by RWMutex for potential concurrent access.
type FactTracker struct {
	knownPaths map[string]bool
	bashFacts  []BashFact
	testResult *TestFact
	mu         sync.RWMutex
}

// NewFactTracker creates a new fact tracker.
func NewFactTracker() *FactTracker {
	return &FactTracker{
		knownPaths: make(map[string]bool),
	}
}

// RecordToolOutput extracts and stores facts from a tool execution result.
func (ft *FactTracker) RecordToolOutput(toolName string, args map[string]interface{}, output string, exitCode int, outputID int64) {
	ft.mu.Lock()
	defer ft.mu.Unlock()

	switch toolName {
	case "bash":
		ft.recordBash(args, output, exitCode, outputID)
	case "file_read", "grep", "glob":
		ft.recordPathsFromArgs(args)
		ft.extractPathsFromOutput(output)
	case "file_write", "file_edit":
		ft.recordPathsFromArgs(args)
	}
}

func (ft *FactTracker) recordBash(args map[string]interface{}, output string, exitCode int, outputID int64) {
	cmd, _ := args["command"].(string)

	fact := BashFact{
		Command:   cmd,
		ExitCode:  exitCode,
		LastLines: lastNLines(output, 5),
		OutputID:  outputID,
		Timestamp: time.Now(),
	}
	ft.bashFacts = append(ft.bashFacts, fact)

	// Keep only last 20 bash facts
	if len(ft.bashFacts) > 20 {
		ft.bashFacts = ft.bashFacts[len(ft.bashFacts)-20:]
	}

	// Detect test commands and parse results
	if isTestCommand(cmd) {
		ft.testResult = parseTestResult(output, exitCode, outputID)
	}
}

func (ft *FactTracker) recordPathsFromArgs(args map[string]interface{}) {
	if path, ok := args["path"].(string); ok && path != "" {
		ft.knownPaths[path] = true
	}
	if path, ok := args["file_path"].(string); ok && path != "" {
		ft.knownPaths[path] = true
	}
	if pattern, ok := args["pattern"].(string); ok && pattern != "" {
		// For glob patterns, store the directory part
		if idx := strings.LastIndex(pattern, "/"); idx >= 0 {
			ft.knownPaths[pattern[:idx+1]] = true
		}
	}
}

// pathPattern matches file paths in tool output
var pathPattern = regexp.MustCompile(`(?:^|\s)((?:[a-zA-Z0-9_./-]+/)?[a-zA-Z0-9_.-]+\.(?:go|py|js|ts|rs|java|rb|c|cpp|h|yaml|yml|json|toml|md|sql|sh))`)

func (ft *FactTracker) extractPathsFromOutput(output string) {
	matches := pathPattern.FindAllStringSubmatch(output, 50)
	for _, m := range matches {
		if len(m) > 1 {
			ft.knownPaths[m[1]] = true
		}
	}
}

// KnownPaths returns all file paths seen in tool outputs.
func (ft *FactTracker) KnownPaths() map[string]bool {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	// Return a copy to prevent races
	paths := make(map[string]bool, len(ft.knownPaths))
	for k, v := range ft.knownPaths {
		paths[k] = v
	}
	return paths
}

// LastTestResult returns the most recent test run result, or nil.
func (ft *FactTracker) LastTestResult() *TestFact {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	return ft.testResult
}

// LastBashResult returns the most recent bash result matching a command prefix, or nil.
func (ft *FactTracker) LastBashResult(prefix string) *BashFact {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	prefix = strings.ToLower(prefix)
	for i := len(ft.bashFacts) - 1; i >= 0; i-- {
		if strings.HasPrefix(strings.ToLower(ft.bashFacts[i].Command), prefix) {
			fact := ft.bashFacts[i]
			return &fact
		}
	}
	return nil
}

// RecentBashFacts returns the last N bash facts.
func (ft *FactTracker) RecentBashFacts(n int) []BashFact {
	ft.mu.RLock()
	defer ft.mu.RUnlock()
	if n > len(ft.bashFacts) {
		n = len(ft.bashFacts)
	}
	result := make([]BashFact, n)
	copy(result, ft.bashFacts[len(ft.bashFacts)-n:])
	return result
}

// --- helpers ---

var testCommandPrefixes = []string{
	"go test", "pytest", "npm test", "npx jest", "cargo test",
	"python -m pytest", "python3 -m pytest", "make test",
	"gradle test", "gradlew test", "./gradlew test",
	"mvn test", "mvnw test", "./mvnw test",
	"jest", "npx vitest", "npm run test",
}

func isTestCommand(cmd string) bool {
	lower := strings.ToLower(cmd)
	for _, prefix := range testCommandPrefixes {
		if strings.HasPrefix(lower, prefix) || strings.Contains(lower, " "+prefix) {
			return true
		}
	}
	return false
}

var goTestPassPattern = regexp.MustCompile(`ok\s+\S+\s+[\d.]+s`)
var goTestFailPattern = regexp.MustCompile(`FAIL\s+\S+`)
var pytestPattern = regexp.MustCompile(`(\d+) passed`)
var pytestFailPattern = regexp.MustCompile(`(\d+) failed`)

func parseTestResult(output string, exitCode int, outputID int64) *TestFact {
	fact := &TestFact{
		Passed:    exitCode == 0,
		OutputID:  outputID,
		Timestamp: time.Now(),
	}
	fact.PassCount = len(goTestPassPattern.FindAllString(output, -1))
	fact.FailCount = len(goTestFailPattern.FindAllString(output, -1))

	// Also try pytest format
	if m := pytestPattern.FindStringSubmatch(output); len(m) > 1 {
		// has pytest output
		if fm := pytestFailPattern.FindStringSubmatch(output); len(fm) > 1 {
			fact.Passed = false
		}
	}
	return fact
}

func lastNLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}
