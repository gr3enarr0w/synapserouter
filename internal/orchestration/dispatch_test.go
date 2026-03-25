package orchestration

import (
	"strings"
	"testing"
)

func TestMatchSkills_GoCode(t *testing.T) {
	registry := DefaultSkills()
	matched := MatchSkills("fix the Go authentication handler", registry)

	names := skillNames(matched)
	assertContains(t, names, "go-patterns", "should match go-patterns for Go+auth compound trigger")
	assertContains(t, names, "security-review", "should match security-review for auth")
	// "fix" no longer triggers code-implement (too generic)
	for _, n := range names {
		if n == "code-implement" {
			t.Error("should NOT match code-implement for bare 'fix'")
		}
	}
}

func TestMatchSkills_Python(t *testing.T) {
	registry := DefaultSkills()
	matched := MatchSkills("write python tests for the parser", registry)

	names := skillNames(matched)
	assertContains(t, names, "python-patterns", "should match python-patterns")
	assertContains(t, names, "code-implement", "should match code-implement for write")
	assertContains(t, names, "python-testing", "should match python-testing for tests")
	assertContains(t, names, "go-testing", "should match go-testing for test keyword")
}

func TestMatchSkills_NoMatch(t *testing.T) {
	registry := DefaultSkills()
	matched := MatchSkills("hello world", registry)

	if len(matched) != 0 {
		t.Errorf("expected no matches for 'hello world', got %d: %v", len(matched), skillNames(matched))
	}
}

func TestMatchSkills_CaseInsensitive(t *testing.T) {
	registry := DefaultSkills()
	matched := MatchSkills("IMPLEMENT a Docker container", registry)

	names := skillNames(matched)
	assertContains(t, names, "code-implement", "should match code-implement case-insensitively")
	assertContains(t, names, "docker-expert", "should match docker-expert case-insensitively")
}

func TestMatchSkills_Research(t *testing.T) {
	registry := DefaultSkills()
	matched := MatchSkills("research how the API works", registry)

	names := skillNames(matched)
	assertContains(t, names, "research", "should match research skill")
	assertContains(t, names, "api-design", "should match api-design for API keyword")
}

func TestBuildSkillChain_PhaseOrdering(t *testing.T) {
	skills := []Skill{
		{Name: "code-review", Phase: "review", Role: "reviewer"},
		{Name: "go-patterns", Phase: "analyze", Role: "coder"},
		{Name: "code-implement", Phase: "implement", Role: "coder"},
		{Name: "go-testing", Phase: "verify", Role: "tester", DependsOn: []string{"code-implement"}},
	}

	chain := BuildSkillChain(skills)

	if len(chain) != 4 {
		t.Fatalf("expected 4 skills in chain, got %d", len(chain))
	}

	expectedPhaseOrder := []string{"analyze", "implement", "verify", "review"}
	for i, skill := range chain {
		if skill.Phase != expectedPhaseOrder[i] {
			t.Errorf("step %d: expected phase %q, got %q (skill: %s)", i, expectedPhaseOrder[i], skill.Phase, skill.Name)
		}
	}
}

func TestBuildSkillChain_Deduplication(t *testing.T) {
	skills := []Skill{
		{Name: "go-patterns", Phase: "analyze", Role: "coder"},
		{Name: "go-patterns", Phase: "analyze", Role: "coder"},
		{Name: "code-implement", Phase: "implement", Role: "coder"},
	}

	chain := BuildSkillChain(skills)

	if len(chain) != 2 {
		t.Errorf("expected 2 unique skills, got %d", len(chain))
	}
}

func TestBuildSkillChain_Empty(t *testing.T) {
	chain := BuildSkillChain(nil)
	if chain != nil {
		t.Errorf("expected nil for empty input, got %v", chain)
	}
}

