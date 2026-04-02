package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAttachments_AtReference(t *testing.T) {
	// Create a temp dir with a test file
	dir := t.TempDir()
	testFile := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(testFile, []byte("# My Spec\nSome content here."), 0644); err != nil {
		t.Fatal(err)
	}

	msg := "look at @spec.md and fix the bug"
	cleaned, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Path != testFile {
		t.Errorf("expected path %s, got %s", testFile, attachments[0].Path)
	}
	if attachments[0].Content != "# My Spec\nSome content here." {
		t.Errorf("unexpected content: %q", attachments[0].Content)
	}
	if attachments[0].MimeType != "text/markdown" {
		t.Errorf("expected text/markdown, got %s", attachments[0].MimeType)
	}
	if attachments[0].Error != "" {
		t.Errorf("unexpected error: %s", attachments[0].Error)
	}
	// The @spec.md should be removed from the cleaned message
	if strings.Contains(cleaned, "@spec.md") {
		t.Errorf("cleaned message still contains @spec.md: %q", cleaned)
	}
	if !strings.Contains(cleaned, "look at") {
		t.Errorf("cleaned message missing original text: %q", cleaned)
	}
	if !strings.Contains(cleaned, "fix the bug") {
		t.Errorf("cleaned message missing original text: %q", cleaned)
	}
}

func TestParseAttachments_AbsolutePath(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	msg := "look at " + testFile + " please"
	cleaned, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Path != testFile {
		t.Errorf("expected path %s, got %s", testFile, attachments[0].Path)
	}
	if attachments[0].MimeType != "text/x-go" {
		t.Errorf("expected text/x-go, got %s", attachments[0].MimeType)
	}
	// Absolute paths are NOT removed from the cleaned message
	if !strings.Contains(cleaned, testFile) {
		t.Errorf("absolute path should be preserved in cleaned message: %q", cleaned)
	}
}

func TestParseAttachments_RelativePath(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "internal")
	os.MkdirAll(subDir, 0755)
	testFile := filepath.Join(subDir, "handler.go")
	if err := os.WriteFile(testFile, []byte("package internal\n"), 0644); err != nil {
		t.Fatal(err)
	}

	msg := "check @internal/handler.go for issues"
	cleaned, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Path != testFile {
		t.Errorf("expected path %s, got %s", testFile, attachments[0].Path)
	}
	if strings.Contains(cleaned, "@internal/handler.go") {
		t.Errorf("cleaned message should not contain @reference: %q", cleaned)
	}
}

func TestParseAttachments_LargeFileTruncation(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "large.txt")
	// Create a file larger than 10KB
	content := strings.Repeat("x", 15*1024)
	if err := os.WriteFile(testFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	msg := "read @large.txt"
	_, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if !attachments[0].Truncated {
		t.Error("expected Truncated to be true")
	}
	if !strings.Contains(attachments[0].Content, "[truncated at 10KB, use file_read for full content]") {
		t.Error("truncated content should contain truncation notice")
	}
	// Content should be roughly 10KB, not the full 15KB
	if len(attachments[0].Content) > 12*1024 {
		t.Errorf("truncated content too large: %d bytes", len(attachments[0].Content))
	}
}

func TestParseAttachments_BinaryFile(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "image.png")
	// Write some binary content with null bytes
	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00}
	if err := os.WriteFile(testFile, binaryContent, 0644); err != nil {
		t.Fatal(err)
	}

	msg := "look at @image.png"
	_, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	// PNG files are now detected as images and base64-encoded
	if !attachments[0].IsImage {
		t.Error("expected PNG to be detected as image")
	}
	if attachments[0].Base64Data == "" {
		t.Error("expected base64 data for image attachment")
	}
	if !strings.Contains(attachments[0].Content, "[image:") {
		t.Errorf("expected image notice, got: %q", attachments[0].Content)
	}
}

func TestParseAttachments_NonexistentFile(t *testing.T) {
	dir := t.TempDir()

	msg := "look at @nonexistent.go"
	_, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Error == "" {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseAttachments_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	file1 := filepath.Join(dir, "a.go")
	file2 := filepath.Join(dir, "b.py")
	os.WriteFile(file1, []byte("package a\n"), 0644)
	os.WriteFile(file2, []byte("print('hi')\n"), 0644)

	msg := "compare @a.go and @b.py"
	cleaned, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(attachments))
	}
	if strings.Contains(cleaned, "@a.go") || strings.Contains(cleaned, "@b.py") {
		t.Errorf("cleaned message still contains @references: %q", cleaned)
	}
}

