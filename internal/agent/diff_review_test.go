package agent

import (
	"strings"
	"testing"
)

func TestParseUnifiedDiff(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc123..def456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {
+    fmt.Println("hello")
 }
diff --git a/new.go b/new.go
new file mode 100644
index 0000000..abc123
--- /dev/null
+++ b/new.go
@@ -0,0 +1,3 @@
+package main
+
+func helper() {}
`

	newFiles := map[string]bool{"new.go": true}
	diffs := parseUnifiedDiff(raw, newFiles)

	if len(diffs) != 2 {
		t.Fatalf("expected 2 file diffs, got %d", len(diffs))
	}

	if diffs[0].Path != "main.go" {
		t.Errorf("first diff path = %q, want main.go", diffs[0].Path)
	}
	if diffs[0].IsNew {
		t.Error("main.go should not be marked as new")
	}

	if diffs[1].Path != "new.go" {
		t.Errorf("second diff path = %q, want new.go", diffs[1].Path)
	}
	if !diffs[1].IsNew {
		t.Error("new.go should be marked as new")
	}
}

func TestFormatDiffContext(t *testing.T) {
	diffs := []FileDiff{
		{Path: "main.go", IsNew: false, Diff: "diff --git a/main.go b/main.go\n+added line", Lines: 2},
		{Path: "new.go", IsNew: true, Diff: "diff --git a/new.go b/new.go\n+package main", Lines: 2},
	}

	output := formatDiffContext(diffs, 500)

	if !strings.Contains(output, "1 files changed, 1 new files") {
		t.Error("missing header summary")
	}
	if !strings.Contains(output, "CHANGED FILES") {
		t.Error("missing CHANGED FILES section")
	}
	if !strings.Contains(output, "NEW FILES") {
		t.Error("missing NEW FILES section")
	}
	if !strings.Contains(output, "+added line") {
		t.Error("missing diff content")
	}
}

func TestFormatDiffContext_Empty(t *testing.T) {
	output := formatDiffContext(nil, 500)
	if output != "" {
		t.Errorf("expected empty output for nil diffs, got %q", output)
	}
}

func TestFormatDiffContext_LargeDiff(t *testing.T) {
	// Create a diff with 200 lines
	var lines []string
	for i := 0; i < 200; i++ {
		lines = append(lines, "+line "+string(rune('a'+i%26)))
	}
	largeDiff := strings.Join(lines, "\n")

	diffs := []FileDiff{
		{Path: "big.go", IsNew: false, Diff: largeDiff, Lines: 200},
	}

	output := formatDiffContext(diffs, 500)

	// Should be truncated at maxDiffLinesPerFile (100)
	if !strings.Contains(output, "more lines") {
		t.Error("large diff should be truncated with 'more lines' message")
	}
}

func TestShouldFilterDiff(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"go.sum", true},
		{"package-lock.json", true},
		{"yarn.lock", true},
		{"main.min.js", true},
		{"styles.min.css", true},
		{"types.pb.go", true},
		{"main.go", false},
		{"internal/agent/agent.go", false},
		{"README.md", false},
	}

	for _, tt := range tests {
		got := shouldFilterDiff(tt.path)
		if got != tt.want {
			t.Errorf("shouldFilterDiff(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShuffleDiffs(t *testing.T) {
	diffs := []FileDiff{
		{Path: "a.go"}, {Path: "b.go"}, {Path: "c.go"}, {Path: "d.go"}, {Path: "e.go"},
	}

	s1 := shuffleDiffs(diffs, 0)
	s2 := shuffleDiffs(diffs, 1000003)

	// Different seeds should produce different orders
	different := false
	for i := range s1 {
		if s1[i].Path != s2[i].Path {
			different = true
			break
		}
	}
	if !different {
		t.Error("different seeds should produce different diff orders")
	}

	// Original should be unchanged
	if diffs[0].Path != "a.go" {
		t.Error("shuffleDiffs should not mutate input")
	}
}

func TestShuffleDiffs_Single(t *testing.T) {
	diffs := []FileDiff{{Path: "only.go"}}
	result := shuffleDiffs(diffs, 42)
	if len(result) != 1 || result[0].Path != "only.go" {
		t.Error("single diff should pass through unchanged")
	}
}