func TestSkillChainToSteps(t *testing.T) {
	chain := []Skill{
		{Name: "go-patterns", Description: "Go patterns", Role: "coder", Phase: "analyze"},
		{Name: "code-implement", Description: "Implement code", Role: "coder", Phase: "implement"},
	}

	steps := SkillChainToSteps(chain, "fix the bug")

	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	if steps[0].Role != "coder" {
		t.Errorf("step 0 role: expected coder, got %s", steps[0].Role)
	}
	if !strings.Contains(steps[0].Prompt, "go-patterns") {
		t.Errorf("step 0 prompt should contain skill name, got: %s", steps[0].Prompt)
	}
	if !strings.Contains(steps[0].Prompt, "fix the bug") {
		t.Errorf("step 0 prompt should contain goal, got: %s", steps[0].Prompt)
	}
	if steps[0].Status != StepStatusPending {
		t.Errorf("step 0 status: expected pending, got %s", steps[0].Status)
	}
}

func TestDispatch_WithSkills(t *testing.T) {
	registry := DefaultSkills()
	steps := Dispatch("refactor the Go code and make sure tests pass", registry)

	if len(steps) == 0 {
		t.Fatal("expected steps from dispatch, got none")
	}

	// Should have at least go-patterns (analyze), code-implement (implement),
	// go-testing (verify)
	hasAnalyze := false
	hasImplement := false
	hasVerify := false
	for _, step := range steps {
		if strings.Contains(step.Prompt, "go-patterns") {
			hasAnalyze = true
		}
		if strings.Contains(step.Prompt, "code-implement") {
			hasImplement = true
		}
		if strings.Contains(step.Prompt, "go-testing") {
			hasVerify = true
		}
	}

	if !hasAnalyze {
		t.Error("expected go-patterns skill in dispatch")
	}
	if !hasImplement {
		t.Error("expected code-implement skill in dispatch")
	}
	if !hasVerify {
		t.Error("expected go-testing skill in dispatch")
	}
}

func TestDispatch_NoMatch(t *testing.T) {
	registry := DefaultSkills()
	steps := Dispatch("hello", registry)

	if steps != nil {
		t.Errorf("expected nil steps for unmatched goal, got %d steps", len(steps))
	}
}

func TestDispatchWithDetails_Fallback(t *testing.T) {
	registry := DefaultSkills()
	result := DispatchWithDetails("hello", registry)

	if !result.FallbackToRole {
		t.Error("expected fallback_to_role for unmatched goal")
	}
	if len(result.Steps) == 0 {
		t.Error("expected role-based steps as fallback")
	}
}

func TestDispatchWithDetails_SkillMatch(t *testing.T) {
	registry := DefaultSkills()
	result := DispatchWithDetails("fix the Go auth handler", registry)

	if result.FallbackToRole {
		t.Error("should not fall back to role when skills match")
	}
	if len(result.MatchedSkills) == 0 {
		t.Error("expected matched skills")
	}
	if len(result.SkillChain) == 0 {
		t.Error("expected skill chain")
	}
	if len(result.Steps) == 0 {
		t.Error("expected steps")
	}
}

func TestMCPToolsForChain(t *testing.T) {
	chain := []Skill{
		{Name: "research", MCPTools: []string{"research-mcp.research_search"}},
		{Name: "go-patterns", MCPTools: nil},
		{Name: "research2", MCPTools: []string{"research-mcp.research_search", "context7.query-docs"}},
	}

	tools := MCPToolsForChain(chain)

	if len(tools) != 2 {
		t.Errorf("expected 2 unique tools, got %d: %v", len(tools), tools)
	}
}

func TestMCPToolsForChain_Empty(t *testing.T) {
	tools := MCPToolsForChain(nil)
	if len(tools) != 0 {
		t.Errorf("expected 0 tools for empty chain, got %d", len(tools))
	}
}

