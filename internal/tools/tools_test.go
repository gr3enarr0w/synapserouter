package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultRegistry(t *testing.T) {
	r := DefaultRegistry()
	expected := []string{"bash", "file_read", "file_write", "file_edit", "grep", "glob", "git"}
	for _, name := range expected {
		if _, ok := r.Get(name); !ok {
			t.Errorf("DefaultRegistry missing tool: %s", name)
		}
	}
}

func TestRegistryOpenAIToolDefinitions(t *testing.T) {
	r := DefaultRegistry()
	defs := r.OpenAIToolDefinitions()
	if len(defs) != 7 {
		t.Errorf("expected 7 tool definitions, got %d", len(defs))
	}
	for _, d := range defs {
		if d["type"] != "function" {
			t.Errorf("expected type=function, got %v", d["type"])
		}
		fn, _ := d["function"].(map[string]interface{})
		if fn["name"] == nil || fn["name"] == "" {
			t.Error("tool definition missing name")
		}
	}
}

func TestRegistryExecuteUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "nonexistent", nil, ".")
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestBashTool(t *testing.T) {
	tool := &BashTool{}
	ctx := context.Background()
	dir := t.TempDir()

	t.Run("simple command", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"command": "echo hello"}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(result.Output) != "hello" {
			t.Errorf("expected 'hello', got %q", result.Output)
		}
		if result.ExitCode != 0 {
			t.Errorf("expected exit code 0, got %d", result.ExitCode)
		}
	})

	t.Run("failing command", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"command": "exit 42"}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.ExitCode != 42 {
			t.Errorf("expected exit code 42, got %d", result.ExitCode)
		}
	})

	t.Run("empty command", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" {
			t.Error("expected error for empty command")
		}
	})

	t.Run("respects workdir", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"command": "pwd"}, dir)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(result.Output)
		// Resolve symlinks for comparison (macOS /var -> /private/var)
		resolvedDir, _ := filepath.EvalSymlinks(dir)
		resolvedGot, _ := filepath.EvalSymlinks(got)
		if resolvedGot != resolvedDir {
			t.Errorf("expected workdir %q, got %q", resolvedDir, resolvedGot)
		}
	})
}

func TestFileReadTool(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()
	dir := t.TempDir()

	content := "line1\nline2\nline3\nline4\nline5\n"
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte(content), 0644)

	t.Run("read entire file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"path": testFile}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "line1") || !strings.Contains(result.Output, "line5") {
			t.Errorf("expected all lines, got %q", result.Output)
		}
	})

	t.Run("read with offset and limit", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": testFile, "offset": float64(2), "limit": float64(2),
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "line2") || !strings.Contains(result.Output, "line3") {
			t.Errorf("expected lines 2-3, got %q", result.Output)
		}
		if strings.Contains(result.Output, "line1") || strings.Contains(result.Output, "line4") {
			t.Errorf("should not contain lines outside range, got %q", result.Output)
		}
	})

	t.Run("nonexistent file", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"path": "/nonexistent"}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" {
			t.Error("expected error for nonexistent file")
		}
	})

	t.Run("relative path", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"path": "test.txt"}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "line1") {
			t.Errorf("expected content with relative path, got %q", result.Output)
		}
	})
}

func TestFileWriteTool(t *testing.T) {
	tool := &FileWriteTool{}
	ctx := context.Background()
	dir := t.TempDir()

	t.Run("create file", func(t *testing.T) {
		path := filepath.Join(dir, "new.txt")
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": path, "content": "hello world",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %s", result.Error)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "hello world" {
			t.Errorf("expected 'hello world', got %q", string(data))
		}
	})

	t.Run("create nested directories", func(t *testing.T) {
		path := filepath.Join(dir, "a", "b", "c", "deep.txt")
		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": path, "content": "deep content",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %s", result.Error)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "deep content" {
			t.Errorf("expected 'deep content', got %q", string(data))
		}
	})
}

