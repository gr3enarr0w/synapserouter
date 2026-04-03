package agent

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
)

// CompressedContext holds a structured summary of compressed conversation messages.
// Extracted via regex (no LLM calls) for instant, zero-cost compression.
type CompressedContext struct {
	Phase        string
	Decisions    []string // key decisions: "decided", "chose", "will use", "going with"
	Rationale    []string // reasoning: "because", "since", "the reason"
	FilesChanged []string // file paths touched
	TestResults  []string // test output lines
	Errors       []string // error lines
	OpenItems    []string // unresolved: "TODO", "FIXME", "need to", "should we"
	MsgCount     int      // how many messages were compressed
}

// observationMaskThreshold is the minimum tool output size (bytes) to mask.
// Outputs smaller than this are kept verbatim (not worth masking).
const observationMaskThreshold = 512

// Regex patterns for structured extraction.
var (
	decisionRe  = regexp.MustCompile(`(?i)\b(decided|chose|chosen|will use|going with|selected|picking|opted for)\b`)
	rationaleRe = regexp.MustCompile(`(?i)\b(because|since|the reason|due to|in order to|so that)\b`)
	errorRe     = regexp.MustCompile(`(?i)(error:|failed:|panic:|fatal:|FAIL\b|cannot |could not )`)
	openItemRe  = regexp.MustCompile(`(?i)\b(TODO|FIXME|HACK|need to|should we|might want to|consider)\b`)
	testLineRe  = regexp.MustCompile(`(?i)(PASS|FAIL|ok\s+\S+|---\s+(PASS|FAIL)|test.*passed|test.*failed|\d+\s+passed|\d+\s+failed)`)
)

// ExtractStructuredSummary extracts a structured summary from conversation messages.
// Uses regex-based heuristics — no LLM calls, instant, zero cost.
func ExtractStructuredSummary(msgs []providers.Message, phase string) CompressedContext {
	cc := CompressedContext{
		Phase:    phase,
		MsgCount: len(msgs),
	}

	seenDecisions := make(map[string]bool)
	seenRationale := make(map[string]bool)
	seenErrors := make(map[string]bool)
	seenOpen := make(map[string]bool)
	seenTests := make(map[string]bool)

	for _, msg := range msgs {
		if msg.Content == "" {
			continue
		}

		for _, line := range strings.Split(msg.Content, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || len(line) < 10 {
				continue
			}

			// Cap line length for summary
			displayLine := line
			if len(displayLine) > 200 {
				displayLine = displayLine[:197] + "..."
			}

			if decisionRe.MatchString(line) && !seenDecisions[displayLine] {
				cc.Decisions = append(cc.Decisions, displayLine)
				seenDecisions[displayLine] = true
			}
			if rationaleRe.MatchString(line) && !seenRationale[displayLine] {
				cc.Rationale = append(cc.Rationale, displayLine)
				seenRationale[displayLine] = true
			}
			if errorRe.MatchString(line) && !seenErrors[displayLine] {
				cc.Errors = append(cc.Errors, displayLine)
				seenErrors[displayLine] = true
			}
			if openItemRe.MatchString(line) && !seenOpen[displayLine] {
				cc.OpenItems = append(cc.OpenItems, displayLine)
				seenOpen[displayLine] = true
			}
			if testLineRe.MatchString(line) && !seenTests[displayLine] {
				cc.TestResults = append(cc.TestResults, displayLine)
				seenTests[displayLine] = true
			}
		}

		// Extract file paths
		paths := extractFilePaths(msg.Content)
		for _, p := range paths {
			found := false
			for _, existing := range cc.FilesChanged {
				if existing == p {
					found = true
					break
				}
			}
			if !found {
				cc.FilesChanged = append(cc.FilesChanged, p)
			}
		}
	}

	// Cap sections to prevent summary bloat
	cc.Decisions = capSlice(cc.Decisions, 10)
	cc.Rationale = capSlice(cc.Rationale, 10)
	cc.Errors = capSlice(cc.Errors, 10)
	cc.OpenItems = capSlice(cc.OpenItems, 10)
	cc.TestResults = capSlice(cc.TestResults, 10)
	cc.FilesChanged = capSlice(cc.FilesChanged, 20)

	return cc
}

// FormatCompressedContext formats a CompressedContext as structured text
// suitable for injection as a conversation message.
func FormatCompressedContext(cc CompressedContext) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "=== COMPRESSED CONTEXT (%d messages from %s phase) ===\n\n", cc.MsgCount, cc.Phase)

	if len(cc.Decisions) > 0 {
		sb.WriteString("DECISIONS MADE:\n")
		for _, d := range cc.Decisions {
			fmt.Fprintf(&sb, "  - %s\n", d)
		}
		sb.WriteString("\n")
	}

	if len(cc.Rationale) > 0 {
		sb.WriteString("RATIONALE:\n")
		for _, r := range cc.Rationale {
			fmt.Fprintf(&sb, "  - %s\n", r)
		}
		sb.WriteString("\n")
	}

	if len(cc.FilesChanged) > 0 {
		sb.WriteString("FILES TOUCHED:\n")
		for _, f := range cc.FilesChanged {
			fmt.Fprintf(&sb, "  - %s\n", f)
		}
		sb.WriteString("\n")
	}

	if len(cc.TestResults) > 0 {
		sb.WriteString("TEST RESULTS:\n")
		for _, t := range cc.TestResults {
			fmt.Fprintf(&sb, "  - %s\n", t)
		}
		sb.WriteString("\n")
	}

	if len(cc.Errors) > 0 {
		sb.WriteString("ERRORS ENCOUNTERED:\n")
		for _, e := range cc.Errors {
			fmt.Fprintf(&sb, "  - %s\n", e)
		}
		sb.WriteString("\n")
	}

	if len(cc.OpenItems) > 0 {
		sb.WriteString("OPEN ITEMS:\n")
		for _, o := range cc.OpenItems {
			fmt.Fprintf(&sb, "  - %s\n", o)
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Use the recall tool to retrieve full details from earlier in the conversation.")
	return sb.String()
}

// MaskObservations replaces large tool output bodies with a placeholder,
// keeping tool name and args summary visible. This reduces token count by
// ~52% (per JetBrains research) without losing the conversation flow.
// Only masks tool-role messages exceeding observationMaskThreshold bytes.
// Returns a new slice; does not mutate the input.
func MaskObservations(msgs []providers.Message) []providers.Message {
	result := make([]providers.Message, len(msgs))
	for i, msg := range msgs {
		if msg.Role == "tool" && len(msg.Content) > observationMaskThreshold {
			// Keep first line (usually tool name/summary) + mask the rest
			firstLine := msg.Content
			if idx := strings.Index(msg.Content, "\n"); idx > 0 {
				firstLine = msg.Content[:idx]
			}
			result[i] = providers.Message{
				Role:       msg.Role,
				Content:    firstLine + "\n[output stored — use recall tool to retrieve full details]",
				ToolCallID: msg.ToolCallID,
			}
		} else {
			result[i] = msg
		}
	}
	return result
}

// capSlice returns at most n elements from a string slice.
func capSlice(s []string, n int) []string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
