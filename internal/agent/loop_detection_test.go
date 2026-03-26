package agent

import (
	"testing"
)

func TestToolCallFingerprint_SameArgs(t *testing.T) {
	args := map[string]interface{}{"command": "npm install"}
	fp1 := toolCallFingerprint("bash", args)
	fp2 := toolCallFingerprint("bash", args)
	if fp1 != fp2 {
		t.Errorf("same tool+args should produce same fingerprint: %s != %s", fp1, fp2)
	}
}

func TestToolCallFingerprint_DifferentTools(t *testing.T) {
	args := map[string]interface{}{"path": "/src/app.ts"}
	fp1 := toolCallFingerprint("file_read", args)
	fp2 := toolCallFingerprint("file_write", args)
	if fp1 == fp2 {
		t.Errorf("different tools should produce different fingerprint")
	}
}

func TestToolCallFingerprint_FileWriteSamePathDifferentContent(t *testing.T) {
	fp1 := toolCallFingerprint("file_write", map[string]interface{}{
		"path":    "/src/app.ts",
		"content": "const x = 1;",
	})
	fp2 := toolCallFingerprint("file_write", map[string]interface{}{
		"path":    "/src/app.ts",
		"content": "const x = 2;",
	})
	if fp1 != fp2 {
		t.Errorf("file_write to same path should produce same fingerprint regardless of content")
	}
}

func TestToolCallFingerprint_FileWriteDifferentPaths(t *testing.T) {
	fp1 := toolCallFingerprint("file_write", map[string]interface{}{
		"path":    "/src/a.ts",
		"content": "hello",
	})
	fp2 := toolCallFingerprint("file_write", map[string]interface{}{
		"path":    "/src/b.ts",
		"content": "hello",
	})
	if fp1 == fp2 {
		t.Errorf("file_write to different paths should produce different fingerprints")
	}
}

func TestToolCallFingerprint_FileEditSamePathDifferentText(t *testing.T) {
	fp1 := toolCallFingerprint("file_edit", map[string]interface{}{
		"path": "/src/app.ts", "old_string": "foo", "new_string": "bar",
	})
	fp2 := toolCallFingerprint("file_edit", map[string]interface{}{
		"path": "/src/app.ts", "old_string": "baz", "new_string": "qux",
	})
	if fp1 != fp2 {
		t.Errorf("file_edit to same path should produce same fingerprint regardless of text")
	}
}

func TestToolCallFingerprint_BashNormalization(t *testing.T) {
	cases := []struct {
		a, b  string
		match bool
	}{
		{"npm install", "npm install --legacy-peer-deps", true},
		{"go test -race ./...", "go test ./...", true},
		{"cargo build", "cargo build --release", true},
		{"npm install", "npm test", false},
		{"go build", "go test", false},
		{"python3 train.py", "python3 train.py --epochs 50", true},
		{"python3 train.py", "python3 eval.py", false},
	}
	for _, tc := range cases {
		fp1 := toolCallFingerprint("bash", map[string]interface{}{"command": tc.a})
		fp2 := toolCallFingerprint("bash", map[string]interface{}{"command": tc.b})
		if tc.match && fp1 != fp2 {
			t.Errorf("expected same fingerprint: %q vs %q", tc.a, tc.b)
		}
		if !tc.match && fp1 == fp2 {
			t.Errorf("expected different fingerprint: %q vs %q", tc.a, tc.b)
		}
	}
}

func TestToolCallFingerprint_UnknownToolFallback(t *testing.T) {
	fp1 := toolCallFingerprint("custom_tool", map[string]interface{}{"x": "1"})
	fp2 := toolCallFingerprint("custom_tool", map[string]interface{}{"x": "2"})
	if fp1 == fp2 {
		t.Error("unknown tool with different args should differ (fallback behavior)")
	}
}

func TestMaxRepeatCount_NoRepeats(t *testing.T) {
	fps := []string{"a", "b", "c", "d"}
	if got := maxRepeatCount(fps); got != 1 {
		t.Errorf("maxRepeatCount with no repeats = %d, want 1", got)
	}
}

func TestMaxRepeatCount_ThreeRepeats(t *testing.T) {
	fps := []string{"a", "b", "a", "c", "a"}
	if got := maxRepeatCount(fps); got != 3 {
		t.Errorf("maxRepeatCount = %d, want 3", got)
	}
}

func TestMaxRepeatCount_Empty(t *testing.T) {
	if got := maxRepeatCount(nil); got != 0 {
		t.Errorf("maxRepeatCount(nil) = %d, want 0", got)
	}
}

func TestMaxRepeatCount_WindowOf40(t *testing.T) {
	// With window=40 and 7 files, max repeats = ceil(40/7) = 6
	var fps []string
	files := []string{"a", "b", "c", "d", "e", "f", "g"}
	for i := 0; i < 40; i++ {
		fps = append(fps, files[i%len(files)])
	}
	got := maxRepeatCount(fps)
	if got < 5 {
		t.Errorf("maxRepeatCount in window of 40 with 7 files = %d, want >= 5", got)
	}
}

func TestLoopWarningCounter_SevenFileRotation(t *testing.T) {
	// Simulate 7-file rotation: even if per-window repeats = 3,
	// cumulative warnings should trigger escalation at 5.
	files := []string{"a.ts", "b.ts", "c.ts", "d.ts", "e.ts", "f.ts", "g.ts"}
	var fps []string
	warningCount := 0

	for turn := 0; turn < 50; turn++ {
		fp := toolCallFingerprint("file_write", map[string]interface{}{"path": files[turn%len(files)]})
		fps = append(fps, fp)
		if len(fps) > 40 {
			fps = fps[len(fps)-40:]
		}
		repeats := maxRepeatCount(fps)
		if repeats >= 3 {
			warningCount++
			if warningCount >= 5 {
				t.Logf("escalation at turn %d (warnings=%d, repeats=%d)", turn, warningCount, repeats)
				return
			}
		} else {
			warningCount = 0
		}
	}
	t.Error("escalation never triggered after 50 turns of 7-file rotation")
}

func TestNormalizeBashCommand(t *testing.T) {
	cases := []struct {
		input, want string
	}{
		{"npm install --save", "npm|install"},
		{"go test -race ./...", "go|test"},
		{"ls -la", "ls"},
		{"python3 train.py --epochs 50", "python3|train.py"},
		{"  npm install  ", "npm|install"},
		{"", ""},
		{"cd /tmp && npm install", "cd|/tmp"},
	}
	for _, tc := range cases {
		got := normalizeBashCommand(tc.input)
		if got != tc.want {
			t.Errorf("normalizeBashCommand(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
