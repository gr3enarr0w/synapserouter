package orchestration

import (
	"strings"
	"time"
)

type AgentTemplate struct {
	Type         string
	Description  string
	Capabilities []string
}

func DefaultAgentTemplates() []AgentTemplate {
	return []AgentTemplate{
		{Type: "coordinator", Description: "Coordinates the swarm and assigns work", Capabilities: []string{"coordinate", "prioritize", "synthesize"}},
		{Type: "queen-coordinator", Description: "Primary coordinator for broader swarm execution and consensus", Capabilities: []string{"coordinate", "consensus", "delegation"}},
		{Type: "architect", Description: "Designs systems and interfaces", Capabilities: []string{"design", "interfaces", "tradeoffs"}},
		{Type: "coder", Description: "Implements code changes", Capabilities: []string{"implementation", "refactor", "integration"}},
		{Type: "tester", Description: "Builds and runs test plans", Capabilities: []string{"testing", "verification", "coverage"}},
		{Type: "reviewer", Description: "Reviews quality and regressions", Capabilities: []string{"review", "risk", "quality"}},
		{Type: "researcher", Description: "Gathers context and options", Capabilities: []string{"research", "summarize", "compare"}},
		{Type: "debugger", Description: "Finds root causes and recovery paths", Capabilities: []string{"debugging", "triage", "root-cause"}},
		{Type: "documenter", Description: "Produces technical summaries", Capabilities: []string{"documentation", "handoff", "explanation"}},
		{Type: "designer", Description: "Designs product and interface behavior", Capabilities: []string{"ux", "design", "flows"}},
		{Type: "deployer", Description: "Handles deployment and rollout tasks", Capabilities: []string{"deployment", "release", "rollout"}},
		{Type: "security-audit", Description: "Runs security analysis and audit passes", Capabilities: []string{"security", "audit", "review"}},
		{Type: "security-alert", Description: "Flags security incidents and risky changes", Capabilities: []string{"security", "alerting", "triage"}},
		{Type: "security-block", Description: "Prevents unsafe operations or changes from progressing", Capabilities: []string{"security", "policy", "blocking"}},
		{Type: "security-architect", Description: "Focuses on security design and review", Capabilities: []string{"security", "review", "hardening"}},
		{Type: "perf-analyzer", Description: "Finds performance bottlenecks", Capabilities: []string{"performance", "profiling", "optimization"}},
		{Type: "task-orchestrator", Description: "Splits work into executable tasks", Capabilities: []string{"planning", "tasking", "coordination"}},
		{Type: "memory-coordinator", Description: "Manages shared memory and context", Capabilities: []string{"memory", "context", "retrieval"}},
		{Type: "smart-agent", Description: "General adaptive specialist", Capabilities: []string{"adaptation", "routing", "execution"}},
		{Type: "pr-manager", Description: "Handles review and merge workflow", Capabilities: []string{"pr", "review", "release"}},
		{Type: "issue-tracker", Description: "Tracks work items and defects", Capabilities: []string{"tracking", "triage", "prioritization"}},
		{Type: "release-manager", Description: "Coordinates releases", Capabilities: []string{"release", "coordination", "checklists"}},
		{Type: "workflow-automation", Description: "Automates workflow glue", Capabilities: []string{"automation", "integration", "workflow"}},
		{Type: "repo-architect", Description: "Shapes repo structure and boundaries", Capabilities: []string{"repo-design", "architecture", "cleanup"}},
		{Type: "multi-repo-swarm", Description: "Coordinates across repositories", Capabilities: []string{"multi-repo", "coordination", "planning"}},
		{Type: "system-architect", Description: "Designs broader platform systems", Capabilities: []string{"systems", "architecture", "integration"}},
		{Type: "production-validator", Description: "Validates release readiness", Capabilities: []string{"validation", "readiness", "ops"}},
		{Type: "tdd-london-swarm", Description: "Executes TDD-style workflows", Capabilities: []string{"tdd", "testing", "design"}},
		{Type: "code-review-swarm", Description: "Performs coordinated code reviews", Capabilities: []string{"review", "multi-review", "risk"}},
		{Type: "performance-benchmarker", Description: "Builds benchmarks and measures systems", Capabilities: []string{"benchmarking", "performance", "analysis"}},
		{Type: "neural-coordinator", Description: "Routes tasks across specialists", Capabilities: []string{"routing", "coordination", "specialization"}},
		{Type: "swarm-memory-manager", Description: "Maintains swarm state memory", Capabilities: []string{"memory", "state", "context"}},
		{Type: "mesh-coordinator", Description: "Coordinates mesh topology swarms", Capabilities: []string{"mesh", "coordination", "consensus"}},
		{Type: "hierarchical-coordinator", Description: "Coordinates hierarchical swarms", Capabilities: []string{"hierarchical", "coordination", "delegation"}},
		{Type: "adaptive-coordinator", Description: "Adjusts plans dynamically", Capabilities: []string{"adaptation", "coordination", "replanning"}},
		{Type: "collective-intelligence-coordinator", Description: "Aggregates outputs from many agents", Capabilities: []string{"aggregation", "consensus", "coordination"}},
		{Type: "github-modes", Description: "Handles GitHub-related workflows", Capabilities: []string{"github", "pr", "issue"}},
		{Type: "project-board-sync", Description: "Syncs work tracking state", Capabilities: []string{"tracking", "sync", "project-management"}},
		{Type: "tech-writer", Description: "Writes detailed technical docs", Capabilities: []string{"documentation", "guides", "reference"}},
		{Type: "api-designer", Description: "Designs APIs and schemas", Capabilities: []string{"api", "schema", "contracts"}},
		{Type: "frontend-architect", Description: "Designs frontend systems", Capabilities: []string{"frontend", "ux", "architecture"}},
		{Type: "backend-architect", Description: "Designs backend systems", Capabilities: []string{"backend", "services", "architecture"}},
		{Type: "database-architect", Description: "Designs persistence systems", Capabilities: []string{"database", "schema", "queries"}},
		{Type: "devops-engineer", Description: "Handles infra and automation", Capabilities: []string{"devops", "ci", "deployment"}},
		{Type: "sre", Description: "Focuses on reliability and operations", Capabilities: []string{"reliability", "operations", "incident-response"}},
		{Type: "qa-engineer", Description: "Focuses on quality assurance", Capabilities: []string{"qa", "testing", "regression"}},
		{Type: "integration-engineer", Description: "Connects systems together", Capabilities: []string{"integration", "interfaces", "glue"}},
		{Type: "migration-specialist", Description: "Ports systems between stacks", Capabilities: []string{"migration", "porting", "cleanup"}},
		{Type: "prompt-engineer", Description: "Shapes prompts and role behavior", Capabilities: []string{"prompting", "roles", "optimization"}},
		{Type: "compliance-auditor", Description: "Checks policy and compliance constraints", Capabilities: []string{"compliance", "audit", "policy"}},
		{Type: "observability-engineer", Description: "Improves metrics, logs, and tracing", Capabilities: []string{"metrics", "logs", "tracing"}},
		{Type: "toolsmith", Description: "Builds and integrates tools", Capabilities: []string{"tools", "integration", "automation"}},
	}
}

