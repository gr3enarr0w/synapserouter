package agent

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ReviewCycleTracker detects when review cycles are making no progress.
// If LOC and issues are unchanged for 2 consecutive cycles, we accept and move on.
// NOT goroutine-safe — must only be called from the agent's main loop.
type ReviewCycleTracker struct {
	prevLOC          int
	prevIssueHash    string
	stableCount      int // consecutive cycles with no change
	prevFindingCount int // track number of issues found
	divergeCount     int // consecutive cycles where findings INCREASED
}

// CheckStability compares current state against previous cycle.
// Returns true if the review has been stable for 2+ consecutive cycles.
func (r *ReviewCycleTracker) CheckStability(workDir string, reviewOutput string) bool {
	currentLOC := countLOC(workDir)
	currentHash := hashIssues(reviewOutput)

	stable := false
	if r.prevLOC > 0 && currentLOC == r.prevLOC && currentHash == r.prevIssueHash {
		r.stableCount++
		if r.stableCount >= 2 {
			stable = true
			log.Printf("[Agent] review cycle stable — no changes in %d cycles, accepting", r.stableCount)
		} else {
			log.Printf("[Agent] review cycle unchanged (%d/2 stable cycles)", r.stableCount)
		}
	} else {
		r.stableCount = 0
	}

	r.prevLOC = currentLOC
	r.prevIssueHash = currentHash
	return stable
}

// Reset clears the tracker state (e.g., when entering a new phase).
func (r *ReviewCycleTracker) Reset() {
	r.prevLOC = 0
	r.prevIssueHash = ""
	r.stableCount = 0
	r.prevFindingCount = 0
	r.divergeCount = 0
}

// CheckDivergence returns true if review findings are growing (cost diverging).
// This means cycling is making things worse, not better.
func (r *ReviewCycleTracker) CheckDivergence(reviewOutput string) bool {
	findingCount := countFindings(reviewOutput)

	if r.prevFindingCount > 0 && findingCount > r.prevFindingCount {
		r.divergeCount++
	} else {
		r.divergeCount = 0
	}
	r.prevFindingCount = findingCount

	// If findings increased for 2+ consecutive cycles, we're diverging
	return r.divergeCount >= 2
}

// CheckDivergenceCount uses an explicit finding count instead of text parsing.
// Used by K-LLM merge to provide accurate structured counts.
func (r *ReviewCycleTracker) CheckDivergenceCount(findingCount int) bool {
	if r.prevFindingCount > 0 && findingCount > r.prevFindingCount {
		r.divergeCount++
	} else {
		r.divergeCount = 0
	}
	r.prevFindingCount = findingCount
	return r.divergeCount >= 2
}

func countFindings(output string) int {
	count := 0
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			strings.Contains(trimmed, "FAIL") || strings.Contains(trimmed, "NEEDS_FIX") ||
			strings.Contains(trimmed, "ERROR") {
			count++
		}
	}
	return count
}

const maxLOCBytesPerFile = 1 << 20 // 1MB cap per file to prevent OOM on large generated files
const maxLOCTotalFiles = 10000     // safety cap on total files walked

// countLOC counts total lines of code in the working directory.
// Only counts .go, .py, .js, .ts, .rs, .java, .rb, .c, .cpp, .h files.
// Uses WalkDir (not Walk) for efficiency. Skips symlinks to prevent traversal attacks.
func countLOC(workDir string) int {
	if workDir == "" {
		return 0
	}

	codeExts := map[string]bool{
		".go": true, ".py": true, ".js": true, ".ts": true,
		".rs": true, ".java": true, ".rb": true,
		".c": true, ".cpp": true, ".h": true,
	}

	total := 0
	fileCount := 0
	_ = filepath.WalkDir(workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		// Skip symlinks entirely to prevent traversal/loops
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "vendor" || base == "node_modules" || base == ".claude" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if !codeExts[ext] {
			return nil
		}
		fileCount++
		if fileCount > maxLOCTotalFiles {
			return filepath.SkipAll
		}
		// Read with size cap to prevent OOM on large generated files
		f, err := os.Open(path) //nolint:G122 // path from WalkDir on agent work directory
		if err != nil {
			return nil
		}
		defer f.Close()
		buf := make([]byte, maxLOCBytesPerFile)
		n, _ := f.Read(buf)
		total += bytes.Count(buf[:n], []byte{'\n'})
		return nil
	})
	return total
}

// lineNumberPattern matches file:line patterns like "main.go:42:" or "line 57"
var lineNumberPattern = regexp.MustCompile(`(?::\d+:?|line \d+)`)

// hashIssues produces a stable hash of the review output to detect duplicate issues.
// Strips line numbers so the same issue at different positions hashes identically.
func hashIssues(reviewOutput string) string {
	normalized := strings.ToLower(strings.TrimSpace(reviewOutput))
	// Strip line numbers so same issue at different positions hashes the same
	normalized = lineNumberPattern.ReplaceAllString(normalized, ":")
	lines := strings.Split(normalized, "\n")
	var significant []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") || strings.HasPrefix(line, "===") {
			continue
		}
		significant = append(significant, line)
	}
	content := strings.Join(significant, "\n")
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h[:8])
}
