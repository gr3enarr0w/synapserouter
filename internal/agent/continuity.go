package agent

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProjectContinuity holds cross-session state for a project directory.
type ProjectContinuity struct {
	ProjectDir     string    `json:"project_dir"`
	SessionID      string    `json:"session_id"`
	Phase          string    `json:"phase"`
	BuildStatus    string    `json:"build_status"`
	TestStatus     string    `json:"test_status"`
	Language       string    `json:"language"`
	Model          string    `json:"model"`
	FileManifest   []string  `json:"file_manifest"`
	ContextSummary string    `json:"context_summary"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// SaveContinuity persists project continuity state to DB and synroute.md.
func SaveContinuity(db *sql.DB, c *ProjectContinuity) error {
	if db == nil {
		return nil
	}

	manifest, _ := json.Marshal(c.FileManifest)

	_, err := db.Exec(`
		INSERT INTO project_continuity
			(project_dir, session_id, phase, build_status, test_status, language, model, file_manifest, context_summary, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(project_dir) DO UPDATE SET
			session_id = excluded.session_id,
			phase = excluded.phase,
			build_status = excluded.build_status,
			test_status = excluded.test_status,
			language = excluded.language,
			model = excluded.model,
			file_manifest = excluded.file_manifest,
			context_summary = excluded.context_summary,
			updated_at = CURRENT_TIMESTAMP`,
		c.ProjectDir, c.SessionID, c.Phase, c.BuildStatus, c.TestStatus,
		c.Language, c.Model, string(manifest), c.ContextSummary)
	if err != nil {
		return fmt.Errorf("save continuity: %w", err)
	}

	// Also write synroute.md with YAML frontmatter
	if err := writeSynrouteMD(c); err != nil {
		log.Printf("[Continuity] Warning: failed to write synroute.md: %v", err)
	}

	return nil
}

// LoadContinuity loads the most recent continuity state for a project directory.
func LoadContinuity(db *sql.DB, projectDir string) (*ProjectContinuity, error) {
	if db == nil {
		return nil, fmt.Errorf("no database")
	}

	var c ProjectContinuity
	var manifestJSON string
	err := db.QueryRow(`
		SELECT project_dir, session_id, phase, build_status, test_status,
			   language, model, file_manifest, context_summary, created_at, updated_at
		FROM project_continuity WHERE project_dir = ?`, projectDir).
		Scan(&c.ProjectDir, &c.SessionID, &c.Phase, &c.BuildStatus, &c.TestStatus,
			&c.Language, &c.Model, &manifestJSON, &c.ContextSummary,
			&c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil // no prior state
	}
	if err != nil {
		return nil, fmt.Errorf("load continuity: %w", err)
	}

	json.Unmarshal([]byte(manifestJSON), &c.FileManifest)
	return &c, nil
}

// LoadContinuityFromFile reads synroute.md YAML frontmatter for continuity.
// Falls back gracefully if the file doesn't exist or has no frontmatter.
func LoadContinuityFromFile(projectDir string) *ProjectContinuity {
	path := filepath.Join(projectDir, "synroute.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		return nil
	}

	end := strings.Index(content[4:], "\n---\n")
	if end < 0 {
		return nil
	}

	frontmatter := content[4 : 4+end]
	c := &ProjectContinuity{ProjectDir: projectDir}

	for _, line := range strings.Split(frontmatter, "\n") {
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "session_id":
			c.SessionID = val
		case "phase":
			c.Phase = val
		case "build_status":
			c.BuildStatus = val
		case "test_status":
			c.TestStatus = val
		case "language":
			c.Language = val
		case "model":
			c.Model = val
		case "context_summary":
			c.ContextSummary = val
		}
	}

	return c
}

// BuildContinuityFromAgent creates a ProjectContinuity from the agent's current state.
func BuildContinuityFromAgent(a *Agent) *ProjectContinuity {
	phase := ""
	if a.pipeline != nil && a.pipelinePhase < len(a.pipeline.Phases) {
		phase = a.pipeline.Phases[a.pipelinePhase].Name
	}

	return &ProjectContinuity{
		ProjectDir:     a.config.WorkDir,
		SessionID:      a.sessionID,
		Phase:          phase,
		Language:       a.config.ProjectLanguage,
		Model:          a.config.Model,
		ContextSummary: truncateStr(a.originalRequest, 500),
	}
}

// writeSynrouteMD writes project state to synroute.md with YAML frontmatter.
func writeSynrouteMD(c *ProjectContinuity) error {
	path := filepath.Join(c.ProjectDir, "synroute.md")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("session_id: %s\n", c.SessionID))
	b.WriteString(fmt.Sprintf("phase: %s\n", c.Phase))
	b.WriteString(fmt.Sprintf("build_status: %s\n", c.BuildStatus))
	b.WriteString(fmt.Sprintf("test_status: %s\n", c.TestStatus))
	b.WriteString(fmt.Sprintf("language: %s\n", c.Language))
	b.WriteString(fmt.Sprintf("model: %s\n", c.Model))
	b.WriteString(fmt.Sprintf("updated_at: %s\n", time.Now().Format(time.RFC3339)))
	b.WriteString("---\n\n")
	b.WriteString("# synroute.md — Project State\n\n")

	if c.ContextSummary != "" {
		b.WriteString("## Last Request\n")
		b.WriteString(c.ContextSummary)
		b.WriteString("\n\n")
	}

	if len(c.FileManifest) > 0 {
		b.WriteString("## Files Modified\n")
		for _, f := range c.FileManifest {
			b.WriteString(fmt.Sprintf("- %s\n", f))
		}
		b.WriteString("\n")
	}

	// Write atomically via temp file
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(b.String()), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// InjectContinuityContext adds prior session context to the system prompt
// if continuity data is available.
func InjectContinuityContext(c *ProjectContinuity) string {
	if c == nil || c.SessionID == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## Prior Session Context\n")
	b.WriteString(fmt.Sprintf("Previous session: %s\n", c.SessionID))
	if c.Phase != "" {
		b.WriteString(fmt.Sprintf("Last phase: %s\n", c.Phase))
	}
	if c.BuildStatus != "" {
		b.WriteString(fmt.Sprintf("Build status: %s\n", c.BuildStatus))
	}
	if c.TestStatus != "" {
		b.WriteString(fmt.Sprintf("Test status: %s\n", c.TestStatus))
	}
	if c.ContextSummary != "" {
		b.WriteString(fmt.Sprintf("Last request: %s\n", c.ContextSummary))
	}
	return b.String()
}
