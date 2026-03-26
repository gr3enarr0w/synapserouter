package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNeedsSpecGeneration(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name    string
		workDir string
		message string
		want    bool
	}{
		{"task request", dir, "build a REST API for managing books with SQLite", true},
		{"create request", dir, "create a CLI tool for parsing logs", true},
		{"implement request", dir, "implement OAuth2 token refresh flow", true},
		{"fix request", dir, "fix the authentication handler to return proper errors", true},
		{"refactor request", dir, "refactor the database layer to use connection pooling", true},
		{"setup request", dir, "set up a Docker container for the Go service", true},

		// Should NOT match
		{"question what", dir, "what does the router do?", false},
		{"question how", dir, "how does the circuit breaker work?", false},
		{"question explain", dir, "explain the pipeline architecture", false},
		{"too short", dir, "fix it", false},
		{"empty", dir, "", false},
		{"empty workdir", "", "build something", false},
		{"greeting", dir, "hello, how are you today?", false},
		{"vague", dir, "hmm ok let me think about this", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsSpecGeneration(tt.workDir, tt.message)
			if got != tt.want {
				t.Errorf("needsSpecGeneration(%q, %q) = %v, want %v", tt.workDir, tt.message, got, tt.want)
			}
		})
	}
}

func TestNeedsSpecGeneration_SpecExists(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "spec.md"), []byte("# Existing Spec"), 0644)

	if needsSpecGeneration(dir, "build a REST API for managing books") {
		t.Error("should return false when spec.md already exists")
	}
}

func TestSpecPromptTemplate_ContainsRequiredSections(t *testing.T) {
	prompt := specPromptTemplate
	required := []string{
		"Acceptance Criteria",
		"Technology Constraints",
		"Verify Commands",
		"Description",
	}
	for _, section := range required {
		if !contains(prompt, section) {
			t.Errorf("spec prompt template should contain %q section", section)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr)
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
