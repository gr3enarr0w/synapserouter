package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReviewCycleTracker_DetectsStability(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	tracker := &ReviewCycleTracker{}
	issues := "error: unused variable x\nerror: missing return"

	// First check — establishes baseline
	if tracker.CheckStability(dir, issues) {
		t.Error("first check should not be stable")
	}

	// Second check — same LOC, same issues (1/2 stable)
	if tracker.CheckStability(dir, issues) {
		t.Error("second check should not be stable yet (1/2)")
	}

	// Third check — same again (2/2 stable) → should accept
	if !tracker.CheckStability(dir, issues) {
		t.Error("third check should be stable (2/2 consecutive)")
	}
}

func TestReviewCycleTracker_ResetsOnChange(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	tracker := &ReviewCycleTracker{}
	issues := "error: unused variable x"

	tracker.CheckStability(dir, issues) // baseline
	tracker.CheckStability(dir, issues) // 1/2 stable

	// Change the code — should reset stability counter
	writeGoFile(t, dir, "main.go", "package main\nfunc main() {\n\tfmt.Println(\"changed\")\n}\n")
	if tracker.CheckStability(dir, issues) {
		t.Error("should not be stable after code change")
	}
}

func TestReviewCycleTracker_ResetsOnNewIssues(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", "package main\nfunc main() {}\n")

	tracker := &ReviewCycleTracker{}

	tracker.CheckStability(dir, "error: A")  // baseline
	tracker.CheckStability(dir, "error: A")  // 1/2 stable

	// Different issues — should reset
	if tracker.CheckStability(dir, "error: B") {
		t.Error("should not be stable when issues change")
	}
}

func TestReviewCycleTracker_Reset(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", "package main\n")

	tracker := &ReviewCycleTracker{}
	tracker.CheckStability(dir, "err")
	tracker.CheckStability(dir, "err") // 1/2

	tracker.Reset()

	// After reset, should need 2 more stable cycles
	tracker.CheckStability(dir, "err") // new baseline
	if tracker.CheckStability(dir, "err") {
		// Only 1/2 after reset, not 2/2
		t.Error("should not be stable after reset with only 1 stable cycle")
	}
}

func TestCountLOC(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", "package main\n\nfunc main() {\n}\n")
	writeGoFile(t, dir, "util.go", "package main\n\nfunc helper() {}\n")

	loc := countLOC(dir)
	// "package main\n\nfunc main() {\n}\n" = 5 lines (trailing newline creates empty split)
	// "package main\n\nfunc helper() {}\n" = 4 lines
	if loc < 7 {
		t.Errorf("expected at least 7 LOC, got %d", loc)
	}
}

func TestCountLOC_SkipsVendor(t *testing.T) {
	dir := t.TempDir()
	writeGoFile(t, dir, "main.go", "package main\n")
	os.MkdirAll(filepath.Join(dir, "vendor"), 0755)
	writeGoFile(t, filepath.Join(dir, "vendor"), "dep.go", "package dep\n\n// lots of code\n")

	loc := countLOC(dir)
	if loc > 2 { // only main.go ("package main\n" = 1-2 lines), vendor excluded
		t.Errorf("expected 1-2 LOC (vendor excluded), got %d", loc)
	}
}

func TestHashIssues_Deterministic(t *testing.T) {
	h1 := hashIssues("error: unused variable x\nerror: missing return")
	h2 := hashIssues("error: unused variable x\nerror: missing return")
	if h1 != h2 {
		t.Error("same issues should produce same hash")
	}
}

func TestHashIssues_DifferentForDifferentIssues(t *testing.T) {
	h1 := hashIssues("error: unused variable x")
	h2 := hashIssues("error: undefined function y")
	if h1 == h2 {
		t.Error("different issues should produce different hashes")
	}
}

func TestCountLOC_EmptyDir(t *testing.T) {
	if loc := countLOC(""); loc != 0 {
		t.Errorf("empty dir should return 0, got %d", loc)
	}
}

func writeGoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}
}