func TestFileEditTool(t *testing.T) {
	tool := &FileEditTool{}
	ctx := context.Background()
	dir := t.TempDir()

	t.Run("single replacement", func(t *testing.T) {
		path := filepath.Join(dir, "edit.txt")
		os.WriteFile(path, []byte("hello world"), 0644)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": path, "old_string": "world", "new_string": "go",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %s", result.Error)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "hello go" {
			t.Errorf("expected 'hello go', got %q", string(data))
		}
	})

	t.Run("ambiguous match fails", func(t *testing.T) {
		path := filepath.Join(dir, "ambiguous.txt")
		os.WriteFile(path, []byte("foo bar foo"), 0644)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": path, "old_string": "foo", "new_string": "baz",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" {
			t.Error("expected error for ambiguous match")
		}
	})

	t.Run("replace_all", func(t *testing.T) {
		path := filepath.Join(dir, "replaceall.txt")
		os.WriteFile(path, []byte("foo bar foo"), 0644)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": path, "old_string": "foo", "new_string": "baz", "replace_all": true,
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %s", result.Error)
		}
		data, _ := os.ReadFile(path)
		if string(data) != "baz bar baz" {
			t.Errorf("expected 'baz bar baz', got %q", string(data))
		}
	})

	t.Run("not found", func(t *testing.T) {
		path := filepath.Join(dir, "notfound.txt")
		os.WriteFile(path, []byte("hello"), 0644)

		result, err := tool.Execute(ctx, map[string]interface{}{
			"path": path, "old_string": "missing", "new_string": "x",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" {
			t.Error("expected error for not found")
		}
	})
}

func TestGlobTool(t *testing.T) {
	tool := &GlobTool{}
	ctx := context.Background()
	dir := t.TempDir()

	// Create test structure
	os.MkdirAll(filepath.Join(dir, "src"), 0755)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644)
	os.WriteFile(filepath.Join(dir, "src", "lib.go"), []byte("package src"), 0644)
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("# readme"), 0644)

	t.Run("recursive glob", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "**/*.go", "path": dir,
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "main.go") {
			t.Errorf("expected main.go in results, got %q", result.Output)
		}
		if !strings.Contains(result.Output, "lib.go") {
			t.Errorf("expected lib.go in results, got %q", result.Output)
		}
	})

	t.Run("specific pattern", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "*.md", "path": dir,
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "README.md") {
			t.Errorf("expected README.md, got %q", result.Output)
		}
		if strings.Contains(result.Output, ".go") {
			t.Errorf("should not contain .go files, got %q", result.Output)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "*.xyz", "path": dir,
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "no files matched") {
			t.Errorf("expected 'no files matched', got %q", result.Output)
		}
	})
}

func TestGrepTool(t *testing.T) {
	tool := &GrepTool{}
	ctx := context.Background()
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"), 0644)
	os.WriteFile(filepath.Join(dir, "lib.go"), []byte("package main\nfunc helper() {}\n"), 0644)

	t.Run("basic search", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "func main", "path": dir,
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "main.go") {
			t.Errorf("expected main.go in results, got %q", result.Output)
		}
	})

	t.Run("with include filter", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "func", "path": dir, "include": "*.go",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "func") {
			t.Errorf("expected func in results, got %q", result.Output)
		}
	})

	t.Run("no matches", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"pattern": "nonexistent_string_xyz", "path": dir,
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "no matches") {
			t.Errorf("expected 'no matches found', got %q", result.Output)
		}
	})
}

func TestGitTool(t *testing.T) {
	tool := &GitTool{}
	ctx := context.Background()
	dir := t.TempDir()

	// Init a test repo
	runGit := func(args ...string) {
		t.Helper()
		cmd := strings.Join(args, " ")
		bashTool := &BashTool{}
		bashTool.Execute(ctx, map[string]interface{}{"command": "git " + cmd}, dir)
	}
	runGit("init")
	runGit("config", "user.email", "test@test.com")
	runGit("config", "user.name", "Test")
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0644)
	runGit("add", ".")
	runGit("commit", "-m", "initial")

	t.Run("status", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{"subcommand": "status"}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error != "" {
			t.Errorf("unexpected error: %s", result.Error)
		}
	})

	t.Run("log", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"subcommand": "log", "args": "--oneline -5",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(result.Output, "initial") {
			t.Errorf("expected commit message, got %q", result.Output)
		}
	})

	t.Run("blocks force push", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"subcommand": "push", "args": "--force origin main",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" || !strings.Contains(result.Error, "dangerous") {
			t.Errorf("expected dangerous flag blocked, got error: %q", result.Error)
		}
	})

	t.Run("blocks force delete branch", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"subcommand": "branch", "args": "-D some-branch",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" || !strings.Contains(result.Error, "dangerous") {
			t.Errorf("expected dangerous flag blocked, got error: %q", result.Error)
		}
	})

	t.Run("blocks disallowed subcommand", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"subcommand": "config", "args": "user.email test@test.com",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" || !strings.Contains(result.Error, "not allowed") {
			t.Errorf("expected subcommand blocked, got error: %q", result.Error)
		}
	})

	t.Run("blocks force push via refspec", func(t *testing.T) {
		result, err := tool.Execute(ctx, map[string]interface{}{
			"subcommand": "push", "args": "+main:main",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" || !strings.Contains(result.Error, "force-push refspec") {
			t.Errorf("expected refspec blocked, got error: %q", result.Error)
		}
	})
}

