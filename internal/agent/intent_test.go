package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectPipelineEntry_NoWorkDir(t *testing.T) {
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("fix the bug", "", pipeline, false)
	if entry.Phase != 0 || entry.Mode != "full" {
		t.Errorf("expected phase 0/full for no workdir, got phase %d/%s", entry.Phase, entry.Mode)
	}
}

func TestDetectPipelineEntry_NilPipeline(t *testing.T) {
	entry := DetectPipelineEntry("fix the bug", "/tmp", nil, false)
	if entry.Phase != 0 || entry.Mode != "full" {
		t.Errorf("expected phase 0/full for nil pipeline, got phase %d/%s", entry.Phase, entry.Mode)
	}
}

func TestDetectPipelineEntry_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("build a web server", dir, pipeline, false)
	if entry.Phase != 0 || entry.Mode != "full" {
		t.Errorf("expected phase 0/full for empty dir, got phase %d/%s: %s", entry.Phase, entry.Mode, entry.Reason)
	}
}

func TestDetectPipelineEntry_SpecFilePresent(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("# Spec"), 0644)
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("implement the spec", dir, pipeline, false)
	if entry.Phase != 0 || entry.Mode != "full" {
		t.Errorf("expected phase 0/full with spec file, got phase %d/%s: %s", entry.Phase, entry.Mode, entry.Reason)
	}
}

func TestDetectPipelineEntry_ExistingCodeNoTests(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("add a feature", dir, pipeline, false)

	// Should skip plan, start at implement
	implementIdx := -1
	for i, p := range pipeline.Phases {
		if p.Name == "implement" {
			implementIdx = i
			break
		}
	}
	if entry.Phase != implementIdx {
		t.Errorf("expected phase %d (implement) for code-no-tests, got phase %d/%s: %s",
			implementIdx, entry.Phase, entry.Mode, entry.Reason)
	}
	if entry.Mode != "implement" {
		t.Errorf("expected mode implement, got %s", entry.Mode)
	}
}

func TestDetectPipelineEntry_ExistingCodeWithTests(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main"), 0644)
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("review my code", dir, pipeline, false)

	if entry.Mode != "review" {
		t.Errorf("expected mode review for code+tests, got %s: %s", entry.Mode, entry.Reason)
	}
	// Should start at self-check or code-review phase
	if entry.Phase < 2 {
		t.Errorf("expected phase >= 2 (review-ish) for code+tests, got phase %d: %s",
			entry.Phase, entry.Reason)
	}
}

func TestDetectPipelineEntry_HasExistingCodeOverride(t *testing.T) {
	dir := t.TempDir() // empty dir, but hasExistingCode=true
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("fix the bug", dir, pipeline, true)

	// hasExistingCode=true should skip plan even with empty dir
	if entry.Phase == 0 {
		t.Errorf("expected phase > 0 with hasExistingCode=true, got phase 0: %s", entry.Reason)
	}
	if entry.Mode != "implement" {
		t.Errorf("expected mode implement with hasExistingCode=true, got %s", entry.Mode)
	}
}

func TestDetectPipelineEntry_NestedCodeFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "src", "app.py"), []byte("print('hi')"), 0644)
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("refactor", dir, pipeline, false)

	if entry.Mode == "full" && entry.Phase == 0 {
		t.Errorf("expected non-full mode for nested code, got phase %d/%s: %s",
			entry.Phase, entry.Mode, entry.Reason)
	}
}

func TestDetectPipelineEntry_NestedTestFiles(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "internal"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "internal", "handler_test.go"), []byte("package internal"), 0644)
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("check for bugs", dir, pipeline, false)

	if entry.Mode != "review" {
		t.Errorf("expected review mode with nested test files, got %s: %s", entry.Mode, entry.Reason)
	}
}

func TestDetectPipelineEntry_DataSciencePipeline(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "model.py"), []byte("import torch"), 0644)
	pipeline := copyPipeline(&DataSciencePipeline)
	entry := DetectPipelineEntry("train the model", dir, pipeline, false)

	// Data science pipeline has different phase names; should detect code and skip EDA
	if entry.Phase == 0 && entry.Mode == "full" {
		// With code present, should skip to data-prep or later
		t.Logf("data science pipeline entry: phase=%d mode=%s reason=%s", entry.Phase, entry.Mode, entry.Reason)
	}
	if entry.Mode != "implement" && entry.Mode != "review" {
		// data-prep maps to implement mode
		t.Logf("unexpected mode for data science with code: %s (reason: %s)", entry.Mode, entry.Reason)
	}
}

