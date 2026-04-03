package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

// mockPipelineAgent implements PipelineToolAgent for testing.
type mockPipelineAgent struct {
	criteria string
	request  string
	config   Config

	childResult string
	childErr    error

	verifyPassed  bool
	verifyResults []VerifyResult
}

func (m *mockPipelineAgent) RunChild(ctx context.Context, cfg SpawnConfig, task string) (string, error) {
	return m.childResult, m.childErr
}

func (m *mockPipelineAgent) RunVerificationGate(phaseName string) (bool, []VerifyResult) {
	return m.verifyPassed, m.verifyResults
}

func (m *mockPipelineAgent) GetAcceptanceCriteria() string      { return m.criteria }
func (m *mockPipelineAgent) SetAcceptanceCriteria(c string)     { m.criteria = c }
func (m *mockPipelineAgent) GetOriginalRequest() string         { return m.request }
func (m *mockPipelineAgent) GetConfig() Config                  { return m.config }
func (m *mockPipelineAgent) Emit(EventType, string, map[string]any) {}

func TestPipelinePlanTool(t *testing.T) {
	agent := &mockPipelineAgent{}
	tool := NewPipelinePlanTool(agent)

	if tool.Name() != "pipeline_plan" {
		t.Fatalf("name = %q", tool.Name())
	}
	if tool.Category() != tools.CategoryWrite {
		t.Fatal("wrong category")
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"task":  "Build a REST API with CRUD operations",
		"scope": "medium",
	}, "/tmp")

	if err != nil {
		t.Fatal(err)
	}
	if result.Output == "" {
		t.Fatal("empty output")
	}
	if agent.criteria != "Build a REST API with CRUD operations" {
		t.Fatalf("criteria not stored: %q", agent.criteria)
	}
}

func TestPipelinePlanToolMissingTask(t *testing.T) {
	agent := &mockPipelineAgent{}
	tool := NewPipelinePlanTool(agent)

	result, _ := tool.Execute(context.Background(), map[string]interface{}{}, "/tmp")
	if result.Error == "" {
		t.Fatal("expected error for missing task")
	}
}

func TestPipelineVerifyTool(t *testing.T) {
	agent := &mockPipelineAgent{
		verifyPassed: true,
		verifyResults: []VerifyResult{
			{Name: "build/go", Passed: true, ExitCode: 0},
			{Name: "test/go", Passed: true, ExitCode: 0},
		},
	}
	tool := NewPipelineVerifyTool(agent)

	if tool.Name() != "pipeline_verify" {
		t.Fatalf("name = %q", tool.Name())
	}
	if tool.Category() != tools.CategoryReadOnly {
		t.Fatal("wrong category")
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{}, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if result.Output == "" {
		t.Fatal("empty output")
	}
	if !strings.Contains(result.Output, "ALL CHECKS PASSED") {
		t.Fatalf("expected 'ALL CHECKS PASSED' in output: %s", result.Output)
	}
}

func TestPipelineVerifyToolFailure(t *testing.T) {
	agent := &mockPipelineAgent{
		verifyPassed: false,
		verifyResults: []VerifyResult{
			{Name: "build/go", Passed: true, ExitCode: 0},
			{Name: "test/go", Passed: false, ExitCode: 1, Output: "FAIL: TestSomething"},
		},
	}
	tool := NewPipelineVerifyTool(agent)

	result, _ := tool.Execute(context.Background(), map[string]interface{}{}, "/tmp")
	if !strings.Contains(result.Output, "SOME CHECKS FAILED") {
		t.Fatal("expected failure message")
	}
	if !strings.Contains(result.Output, "FAIL: TestSomething") {
		t.Fatal("expected failure output")
	}
}

func TestPipelineReviewTool(t *testing.T) {
	agent := &mockPipelineAgent{
		criteria:    "All tests pass",
		request:     "Fix the auth bug",
		childResult: "CODE_REVIEW_PASS — all criteria met",
	}
	tool := NewPipelineReviewTool(agent)

	if tool.Name() != "pipeline_review" {
		t.Fatalf("name = %q", tool.Name())
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"criteria": "All tests pass",
	}, "/tmp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Output, "CODE_REVIEW_PASS") {
		t.Fatal("expected pass signal in output")
	}
}

func TestPipelineTestTool(t *testing.T) {
	agent := &mockPipelineAgent{
		criteria:    "API returns JSON",
		request:     "Build REST API",
		childResult: "ACCEPTANCE_PASS — all checks passed",
	}
	tool := NewPipelineTestTool(agent)

	if tool.Name() != "pipeline_test" {
		t.Fatalf("name = %q", tool.Name())
	}

	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"criteria": "API returns JSON",
	}, "/tmp")
	if !strings.Contains(result.Output, "ACCEPTANCE_PASS") {
		t.Fatal("expected acceptance pass")
	}
}

func TestPipelineImplementTool(t *testing.T) {
	agent := &mockPipelineAgent{
		childResult: "Implementation complete. All tests pass.",
	}
	tool := NewPipelineImplementTool(agent)

	if tool.Name() != "pipeline_implement" {
		t.Fatalf("name = %q", tool.Name())
	}

	result, _ := tool.Execute(context.Background(), map[string]interface{}{
		"plan": "1. Create handler.go\n2. Add routes\n3. Test",
	}, "/tmp")
	if !strings.Contains(result.Output, "Implementation complete") {
		t.Fatal("expected implementation result")
	}
}

func TestPipelineStatusTool(t *testing.T) {
	agent := &mockPipelineAgent{
		criteria: "Tests pass with 80% coverage",
		request:  "Add unit tests",
	}
	tool := NewPipelineStatusTool(agent)

	if tool.Name() != "pipeline_status" {
		t.Fatalf("name = %q", tool.Name())
	}
	if tool.Category() != tools.CategoryReadOnly {
		t.Fatal("wrong category")
	}

	result, _ := tool.Execute(context.Background(), map[string]interface{}{}, "/tmp")
	if !strings.Contains(result.Output, "has_plan") {
		t.Fatal("expected has_plan field")
	}
}

func TestRegisterPipelineTools(t *testing.T) {
	registry := tools.NewRegistry()
	agent := &mockPipelineAgent{}

	RegisterPipelineTools(registry, agent)

	names := []string{"pipeline_plan", "pipeline_implement", "pipeline_verify", "pipeline_review", "pipeline_test", "pipeline_status"}
	for _, name := range names {
		if _, ok := registry.Get(name); !ok {
			t.Errorf("tool %q not registered", name)
		}
	}
}

