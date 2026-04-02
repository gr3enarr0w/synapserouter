package agent

import (
	"strings"
	"testing"

	"github.com/gr3enarr0w/mcp-ecosystem/synapse-router/internal/providers"
)

func TestExtractStructuredSummary(t *testing.T) {
	msgs := []providers.Message{
		{Role: "assistant", Content: "I decided to use the factory pattern because it provides better extensibility."},
		{Role: "assistant", Content: "Going with SQLite since it requires no server setup."},
		{Role: "tool", Content: "modified internal/agent/agent.go\nmodified internal/tools/web_search.go"},
		{Role: "assistant", Content: "error: cannot find package \"foo\"\nFAIL TestSomething"},
		{Role: "assistant", Content: "TODO: need to add error handling for edge cases"},
		{Role: "assistant", Content: "ok  github.com/example/pkg  1.234s\n--- PASS: TestFoo (0.01s)"},
	}

	cc := ExtractStructuredSummary(msgs, "implement")

	if cc.Phase != "implement" {
		t.Errorf("phase = %q, want implement", cc.Phase)
	}
	if cc.MsgCount != 6 {
		t.Errorf("MsgCount = %d, want 6", cc.MsgCount)
	}
	if len(cc.Decisions) == 0 {
		t.Error("expected decisions to be extracted")
	}
	if len(cc.Rationale) == 0 {
		t.Error("expected rationale to be extracted")
	}
	if len(cc.FilesChanged) == 0 {
		t.Error("expected files to be extracted")
	}
	if len(cc.Errors) == 0 {
		t.Error("expected errors to be extracted")
	}
	if len(cc.OpenItems) == 0 {
		t.Error("expected open items to be extracted")
	}
	if len(cc.TestResults) == 0 {
		t.Error("expected test results to be extracted")
	}
}

func TestExtractStructuredSummary_Empty(t *testing.T) {
	cc := ExtractStructuredSummary(nil, "plan")
	if cc.MsgCount != 0 {
		t.Errorf("MsgCount = %d, want 0", cc.MsgCount)
	}
	if len(cc.Decisions) != 0 || len(cc.Errors) != 0 || len(cc.FilesChanged) != 0 {
		t.Error("empty messages should produce empty sections")
	}
}

func TestExtractStructuredSummary_Dedup(t *testing.T) {
	msgs := []providers.Message{
		{Role: "assistant", Content: "I decided to use Go for this project."},
		{Role: "assistant", Content: "I decided to use Go for this project."}, // duplicate
	}

	cc := ExtractStructuredSummary(msgs, "plan")
	if len(cc.Decisions) != 1 {
		t.Errorf("expected 1 deduped decision, got %d", len(cc.Decisions))
	}
}

func TestFormatCompressedContext(t *testing.T) {
	cc := CompressedContext{
		Phase:        "implement",
		MsgCount:     50,
		Decisions:    []string{"Use factory pattern"},
		Rationale:    []string{"Because it provides extensibility"},
		FilesChanged: []string{"agent.go", "web_search.go"},
		TestResults:  []string{"ok  pkg  1.2s"},
		Errors:       []string{"error: missing import"},
		OpenItems:    []string{"TODO: add validation"},
	}

	output := FormatCompressedContext(cc)

	if !strings.Contains(output, "50 messages from implement phase") {
		t.Error("missing header with message count and phase")
	}
	if !strings.Contains(output, "DECISIONS MADE:") {
		t.Error("missing decisions section")
	}
	if !strings.Contains(output, "Use factory pattern") {
		t.Error("missing decision content")
	}
	if !strings.Contains(output, "FILES TOUCHED:") {
		t.Error("missing files section")
	}
	if !strings.Contains(output, "ERRORS ENCOUNTERED:") {
		t.Error("missing errors section")
	}
	if !strings.Contains(output, "recall tool") {
		t.Error("missing recall tool footer")
	}
}

func TestFormatCompressedContext_EmptySections(t *testing.T) {
	cc := CompressedContext{Phase: "plan", MsgCount: 5}
	output := FormatCompressedContext(cc)

	if strings.Contains(output, "DECISIONS MADE:") {
		t.Error("empty decisions section should not appear")
	}
	if !strings.Contains(output, "5 messages from plan phase") {
		t.Error("header should still appear")
	}
}

func TestMaskObservations(t *testing.T) {
	largeOutput := strings.Repeat("x", 1000) // > 512 threshold
	msgs := []providers.Message{
		{Role: "user", Content: "do something"},
		{Role: "assistant", Content: "I'll run a command"},
		{Role: "tool", Content: "summary line\n" + largeOutput, ToolCallID: "tc1"},
		{Role: "tool", Content: "small output", ToolCallID: "tc2"}, // < threshold
	}

	masked := MaskObservations(msgs)

	// User and assistant messages unchanged
	if masked[0].Content != "do something" {
		t.Error("user message should be unchanged")
	}
	if masked[1].Content != "I'll run a command" {
		t.Error("assistant message should be unchanged")
	}

	// Large tool output should be masked
	if !strings.Contains(masked[2].Content, "summary line") {
		t.Error("first line of tool output should be preserved")
	}
	if !strings.Contains(masked[2].Content, "[output stored") {
		t.Error("large tool output should be masked")
	}
	if masked[2].ToolCallID != "tc1" {
		t.Error("ToolCallID should be preserved")
	}

	// Small tool output should be unchanged
	if masked[3].Content != "small output" {
		t.Error("small tool output should be unchanged")
	}
}

func TestMaskObservations_PreservesNonToolMessages(t *testing.T) {
	longAssistant := strings.Repeat("detailed explanation ", 100) // large but not tool role
	msgs := []providers.Message{
		{Role: "assistant", Content: longAssistant},
	}

	masked := MaskObservations(msgs)
	if masked[0].Content != longAssistant {
		t.Error("non-tool messages should never be masked regardless of size")
	}
}

func TestMaskObservations_EmptyInput(t *testing.T) {
	masked := MaskObservations(nil)
	if len(masked) != 0 {
		t.Error("nil input should return empty slice")
	}
}

func TestCapSlice(t *testing.T) {
	s := []string{"a", "b", "c", "d", "e"}
	if got := capSlice(s, 3); len(got) != 3 {
		t.Errorf("capSlice(5, 3) = %d, want 3", len(got))
	}
	if got := capSlice(s, 10); len(got) != 5 {
		t.Errorf("capSlice(5, 10) = %d, want 5", len(got))
	}
	if got := capSlice(nil, 5); len(got) != 0 {
		t.Errorf("capSlice(nil, 5) = %d, want 0", len(got))
	}
}
