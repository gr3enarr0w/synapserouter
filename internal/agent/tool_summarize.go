package agent

import (
	"fmt"
	"regexp"
	"strings"
)

// summarizeThreshold is the minimum output size (bytes) before summarization kicks in.
// Below this, full output is kept in conversation.
const summarizeThreshold = 2048

// ShouldSummarize returns true if the tool output is large enough to benefit
// from summarization. Small outputs are kept verbatim in conversation.
func ShouldSummarize(toolName, output string) bool {
	// file_write and file_edit already return summaries — never re-summarize
	if toolName == "file_write" || toolName == "file_edit" {
		return false
	}
	return len(output) > summarizeThreshold
}

// SummarizeToolOutput creates a concise summary of a tool's output for conversation.
// The full output is stored separately in the DB.
func SummarizeToolOutput(toolName string, args map[string]interface{}, output string, exitCode int) string {
	lines := strings.Split(output, "\n")
	totalLines := len(lines)

	switch toolName {
	case "bash":
		return summarizeBash(output, exitCode, lines, totalLines)
	case "file_read":
		path, _ := args["path"].(string)
		return summarizeFileRead(path, lines, totalLines)
	case "grep":
		pattern, _ := args["pattern"].(string)
		return summarizeGrep(pattern, lines, totalLines)
	case "glob":
		pattern, _ := args["pattern"].(string)
		return summarizeGlob(pattern, lines, totalLines)
	case "git":
		subcmd, _ := args["subcommand"].(string)
		return summarizeGit(subcmd, output, lines, totalLines)
	default:
		return summarizeGeneric(toolName, lines, totalLines)
	}
}

// compilationErrorPattern matches file:line:col error patterns from compilers (javac, gcc, go, rustc, tsc).
var compilationErrorPattern = regexp.MustCompile(`(?i)^.*\.(java|go|rs|ts|js|c|cpp|cs|py|rb|kt|swift):\d+[:\d]*:?\s*(error|cannot|undefined|unresolved|expected|illegal|invalid)`)

func summarizeBash(output string, exitCode int, lines []string, totalLines int) string {
	var b strings.Builder
	if exitCode == 0 {
		b.WriteString("exit 0 (success)")
	} else {
		b.WriteString(fmt.Sprintf("exit %d (error)", exitCode))
	}
	b.WriteString(fmt.Sprintf(" | %d lines\n", totalLines))

	// For errors: extract compilation error lines (file:line patterns) so the LLM
	// can see exactly which files and lines need fixing. Without this, Maven/Gradle
	// errors > 2KB get truncated to just "BUILD FAILURE" and the agent can't self-correct.
	if exitCode != 0 {
		var errorLines []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if compilationErrorPattern.MatchString(trimmed) {
				if len(trimmed) > 300 {
					trimmed = trimmed[:300] + "..."
				}
				errorLines = append(errorLines, trimmed)
			}
		}
		if len(errorLines) > 0 {
			b.WriteString("--- compilation errors ---\n")
			shown := errorLines
			if len(shown) > 15 {
				shown = shown[:15]
			}
			for _, l := range shown {
				b.WriteString(l + "\n")
			}
			if len(errorLines) > 15 {
				b.WriteString(fmt.Sprintf("... and %d more errors\n", len(errorLines)-15))
			}
			b.WriteString("--- end errors ---\n")
		}
	}

	// Show first 5 + last 5 non-empty lines for context
	var firstLines, lastLines []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && len(firstLines) < 5 {
			firstLines = append(firstLines, trimmed)
		}
	}
	for i := totalLines - 1; i >= 0 && len(lastLines) < 5; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			lastLines = append([]string{line}, lastLines...)
		}
	}

	for _, l := range firstLines {
		if len(l) > 200 {
			l = l[:200] + "..."
		}
		b.WriteString(l + "\n")
	}
	if totalLines > 10 {
		b.WriteString(fmt.Sprintf("... (%d lines omitted) ...\n", totalLines-10))
	}
	for _, l := range lastLines {
		if len(l) > 200 {
			l = l[:200] + "..."
		}
		b.WriteString(l + "\n")
	}
	return b.String()
}

func summarizeFileRead(path string, lines []string, totalLines int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s: %d lines\n", path, totalLines))

	if totalLines <= 10 {
		return strings.Join(lines, "\n")
	}

	// First 5 + last 5 lines
	for _, l := range lines[:5] {
		b.WriteString(l + "\n")
	}
	b.WriteString(fmt.Sprintf("... (%d lines omitted) ...\n", totalLines-10))
	for _, l := range lines[totalLines-5:] {
		b.WriteString(l + "\n")
	}
	return b.String()
}

