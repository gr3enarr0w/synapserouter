package orchestration

import (
	"fmt"
	"sort"
	"sync"
)

// DAGScheduler manages parallel task execution with dependencies
type DAGScheduler struct {
	mu sync.RWMutex
}

// TaskNode represents a task in the dependency graph
type TaskNode struct {
	TaskID    string
	DependsOn []string
	Blocks    []string
	Priority  int
	Status    TaskStatus
}

// NewDAGScheduler creates a new dependency graph scheduler
func NewDAGScheduler() *DAGScheduler {
	return &DAGScheduler{}
}

// CanExecute checks if a task can execute (all dependencies completed)
func (d *DAGScheduler) CanExecute(task *Task, allTasks map[string]*Task) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(task.DependsOn) == 0 {
		return true
	}

	for _, depID := range task.DependsOn {
		depTask, exists := allTasks[depID]
		if !exists {
			// Dependency doesn't exist - can't execute
			return false
		}

		if depTask.Status != TaskStatusCompleted {
			// Dependency not completed yet
			return false
		}
	}

	return true
}

// GetExecutableTasks returns the next tasks that can execute
func (d *DAGScheduler) GetExecutableTasks(allTasks map[string]*Task) []*Task {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var executable []*Task

	for _, task := range allTasks {
		// Only consider pending tasks
		if task.Status != TaskStatusPending && task.Status != TaskStatusQueued {
			continue
		}

		// Check if all dependencies are met
		if d.CanExecute(task, allTasks) {
			executable = append(executable, task)
		}
	}

	// Sort by priority (higher first)
	sort.Slice(executable, func(i, j int) bool {
		if executable[i].Priority == executable[j].Priority {
			// Same priority - use creation time (older first)
			return executable[i].CreatedAt.Before(executable[j].CreatedAt)
		}
		return executable[i].Priority > executable[j].Priority
	})

	return executable
}

// DetectCycles detects circular dependencies in task graph
func (d *DAGScheduler) DetectCycles(tasks map[string]*Task) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	for taskID := range tasks {
		if !visited[taskID] {
			if d.detectCyclesUtil(taskID, tasks, visited, recStack) {
				return fmt.Errorf("circular dependency detected involving task %s", taskID)
			}
		}
	}

	return nil
}

func (d *DAGScheduler) detectCyclesUtil(taskID string, tasks map[string]*Task, visited, recStack map[string]bool) bool {
	visited[taskID] = true
	recStack[taskID] = true

	task, exists := tasks[taskID]
	if !exists {
		return false
	}

	for _, depID := range task.DependsOn {
		if !visited[depID] {
			if d.detectCyclesUtil(depID, tasks, visited, recStack) {
				return true
			}
		} else if recStack[depID] {
			return true
		}
	}

	recStack[taskID] = false
	return false
}

// GetDependencyChain returns all tasks in dependency chain for a given task
func (d *DAGScheduler) GetDependencyChain(taskID string, tasks map[string]*Task) []string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	visited := make(map[string]bool)
	chain := []string{}

	d.getDependencyChainUtil(taskID, tasks, visited, &chain)

	return chain
}

func (d *DAGScheduler) getDependencyChainUtil(taskID string, tasks map[string]*Task, visited map[string]bool, chain *[]string) {
	if visited[taskID] {
		return
	}

	visited[taskID] = true
	task, exists := tasks[taskID]
	if !exists {
		return
	}

	// Visit dependencies first (topological order)
	for _, depID := range task.DependsOn {
		d.getDependencyChainUtil(depID, tasks, visited, chain)
	}

	*chain = append(*chain, taskID)
}

// ValidateDependencies checks if dependencies are valid
func (d *DAGScheduler) ValidateDependencies(task *Task, allTasks map[string]*Task) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Check if all dependencies exist
	for _, depID := range task.DependsOn {
		if _, exists := allTasks[depID]; !exists {
			return fmt.Errorf("dependency task %s does not exist", depID)
		}
	}

	// Check for self-dependency
	for _, depID := range task.DependsOn {
		if depID == task.ID {
			return fmt.Errorf("task cannot depend on itself")
		}
	}

	return nil
}

