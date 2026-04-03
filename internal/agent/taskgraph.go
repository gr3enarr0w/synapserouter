package agent

import (
	"regexp"
	"strings"
)

// TaskNode represents a single unit of work in a task graph.
type TaskNode struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	Dependencies []string `json:"dependencies"`
	Complexity   string   `json:"complexity"`
	Status       string   `json:"status"` // "pending", "in_progress", "completed"
}

// TaskGraph represents a directed acyclic graph of tasks.
type TaskGraph struct {
	Nodes []TaskNode `json:"nodes"`
}

// AddNode adds a task node to the graph.
func (g *TaskGraph) AddNode(node TaskNode) {
	g.Nodes = append(g.Nodes, node)
}

// GetExecutable returns nodes that have all dependencies completed.
// A node is executable if:
// - It has no dependencies, OR
// - All its dependencies have status "completed"
func (g *TaskGraph) GetExecutable() []TaskNode {
	var executable []TaskNode
	for _, node := range g.Nodes {
		if node.Status != "pending" {
			continue
		}
		if len(node.Dependencies) == 0 {
			executable = append(executable, node)
			continue
		}
		allDepsComplete := true
		for _, depID := range node.Dependencies {
			dep := g.getNodeByID(depID)
			if dep == nil || dep.Status != "completed" {
				allDepsComplete = false
				break
			}
		}
		if allDepsComplete {
			executable = append(executable, node)
		}
	}
	return executable
}

// getNodeByID finds a node by its ID.
func (g *TaskGraph) getNodeByID(id string) *TaskNode {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// MarkCompleted marks a node as completed.
func (g *TaskGraph) MarkCompleted(id string) {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			g.Nodes[i].Status = "completed"
			return
		}
	}
}

// CountIndependent returns the number of nodes with no dependencies.
func (g *TaskGraph) CountIndependent() int {
	count := 0
	for _, node := range g.Nodes {
		if len(node.Dependencies) == 0 {
			count++
		}
	}
	return count
}

// ParseTaskGraph parses LLM plan output into a TaskGraph.
// Looks for numbered items (e.g., "1.", "2.") and dependency markers (e.g., "depends: 1, 2").
// This is a simple parser - keeps it lightweight.
func ParseTaskGraph(planOutput string) *TaskGraph {
	graph := &TaskGraph{}

	// Regex to match numbered items: "1.", "2.", etc.
	itemRegex := regexp.MustCompile(`(?m)^(\d+)\.\s+(.+)`)
	// Regex to match dependency markers: "depends: 1, 2" or "deps: 1"
	depRegex := regexp.MustCompile(`(?i)(?:depends?:?\s*)([\d,\s]+)`)

	matches := itemRegex.FindAllStringSubmatch(planOutput, -1)
	for _, match := range matches {
		itemNum := match[1]
		description := strings.TrimSpace(match[2])

		// Extract dependencies from the description or nearby lines
		var deps []string
		depMatch := depRegex.FindStringSubmatch(description)
		if depMatch != nil {
			depStrs := strings.Split(depMatch[1], ",")
			for _, d := range depStrs {
				d = strings.TrimSpace(d)
				if d != "" {
					deps = append(deps, d)
				}
			}
			// Remove dependency marker from description
			description = depRegex.ReplaceAllString(description, "")
			description = strings.TrimSpace(description)
		}

		// Detect complexity keywords
		complexity := "medium"
		lowerDesc := strings.ToLower(description)
		if strings.Contains(lowerDesc, "simple") || strings.Contains(lowerDesc, "easy") {
			complexity = "low"
		} else if strings.Contains(lowerDesc, "complex") || strings.Contains(lowerDesc, "hard") {
			complexity = "high"
		}

		node := TaskNode{
			ID:           itemNum,
			Description:  description,
			Dependencies: deps,
			Complexity:   complexity,
			Status:       "pending",
		}
		graph.AddNode(node)
	}

	return graph
}

// HasMultipleIndependentNodes returns true if the graph has more than one
// node with no dependencies (can be executed in parallel).
func (g *TaskGraph) HasMultipleIndependentNodes() bool {
	return g.CountIndependent() > 1
}

// ToDelegateTasks converts executable nodes to DelegateTask format.
func (g *TaskGraph) ToDelegateTasks(role string) []DelegateTask {
	executable := g.GetExecutable()
	tasks := make([]DelegateTask, len(executable))
	for i, node := range executable {
		tasks[i] = DelegateTask{
			Config: SpawnConfig{
				Role: role,
			},
			Task: node.Description,
		}
	}
	return tasks
}
