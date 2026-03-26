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

// DetectPipelineType picks the right pipeline dynamically from matched skills.
// Skills declare their pipeline type (e.g., "data-science") and language in frontmatter.
// Language-specific skills are filtered by project language before checking pipeline type.
// No hardcoded language or skill name lists — scales with new skills automatically.
func DetectPipelineType(matched []orchestration.Skill, projectLanguage string) *Pipeline {
	for _, skill := range matched {
		if skill.Pipeline == "" {
			continue
		}
		// Skip language-specific skills that don't match project language
		if skill.Language != "" && projectLanguage != "" &&
			skill.Language != projectLanguage {
			continue
		}
		if skill.Pipeline == "data-science" {
			return copyPipeline(&DataSciencePipeline)
		}
	}
	return copyPipeline(&DefaultPipeline)
}

// copyPipeline returns a deep copy so concurrent agents don't race on global state.
func copyPipeline(src *Pipeline) *Pipeline {
	p := *src
	p.Phases = make([]PipelinePhase, len(src.Phases))
	copy(p.Phases, src.Phases)
	return &p
}