// TestMCPToolsForChain_DefaultSkillsCoverage verifies most skills have MCP
// tool bindings (to avoid burning LLM tokens on context discovery).
func TestMCPToolsForChain_DefaultSkillsCoverage(t *testing.T) {
	skills := DefaultSkills()

	withMCP := 0
	for _, skill := range skills {
		if len(skill.MCPTools) > 0 {
			withMCP++
		}
	}

	// At least 50% of skills should have MCP bindings (some domain skills
	// like git-expert, data-scrubber, task-orchestrator don't need external tools)
	coverage := float64(withMCP) / float64(len(skills))
	if coverage < 0.5 {
		t.Errorf("only %d/%d (%.0f%%) skills have MCP tool bindings — most analysis/implement skills should bind MCP tools to save tokens",
			withMCP, len(skills), coverage*100)
	}
}

// TestMCPToolsForChain_DeduplicatesAcrossSkills verifies that when multiple
// skills in a chain reference the same MCP tool, it only appears once.
func TestMCPToolsForChain_DeduplicatesAcrossSkills(t *testing.T) {
	registry := DefaultSkills()
	// "implement Go tests" matches go-patterns + code-implement + go-testing
	// Both go-patterns and go-testing bind context7.query-docs
	matched := MatchSkills("implement Go tests", registry)
	chain := BuildSkillChain(matched)
	tools := MCPToolsForChain(chain)

	// Count occurrences of each tool
	counts := map[string]int{}
	for _, tool := range tools {
		counts[tool]++
	}
	for tool, count := range counts {
		if count > 1 {
			t.Errorf("MCP tool %q appears %d times — should be deduplicated", tool, count)
		}
	}
}

// TestMCPToolsForChain_ResearchBindsBoth verifies research skill binds both
// research-mcp and context7 (deep + docs lookup).
func TestMCPToolsForChain_ResearchBindsBoth(t *testing.T) {
	registry := DefaultSkills()
	matched := MatchSkills("research how gRPC streaming works", registry)
	chain := BuildSkillChain(matched)
	tools := MCPToolsForChain(chain)

	hasResearch := false
	hasContext7 := false
	for _, tool := range tools {
		if strings.Contains(tool, "research-mcp") {
			hasResearch = true
		}
		if strings.Contains(tool, "context7") {
			hasContext7 = true
		}
	}

	if !hasResearch {
		t.Error("research skill should bind research-mcp.research_search")
	}
	if !hasContext7 {
		t.Error("research skill should bind context7.query-docs")
	}
}

func TestDispatch_SecurityChain(t *testing.T) {
	registry := DefaultSkills()
	result := DispatchWithDetails("fix the auth bug and verify security", registry)

	if result.FallbackToRole {
		t.Fatal("should not fall back for security-related goal")
	}

	names := skillNames(result.SkillChain)
	assertContains(t, names, "security-review", "should include security-review")
	// "fix" no longer triggers code-implement (removed generic triggers)
	assertContains(t, names, "go-testing", "should include testing for verify")

	// Verify phase ordering: analyze skills before implement, implement before verify
	phasesSeen := map[string]int{}
	for i, skill := range result.SkillChain {
		phasesSeen[skill.Phase] = i
	}
	if analyzeIdx, ok := phasesSeen["analyze"]; ok {
		if implIdx, ok := phasesSeen["implement"]; ok {
			if analyzeIdx > implIdx {
				t.Error("analyze phase should come before implement phase")
			}
		}
	}
}

// === Realistic prompt table-driven tests ===
// These simulate real user prompts and verify the correct skill chain fires.

