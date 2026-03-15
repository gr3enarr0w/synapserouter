package agent

import (
	"errors"
	"testing"
	"time"
)

func TestTraceStartSpan(t *testing.T) {
	tr := NewTrace("agent-1", "")

	endSpan := tr.StartSpan("llm_call", "llm_call", map[string]interface{}{
		"model": "claude-sonnet-4-6",
	})
	time.Sleep(5 * time.Millisecond)
	endSpan(nil)

	if tr.SpanCount() != 1 {
		t.Errorf("span count = %d, want 1", tr.SpanCount())
	}

	spans := tr.SpansByType("llm_call")
	if len(spans) != 1 {
		t.Fatalf("llm_call spans = %d, want 1", len(spans))
	}
	if spans[0].Duration < 5*time.Millisecond {
		t.Errorf("span duration = %v, want >= 5ms", spans[0].Duration)
	}
	if spans[0].Metadata["model"] != "claude-sonnet-4-6" {
		t.Errorf("model = %v", spans[0].Metadata["model"])
	}
}

func TestTraceSpanWithError(t *testing.T) {
	tr := NewTrace("agent-1", "")

	endSpan := tr.StartSpan("tool_call", "tool_call", nil)
	endSpan(errors.New("tool failed"))

	spans := tr.SpansByType("tool_call")
	if len(spans) != 1 {
		t.Fatal("expected 1 tool_call span")
	}
	if spans[0].Error != "tool failed" {
		t.Errorf("error = %q, want 'tool failed'", spans[0].Error)
	}
}

func TestTraceAddSpan(t *testing.T) {
	tr := NewTrace("agent-1", "parent-1")

	tr.AddSpan(Span{
		Name:     "file_read",
		Type:     "tool_call",
		Duration: 10 * time.Millisecond,
	})

	if tr.SpanCount() != 1 {
		t.Errorf("span count = %d, want 1", tr.SpanCount())
	}
	if tr.ParentID != "parent-1" {
		t.Errorf("parent ID = %q, want parent-1", tr.ParentID)
	}
}

func TestTraceJSON(t *testing.T) {
	tr := NewTrace("agent-1", "")
	tr.AddSpan(Span{Name: "test", Type: "tool_call"})

	data, err := tr.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Error("JSON output should not be empty")
	}
}

func TestTraceSpansByType(t *testing.T) {
	tr := NewTrace("agent-1", "")
	tr.AddSpan(Span{Name: "a", Type: "llm_call"})
	tr.AddSpan(Span{Name: "b", Type: "tool_call"})
	tr.AddSpan(Span{Name: "c", Type: "llm_call"})

	llm := tr.SpansByType("llm_call")
	if len(llm) != 2 {
		t.Errorf("llm_call spans = %d, want 2", len(llm))
	}

	tool := tr.SpansByType("tool_call")
	if len(tool) != 1 {
		t.Errorf("tool_call spans = %d, want 1", len(tool))
	}

	handoff := tr.SpansByType("handoff")
	if len(handoff) != 0 {
		t.Errorf("handoff spans = %d, want 0", len(handoff))
	}
}

func TestTraceTotalDuration(t *testing.T) {
	tr := NewTrace("agent-1", "")
	time.Sleep(5 * time.Millisecond)
	if tr.TotalDuration() < 5*time.Millisecond {
		t.Errorf("total duration = %v, want >= 5ms", tr.TotalDuration())
	}
}
