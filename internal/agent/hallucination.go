package agent

import (
	"math"
	"regexp"
	"strings"
)

// SignalType categorizes a hallucination signal.
type SignalType int

const (
	SignalFalseSuccess  SignalType = iota // claims success when tool showed failure
	SignalUnknownPath                     // references file never seen in tool outputs
	SignalContradiction                   // claim contradicts tool output
	SignalFabricatedData                  // quotes output that doesn't match stored
)

// HallucinationSignal represents a single detected inconsistency.
type HallucinationSignal struct {
	Type        SignalType
	Description string
	Evidence    string  // the contradicting fact
	Severity    float64 // 0.0-1.0
}

// HallucinationCheckResult is the outcome of checking an LLM response for hallucinations.
type HallucinationCheckResult struct {
	Detected   bool
	Signals    []HallucinationSignal
	Confidence float64 // 0.0-1.0
}

const hallucinationThreshold = 0.7

// hedgePatterns suppress signals when the LLM is hedging (not making definitive claims).
var hedgePatterns = regexp.MustCompile(`(?i)\b(likely|probably|should|might|could|possibly|may|appears to|seems to)\b`)

// successClaims patterns for detecting claims of success.
var testPassPatterns = regexp.MustCompile(`(?i)(all tests pass|tests pass|test suite passes|tests succeed|tests are passing|PASS\b)`)
var buildSuccessPatterns = regexp.MustCompile(`(?i)(builds? successfully|compilation succeeds|build passes|compiles? without errors|compiles? cleanly)`)
var successPatterns = regexp.MustCompile(`(?i)(exit 0|succeeded|completed successfully|no errors|ran successfully)`)
var failurePatterns = regexp.MustCompile(`(?i)(failed|error occurred|doesn't work|broken|exit code [1-9])`)

// filePathExtractor finds file paths in LLM responses.
var filePathExtractor = regexp.MustCompile(`(?:^|\s|` + "`" + `)((?:[a-zA-Z0-9_./-]+/)?[a-zA-Z0-9_.-]+\.(?:go|py|js|ts|rs|java|rb|c|cpp|h|yaml|yml|json|toml|md|sql|sh))`)

// CheckForHallucinations compares an LLM response against ground-truth facts.
// Takes FactProvider interface for testability. Returns nil if facts is nil.
// All checks are string matching — no LLM calls, <1ms latency.
func CheckForHallucinations(content string, facts FactProvider) *HallucinationCheckResult {
	if facts == nil || content == "" {
		return &HallucinationCheckResult{}
	}

	// Skip if content contains hedging language throughout
	hedgeCount := len(hedgePatterns.FindAllString(content, -1))
	contentWords := len(strings.Fields(content))
	heavilyHedged := contentWords > 0 && float64(hedgeCount)/float64(contentWords) > 0.1

	var signals []HallucinationSignal

	// Rule 1: Test pass/fail contradiction
	if !heavilyHedged {
		signals = append(signals, checkTestClaims(content, facts)...)
	}

	// Rule 2: Build success contradiction
	if !heavilyHedged {
		signals = append(signals, checkBuildClaims(content, facts)...)
	}

	// Rule 3: Unknown file paths
	signals = append(signals, checkUnknownPaths(content, facts)...)

	// Rule 4: Exit code contradictions
	if !heavilyHedged {
		signals = append(signals, checkExitCodeClaims(content, facts)...)
	}

	// Calculate confidence
	totalSeverity := 0.0
	for _, s := range signals {
		totalSeverity += s.Severity
	}
	// A single high-severity signal (0.9) should cross the threshold (0.7).
	// Two medium signals (0.5+0.5) should also cross.
	confidence := math.Min(1.0, totalSeverity)

	return &HallucinationCheckResult{
		Detected:   confidence >= hallucinationThreshold,
		Signals:    signals,
		Confidence: confidence,
	}
}

