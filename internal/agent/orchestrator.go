package agent

import (
	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration"
)

// SkillsForPhase filters a skill chain to only include skills in the given phase.
func SkillsForPhase(chain []orchestration.Skill, phase string) []orchestration.Skill {
	var filtered []orchestration.Skill
	for _, s := range chain {
		if s.Phase == phase {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

// DetectPipelineType picks the right pipeline based on matched skills.
func DetectPipelineType(skillNames []string) *Pipeline {
	for _, name := range skillNames {
		switch name {
		case "eda-explorer", "feature-engineer", "predictive-modeler", "data-scrubber":
			return &DataSciencePipeline
		}
	}
	return &DefaultPipeline
}