func TestDispatch_RealisticPrompts(t *testing.T) {
	registry := DefaultSkills()

	tests := []struct {
		name            string
		prompt          string
		wantSkills      []string // must be present
		wantAbsent      []string // must NOT be present
		wantFallback    bool     // expect role-based fallback
		wantPhaseOrder  bool     // verify phases are ordered
		minSkills       int      // minimum skills expected
	}{
		// --- Go development scenarios ---
		{
			name:       "fix Go bug with auth",
			prompt:     "fix the authentication bug in the Go middleware",
			wantSkills: []string{"go-patterns", "security-review"},
			wantAbsent: []string{"python-patterns", "docker-expert", "code-implement"},
			minSkills:  2,
		},
		{
			name:       "refactor Go handler",
			prompt:     "refactor the handler to use interfaces in Go",
			wantSkills: []string{"go-patterns", "code-implement"},
			wantAbsent: []string{"python-patterns"},
			minSkills:  2,
		},
		{
			name:       "add Go tests for router",
			prompt:     "add unit tests for the router package in Go",
			wantSkills: []string{"go-patterns", "go-testing"},
			minSkills:  2,
		},
		{
			name:       "Go file extension trigger",
			prompt:     "update the handler.go file to return proper errors",
			wantSkills: []string{"go-patterns"},
			minSkills:  1,
		},

		// --- Python development scenarios ---
		{
			name:       "write Python parser",
			prompt:     "write a Python parser for the config file format",
			wantSkills: []string{"python-patterns", "code-implement"},
			wantAbsent: []string{"go-patterns"},
			minSkills:  2,
		},
		{
			name:       "pytest fixtures",
			prompt:     "add pytest fixtures for the database layer",
			wantSkills: []string{"python-patterns", "python-testing"},
			minSkills:  2,
		},
		{
			name:       "Python .py trigger",
			prompt:     "update the utils.py file to validate input",
			wantSkills: []string{"python-patterns"},
			minSkills:  1,
		},

		// --- Security scenarios ---
		{
			name:       "OAuth implementation",
			prompt:     "implement OAuth2 token refresh flow",
			wantSkills: []string{"security-review", "code-implement"},
			minSkills:  2,
		},
		{
			name:       "credential rotation",
			prompt:     "rotate the API key credentials for production",
			wantSkills: []string{"security-review"},
			minSkills:  1,
		},
		{
			name:       "password hashing",
			prompt:     "update the password hashing to use bcrypt",
			wantSkills: []string{"security-review"},
			minSkills:  1,
		},
		{
			name:       "secret management",
			prompt:     "move secrets out of the config into vault",
			wantSkills: []string{"security-review"},
			minSkills:  1,
		},

		// --- Docker/container scenarios ---
		{
			name:       "Dockerfile optimization",
			prompt:     "optimize the Dockerfile for smaller image size",
			wantSkills: []string{"docker-expert"},
			wantAbsent: []string{"go-patterns", "python-patterns"},
			minSkills:  1,
		},
		{
			name:       "docker compose setup",
			prompt:     "create a docker compose file for local development",
			wantSkills: []string{"docker-expert"},
			minSkills:  1,
		},
		{
			name:       "container networking",
			prompt:     "fix the container networking between services",
			wantSkills: []string{"docker-expert"},
			minSkills:  1,
		},

		// --- API design scenarios ---
		{
			name:       "REST endpoint design",
			prompt:     "design a REST endpoint for user management",
			wantSkills: []string{"api-design"},
			minSkills:  1,
		},
		{
			name:       "route handler",
			prompt:     "add a new route handler for /v1/webhooks",
			wantSkills: []string{"api-design"},
			minSkills:  1,
		},
		{
			name:       "API authentication",
			prompt:     "add API key authentication to the endpoint",
			wantSkills: []string{"api-design", "security-review"},
			minSkills:  2,
		},

		// --- Research scenarios ---
		{
			name:       "library research",
			prompt:     "research how the OpenTelemetry SDK works",
			wantSkills: []string{"research"},
			minSkills:  1,
		},
		{
			name:       "investigate bug",
			prompt:     "investigate why the connection pool is exhausted",
			wantSkills: []string{"research"},
			minSkills:  1,
		},
		{
			name:       "explain pattern",
			prompt:     "explain how the circuit breaker pattern works",
			wantSkills: []string{"research"},
			minSkills:  1,
		},

		// --- Multi-skill chain scenarios ---
		{
			name:       "full Go pipeline",
			prompt:     "implement the new Go handler, add tests, and review the code quality",
			wantSkills: []string{"go-patterns", "code-implement", "go-testing", "code-review"},
			minSkills:  4,
			wantPhaseOrder: true,
		},
		{
			name:       "security audit with fix",
			prompt:     "fix the OAuth token refresh and verify the credential handling is secure",
			wantSkills: []string{"security-review", "go-testing"},
			minSkills:  2,
			wantPhaseOrder: true,
		},
		{
			name:       "Docker + Go",
			prompt:     "build a Go microservice with Docker container support",
			wantSkills: []string{"go-patterns", "docker-expert", "code-implement"},
			minSkills:  3,
		},
		{
			name:       "API + security + test",
			prompt:     "create a new API endpoint with token auth and validate it with tests",
			wantSkills: []string{"api-design", "security-review", "go-testing"},
			minSkills:  3,
			wantPhaseOrder: true,
		},
		{
			name:       "research then implement",
			prompt:     "research how gRPC streaming works then implement it in Go",
			wantSkills: []string{"research", "go-patterns", "code-implement"},
			minSkills:  3,
			wantPhaseOrder: true,
		},

		// --- Code review scenarios ---
		{
			name:       "code review request",
			prompt:     "review the recent changes for quality issues",
			wantSkills: []string{"code-review"},
			minSkills:  1,
		},
		{
			name:       "review and check",
			prompt:     "check the code for any problems and clean it up",
			wantSkills: []string{"code-review"},
			minSkills:  1,
		},

		// --- New skill scenarios ---
		{
			name:       "JavaScript React component",
			prompt:     "write a React component in TypeScript for the dashboard",
			wantSkills: []string{"javascript-patterns", "code-implement"},
			minSkills:  2,
		},
		{
			name:       "Rust ownership fix",
			prompt:     "fix the ownership issue in the Rust parser",
			wantSkills: []string{"rust-patterns"},
			minSkills:  1,
		},
		{
			name:       "Spring Boot endpoint",
			prompt:     "add a Spring Boot REST endpoint for user registration",
			wantSkills: []string{"java-spring"},
			minSkills:  1,
		},
		{
			name:       "FastAPI with Pydantic",
			prompt:     "create a FastAPI endpoint with Pydantic validation",
			wantSkills: []string{"fastapi-patterns"},
			minSkills:  1,
		},
		{
			name:       "SQL schema design",
			prompt:     "design the database schema for the orders table",
			wantSkills: []string{"sql-expert"},
			minSkills:  1,
		},
		{
			name:       "dbt incremental model",
			prompt:     "create a dbt incremental model for the events table",
			wantSkills: []string{"dbt-modeler"},
			minSkills:  1,
		},
		{
			name:       "Snowflake warehouse",
			prompt:     "tune the Snowflake warehouse for better query performance",
			wantSkills: []string{"snowflake-query"},
			minSkills:  1,
		},
		{
			name:       "deep market research",
			prompt:     "do deep research on competitor pricing models",
			wantSkills: []string{"deep-research"},
			minSkills:  1,
		},
		{
			name:       "Git rebase conflict",
			prompt:     "help me rebase and resolve the merge conflict",
			wantSkills: []string{"git-expert"},
			minSkills:  1,
		},
		{
			name:       "GitHub PR workflow",
			prompt:     "create a GitHub Actions workflow for pull request checks",
			wantSkills: []string{"github-workflows"},
			minSkills:  1,
		},
		{
			name:       "DevOps Kubernetes deploy",
			prompt:     "set up a Kubernetes deployment with helm charts",
			wantSkills: []string{"devops-engineer", "code-implement"},
			minSkills:  2,
		},
		{
			name:       "Terraform infrastructure",
			prompt:     "write Terraform for the AWS infrastructure",
			wantSkills: []string{"devops-engineer", "code-implement"},
			minSkills:  2,
		},
		{
			name:       "PPTX presentation",
			prompt:     "create a presentation about the Q4 results",
			wantSkills: []string{"document-mcp"},
			minSkills:  1,
		},
		{
			name:       "Slack notification",
			prompt:     "post a message to the Slack channel about the deploy",
			wantSkills: []string{"slack-integration"},
			minSkills:  1,
		},
		{
			name:       "Jira ticket management",
			prompt:     "create a Jira ticket for the auth bug and link it to the epic",
			wantSkills: []string{"jira-manage"},
			minSkills:  1,
		},
		{
			name:       "EDA on dataset",
			prompt:     "run exploratory data analysis on the sales data for outliers",
			wantSkills: []string{"eda-explorer"},
			minSkills:  1,
		},
		{
			name:       "ML feature engineering",
			prompt:     "do feature engineering on the user activity data with one-hot encoding",
			wantSkills: []string{"feature-engineer"},
			minSkills:  1,
		},
		{
			name:       "time series forecast",
			prompt:     "build a time series forecast model for revenue prediction",
			wantSkills: []string{"predictive-modeler"},
			minSkills:  1,
		},
		{
			name:       "PII data scrubbing",
			prompt:     "anonymize the PII in the customer database for GDPR compliance",
			wantSkills: []string{"data-scrubber"},
			minSkills:  1,
		},
		{
			name:       "Python venv setup",
			prompt:     "create a venv and install the requirements.txt dependencies",
			wantSkills: []string{"python-venv"},
			minSkills:  1,
		},
		{
			name:       "prompt optimization",
			prompt:     "optimize the system prompt for better few-shot performance",
			wantSkills: []string{"prompt-engineer"},
			minSkills:  1,
		},
		{
			name:       "spec document",
			prompt:     "create a specification for the new notification system",
			wantSkills: []string{"spec-workflow"},
			minSkills:  1,
		},
		{
			name:       "parallel task decomposition",
			prompt:     "decompose this refactor into parallel subtasks",
			wantSkills: []string{"task-orchestrator", "code-implement"}, // "refactor" still triggers code-implement
			minSkills:  2,
		},
		{
			name:       "doc coauthoring",
			prompt:     "draft a technical spec proposal for the API redesign",
			wantSkills: []string{"doc-coauthoring"},
			minSkills:  1,
		},
		{
			name:       "full polyglot pipeline",
			prompt:     "implement the Python FastAPI endpoint, add pytest coverage, and review quality",
			wantSkills: []string{"python-patterns", "fastapi-patterns", "code-implement", "python-testing", "code-review"},
			minSkills:  5,
			wantPhaseOrder: true,
		},

		// --- No-match / fallback scenarios ---
		{
			name:         "simple greeting",
			prompt:       "hello, how are you?",
			wantFallback: true,
		},
		{
			name:         "unrelated question",
			prompt:       "what's the weather like today?",
			wantFallback: true,
		},
		{
			name:         "vague statement",
			prompt:       "I'm thinking about lunch",
			wantFallback: true,
		},
		{
			name:         "empty-ish prompt",
			prompt:       "hmm ok",
			wantFallback: true,
		},

		// --- Edge cases ---
		{
			name:       "mixed case and punctuation",
			prompt:     "FIX the GO handler!! Make tests PASS!!!",
			wantSkills: []string{"go-patterns", "go-testing"},
			minSkills:  2,
		},
		{
			name:       "very long prompt",
			prompt:     "I need you to look at the Go code in the authentication module, specifically the OAuth2 token handling where we store credentials, refactor it to use the new interface pattern, add comprehensive test coverage, and then review the whole thing for any security vulnerabilities",
			wantSkills: []string{"go-patterns", "security-review", "code-implement", "go-testing", "code-review"},
			minSkills:  5,
			wantPhaseOrder: true,
		},
		{
			name:       "trigger substring in word",
			prompt:     "update the golang service",
			wantSkills: []string{"go-patterns"},
			minSkills:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DispatchWithDetails(tt.prompt, registry)

			if tt.wantFallback {
				if !result.FallbackToRole {
					t.Errorf("expected fallback to role-based planning, but got %d skills: %v",
						len(result.MatchedSkills), skillNames(result.MatchedSkills))
				}
				if len(result.Steps) == 0 {
					t.Error("fallback should still produce role-based steps")
				}
				return
			}

			// Should NOT fall back
			if result.FallbackToRole {
				t.Fatalf("should not fall back for prompt %q", tt.prompt)
			}

			// Check minimum skill count
			if tt.minSkills > 0 && len(result.SkillChain) < tt.minSkills {
				t.Errorf("expected at least %d skills, got %d: %v",
					tt.minSkills, len(result.SkillChain), skillNames(result.SkillChain))
			}

			// Check required skills present
			chainNames := skillNames(result.SkillChain)
			for _, want := range tt.wantSkills {
				assertContains(t, chainNames, want,
					"expected skill "+want+" in chain")
			}

			// Check absent skills
			for _, absent := range tt.wantAbsent {
				for _, name := range chainNames {
					if name == absent {
						t.Errorf("skill %s should NOT be in chain, got: %v", absent, chainNames)
						break
					}
				}
			}

			// Check phase ordering
			if tt.wantPhaseOrder {
				lastPhase := -1
				for _, skill := range result.SkillChain {
					phase := PhaseOrder[skill.Phase]
					if phase < lastPhase {
						t.Errorf("phase ordering violated: %s (phase %d) came after phase %d — chain: %v",
							skill.Name, phase, lastPhase, skillNames(result.SkillChain))
					}
					lastPhase = phase
				}
			}

			// Every skill should produce a step
			if len(result.Steps) != len(result.SkillChain) {
				t.Errorf("skill chain has %d skills but produced %d steps",
					len(result.SkillChain), len(result.Steps))
			}

			// Steps should have proper structure
			for i, step := range result.Steps {
				if step.ID == "" {
					t.Errorf("step %d has empty ID", i)
				}
				if step.Role == "" {
					t.Errorf("step %d has empty Role", i)
				}
				if step.Prompt == "" {
					t.Errorf("step %d has empty Prompt", i)
				}
				if step.Status != StepStatusPending {
					t.Errorf("step %d status should be pending, got %s", i, step.Status)
				}
				if !strings.Contains(step.Prompt, tt.prompt) {
					t.Errorf("step %d prompt should contain the goal text", i)
				}
			}
		})
	}
}

