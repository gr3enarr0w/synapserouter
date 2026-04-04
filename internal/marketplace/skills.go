package marketplace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gr3enarr0w/synapserouter/internal/orchestration/skilldata"
)

// SkillInfo represents a skill available in the marketplace
type SkillInfo struct {
	Name        string
	Description string
	Installed   bool
	Category    string
}

// bundledCatalog is the hardcoded catalog of available skills for v1.09
var bundledCatalog = []SkillInfo{
	{Name: "api-design", Description: "REST/OpenAPI endpoint design — pagination, auth schemes, error handling", Category: "backend"},
	{Name: "code-implement", Description: "Implementation patterns — clean code, error handling, testing", Category: "general"},
	{Name: "code-review", Description: "Code review checklist — security, performance, maintainability", Category: "general"},
	{Name: "data-modeling", Description: "Database schema design — normalization, indexing, relationships", Category: "backend"},
	{Name: "dbt-modeler", Description: "dbt development — medallion architecture, staging/marts, incremental models", Category: "data"},
	{Name: "fastapi-patterns", Description: "FastAPI development — async patterns, dependency injection, Pydantic", Category: "backend"},
	{Name: "go-patterns", Description: "Idiomatic Go development — concurrency, error handling, interfaces", Category: "backend"},
	{Name: "java-patterns", Description: "Modern Java (17+) — records, sealed classes, streams, virtual threads", Category: "backend"},
	{Name: "java-spring", Description: "Spring Boot 3.x — JPA, constructor injection, layered architecture", Category: "backend"},
	{Name: "javascript-patterns", Description: "Modern JS/TS — React, Node.js, TypeScript-first, Next.js", Category: "frontend"},
	{Name: "ml-patterns", Description: "Machine learning — train/test splits, feature engineering, sklearn", Category: "data"},
	{Name: "node-toolchain", Description: "Node.js toolchain — npm/yarn/pnpm, workspaces, monorepos", Category: "frontend"},
	{Name: "python-patterns", Description: "Idiomatic Python — PEP 8, type hints, async patterns", Category: "backend"},
	{Name: "rust-patterns", Description: "Idiomatic Rust — ownership, memory safety, concurrency", Category: "backend"},
	{Name: "snowflake-query", Description: "Snowflake SQL — schema exploration, warehouse management, streams", Category: "data"},
	{Name: "task-orchestrator", Description: "Task orchestration patterns — workflow management, retries", Category: "general"},
	{Name: "terminal-interaction", Description: "CLI/Terminal UX patterns — progress bars, colored output", Category: "general"},
	{Name: "typescript-patterns", Description: "TypeScript patterns — generics, utility types, strict mode", Category: "frontend"},
	{Name: "typescript-testing", Description: "TS testing — Jest, Vitest, mocking, integration tests", Category: "frontend"},
	{Name: "vhs-verify", Description: "VHS tape verification — terminal recording, screenshot validation", Category: "testing"},
}

// ListSkills returns all available skills with their installation status
func ListSkills() ([]SkillInfo, error) {
	// Get user home directory for checking installed skills
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	userSkillsDir := ""
	if home != "" {
		userSkillsDir = filepath.Join(home, ".synroute", "skills")
	}

	// Build set of installed skills (embedded + user)
	installedSet := make(map[string]bool)

	// Check embedded skills (always installed)
	entries, err := skilldata.Skills.ReadDir(".")
	if err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".md") {
				name := strings.TrimSuffix(entry.Name(), ".md")
				installedSet[name] = true
			}
		}
	}

	// Check user-installed skills
	if userSkillsDir != "" {
		if userEntries, err := os.ReadDir(userSkillsDir); err == nil {
			for _, entry := range userEntries {
				if strings.HasSuffix(entry.Name(), ".md") {
					name := strings.TrimSuffix(entry.Name(), ".md")
					installedSet[name] = true
				}
			}
		}
	}

	// Merge catalog with installation status
	result := make([]SkillInfo, 0, len(bundledCatalog))
	for _, skill := range bundledCatalog {
		skill.Installed = installedSet[skill.Name]
		result = append(result, skill)
	}

	return result, nil
}

// InstallSkill installs a skill by copying it to the user skills directory
func InstallSkill(name string) error {
	// Check if skill exists in catalog
	found := false
	for _, skill := range bundledCatalog {
		if skill.Name == name {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("skill '%s' not found in catalog", name)
	}

	// Check if already installed (embedded)
	entries, err := skilldata.Skills.ReadDir(".")
	if err == nil {
		for _, entry := range entries {
			if strings.TrimSuffix(entry.Name(), ".md") == name {
				return fmt.Errorf("skill '%s' is already installed (built-in)", name)
			}
		}
	}

	// Get user skills directory
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	userSkillsDir := filepath.Join(home, ".synroute", "skills")

	// Create directory if needed
	if err := os.MkdirAll(userSkillsDir, 0755); err != nil {
		return fmt.Errorf("cannot create skills directory: %w", err)
	}

	// For v1.09, we just create a placeholder since the skill is already embedded
	// In a future version, this would download from a registry
	skillPath := filepath.Join(userSkillsDir, name+".md")

	// Check if already exists
	if _, err := os.Stat(skillPath); err == nil {
		return fmt.Errorf("skill '%s' is already installed", name)
	}

	// Create a reference file pointing to the embedded skill
	content := fmt.Sprintf(`---
name: %s
source: embedded
---

This skill is installed from the embedded catalog.
See internal/orchestration/skilldata/%s.md for the full definition.
`, name, name)

	if err := os.WriteFile(skillPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("cannot write skill file: %w", err)
	}

	return nil
}


