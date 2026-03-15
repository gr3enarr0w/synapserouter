package agent

import (
	"bytes"
	"strings"
	"testing"
)

func TestStreamWriter(t *testing.T) {
	var lines []string
	sw := NewStreamWriter(func(line string) {
		lines = append(lines, line)
	}, nil)

	sw.Write([]byte("line1\nline2\n"))

	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	if lines[0] != "line1" {
		t.Errorf("lines[0] = %q, want line1", lines[0])
	}
	if lines[1] != "line2" {
		t.Errorf("lines[1] = %q, want line2", lines[1])
	}
}

func TestStreamWriterPartialLines(t *testing.T) {
	var lines []string
	sw := NewStreamWriter(func(line string) {
		lines = append(lines, line)
	}, nil)

	sw.Write([]byte("partial"))
	if len(lines) != 0 {
		t.Error("should not emit partial line")
	}

	sw.Write([]byte(" more\n"))
	if len(lines) != 1 {
		t.Fatal("should emit after newline")
	}
	if lines[0] != "partial more" {
		t.Errorf("line = %q, want 'partial more'", lines[0])
	}
}

func TestStreamWriterFlush(t *testing.T) {
	var lines []string
	sw := NewStreamWriter(func(line string) {
		lines = append(lines, line)
	}, nil)

	sw.Write([]byte("no newline"))
	sw.Flush()

	if len(lines) != 1 {
		t.Fatal("flush should emit remaining buffer")
	}
	if lines[0] != "no newline" {
		t.Errorf("line = %q, want 'no newline'", lines[0])
	}
}

func TestStreamWriterPassthrough(t *testing.T) {
	var buf bytes.Buffer
	var lines []string

	sw := NewStreamWriter(func(line string) {
		lines = append(lines, line)
	}, &buf)

	sw.Write([]byte("hello\nworld\n"))

	// Should have both callback and passthrough
	if len(lines) != 2 {
		t.Errorf("callback lines = %d, want 2", len(lines))
	}
	if buf.String() != "hello\nworld\n" {
		t.Errorf("passthrough = %q", buf.String())
	}
}

func TestStreamWriterNilCallback(t *testing.T) {
	sw := NewStreamWriter(nil, nil)
	n, err := sw.Write([]byte("test\n"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("written = %d, want 5", n)
	}
}

func TestLineScanner(t *testing.T) {
	var lines []string
	r := strings.NewReader("line1\nline2\nline3\n")

	LineScanner(r, func(line string) {
		lines = append(lines, line)
	})

	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if lines[2] != "line3" {
		t.Errorf("lines[2] = %q, want line3", lines[2])
	}
}