// TestDispatch_PhaseOrderInvariant verifies that no dispatch result ever
// has phases out of order, regardless of which skills match.
func TestDispatch_PhaseOrderInvariant(t *testing.T) {
	registry := DefaultSkills()
	prompts := []string{
		"implement Go tests and review",
		"research the API, build a docker container, add Go tests, review quality",
		"fix the auth in Python and validate with tests",
		"create an endpoint handler, add tests, review code",
		"refactor Go code, add coverage, and check quality",
	}

	for _, prompt := range prompts {
		result := DispatchWithDetails(prompt, registry)
		if result.FallbackToRole {
			continue
		}

		lastPhase := -1
		for _, skill := range result.SkillChain {
			phase := PhaseOrder[skill.Phase]
			if phase < lastPhase {
				t.Errorf("prompt %q: phase order violated at skill %s (phase %s=%d after %d)",
					prompt, skill.Name, skill.Phase, phase, lastPhase)
			}
			lastPhase = phase
		}
	}
}

// TestDispatch_StepIDsUnique verifies every step in a dispatch result has a unique ID.
func TestDispatch_StepIDsUnique(t *testing.T) {
	registry := DefaultSkills()
	steps := Dispatch("implement Go API endpoint with auth and tests and review", registry)
	if len(steps) == 0 {
		t.Fatal("expected steps")
	}

	seen := map[string]bool{}
	for _, step := range steps {
		if seen[step.ID] {
			t.Errorf("duplicate step ID: %s", step.ID)
		}
		seen[step.ID] = true
	}
}