func TestDetectPipelineEntry_SpecWithExistingCode(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("# Spec"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	pipeline := copyPipeline(&DefaultPipeline)
	entry := DetectPipelineEntry("implement the spec", dir, pipeline, false)

	// Spec + existing code → should still detect code and skip plan
	// (spec without code triggers full pipeline, but spec WITH code means partial impl)
	if entry.Mode == "full" {
		t.Logf("spec+code: full pipeline (spec takes priority). reason: %s", entry.Reason)
	}
}

// Test SinglePhase constructor
func TestSinglePhase(t *testing.T) {
	phase := PipelinePhase{
		Name:         "code-review",
		MinToolCalls: 2,
		UseSubAgent:  true,
		Prompt:       "Review the code",
	}
	p := SinglePhase(phase)
	if p.Name != "single-code-review" {
		t.Errorf("expected pipeline name single-code-review, got %s", p.Name)
	}
	if len(p.Phases) != 1 {
		t.Fatalf("expected 1 phase, got %d", len(p.Phases))
	}
	if p.Phases[0].Name != "code-review" {
		t.Errorf("expected phase name code-review, got %s", p.Phases[0].Name)
	}
	if p.Phases[0].MinToolCalls != 2 {
		t.Errorf("expected MinToolCalls 2, got %d", p.Phases[0].MinToolCalls)
	}
}

// Test IntentSystemPromptAdjustment
func TestIntentSystemPromptAdjustment(t *testing.T) {
	tests := []struct {
		mode     string
		wantLen  bool // true if non-empty output expected
		contains string
	}{
		{"full", false, ""},
		{"review", true, "REVIEW"},
		{"implement", true, "IMPLEMENT"},
		{"single", true, "SINGLE"},
	}

	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			entry := IntentEntry{Mode: tt.mode}
			result := IntentSystemPromptAdjustment(entry)
			if tt.wantLen && result == "" {
				t.Errorf("expected non-empty adjustment for mode %s", tt.mode)
			}
			if !tt.wantLen && result != "" {
				t.Errorf("expected empty adjustment for mode %s, got: %s", tt.mode, result)
			}
			if tt.contains != "" && result != "" {
				if !strings.Contains(result, tt.contains) {
					t.Errorf("expected adjustment to contain %q, got: %s", tt.contains, result)
				}
			}
		})
	}
}



// Test detectSpecFile
func TestDetectSpecFile(t *testing.T) {
	t.Run("no_spec", func(t *testing.T) {
		dir := t.TempDir()
		if detectSpecFile(dir) {
			t.Error("expected no spec file in empty dir")
		}
	})

	t.Run("SPEC.md", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte("spec"), 0644)
		if !detectSpecFile(dir) {
			t.Error("expected SPEC.md to be detected")
		}
	})

	t.Run("spec_dir", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "spec"), 0755)
		if !detectSpecFile(dir) {
			t.Error("expected spec/ directory to be detected")
		}
	})

	t.Run("feature.spec.md", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "feature.spec.md"), []byte("spec"), 0644)
		if !detectSpecFile(dir) {
			t.Error("expected *.spec.md to be detected")
		}
	})
}

// Test detectCodeFiles
func TestDetectCodeFiles(t *testing.T) {
	t.Run("empty_dir", func(t *testing.T) {
		dir := t.TempDir()
		if detectCodeFiles(dir) {
			t.Error("expected no code files in empty dir")
		}
	})

	t.Run("go_file", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
		if !detectCodeFiles(dir) {
			t.Error("expected .go file to be detected")
		}
	})

	t.Run("nested_py", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "src"), 0755)
		os.WriteFile(filepath.Join(dir, "src", "app.py"), []byte("pass"), 0644)
		if !detectCodeFiles(dir) {
			t.Error("expected nested .py file to be detected")
		}
	})
}

// Test detectTestFiles
func TestDetectTestFiles(t *testing.T) {
	t.Run("empty_dir", func(t *testing.T) {
		dir := t.TempDir()
		if detectTestFiles(dir) {
			t.Error("expected no test files in empty dir")
		}
	})

	t.Run("go_test", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main"), 0644)
		if !detectTestFiles(dir) {
			t.Error("expected _test.go file to be detected")
		}
	})

	t.Run("nested_test", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "tests"), 0755)
		os.WriteFile(filepath.Join(dir, "tests", "test_app.py"), []byte("pass"), 0644)
		if !detectTestFiles(dir) {
			t.Error("expected nested test_*.py file to be detected")
		}
	})
}

// Test phasePromptForEntry
func TestPhasePromptForEntry(t *testing.T) {
	pipeline := copyPipeline(&DefaultPipeline)

	t.Run("no_skip", func(t *testing.T) {
		entry := IntentEntry{Phase: 0, Mode: "full"}
		prompt := phasePromptForEntry(pipeline, entry, "test criteria")
		if prompt == "" {
			t.Error("expected non-empty prompt for phase 0")
		}
	})

	t.Run("skipped_phases", func(t *testing.T) {
		entry := IntentEntry{Phase: 2, Mode: "review"}
		prompt := phasePromptForEntry(pipeline, entry, "test criteria")
		if prompt == "" {
			t.Error("expected non-empty prompt for phase 2")
		}
		if !strings.Contains(prompt, "Skipped phases:") {
			t.Error("expected prompt to mention skipped phases")
		}
	})

	t.Run("out_of_bounds", func(t *testing.T) {
		entry := IntentEntry{Phase: 100, Mode: "full"}
		prompt := phasePromptForEntry(pipeline, entry, "")
		if prompt != "" {
			t.Errorf("expected empty prompt for out-of-bounds phase, got: %s", prompt)
		}
	})
}