// GetBlockedTasks returns tasks that are blocked by a given task
func (d *DAGScheduler) GetBlockedTasks(taskID string, allTasks map[string]*Task) []*Task {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var blocked []*Task

	for _, task := range allTasks {
		for _, depID := range task.DependsOn {
			if depID == taskID {
				blocked = append(blocked, task)
				break
			}
		}
	}

	return blocked
}

// GetParallelGroups returns groups of tasks that can run in parallel
func (d *DAGScheduler) GetParallelGroups(tasks map[string]*Task) [][]*Task {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Get topological ordering
	ordered := d.topologicalSort(tasks)
	if ordered == nil {
		return nil
	}

	// Group tasks by level (tasks with no dependencies between them)
	groups := [][]*Task{}
	processed := make(map[string]bool)

	for len(processed) < len(tasks) {
		group := []*Task{}

		for _, taskID := range ordered {
			if processed[taskID] {
				continue
			}

			task := tasks[taskID]

			// Check if all dependencies are processed
			allDepsProcessed := true
			for _, depID := range task.DependsOn {
				if !processed[depID] {
					allDepsProcessed = false
					break
				}
			}

			if allDepsProcessed {
				group = append(group, task)
				processed[taskID] = true
			}
		}

		if len(group) > 0 {
			groups = append(groups, group)
		} else {
			// No progress - break to avoid infinite loop
			break
		}
	}

	return groups
}

func (d *DAGScheduler) topologicalSort(tasks map[string]*Task) []string {
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)

	// Initialize
	for taskID := range tasks {
		inDegree[taskID] = 0
		adjList[taskID] = []string{}
	}

	// Build graph
	for taskID, task := range tasks {
		for _, depID := range task.DependsOn {
			adjList[depID] = append(adjList[depID], taskID)
			inDegree[taskID]++
		}
	}

	// Find nodes with no dependencies
	queue := []string{}
	for taskID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, taskID)
		}
	}

	result := []string{}
	for len(queue) > 0 {
		taskID := queue[0]
		queue = queue[1:]
		result = append(result, taskID)

		// Reduce in-degree for neighbors
		for _, neighbor := range adjList[taskID] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Check if all tasks were processed (no cycles)
	if len(result) != len(tasks) {
		return nil // Cycle detected
	}

	return result
}

// ExecutionPlan represents a detailed execution plan for a set of tasks
type ExecutionPlan struct {
	Groups      [][]*Task                `json:"groups"`
	TotalStages int                      `json:"total_stages"`
	Parallelism map[int]int              `json:"parallelism"` // Stage -> task count
	Critical    []string                 `json:"critical_path"`
	Dependencies map[string][]string     `json:"dependencies"`
}

func (d *DAGScheduler) GetExecutionPlan(tasks map[string]*Task) (*ExecutionPlan, error) {
	// Detect cycles first
	if err := d.DetectCycles(tasks); err != nil {
		return nil, err
	}

	groups := d.GetParallelGroups(tasks)

	plan := &ExecutionPlan{
		Groups:      groups,
		TotalStages: len(groups),
		Parallelism: make(map[int]int),
		Dependencies: make(map[string][]string),
	}

	// Calculate parallelism per stage
	for i, group := range groups {
		plan.Parallelism[i] = len(group)
	}

	// Find critical path (longest dependency chain)
	maxDepth := 0
	var criticalTask string
	for taskID := range tasks {
		chain := d.GetDependencyChain(taskID, tasks)
		if len(chain) > maxDepth {
			maxDepth = len(chain)
			criticalTask = taskID
		}
	}

	if criticalTask != "" {
		plan.Critical = d.GetDependencyChain(criticalTask, tasks)
	}

	// Record dependencies
	for taskID, task := range tasks {
		if len(task.DependsOn) > 0 {
			plan.Dependencies[taskID] = task.DependsOn
		}
	}

	return plan, nil
}
