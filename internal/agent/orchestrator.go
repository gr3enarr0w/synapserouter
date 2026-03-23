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

// DetectPipelineType picks the right pipeline based on matched skills AND project language.
// The data-science pipeline is only used for Python/R projects with ML skills.
// All other languages (SQL, Go, Rust, Java, TypeScript, C#) use the software pipeline
// even if data-science skills match (e.g., eda-explorer can be used for SQL analysis
// but shouldn't trigger EDA→Model phases).
func DetectPipelineType(skillNames []string, projectLanguage string) *Pipeline {
	// Non-Python/R languages always use software pipeline
	switch projectLanguage {
	case "go", "rust", "java", "javascript", "typescript", "cpp", "ruby", "sql", "csharp":
		return &DefaultPipeline
	}

	// For Python/R/unknown: check if ML skills are present
	for _, name := range skillNames {
		switch name {
		case "eda-explorer", "feature-engineer", "predictive-modeler", "data-scrubber":
			return &DataSciencePipeline
		}
	}
	return &DefaultPipeline
}
