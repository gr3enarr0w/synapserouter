package agent

import (
	"fmt"
	"strings"
)

const maxCorrectiveMessageSize = 4 * 1024 // 4KB cap
const maxHallucinationRecalls = 3         // stop after 3 consecutive corrections

// autoRecall builds a corrective message when hallucination is detected.
// Queries ToolOutputStore for the actual output that contradicts the LLM's claims.
// Returns empty string if rate-limited or no useful correction can be made.
func (a *Agent) autoRecall(check *HallucinationCheckResult) string {
	if check == nil || !check.Detected {
		return ""
	}

	// Rate limiting: stop after maxHallucinationRecalls consecutive corrections
	if a.hallucinationRecallCount >= maxHallucinationRecalls {
		return ""
	}

	var b strings.Builder
	b.WriteString("CORRECTION: Your previous response contains claims that contradict actual tool outputs. Please re-assess.\n\n")

	for _, signal := range check.Signals {
		switch signal.Type {
		case SignalFalseSuccess:
			b.WriteString(fmt.Sprintf("- FALSE CLAIM: %s\n", signal.Description))
			b.WriteString(fmt.Sprintf("  ACTUAL: %s\n\n", signal.Evidence))

			// Try to retrieve the full output for context
			if a.factTracker != nil {
				if tr := a.factTracker.LastTestResult(); tr != nil && tr.OutputID > 0 && a.config.ToolStore != nil {
					if output, err := a.config.ToolStore.Retrieve(a.sessionID, tr.OutputID); err == nil {
						excerpt := truncateForCorrection(output)
						b.WriteString(fmt.Sprintf("  ACTUAL OUTPUT:\n```\n%s\n```\n\n", excerpt))
					}
				}
			}

		case SignalUnknownPath:
			b.WriteString(fmt.Sprintf("- %s\n", signal.Description))
			b.WriteString(fmt.Sprintf("  %s\n", signal.Evidence))
			if a.factTracker != nil {
				paths := a.factTracker.KnownPaths()
				if len(paths) > 0 {
					pathList := make([]string, 0, len(paths))
					for p := range paths {
						pathList = append(pathList, p)
						if len(pathList) >= 10 {
							break
						}
					}
					b.WriteString(fmt.Sprintf("  KNOWN PATHS: %s\n\n", strings.Join(pathList, ", ")))
				}
			}

		case SignalContradiction:
			b.WriteString(fmt.Sprintf("- CONTRADICTION: %s\n", signal.Description))
			b.WriteString(fmt.Sprintf("  ACTUAL: %s\n\n", signal.Evidence))

		case SignalFabricatedData:
			b.WriteString(fmt.Sprintf("- FABRICATED DATA: %s\n", signal.Description))
			b.WriteString(fmt.Sprintf("  ACTUAL: %s\n\n", signal.Evidence))
		}
	}

	b.WriteString("Please provide a corrected response based on the actual data above.")

	result := b.String()

	// Cap size
	if len(result) > maxCorrectiveMessageSize {
		result = result[:maxCorrectiveMessageSize] + "\n...(truncated)"
	}

	// Scrub secrets from corrective message (security-review skill requirement)
	result = scrubSecrets(result)

	a.hallucinationRecallCount++
	return result
}

func truncateForCorrection(output string) string {
	if len(output) > 1024 {
		return output[:512] + "\n...\n" + output[len(output)-512:]
	}
	return output
}
