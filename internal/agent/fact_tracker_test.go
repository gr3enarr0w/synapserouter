package agent

import (
	"sync"
	"testing"
)

func TestFactTracker_RecordBashSuccess(t *testing.T) {
	ft := NewFactTracker()
	ft.RecordToolOutput("bash", map[string]interface{}{
		"command": "echo hello",
	}, "hello\n", 0, 1)

	fact := ft.LastBashResult("echo")
	if fact == nil {
		t.Fatal("expected bash fact for 'echo'")
	}
	if fact.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", fact.ExitCode)
	}
	if fact.OutputID != 1 {
		t.Errorf("expected output ID 1, got %d", fact.OutputID)
	}
}

func TestFactTracker_RecordBashFailure(t *testing.T) {
	ft := NewFactTracker()
	ft.RecordToolOutput("bash", map[string]interface{}{
		"command": "go build ./...",
	}, "main.go:42: undefined: foo\n", 1, 2)

	fact := ft.LastBashResult("go build")
	if fact == nil {
		t.Fatal("expected bash fact for 'go build'")
	}
	if fact.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", fact.ExitCode)
	}
}

func TestFactTracker_RecordTestRun(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		output     string
		exitCode   int
		wantPassed bool
	}{
		{
			name:       "go test pass",
			cmd:        "go test ./...",
			output:     "ok  \tgithub.com/foo/bar\t0.5s\nok  \tgithub.com/foo/baz\t1.2s\n",
			exitCode:   0,
			wantPassed: true,
		},
		{
			name:       "go test fail",
			cmd:        "go test -race ./...",
			output:     "FAIL\tgithub.com/foo/bar\t0.5s\nok  \tgithub.com/foo/baz\t1.2s\n",
			exitCode:   1,
			wantPassed: false,
		},
		{
			name:       "pytest pass",
			cmd:        "pytest tests/",
			output:     "5 passed in 2.3s\n",
			exitCode:   0,
			wantPassed: true,
		},
		{
			name:       "pytest fail",
			cmd:        "pytest tests/",
			output:     "3 passed, 2 failed in 1.5s\n",
			exitCode:   1,
			wantPassed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := NewFactTracker()
			ft.RecordToolOutput("bash", map[string]interface{}{
				"command": tt.cmd,
			}, tt.output, tt.exitCode, 10)

			result := ft.LastTestResult()
			if result == nil {
				t.Fatal("expected test result")
			}
			if result.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", result.Passed, tt.wantPassed)
			}
		})
	}
}

func TestFactTracker_RecordFilePaths(t *testing.T) {
	ft := NewFactTracker()

	// file_read adds path from args
	ft.RecordToolOutput("file_read", map[string]interface{}{
		"path": "/src/main.go",
	}, "package main\nfunc main() {}\n", 0, 1)

	// grep adds paths from output
	ft.RecordToolOutput("grep", map[string]interface{}{
		"pattern": "TODO",
	}, "internal/handler.go:42: TODO fix this\ninternal/server.go:10: TODO cleanup\n", 0, 2)

	paths := ft.KnownPaths()
	if !paths["/src/main.go"] {
		t.Error("expected /src/main.go in known paths")
	}
	if !paths["internal/handler.go"] {
		t.Error("expected internal/handler.go in known paths (from grep output)")
	}
	if !paths["internal/server.go"] {
		t.Error("expected internal/server.go in known paths (from grep output)")
	}
}

func TestFactTracker_FileWriteAddsPath(t *testing.T) {
	ft := NewFactTracker()
	ft.RecordToolOutput("file_write", map[string]interface{}{
		"file_path": "/src/new_file.go",
	}, "wrote 42 bytes", 0, 1)

	paths := ft.KnownPaths()
	if !paths["/src/new_file.go"] {
		t.Error("expected /src/new_file.go in known paths after file_write")
	}
}

func TestFactTracker_ConcurrentSafe(t *testing.T) {
	ft := NewFactTracker()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			ft.RecordToolOutput("bash", map[string]interface{}{
				"command": "echo test",
			}, "output", 0, int64(n))
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = ft.KnownPaths()
			_ = ft.LastTestResult()
			_ = ft.LastBashResult("echo")
			_ = ft.RecentBashFacts(5)
		}()
	}

	wg.Wait()
	// No race detector failure = pass
}

func TestFactTracker_LastBashResult_NoMatch(t *testing.T) {
	ft := NewFactTracker()
	ft.RecordToolOutput("bash", map[string]interface{}{
		"command": "ls -la",
	}, "total 42\n", 0, 1)

	if ft.LastBashResult("go build") != nil {
		t.Error("expected nil for unmatched prefix")
	}
}

func TestFactTracker_RecentBashFacts(t *testing.T) {
	ft := NewFactTracker()
	for i := 0; i < 5; i++ {
		ft.RecordToolOutput("bash", map[string]interface{}{
			"command": "echo",
		}, "out", 0, int64(i))
	}
	facts := ft.RecentBashFacts(3)
	if len(facts) != 3 {
		t.Errorf("expected 3 recent facts, got %d", len(facts))
	}
}