// TestDispatch_EveryDefaultSkillIsReachable verifies that every skill in
// DefaultSkills() can be triggered by at least one of its own trigger words.
func TestDispatch_EveryDefaultSkillIsReachable(t *testing.T) {
	registry := DefaultSkills()

	for _, skill := range registry {
		// Try each trigger
		reachable := false
		for _, trigger := range skill.Triggers {
			matched := MatchSkills(trigger, registry)
			for _, m := range matched {
				if m.Name == skill.Name {
					reachable = true
					break
				}
			}
			if reachable {
				break
			}
		}
		if !reachable {
			t.Errorf("skill %q is unreachable — none of its triggers %v match it", skill.Name, skill.Triggers)
		}
	}
}

// TestDispatch_SkillPromptContainsGoalAndSkillName verifies prompt structure.
func TestDispatch_SkillPromptContainsGoalAndSkillName(t *testing.T) {
	registry := DefaultSkills()
	goal := "fix the Go authentication handler"
	result := DispatchWithDetails(goal, registry)

	for _, step := range result.Steps {
		if !strings.Contains(step.Prompt, goal) {
			t.Errorf("step prompt missing goal text: %s", step.Prompt)
		}
		// Prompt should contain a skill name (bracketed)
		if !strings.Contains(step.Prompt, "[") {
			t.Errorf("step prompt missing skill name bracket: %s", step.Prompt)
		}
	}
}