func DefaultSwarmAgentTypes() []string {
	return []string{"queen-coordinator", "architect", "coder", "tester", "reviewer", "researcher", "debugger", "documenter"}
}

func AgentTypesForTopology(topology string) []string {
	switch strings.TrimSpace(strings.ToLower(topology)) {
	case "mesh":
		return []string{"mesh-coordinator", "coder", "coder", "tester", "reviewer", "researcher", "designer"}
	case "hierarchical-mesh":
		return []string{"queen-coordinator", "hierarchical-coordinator", "mesh-coordinator", "architect", "coder", "coder", "tester", "reviewer", "researcher", "memory-coordinator"}
	case "ring":
		return []string{"coordinator", "researcher", "architect", "coder", "tester", "reviewer", "documenter"}
	case "star":
		return []string{"queen-coordinator", "architect", "coder", "tester", "reviewer", "deployer"}
	case "adaptive":
		return []string{"adaptive-coordinator", "architect", "coder", "tester", "reviewer", "researcher", "debugger", "documenter", "designer"}
	case "hybrid":
		return []string{"queen-coordinator", "mesh-coordinator", "architect", "coder", "tester", "reviewer", "researcher", "deployer"}
	case "security":
		return []string{"queen-coordinator", "security-architect", "security-audit", "security-alert", "security-block", "reviewer", "tester"}
	case "delivery":
		return []string{"queen-coordinator", "coder", "tester", "reviewer", "deployer", "release-manager", "devops-engineer"}
	default:
		return DefaultSwarmAgentTypes()
	}
}

func DefaultSwarmTopology() string {
	return "hierarchical"
}

func DefaultSwarmStrategy() string {
	return "specialized"
}

func ResolveAgentTemplate(agentType string) AgentTemplate {
	agentType = strings.TrimSpace(strings.ToLower(agentType))
	for _, template := range DefaultAgentTemplates() {
		if template.Type == agentType {
			return template
		}
	}
	return AgentTemplate{
		Type:         agentType,
		Description:  "Custom swarm agent",
		Capabilities: []string{"execution"},
	}
}

func NewAgent(agentID, agentType, name, swarmID string) Agent {
	template := ResolveAgentTemplate(agentType)
	if strings.TrimSpace(name) == "" {
		name = template.Type
	}
	now := time.Now()
	return Agent{
		ID:           agentID,
		Type:         template.Type,
		Name:         name,
		Description:  template.Description,
		Capabilities: append([]string(nil), template.Capabilities...),
		Status:       AgentStatusIdle,
		SwarmID:      strings.TrimSpace(swarmID),
		CreatedAt:    now,
		LastSeenAt:   now,
	}
}
