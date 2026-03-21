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
	p := DetectPipelineType([]string{"go-patterns", "code-implement"})
	if p.Name != "software" {
		t.Errorf("expected software pipeline, got %s", p.Name)
	}
}

func TestDetectPipelineType_DataScience(t *testing.T) {
	p := DetectPipelineType([]string{"eda-explorer", "predictive-modeler"})
	if p.Name != "data-science" {
		t.Errorf("expected data-science pipeline, got %s", p.Name)
	}
}