func TestParseAttachments_DuplicateReference(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "dup.go")
	os.WriteFile(testFile, []byte("package dup\n"), 0644)

	msg := "look at @dup.go and then @dup.go again"
	_, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment (deduped), got %d", len(attachments))
	}
}

func TestParseAttachments_NoReferences(t *testing.T) {
	dir := t.TempDir()

	msg := "just fix the tests please"
	cleaned, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 0 {
		t.Fatalf("expected 0 attachments, got %d", len(attachments))
	}
	if cleaned != msg {
		t.Errorf("cleaned message changed: %q vs %q", cleaned, msg)
	}
}

func TestParseAttachments_EmailNotMatched(t *testing.T) {
	dir := t.TempDir()

	msg := "send to user@example.com"
	_, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 0 {
		t.Fatalf("email should not be treated as attachment, got %d attachments", len(attachments))
	}
}

func TestParseAttachments_Directory(t *testing.T) {
	dir := t.TempDir()
	subDir := filepath.Join(dir, "subdir.ext")
	os.MkdirAll(subDir, 0755)

	msg := "look at @subdir.ext"
	_, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	if attachments[0].Error == "" {
		t.Error("expected error for directory")
	}
	if !strings.Contains(attachments[0].Error, "directory") {
		t.Errorf("error should mention directory: %q", attachments[0].Error)
	}
}

func TestParseAttachments_HomePath(t *testing.T) {
	// This test verifies ~ expansion works without actually reading a home file
	dir := t.TempDir()

	msg := "look at @~/nonexistent-test-file.go"
	_, attachments := ParseAttachments(msg, dir)

	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(attachments))
	}
	home, _ := os.UserHomeDir()
	expectedPath := filepath.Join(home, "nonexistent-test-file.go")
	if attachments[0].Path != expectedPath {
		t.Errorf("expected path %s, got %s", expectedPath, attachments[0].Path)
	}
	// File won't exist, so we expect an error
	if attachments[0].Error == "" {
		t.Error("expected error for nonexistent home file")
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "text/x-go"},
		{"script.py", "text/x-python"},
		{"app.js", "text/javascript"},
		{"types.ts", "text/typescript"},
		{"lib.rs", "text/x-rust"},
		{"Main.java", "text/x-java"},
		{"config.yaml", "text/yaml"},
		{"data.json", "application/json"},
		{"readme.md", "text/markdown"},
		{"build.sh", "text/x-shellscript"},
		{"Dockerfile", "text/x-dockerfile"},
		{"Makefile", "text/x-makefile"},
		{"style.css", "text/css"},
		{"page.html", "text/html"},
		{"query.sql", "text/x-sql"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectMimeType(tt.path, nil)
			if got != tt.expected {
				t.Errorf("detectMimeType(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}

func TestIsBinaryContent(t *testing.T) {
	if isBinaryContent([]byte("hello world\n")) {
		t.Error("text should not be binary")
	}
	if isBinaryContent([]byte("")) {
		t.Error("empty should not be binary")
	}
	if !isBinaryContent([]byte{0x00, 0x01, 0x02}) {
		t.Error("null bytes should be binary")
	}
	// High ratio of control characters
	content := make([]byte, 100)
	for i := range content {
		content[i] = byte(i % 16) // lots of control chars
	}
	if !isBinaryContent(content) {
		t.Error("high control char ratio should be binary")
	}
}

func TestFormatAttachments(t *testing.T) {
	atts := []Attachment{
		{
			Path:     "/path/to/file.go",
			Content:  "package main\n",
			MimeType: "text/x-go",
		},
		{
			Path:  "/path/to/missing.go",
			Error: "file not found",
		},
	}

	result := FormatAttachments(atts)

	if !strings.Contains(result, "--- Attached Files ---") {
		t.Error("missing header")
	}
	if !strings.Contains(result, "/path/to/file.go") {
		t.Error("missing file path")
	}
	if !strings.Contains(result, "package main") {
		t.Error("missing file content")
	}
	if !strings.Contains(result, "text/x-go") {
		t.Error("missing MIME type")
	}
	if !strings.Contains(result, "[error: file not found]") {
		t.Error("missing error for second file")
	}
	if !strings.Contains(result, "--- End Attached Files ---") {
		t.Error("missing footer")
	}
}

func TestFormatAttachments_Empty(t *testing.T) {
	result := FormatAttachments(nil)
	if result != "" {
		t.Errorf("expected empty string for no attachments, got: %q", result)
	}
}