func checkTestClaims(content string, facts FactProvider) []HallucinationSignal {
	var signals []HallucinationSignal
	testResult := facts.LastTestResult()
	if testResult == nil {
		return nil
	}

	if testPassPatterns.MatchString(content) && !testResult.Passed {
		signals = append(signals, HallucinationSignal{
			Type:        SignalFalseSuccess,
			Description: "Claims tests pass but most recent test run failed",
			Evidence:    "Last test: exit code non-zero, " + strings.TrimSpace(testResult.lastSummary()),
			Severity:    0.9,
		})
	}
	return signals
}

func checkBuildClaims(content string, facts FactProvider) []HallucinationSignal {
	var signals []HallucinationSignal
	buildFact := facts.LastBashResult("go build")
	if buildFact == nil {
		buildFact = facts.LastBashResult("npm run build")
	}
	if buildFact == nil {
		buildFact = facts.LastBashResult("cargo build")
	}
	if buildFact == nil {
		return nil
	}

	if buildSuccessPatterns.MatchString(content) && buildFact.ExitCode != 0 {
		signals = append(signals, HallucinationSignal{
			Type:        SignalFalseSuccess,
			Description: "Claims build succeeds but most recent build failed",
			Evidence:    "Last build exit code: " + strings.TrimSpace(buildFact.LastLines),
			Severity:    0.9,
		})
	}
	return signals
}

func checkUnknownPaths(content string, facts FactProvider) []HallucinationSignal {
	var signals []HallucinationSignal
	knownPaths := facts.KnownPaths()
	if len(knownPaths) == 0 {
		return nil // no facts yet, can't judge
	}

	matches := filePathExtractor.FindAllStringSubmatch(content, 20)
	unknownCount := 0
	var unknownExamples []string

	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		path := m[1]
		// Skip very common paths that might be in system prompts
		if isCommonPath(path) {
			continue
		}
		if !knownPaths[path] {
			// Check if any known path ends with this filename
			found := false
			for known := range knownPaths {
				if strings.HasSuffix(known, "/"+path) || known == path {
					found = true
					break
				}
			}
			if !found {
				unknownCount++
				if len(unknownExamples) < 3 {
					unknownExamples = append(unknownExamples, path)
				}
			}
		}
	}

	if unknownCount > 0 {
		signals = append(signals, HallucinationSignal{
			Type:        SignalUnknownPath,
			Description: "References file paths never seen in tool outputs",
			Evidence:    "Unknown paths: " + strings.Join(unknownExamples, ", "),
			Severity:    0.5,
		})
	}
	return signals
}

func checkExitCodeClaims(content string, facts FactProvider) []HallucinationSignal {
	var signals []HallucinationSignal
	recentFacts := facts.RecentBashFacts(3)
	if len(recentFacts) == 0 {
		return nil
	}

	lastFact := recentFacts[len(recentFacts)-1]

	// Claims success when last command failed
	if successPatterns.MatchString(content) && lastFact.ExitCode != 0 {
		signals = append(signals, HallucinationSignal{
			Type:        SignalContradiction,
			Description: "Claims success but most recent command had non-zero exit code",
			Evidence:    "Command: " + lastFact.Command + " (exit " + string(rune('0'+lastFact.ExitCode)) + ")",
			Severity:    0.7,
		})
	}

	// Claims failure when last command succeeded
	if failurePatterns.MatchString(content) && lastFact.ExitCode == 0 && !testPassPatterns.MatchString(content) {
		signals = append(signals, HallucinationSignal{
			Type:        SignalContradiction,
			Description: "Claims failure but most recent command succeeded (exit 0)",
			Evidence:    "Command: " + lastFact.Command + " (exit 0)",
			Severity:    0.7,
		})
	}

	return signals
}

func isCommonPath(path string) bool {
	common := []string{
		"go.mod", "go.sum", "Makefile", "README.md", "CLAUDE.md",
		"package.json", "Cargo.toml", "requirements.txt",
		"main.go", "main.py", "index.js", "index.ts",
	}
	for _, c := range common {
		if path == c {
			return true
		}
	}
	return false
}

// lastSummary returns a brief summary string for a TestFact.
func (tf *TestFact) lastSummary() string {
	if tf.Passed {
		return "passed"
	}
	return "failed"
}