func summarizeGrep(pattern string, lines []string, totalLines int) string {
	// Count unique files
	files := make(map[string]bool)
	for _, line := range lines {
		if idx := strings.Index(line, ":"); idx > 0 {
			files[line[:idx]] = true
		}
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d matches in %d files (pattern: %s)\n", totalLines, len(files), pattern))

	// Show first 5 matches
	show := 5
	if totalLines < show {
		show = totalLines
	}
	for _, l := range lines[:show] {
		if len(l) > 200 {
			l = l[:200] + "..."
		}
		b.WriteString(l + "\n")
	}
	if totalLines > show {
		b.WriteString(fmt.Sprintf("... and %d more matches\n", totalLines-show))
	}
	return b.String()
}

func summarizeGlob(pattern string, lines []string, totalLines int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d files matched (pattern: %s)\n", totalLines, pattern))

	show := 10
	if totalLines < show {
		show = totalLines
	}
	for _, l := range lines[:show] {
		b.WriteString(l + "\n")
	}
	if totalLines > show {
		b.WriteString(fmt.Sprintf("... and %d more files\n", totalLines-show))
	}
	return b.String()
}

func summarizeGit(subcmd, output string, lines []string, totalLines int) string {
	switch subcmd {
	case "diff":
		// Count files and insertions/deletions
		filesChanged := 0
		insertions := 0
		deletions := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "diff --git") {
				filesChanged++
			} else if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
				insertions++
			} else if strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "---") {
				deletions++
			}
		}
		return fmt.Sprintf("git diff: %d files, +%d/-%d lines\n", filesChanged, insertions, deletions)

	case "log":
		// Count commits
		commits := 0
		for _, l := range lines {
			if strings.HasPrefix(l, "commit ") {
				commits++
			}
		}
		// Show first 3 commit messages
		var b strings.Builder
		b.WriteString(fmt.Sprintf("git log: %d commits\n", commits))
		shown := 0
		for _, l := range lines {
			if shown >= 3 {
				break
			}
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "commit ") && !strings.HasPrefix(l, "Author:") && !strings.HasPrefix(l, "Date:") {
				b.WriteString("  " + l + "\n")
				shown++
			}
		}
		return b.String()

	default:
		// status, add, commit, branch — usually small, just truncate if huge
		return summarizeGeneric("git "+subcmd, lines, totalLines)
	}
}

func summarizeGeneric(toolName string, lines []string, totalLines int) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s: %d lines\n", toolName, totalLines))
	show := 5
	if totalLines < show {
		show = totalLines
	}
	for _, l := range lines[:show] {
		if len(l) > 200 {
			l = l[:200] + "..."
		}
		b.WriteString(l + "\n")
	}
	if totalLines > show {
		b.WriteString(fmt.Sprintf("... and %d more lines\n", totalLines-show))
	}
	return b.String()
}

// FormatArgsSummary creates a brief string describing the tool call arguments.
func FormatArgsSummary(toolName string, args map[string]interface{}) string {
	result := formatArgsSummaryRaw(toolName, args)
	return scrubSecrets(result)
}

func formatArgsSummaryRaw(toolName string, args map[string]interface{}) string {
	switch toolName {
	case "bash":
		cmd, _ := args["command"].(string)
		if len(cmd) > 100 {
			cmd = cmd[:100] + "..."
		}
		return cmd
	case "file_read", "file_write", "file_edit":
		path, _ := args["path"].(string)
		return path
	case "grep":
		pattern, _ := args["pattern"].(string)
		path, _ := args["path"].(string)
		return fmt.Sprintf("pattern=%s path=%s", pattern, path)
	case "glob":
		pattern, _ := args["pattern"].(string)
		return pattern
	case "git":
		subcmd, _ := args["subcommand"].(string)
		gitArgs, _ := args["args"].(string)
		return subcmd + " " + gitArgs
	default:
		return toolName
	}
}

// secretPatterns matches common credential patterns for redaction before DB storage.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(Bearer\s+)[^\s"']+`),
	regexp.MustCompile(`(?i)(token=)[^\s&"']+`),
	regexp.MustCompile(`(?i)(password=)[^\s&"']+`),
	regexp.MustCompile(`(?i)(api_key=)[^\s&"']+`),
	regexp.MustCompile(`(?i)(secret=)[^\s&"']+`),
	regexp.MustCompile(`(?i)(api[-_]?key[=:\s]+)[^\s"']+`),
	regexp.MustCompile(`(?i)(authorization:\s+)[^\s"']+`),
}

// scrubSecrets redacts known credential patterns from text.
func scrubSecrets(s string) string {
	for _, p := range secretPatterns {
		s = p.ReplaceAllString(s, "${1}[REDACTED]")
	}
	return s
}
