package agent

import (
	"testing"
)

func TestAssessComplexity(t *testing.T) {
	tests := []struct {
		name    string
		message string
		hasSpec bool
		want    TaskComplexity
	}{
		{
			name:    "trivial question",
			message: "What is the purpose of this function?",
			want:    ComplexityTrivial,
		},
		{
			name:    "trivial explain",
			message: "Explain how the router works",
			want:    ComplexityTrivial,
		},
		{
			name:    "simple fix",
			message: "Fix the typo in main.go",
			want:    ComplexitySimple,
		},
		{
			name:    "simple rename",
			message: "Rename the variable foo to bar",
			want:    ComplexitySimple,
		},
		{
			name:    "simple short message",
			message: "bump the version",
			want:    ComplexitySimple,
		},
		{
			name:    "medium review",
			message: "Review the changes in the last commit",
			want:    ComplexityMedium,
		},
		{
			name:    "medium refactor",
			message: "Refactor the handler to use the new interface",
			want:    ComplexityMedium,
		},
		{
			name:    "full build",
			message: "Build a REST API for user management with CRUD operations",
			want:    ComplexityFull,
		},
		{
			name:    "full implement",
			message: "Implement the authentication middleware for the server",
			want:    ComplexityFull,
		},
		{
			name:    "full with spec file",
			message: "do the thing",
			hasSpec: true,
			want:    ComplexityFull,
		},
		{
			name:    "full create from scratch",
			message: "Create a new microservice from scratch with database integration",
			want:    ComplexityFull,
		},
		{
			name:    "question with question mark",
			message: "How does the circuit breaker work?",
			want:    ComplexityTrivial,
		},
		{
			name:    "short ambiguous defaults to simple",
			message: "do it",
			want:    ComplexitySimple,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AssessComplexity(tt.message, tt.hasSpec)
			if got != tt.want {
				t.Errorf("AssessComplexity(%q) = %s, want %s", tt.message, got, tt.want)
			}
		})
	}
}

func TestAdaptPipeline_Trivial(t *testing.T) {
	pipeline := copyPipeline(&DefaultPipeline)
	result := AdaptPipeline(pipeline, ComplexityTrivial)
	if result != nil {
		t.Errorf("expected nil pipeline for trivial task, got %s with %d phases", result.Name, len(result.Phases))
	}
}

func TestAdaptPipeline_Simple(t *testing.T) {
	pipeline := copyPipeline(&DefaultPipeline)
	result := AdaptPipeline(pipeline, ComplexitySimple)
	if result == nil {
		t.Fatal("expected non-nil pipeline for simple task")
	}
	if len(result.Phases) != 2 {
		t.Errorf("expected 2 phases for simple task, got %d", len(result.Phases))
	}
	if result.Phases[0].Name != "implement" {
		t.Errorf("expected first phase to be implement, got %s", result.Phases[0].Name)
	}
	if result.Phases[1].Name != "self-check" {
		t.Errorf("expected second phase to be self-check, got %s", result.Phases[1].Name)
	}
}

func TestAdaptPipeline_Medium(t *testing.T) {
	pipeline := copyPipeline(&DefaultPipeline)
	result := AdaptPipeline(pipeline, ComplexityMedium)
	if result == nil {
		t.Fatal("expected non-nil pipeline for medium task")
	}
	if len(result.Phases) != 3 {
		t.Errorf("expected 3 phases for medium task, got %d", len(result.Phases))
	}
	expectedPhases := []string{"implement", "self-check", "code-review"}
	for i, name := range expectedPhases {
		if result.Phases[i].Name != name {
			t.Errorf("phase %d: expected %s, got %s", i, name, result.Phases[i].Name)
		}
	}
}

func TestAdaptPipeline_Full(t *testing.T) {
	pipeline := copyPipeline(&DefaultPipeline)
	result := AdaptPipeline(pipeline, ComplexityFull)
	if result == nil {
		t.Fatal("expected non-nil pipeline for full task")
	}
	if len(result.Phases) != len(DefaultPipeline.Phases) {
		t.Errorf("expected %d phases for full task, got %d", len(DefaultPipeline.Phases), len(result.Phases))
	}
}

func TestAdaptPipeline_NilInput(t *testing.T) {
	result := AdaptPipeline(nil, ComplexitySimple)
	if result != nil {
		t.Error("expected nil result when input pipeline is nil")
	}
}

func TestAdaptPipeline_DataScienceFallback(t *testing.T) {
	// Data science pipeline has different phase names — should fall back to full.
	pipeline := copyPipeline(&DataSciencePipeline)
	result := AdaptPipeline(pipeline, ComplexitySimple)
	if result == nil {
		t.Fatal("expected non-nil pipeline for data science fallback")
	}
	// Should get the full data science pipeline since phase names don't match.
	if len(result.Phases) != len(DataSciencePipeline.Phases) {
		t.Errorf("expected full data science pipeline (%d phases), got %d",
			len(DataSciencePipeline.Phases), len(result.Phases))
	}
}

func TestTaskComplexity_String(t *testing.T) {
	tests := []struct {
		c    TaskComplexity
		want string
	}{
		{ComplexityTrivial, "trivial"},
		{ComplexitySimple, "simple"},
		{ComplexityMedium, "medium"},
		{ComplexityFull, "full"},
		{TaskComplexity(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.c.String(); got != tt.want {
			t.Errorf("TaskComplexity(%d).String() = %q, want %q", tt.c, got, tt.want)
		}
	}
}
