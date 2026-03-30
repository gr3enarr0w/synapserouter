package agent

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed prompts/*.md
var promptFS embed.FS

// LoadPrompt reads an embedded prompt markdown file by name.
func LoadPrompt(name string) string {
	data, err := promptFS.ReadFile("prompts/" + name)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// LoadProjectInstructions reads CLAUDE.md / AGENTS.md from the working directory.
// Follows the industry-standard hierarchy: CLAUDE.md > .claude/CLAUDE.md > AGENTS.md.
// Also reads synroute.md (the agent's own project state file).
// Caps at 200 lines total to prevent instruction rot.
func LoadProjectInstructions(workDir string) string {
	var parts []string

	// Load project rules (CLAUDE.md / AGENTS.md) — first match wins
	for _, name := range []string{"CLAUDE.md", filepath.Join(".claude", "CLAUDE.md"), "AGENTS.md"} {
		path := filepath.Join(workDir, name)
		if data, err := os.ReadFile(path); err == nil {
			parts = append(parts, string(data))
			break
		}
	}

	// Load agent's own project state (synroute.md) — strip YAML frontmatter
	synroutePath := filepath.Join(workDir, "synroute.md")
	if data, err := os.ReadFile(synroutePath); err == nil {
		content := string(data)
		// Strip YAML frontmatter (--- block at start of file)
		if strings.HasPrefix(content, "---") {
			if end := strings.Index(content[3:], "---"); end >= 0 {
				content = strings.TrimSpace(content[end+6:]) // skip past closing ---
			}
		}
		if content != "" {
			parts = append(parts, content)
		}
	}

	if len(parts) == 0 {
		return ""
	}

	result := strings.Join(parts, "\n\n")
	// Cap at 200 lines to prevent instruction rot
	lines := strings.Split(result, "\n")
	if len(lines) > 200 {
		lines = lines[:200]
		lines = append(lines, "\n[...truncated at 200 lines]")
	}
	return strings.Join(lines, "\n")
}

// LoadRoleInstructions reads .claude/agents/{role}.md for role-specific sub-agent instructions.
func LoadRoleInstructions(workDir, role string) string {
	path := filepath.Join(workDir, ".claude", "agents", role+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// forceToolsConfig holds the parsed force-tools.md configuration.
type forceToolsConfig struct {
	Base   string            `yaml:"base"`
	Phases map[string]string `yaml:"phases"`
}

// ParseForceToolsPrompt parses the force-tools.md YAML frontmatter and returns
// the base message + phase-specific suffix for a given phase name.
func ParseForceToolsPrompt(phaseName string) string {
	raw := LoadPrompt("force-tools.md")
	if raw == "" {
		return "You MUST use tools now. Do NOT output text without tool calls."
	}

	// Parse YAML frontmatter (between --- markers)
	parts := strings.SplitN(raw, "---", 3)
	if len(parts) < 3 {
		return raw
	}

	var cfg forceToolsConfig
	if err := yaml.Unmarshal([]byte(parts[1]), &cfg); err != nil {
		return raw
	}

	base := cfg.Base
	if base == "" {
		base = "You MUST use tools now. Do NOT output text without tool calls."
	}

	suffix, ok := cfg.Phases[phaseName]
	if !ok {
		suffix = cfg.Phases["default"]
	}
	if suffix == "" {
		return base
	}
	return base + " " + suffix
}