func TestPathContainment(t *testing.T) {
	dir := t.TempDir()

	t.Run("relative path stays in workdir", func(t *testing.T) {
		path, err := resolveToolPathChecked("sub/file.txt", dir)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(path, dir) {
			t.Errorf("expected path within %s, got %s", dir, path)
		}
	})

	t.Run("traversal blocked", func(t *testing.T) {
		_, err := resolveToolPathChecked("../../../etc/passwd", dir)
		if err == nil {
			t.Error("expected error for path traversal")
		}
	})

	t.Run("absolute path outside workdir blocked", func(t *testing.T) {
		_, err := resolveToolPathChecked("/etc/passwd", dir)
		if err == nil {
			t.Error("expected error for absolute path outside workdir")
		}
	})

	t.Run("absolute path inside workdir allowed", func(t *testing.T) {
		innerPath := filepath.Join(dir, "allowed.txt")
		path, err := resolveToolPathChecked(innerPath, dir)
		if err != nil {
			t.Errorf("expected success for path within workdir, got: %v", err)
		}
		if path != innerPath {
			t.Errorf("expected %s, got %s", innerPath, path)
		}
	})

	t.Run("file_write blocks traversal", func(t *testing.T) {
		tool := &FileWriteTool{}
		result, err := tool.Execute(context.Background(), map[string]interface{}{
			"path": "../../../tmp/escape.txt", "content": "escaped",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" {
			t.Error("expected path traversal to be blocked for file_write")
		}
	})

	t.Run("file_edit blocks traversal", func(t *testing.T) {
		tool := &FileEditTool{}
		result, err := tool.Execute(context.Background(), map[string]interface{}{
			"path": "/etc/hosts", "old_string": "localhost", "new_string": "hacked",
		}, dir)
		if err != nil {
			t.Fatal(err)
		}
		if result.Error == "" {
			t.Error("expected path traversal to be blocked for file_edit")
		}
	})
}

func TestToolCategories(t *testing.T) {
	r := DefaultRegistry()
	readOnlyTools := []string{"file_read", "grep", "glob"}
	writeTools := []string{"bash", "file_write", "file_edit", "git"}

	for _, name := range readOnlyTools {
		tool, ok := r.Get(name)
		if !ok {
			t.Errorf("missing tool: %s", name)
			continue
		}
		if tool.Category() != CategoryReadOnly {
			t.Errorf("%s: expected CategoryReadOnly, got %s", name, tool.Category())
		}
	}

	for _, name := range writeTools {
		tool, ok := r.Get(name)
		if !ok {
			t.Errorf("missing tool: %s", name)
			continue
		}
		if tool.Category() != CategoryWrite {
			t.Errorf("%s: expected CategoryWrite, got %s", name, tool.Category())
		}
	}
}

func TestHelpers(t *testing.T) {
	t.Run("resolveToolPath absolute", func(t *testing.T) {
		result := resolveToolPath("/absolute/path", "/workdir")
		if result != "/absolute/path" {
			t.Errorf("expected /absolute/path, got %s", result)
		}
	})

	t.Run("resolveToolPath relative", func(t *testing.T) {
		result := resolveToolPath("relative/path", "/workdir")
		if result != "/workdir/relative/path" {
			t.Errorf("expected /workdir/relative/path, got %s", result)
		}
	})

	t.Run("stringArg", func(t *testing.T) {
		args := map[string]interface{}{"key": "value"}
		if stringArg(args, "key") != "value" {
			t.Error("stringArg failed")
		}
		if stringArg(args, "missing") != "" {
			t.Error("stringArg should return empty for missing")
		}
	})

	t.Run("intArg", func(t *testing.T) {
		args := map[string]interface{}{"num": float64(42)}
		if intArg(args, "num", 0) != 42 {
			t.Error("intArg failed for float64")
		}
		if intArg(args, "missing", 10) != 10 {
			t.Error("intArg should return default for missing")
		}
	})

	t.Run("boolArg", func(t *testing.T) {
		args := map[string]interface{}{"flag": true}
		if !boolArg(args, "flag") {
			t.Error("boolArg failed")
		}
		if boolArg(args, "missing") {
			t.Error("boolArg should return false for missing")
		}
	})
}
