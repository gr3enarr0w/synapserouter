package agent

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func setupContinuityDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS project_continuity (
		project_dir TEXT PRIMARY KEY,
		session_id TEXT NOT NULL,
		phase TEXT DEFAULT '',
		build_status TEXT DEFAULT '',
		test_status TEXT DEFAULT '',
		language TEXT DEFAULT '',
		model TEXT DEFAULT '',
		file_manifest TEXT DEFAULT '[]',
		context_summary TEXT DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSaveAndLoadContinuity(t *testing.T) {
	db := setupContinuityDB(t)
	defer db.Close()

	c := &ProjectContinuity{
		ProjectDir:     "/tmp/test-project",
		SessionID:      "sess-123",
		Phase:          "implement",
		BuildStatus:    "pass",
		TestStatus:     "pass",
		Language:       "go",
		Model:          "auto",
		FileManifest:   []string{"main.go", "handler.go"},
		ContextSummary: "Build a REST API",
	}

	if err := SaveContinuity(db, c); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadContinuity(db, "/tmp/test-project")
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil continuity")
	}
	if loaded.SessionID != "sess-123" {
		t.Errorf("session_id = %q, want sess-123", loaded.SessionID)
	}
	if loaded.Phase != "implement" {
		t.Errorf("phase = %q, want implement", loaded.Phase)
	}
	if len(loaded.FileManifest) != 2 {
		t.Errorf("file_manifest len = %d, want 2", len(loaded.FileManifest))
	}
}

func TestLoadContinuityNotFound(t *testing.T) {
	db := setupContinuityDB(t)
	defer db.Close()

	c, err := LoadContinuity(db, "/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatal("expected nil for nonexistent project")
	}
}

func TestSaveContinuityUpsert(t *testing.T) {
	db := setupContinuityDB(t)
	defer db.Close()

	c := &ProjectContinuity{
		ProjectDir: "/tmp/test",
		SessionID:  "s1",
		Phase:      "plan",
	}
	SaveContinuity(db, c)

	c.SessionID = "s2"
	c.Phase = "implement"
	SaveContinuity(db, c)

	loaded, _ := LoadContinuity(db, "/tmp/test")
	if loaded.SessionID != "s2" {
		t.Errorf("expected upsert, got session_id = %q", loaded.SessionID)
	}
	if loaded.Phase != "implement" {
		t.Errorf("expected upsert phase, got %q", loaded.Phase)
	}
}

func TestLoadContinuityFromFile(t *testing.T) {
	dir := t.TempDir()
	content := `---
session_id: sess-456
phase: review
language: python
build_status: pass
---

# synroute.md — Project State
`
	os.WriteFile(filepath.Join(dir, "synroute.md"), []byte(content), 0644)

	c := LoadContinuityFromFile(dir)
	if c == nil {
		t.Fatal("expected non-nil continuity from file")
	}
	if c.SessionID != "sess-456" {
		t.Errorf("session_id = %q", c.SessionID)
	}
	if c.Phase != "review" {
		t.Errorf("phase = %q", c.Phase)
	}
	if c.Language != "python" {
		t.Errorf("language = %q", c.Language)
	}
}

func TestLoadContinuityFromFileNoFrontmatter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "synroute.md"), []byte("# No frontmatter\n"), 0644)

	c := LoadContinuityFromFile(dir)
	if c != nil {
		t.Fatal("expected nil for file without frontmatter")
	}
}

func TestLoadContinuityFromFileMissing(t *testing.T) {
	c := LoadContinuityFromFile("/nonexistent/dir")
	if c != nil {
		t.Fatal("expected nil for missing file")
	}
}

func TestInjectContinuityContext(t *testing.T) {
	result := InjectContinuityContext(nil)
	if result != "" {
		t.Fatal("expected empty for nil continuity")
	}

	c := &ProjectContinuity{
		SessionID:      "sess-789",
		Phase:          "implement",
		BuildStatus:    "fail",
		ContextSummary: "Fix the auth bug",
	}
	result = InjectContinuityContext(c)
	if !strings.Contains(result, "sess-789") {
		t.Error("expected session_id in context")
	}
	if !strings.Contains(result, "implement") {
		t.Error("expected phase in context")
	}
	if !strings.Contains(result, "Fix the auth bug") {
		t.Error("expected context summary")
	}
}

func TestWriteSynrouteMD(t *testing.T) {
	dir := t.TempDir()
	c := &ProjectContinuity{
		ProjectDir:     dir,
		SessionID:      "sess-write-test",
		Phase:          "deploy",
		Language:       "go",
		ContextSummary: "Test write",
		FileManifest:   []string{"a.go", "b.go"},
	}

	if err := writeSynrouteMD(c); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "synroute.md"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.HasPrefix(content, "---\n") {
		t.Fatal("expected YAML frontmatter")
	}
	if !strings.Contains(content, "session_id: sess-write-test") {
		t.Error("missing session_id")
	}
	if !strings.Contains(content, "- a.go") {
		t.Error("missing file manifest")
	}
}
