package agent

import (
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
)

func TestSkillsForPhase(t *testing.T) {
	chain := []orchestration.Skill{
		{Name: "go-patterns", Phase: "analyze"},
		{Name: "security-review", Phase: "analyze"},
		{Name: "code-implement", Phase: "implement"},
		{Name: "go-testing", Phase: "verify"},
		{Name: "code-review", Phase: "review"},
	}

	tests := []struct {
		phase string
		want  int
	}{
		{"analyze", 2},
		{"implement", 1},
		{"verify", 1},
		{"review", 1},
		{"nonexistent", 0},
	}

	for _, tt := range tests {
		t.Run(tt.phase, func(t *testing.T) {
			filtered := SkillsForPhase(chain, tt.phase)
			if len(filtered) != tt.want {
				t.Errorf("SkillsForPhase(%q) returned %d skills, want %d", tt.phase, len(filtered), tt.want)
			}
		})
	}
}

func TestDetectPipelineType_Default(t *testing.T) {
	p := DetectPipelineType([]string{"go-patterns", "code-implement"}, "go")
	if p.Name != "software" {
		t.Errorf("expected software pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_DataScience(t *testing.T) {
	// Python + ML skills → data-science pipeline
	p := DetectPipelineType([]string{"eda-explorer", "predictive-modeler"}, "python")
	if p.Name != "data-science" {
		t.Errorf("expected data-science pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_SQLProject(t *testing.T) {
	// SQL + EDA skills → software pipeline (NOT data-science)
	p := DetectPipelineType([]string{"sql-expert", "eda-explorer"}, "sql")
	if p.Name != "software" {
		t.Errorf("SQL project should use software pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_GoWithEDA(t *testing.T) {
	// Go + EDA skills → software pipeline
	p := DetectPipelineType([]string{"go-patterns", "eda-explorer"}, "go")
	if p.Name != "software" {
		t.Errorf("Go project should use software pipeline even with EDA, got %s", p.Name)
	}
}

func TestDetectPipelineType_RustAlwaysSoftware(t *testing.T) {
	p := DetectPipelineType([]string{"rust-patterns", "feature-engineer"}, "rust")
	if p.Name != "software" {
		t.Errorf("Rust project should always use software pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_UnknownLanguageWithML(t *testing.T) {
	// Unknown language + ML skills → data-science (backward compat)
	p := DetectPipelineType([]string{"eda-explorer"}, "")
	if p.Name != "data-science" {
		t.Errorf("Unknown language with EDA should use data-science, got %s", p.Name)
	}
}
