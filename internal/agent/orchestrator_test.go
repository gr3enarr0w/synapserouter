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

// Helper to build skill with language and pipeline fields
func skill(name, language, pipeline string) orchestration.Skill {
	return orchestration.Skill{Name: name, Language: language, Pipeline: pipeline}
}

func TestDetectPipelineType_Default(t *testing.T) {
	matched := []orchestration.Skill{
		skill("go-patterns", "go", ""),
		skill("code-implement", "", ""),
	}
	p := DetectPipelineType(matched, "go")
	if p.Name != "software" {
		t.Errorf("expected software pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_DataScience(t *testing.T) {
	// Python + ML skills → data-science pipeline
	matched := []orchestration.Skill{
		skill("eda-explorer", "python", "data-science"),
		skill("predictive-modeler", "python", "data-science"),
	}
	p := DetectPipelineType(matched, "python")
	if p.Name != "data-science" {
		t.Errorf("expected data-science pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_SQLProject(t *testing.T) {
	// SQL + EDA skills → software pipeline (EDA is python-only, filtered out)
	matched := []orchestration.Skill{
		skill("sql-expert", "sql", ""),
		skill("eda-explorer", "python", "data-science"),
	}
	p := DetectPipelineType(matched, "sql")
	if p.Name != "software" {
		t.Errorf("SQL project should use software pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_GoWithEDA(t *testing.T) {
	// Go + EDA skills → software pipeline (EDA filtered by language)
	matched := []orchestration.Skill{
		skill("go-patterns", "go", ""),
		skill("eda-explorer", "python", "data-science"),
	}
	p := DetectPipelineType(matched, "go")
	if p.Name != "software" {
		t.Errorf("Go project should use software pipeline even with EDA, got %s", p.Name)
	}
}

func TestDetectPipelineType_RustAlwaysSoftware(t *testing.T) {
	matched := []orchestration.Skill{
		skill("rust-patterns", "rust", ""),
		skill("feature-engineer", "python", "data-science"),
	}
	p := DetectPipelineType(matched, "rust")
	if p.Name != "software" {
		t.Errorf("Rust project should always use software pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_UnknownLanguageWithML(t *testing.T) {
	// Unknown language + ML skills → data-science (backward compat)
	matched := []orchestration.Skill{
		skill("eda-explorer", "python", "data-science"),
	}
	p := DetectPipelineType(matched, "")
	if p.Name != "data-science" {
		t.Errorf("Unknown language with EDA should use data-science, got %s", p.Name)
	}
}

func TestDetectPipelineType_JavaScriptWithMLKeywords(t *testing.T) {
	// JavaScript project that triggers ML skill keywords → software (ML filtered by language)
	matched := []orchestration.Skill{
		skill("javascript-patterns", "javascript", ""),
		skill("eda-explorer", "python", "data-science"),
		skill("predictive-modeler", "python", "data-science"),
	}
	p := DetectPipelineType(matched, "javascript")
	if p.Name != "software" {
		t.Errorf("JS project should use software pipeline even with ML skills matched, got %s", p.Name)
	}
}

func TestDetectPipelineType_CrossLanguageSkillNoPipeline(t *testing.T) {
	// Cross-language skills without pipeline field never trigger data-science
	matched := []orchestration.Skill{
		skill("code-review", "", ""),
		skill("docker-expert", "", ""),
		skill("data-scrubber", "", ""),
	}
	p := DetectPipelineType(matched, "go")
	if p.Name != "software" {
		t.Errorf("Cross-language skills should default to software, got %s", p.Name)
	}
}
