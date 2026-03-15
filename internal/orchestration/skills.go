package orchestration

// Skill is a named unit of work with trigger conditions, a role mapping,
// optional MCP tool bindings, and phase-based ordering for DAG execution.
type Skill struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Triggers    []string `json:"triggers"`
	Role        string   `json:"role"`
	MCPTools    []string `json:"mcp_tools,omitempty"`
	DependsOn   []string `json:"depends_on,omitempty"`
	Phase       string   `json:"phase"`
}

// PhaseOrder defines the execution ordering of skill phases.
var PhaseOrder = map[string]int{
	"analyze":   0,
	"implement": 1,
	"verify":    2,
	"review":    3,
}

// DefaultSkills returns the built-in skill registry.
// Skills with MCPTools bindings auto-invoke those tools at execution time,
// injecting results as context so the LLM doesn't burn tokens discovering
// library APIs or patterns from scratch.
//
// Trigger design rules:
//   - Short words (<=4 chars) use exact word matching to avoid false positives
//   - Multi-word phrases use substring matching
//   - Include natural human phrasing, not just developer jargon
func DefaultSkills() []Skill {
	return []Skill{
		{
			Name:        "go-patterns",
			Description: "Idiomatic Go development patterns and conventions",
			Triggers:    []string{"go", "golang", ".go"},
			Role:        "coder",
			MCPTools:    []string{"context7.query-docs"},
			Phase:       "analyze",
		},
		{
			Name:        "python-patterns",
			Description: "Idiomatic Python development patterns",
			Triggers:    []string{"python", ".py", "pip", "pytest"},
			Role:        "coder",
			MCPTools:    []string{"context7.query-docs"},
			Phase:       "analyze",
		},
		{
			Name:        "security-review",
			Description: "Security vulnerability detection and audit",
			Triggers: []string{
				"auth", "credential", "token", "oauth", "secret", "password",
				"api key", "apikey", "secure", "security", "vulnerability",
				"permission", "encrypt", "decrypt",
			},
			Role:     "reviewer",
			MCPTools: []string{"research-mcp.research_search"},
			Phase:    "analyze",
		},
		{
			Name:        "code-implement",
			Description: "Produce implementation-ready code changes",
			Triggers: []string{
				"implement", "build", "write", "refactor", "create", "fix",
				"add", "update", "change", "modify", "set up", "setup",
			},
			Role:  "coder",
			Phase: "implement",
		},
		{
			Name:        "go-testing",
			Description: "Go testing — table-driven tests, race detection, benchmarks",
			Triggers:    []string{"test", "verify", "validate", "coverage"},
			Role:        "tester",
			MCPTools:    []string{"context7.query-docs"},
			Phase:       "verify",
			DependsOn:   []string{"code-implement"},
		},
		{
			Name:        "python-testing",
			Description: "Python testing — pytest, fixtures, mocking",
			Triggers:    []string{"test", "verify", "validate", "coverage"},
			Role:        "tester",
			MCPTools:    []string{"context7.query-docs"},
			Phase:       "verify",
			DependsOn:   []string{"code-implement"},
		},
		{
			Name:        "code-review",
			Description: "Structured code review for quality and correctness",
			Triggers: []string{
				"review", "clean", "quality", "check",
			},
			Role:  "reviewer",
			Phase: "review",
		},
		{
			Name:        "api-design",
			Description: "REST/OpenAPI endpoint design patterns",
			Triggers:    []string{"endpoint", "handler", "rest", "route", "api"},
			Role:        "architect",
			MCPTools:    []string{"context7.query-docs"},
			Phase:       "analyze",
		},
		{
			Name:        "docker-expert",
			Description: "Docker and container best practices",
			Triggers:    []string{"docker", "container", "dockerfile", "compose"},
			Role:        "coder",
			MCPTools:    []string{"context7.query-docs"},
			Phase:       "implement",
		},
		{
			Name:        "research",
			Description: "Investigate context, alternatives, and constraints",
			Triggers: []string{
				"research", "investigate", "explain",
				"debug", "diagnose", "troubleshoot",
			},
			Role:     "researcher",
			MCPTools: []string{"research-mcp.research_search", "context7.query-docs"},
			Phase:    "analyze",
		},
	}
}