// === Skill instruction embedding tests ===

func TestDefaultSkills_HaveInstructions(t *testing.T) {
	skills := DefaultSkills()
	withInstructions := 0
	for _, s := range skills {
		if s.Instructions != "" {
			withInstructions++
		}
	}
	// All 36 skills should have embedded instructions
	if withInstructions < 30 {
		t.Errorf("expected at least 30 skills with embedded instructions, got %d/%d", withInstructions, len(skills))
	}
}

func TestSkillPrompt_WithInstructions(t *testing.T) {
	skill := Skill{
		Name:         "test-skill",
		Description:  "Test description",
		Instructions: "## Core Rules\n1. Always do X\n2. Never do Y",
	}
	prompt := skillPrompt(skill, "fix the bug")
	if !strings.Contains(prompt, "## Core Rules") {
		t.Error("expected prompt to contain skill instructions")
	}
	if !strings.Contains(prompt, "--- Skill Instructions ---") {
		t.Error("expected instruction delimiters in prompt")
	}
	if !strings.Contains(prompt, "fix the bug") {
		t.Error("expected prompt to contain the goal")
	}
}

func TestSkillPrompt_WithoutInstructions(t *testing.T) {
	skill := Skill{
		Name:        "test-skill",
		Description: "Test description",
	}
	prompt := skillPrompt(skill, "fix the bug")
	if strings.Contains(prompt, "--- Skill Instructions ---") {
		t.Error("should not have instruction delimiters when no instructions")
	}
	if !strings.Contains(prompt, "Test description") {
		t.Error("should still have description")
	}
}

func TestDefaultSkills_InstructionsContainRules(t *testing.T) {
	skills := DefaultSkills()
	// Spot-check that well-known skills have real content
	for _, s := range skills {
		if s.Name == "go-patterns" && s.Instructions != "" {
			if !strings.Contains(s.Instructions, "Go") {
				t.Error("go-patterns instructions should mention Go")
			}
			return
		}
	}
	t.Error("could not find go-patterns skill with instructions")
}

// helpers

func skillNames(skills []Skill) []string {
	names := make([]string, len(skills))
	for i, s := range skills {
		names[i] = s.Name
	}
	return names
}

func assertContains(t *testing.T, names []string, target, msg string) {
	t.Helper()
	for _, n := range names {
		if n == target {
			return
		}
	}
	t.Errorf("%s — got: %v", msg, names)
}
