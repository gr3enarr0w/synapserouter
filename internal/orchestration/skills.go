package orchestration

import (
	"embed"
	"log"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/orchestration/skilldata"
)

// Skill is a named unit of work with trigger conditions, a role mapping,
// optional MCP tool bindings, and phase-based ordering for DAG execution.
type Skill struct {
	Name         string          `json:"name" yaml:"name"`
	Description  string          `json:"description" yaml:"description"`
	Triggers     []string        `json:"triggers" yaml:"triggers"`
	Role         string          `json:"role" yaml:"role"`
	MCPTools     []string        `json:"mcp_tools,omitempty" yaml:"mcp_tools"`
	DependsOn    []string        `json:"depends_on,omitempty" yaml:"depends_on"`
	Phase        string          `json:"phase" yaml:"phase"`
	Instructions string          `json:"instructions,omitempty" yaml:"-"`
	Verify       []VerifyCommand `json:"verify,omitempty" yaml:"verify"`
}

// VerifyCommand is a concrete shell command the reviewer must execute
// to verify skill compliance. Turns skill documentation into executable checks.
type VerifyCommand struct {
	Name      string `json:"name" yaml:"name"`
	Command   string `json:"command" yaml:"command"`
	Expect    string `json:"expect,omitempty" yaml:"expect"`
	ExpectNot string `json:"expect_not,omitempty" yaml:"expect_not"`
	Manual    string `json:"manual,omitempty" yaml:"manual"`
}

// PhaseOrder defines the execution ordering of skill phases.
var PhaseOrder = map[string]int{
	"analyze":   0,
	"implement": 1,
	"verify":    2,
	"review":    3,
}

// DefaultSkills returns the built-in skill registry by parsing all embedded
// markdown files in skilldata/. Each .md file is self-contained: YAML
// frontmatter defines metadata (name, triggers, role, phase, mcp_tools)
// and the body after frontmatter becomes the Instructions field.
//
// To add a new skill: create a .md file in internal/orchestration/skilldata/
// with proper frontmatter and rebuild. No Go code changes needed.
func DefaultSkills() []Skill {
	return ParseSkillsFromFS(skilldata.Skills)
}

// ParseSkillsFromFS reads all .md files from an embedded filesystem,
// parses YAML frontmatter into Skill metadata, and uses the body as
// Instructions. Files without valid frontmatter are skipped with a warning.
func ParseSkillsFromFS(fs embed.FS) []Skill {
	entries, err := fs.ReadDir(".")
	if err != nil {
		log.Printf("[skills] failed to read embedded skill directory: %v", err)
		return nil
	}

	var skills []Skill
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		data, err := fs.ReadFile(entry.Name())
		if err != nil {
			log.Printf("[skills] failed to read %s: %v", entry.Name(), err)
			continue
		}

		skill, ok := parseSkillMarkdown(string(data))
		if !ok {
			log.Printf("[skills] skipping %s: invalid or missing frontmatter", entry.Name())
			continue
		}

		skills = append(skills, skill)
	}

	return skills
}

// parseSkillMarkdown extracts YAML frontmatter and body from a markdown
// string. Frontmatter must be delimited by --- lines. Returns the parsed
// Skill and true on success, or zero Skill and false on failure.
func parseSkillMarkdown(content string) (Skill, bool) {
	// Must start with ---
	if !strings.HasPrefix(content, "---") {
		return Skill{}, false
	}

	// Find the closing ---
	rest := content[3:]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return Skill{}, false
	}

	frontmatter := rest[:idx]
	body := strings.TrimLeft(rest[idx+4:], "\n")

	var skill Skill
	if err := yaml.Unmarshal([]byte(frontmatter), &skill); err != nil {
		return Skill{}, false
	}

	// Require at minimum name and phase
	if skill.Name == "" || skill.Phase == "" {
		return Skill{}, false
	}

	skill.Instructions = body
	return skill, true
}
