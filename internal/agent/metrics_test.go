package agent

import (
	"testing"
	"time"
)

func TestAgentMetricsRecord(t *testing.T) {
	m := NewAgentMetrics()

	m.RecordRequest(100*time.Millisecond, 500, 3, false)
	m.RecordRequest(200*time.Millisecond, 800, 5, true)

	snap := m.Snapshot()
	if snap.TotalRequests != 2 {
		t.Errorf("requests = %d, want 2", snap.TotalRequests)
	}
	if snap.TotalTokens != 1300 {
		t.Errorf("tokens = %d, want 1300", snap.TotalTokens)
	}
	if snap.TotalTurns != 8 {
		t.Errorf("turns = %d, want 8", snap.TotalTurns)
	}
	if snap.TotalErrors != 1 {
		t.Errorf("errors = %d, want 1", snap.TotalErrors)
	}
}

func TestAgentMetricsToolCalls(t *testing.T) {
	m := NewAgentMetrics()

	m.RecordToolCall("bash", 50*time.Millisecond)
	m.RecordToolCall("file_read", 10*time.Millisecond)
	m.RecordToolCall("bash", 30*time.Millisecond)

	snap := m.Snapshot()
	if snap.TotalToolCalls != 3 {
		t.Errorf("total tool calls = %d, want 3", snap.TotalToolCalls)
	}
	if snap.ToolCalls["bash"] != 2 {
		t.Errorf("bash calls = %d, want 2", snap.ToolCalls["bash"])
	}
	if snap.ToolCalls["file_read"] != 1 {
		t.Errorf("file_read calls = %d, want 1", snap.ToolCalls["file_read"])
	}
	if snap.ToolTime["bash"] != 80*time.Millisecond {
		t.Errorf("bash time = %v, want 80ms", snap.ToolTime["bash"])
	}
}

func TestAgentMetricsSubAgents(t *testing.T) {
	m := NewAgentMetrics()
	m.RecordSubAgent()
	m.RecordSubAgent()
	m.RecordHandoff()

	snap := m.Snapshot()
	if snap.SubAgentsSpawned != 2 {
		t.Errorf("sub-agents = %d, want 2", snap.SubAgentsSpawned)
	}
	if snap.HandoffsExecuted != 1 {
		t.Errorf("handoffs = %d, want 1", snap.HandoffsExecuted)
	}
}

func TestAgentMetricsAverages(t *testing.T) {
	m := NewAgentMetrics()
	m.RecordRequest(100*time.Millisecond, 500, 3, false)
	m.RecordRequest(200*time.Millisecond, 700, 5, false)

	avg := m.AvgRequestDuration()
	if avg != 150*time.Millisecond {
		t.Errorf("avg duration = %v, want 150ms", avg)
	}

	avgTokens := m.AvgTokensPerRequest()
	if avgTokens != 600 {
		t.Errorf("avg tokens = %d, want 600", avgTokens)
	}
}

func TestAgentMetricsEmptyAverages(t *testing.T) {
	m := NewAgentMetrics()
	if m.AvgRequestDuration() != 0 {
		t.Error("avg duration should be 0 with no requests")
	}
	if m.AvgTokensPerRequest() != 0 {
		t.Error("avg tokens should be 0 with no requests")
	}
}

func TestAgentMetricsSnapshotIndependent(t *testing.T) {
	m := NewAgentMetrics()
	m.RecordToolCall("bash", 10*time.Millisecond)

	snap := m.Snapshot()
	m.RecordToolCall("bash", 20*time.Millisecond)

	// Snapshot should not be affected by later changes
	if snap.ToolCalls["bash"] != 1 {
		t.Error("snapshot should be independent of later changes")
	}
}
