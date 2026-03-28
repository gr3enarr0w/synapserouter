package agent

import (
	"log"
	"strings"
)

// TaskComplexity represents the assessed complexity level of a task.
type TaskComplexity int

const (
	// ComplexityTrivial — questions, explanations, lookups. No pipeline needed.
	ComplexityTrivial TaskComplexity = iota
	// ComplexitySimple — small fixes, single-file changes. Implement + self-check only.
	ComplexitySimple
	// ComplexityMedium — targeted operations (review, test, refactor). 1-2 phases.
	ComplexityMedium
	// ComplexityFull — build from spec, multi-file features. Full 6-phase pipeline.
	ComplexityFull
)

// String returns a human-readable label for the complexity level.
func (c TaskComplexity) String() string {
	switch c {
	case ComplexityTrivial:
		return "trivial"
	case ComplexitySimple:
		return "simple"
	case ComplexityMedium:
		return "medium"
	case ComplexityFull:
		return "full"
	default:
		return "unknown"
	}
}

// trivialKeywords signal the user is asking a question, not requesting work.
var trivialKeywords = []string{
	"what is", "what's", "explain", "how does", "how do", "why does", "why do",
	"tell me", "describe", "show me", "list", "help me understand",
	"what are", "can you explain", "what does",
}

// simpleKeywords signal a targeted, small-scope change.
var simpleKeywords = []string{
	"fix", "patch", "tweak", "typo", "rename", "update", "change",
	"add a", "remove", "delete", "bump", "adjust", "correct",
}

// mediumKeywords signal a review, test, or refactor operation.
var mediumKeywords = []string{
	"review", "test", "refactor", "optimize", "clean up", "lint",
	"check", "audit", "analyze", "inspect", "migrate",
}

// fullKeywords signal a substantial build or implementation task.
var fullKeywords = []string{
	"build", "implement", "create", "design", "architect", "develop",
	"write a", "set up", "bootstrap", "scaffold", "port", "rewrite",
	"new feature", "add feature", "from scratch",
}

// AssessComplexity evaluates the task message and context to determine
// how complex the task is, which drives pipeline adaptation.
func AssessComplexity(message string, hasSpecFile bool) TaskComplexity {
	lower := strings.ToLower(message)
	msgLen := len(message)

	// Spec file present always gets full pipeline.
	if hasSpecFile {
		return ComplexityFull
	}

	// Score each complexity level by keyword matches.
	trivialScore := countKeywordMatches(lower, trivialKeywords)
	simpleScore := countKeywordMatches(lower, simpleKeywords)
	mediumScore := countKeywordMatches(lower, mediumKeywords)
	fullScore := countKeywordMatches(lower, fullKeywords)

	// Message length heuristic: very short messages are usually simple.
	// Under 60 chars with no full or medium keywords → likely simple or trivial.
	if msgLen < 60 && fullScore == 0 && mediumScore == 0 {
		if trivialScore > 0 {
			return ComplexityTrivial
		}
		if simpleScore > 0 {
			return ComplexitySimple
		}
	}

	// Question marks with trivial keywords → trivial (regardless of message length).
	if strings.Contains(lower, "?") && trivialScore > 0 && fullScore == 0 {
		return ComplexityTrivial
	}

	// Full keywords or long messages with spec-like detail → full pipeline.
	if fullScore > 0 {
		return ComplexityFull
	}

	// Long messages (>500 chars) with multiple requirements → full pipeline.
	if msgLen > 500 {
		return ComplexityFull
	}

	// Medium keywords → medium complexity.
	if mediumScore > 0 {
		return ComplexityMedium
	}

	// Simple keywords → simple complexity.
	if simpleScore > 0 {
		return ComplexitySimple
	}

	// Default: if message is moderate length, assume medium; short = simple.
	if msgLen > 200 {
		return ComplexityMedium
	}
	return ComplexitySimple
}

// AdaptPipeline returns a pipeline adapted to the task's complexity.
// For trivial tasks it returns nil (no pipeline). For simple/medium tasks
// it returns a reduced pipeline. For complex tasks it returns the full pipeline.
func AdaptPipeline(pipeline *Pipeline, complexity TaskComplexity) *Pipeline {
	if pipeline == nil {
		return nil
	}

	switch complexity {
	case ComplexityTrivial:
		// No pipeline — just answer the question directly.
		return nil

	case ComplexitySimple:
		// Implement + self-check only — skip plan, code-review, acceptance-test, deploy.
		return buildReducedPipeline(pipeline, "simple", []string{"implement", "self-check"})

	case ComplexityMedium:
		// Implement + self-check + code-review — skip plan, acceptance-test, deploy.
		return buildReducedPipeline(pipeline, "medium", []string{"implement", "self-check", "code-review"})

	case ComplexityFull:
		// Full pipeline — no changes.
		return pipeline

	default:
		return pipeline
	}
}

// buildReducedPipeline extracts the named phases from the source pipeline,
// preserving their order and configuration.
func buildReducedPipeline(src *Pipeline, name string, phaseNames []string) *Pipeline {
	wanted := make(map[string]bool, len(phaseNames))
	for _, n := range phaseNames {
		wanted[n] = true
	}

	var phases []PipelinePhase
	for _, p := range src.Phases {
		if wanted[p.Name] {
			phases = append(phases, p)
		}
	}

	// If we couldn't find any of the requested phases (e.g., data-science pipeline
	// has different names), fall back to the full pipeline.
	if len(phases) == 0 {
		log.Printf("[AdaptPipeline] no matching phases for %s in %s pipeline, using full", name, src.Name)
		return src
	}

	return &Pipeline{
		Name:   src.Name + "-" + name,
		Phases: phases,
	}
}

// countKeywordMatches returns how many keywords from the list appear in the text.
func countKeywordMatches(text string, keywords []string) int {
	count := 0
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			count++
		}
	}
	return count
}
