package agent

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// FileDiff represents a per-file diff for review context.
type FileDiff struct {
	Path  string // relative file path
	IsNew bool   // true if file didn't exist before (added, not modified)
	Diff  string // unified diff content (or file preview for new files)
	Lines int    // number of lines in the diff/preview
}

// Files to exclude from review diffs (noise for reviewers).
var diffFilterPatterns = []string{
	".lock", "go.sum", ".min.js", ".min.css",
	"package-lock.json", "yarn.lock", "pnpm-lock.yaml",
	".pb.go", ".gen.go", // generated code
}

// maxDiffLinesPerFile caps individual file diffs to prevent prompt bloat.
const maxDiffLinesPerFile = 100

// maxDiffLinesTotal caps total diff output across all files.
const maxDiffLinesTotal = 500

// maxNewFilePreviewLines limits new file previews.
const maxNewFilePreviewLines = 100

// getChangedDiffs returns structured per-file diffs for review context.
// Falls back gracefully: no git repo → nil, no changes → nil.
func (a *Agent) getChangedDiffs() []FileDiff {
	workDir := a.config.WorkDir
	if workDir == "" {
		workDir = "."
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get full unified diff
	cmd := exec.CommandContext(ctx, "git", "diff", "HEAD")
	cmd.Dir = workDir
	diffOut, err := cmd.Output()
	if err != nil {
		// Try staged
		cmd = exec.CommandContext(ctx, "git", "diff", "--cached")
		cmd.Dir = workDir
		diffOut, err = cmd.Output()
		if err != nil {
			return nil
		}
	}

	if len(diffOut) == 0 {
		return nil
	}

	// Get list of new files (added, not modified)
	cmd = exec.CommandContext(ctx, "git", "diff", "--name-only", "--diff-filter=A", "HEAD")
	cmd.Dir = workDir
	newFilesOut, _ := cmd.Output()
	newFiles := make(map[string]bool)
	for _, f := range strings.Split(strings.TrimSpace(string(newFilesOut)), "\n") {
		f = strings.TrimSpace(f)
		if f != "" {
			newFiles[f] = true
		}
	}

	// Parse unified diff into per-file diffs
	diffs := parseUnifiedDiff(string(diffOut), newFiles)

	// Filter out noise files
	var filtered []FileDiff
	for _, d := range diffs {
		if shouldFilterDiff(d.Path) {
			continue
		}
		filtered = append(filtered, d)
	}

	return filtered
}

// parseUnifiedDiff splits a unified diff output into per-file FileDiff structs.
func parseUnifiedDiff(rawDiff string, newFiles map[string]bool) []FileDiff {
	var diffs []FileDiff
	var current *FileDiff
	var currentLines []string

	for _, line := range strings.Split(rawDiff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			// Flush previous
			if current != nil {
				current.Diff = strings.Join(currentLines, "\n")
				current.Lines = len(currentLines)
				diffs = append(diffs, *current)
			}

			// Extract file path from "diff --git a/path b/path"
			parts := strings.SplitN(line, " b/", 2)
			path := ""
			if len(parts) == 2 {
				path = parts[1]
			}

			current = &FileDiff{
				Path:  path,
				IsNew: newFiles[path],
			}
			currentLines = []string{line}
		} else if current != nil {
			currentLines = append(currentLines, line)
		}
	}

	// Flush last
	if current != nil {
		current.Diff = strings.Join(currentLines, "\n")
		current.Lines = len(currentLines)
		diffs = append(diffs, *current)
	}

	return diffs
}

// formatDiffContext formats FileDiffs for inclusion in review prompts.
// Groups changed and new files, truncates long diffs, caps total lines.
func formatDiffContext(diffs []FileDiff, maxLines int) string {
	if len(diffs) == 0 {
		return ""
	}

	if maxLines <= 0 {
		maxLines = maxDiffLinesTotal
	}

	var changed, newFiles []FileDiff
	for _, d := range diffs {
		if d.IsNew {
			newFiles = append(newFiles, d)
		} else {
			changed = append(changed, d)
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Git diff summary: %d files changed, %d new files\n\n", len(changed), len(newFiles))
	totalLines := 0

	// Changed files with diffs
	if len(changed) > 0 {
		sb.WriteString("=== CHANGED FILES ===\n\n")
		for _, d := range changed {
			if totalLines >= maxLines {
				fmt.Fprintf(&sb, "[... remaining %d files omitted — use file_read for full content]\n", len(changed))
				break
			}

			diffText := d.Diff
			diffLines := strings.Split(diffText, "\n")
			if len(diffLines) > maxDiffLinesPerFile {
				omitted := len(diffLines) - maxDiffLinesPerFile
				diffText = strings.Join(diffLines[:maxDiffLinesPerFile], "\n") +
					fmt.Sprintf("\n[... %d more lines — use file_read %s for full content]", omitted, d.Path)
				totalLines += maxDiffLinesPerFile
			} else {
				totalLines += len(diffLines)
			}

			sb.WriteString(diffText)
			sb.WriteString("\n\n")
		}
	}

	// New files with content preview
	if len(newFiles) > 0 {
		sb.WriteString("=== NEW FILES ===\n\n")
		for _, d := range newFiles {
			if totalLines >= maxLines {
				fmt.Fprintf(&sb, "[... remaining %d new files omitted]\n", len(newFiles))
				break
			}

			diffText := d.Diff
			diffLines := strings.Split(diffText, "\n")
			if len(diffLines) > maxNewFilePreviewLines {
				omitted := len(diffLines) - maxNewFilePreviewLines
				diffText = strings.Join(diffLines[:maxNewFilePreviewLines], "\n") +
					fmt.Sprintf("\n[... %d more lines — use file_read %s for full content]", omitted, d.Path)
				totalLines += maxNewFilePreviewLines
			} else {
				totalLines += len(diffLines)
			}

			sb.WriteString(diffText)
			sb.WriteString("\n\n")
		}
	}

	return sb.String()
}

// shuffleDiffs returns a deterministic shuffle of FileDiffs (for K-LLM bias reduction).
func shuffleDiffs(diffs []FileDiff, seed int64) []FileDiff {
	if len(diffs) <= 1 {
		result := make([]FileDiff, len(diffs))
		copy(result, diffs)
		return result
	}

	// Convert to string slice, shuffle, reorder
	paths := make([]string, len(diffs))
	for i, d := range diffs {
		paths[i] = d.Path
	}
	shuffled := ShuffleFileOrder(paths, seed)

	// Build path-to-diff map
	byPath := make(map[string]FileDiff)
	for _, d := range diffs {
		byPath[d.Path] = d
	}

	result := make([]FileDiff, 0, len(diffs))
	for _, p := range shuffled {
		if d, ok := byPath[p]; ok {
			result = append(result, d)
		}
	}
	return result
}

// shouldFilterDiff returns true if the file should be excluded from review diffs.
func shouldFilterDiff(path string) bool {
	lower := strings.ToLower(path)
	for _, pattern := range diffFilterPatterns {
		if strings.HasSuffix(lower, pattern) {
			return true
		}
	}
	return false
}
