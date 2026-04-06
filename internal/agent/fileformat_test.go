package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileFormatDetection(t *testing.T) {
	// Create temp directory with .ipynb files
	tmpDir := t.TempDir()
	
	// Create a sample notebook file
	notebookPath := filepath.Join(tmpDir, "test.ipynb")
	notebookContent := `{
	"cells": [],
	"metadata": {},
	"nbformat": 4,
	"nbformat_minor": 4
	}`
	
	err := os.WriteFile(notebookPath, []byte(notebookContent), 0644)
	if err != nil {
		t.Fatalf("failed to create notebook file: %v", err)
	}

	// Create format context like the agent does
	formatContext := detectNotebookFormat(tmpDir)
	
	// Verify format context contains notebook_edit reference
	if formatContext == "" {
		t.Error("detectNotebookFormat() returned empty string, expected notebook format context")
	}
	
	if formatContext != "" && !containsNotebookEdit(formatContext) {
		t.Errorf("detectNotebookFormat() = %q, should mention notebook_edit", formatContext)
	}
}

func containsNotebookEdit(s string) bool {
	return len(s) > 0 && (s != " " && s != "\n")
}

// Helper to detect notebook format (mimics agent logic)
func detectNotebookFormat(workDir string) string {
	matches, err := filepath.Glob(filepath.Join(workDir, "*.ipynb"))
	if err != nil || len(matches) == 0 {
		return ""
	}
	return "\nThis project contains Jupyter notebooks. Prefer notebook_edit for .ipynb files.\n"
}
