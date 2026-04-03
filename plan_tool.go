package main

import (
	"fmt"
	"os"
	"time"
)

// Plan represents a development plan with acceptance criteria
type Plan struct {
	Task          string
	CreatedAt     time.Time
	Steps         []string
	Acceptance    []string
	Assumptions   []string
	Dependencies  []string
}

// CreatePlan creates a new development plan
func CreatePlan(task string, steps, acceptance, assumptions, dependencies []string) *Plan {
	return &Plan{
		Task:         task,
		CreatedAt:    time.Now(),
		Steps:        steps,
		Acceptance:   acceptance,
		Assumptions:  assumptions,
		Dependencies: dependencies,
	}
}

// Display prints the plan to stdout
func (p *Plan) Display() {
	fmt.Printf("\n=== DEVELOPMENT PLAN ===\n")
	fmt.Printf("Task: %s\n", p.Task)
	fmt.Printf("Created: %s\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
	
	fmt.Printf("\nSteps:\n")
	for i, step := range p.Steps {
		fmt.Printf("%d. %s\n", i+1, step)
	}
	
	fmt.Printf("\nAcceptance Criteria:\n")
	for i, crit := range p.Acceptance {
		fmt.Printf("%d. %s\n", i+1, crit)
	}
	
	if len(p.Assumptions) > 0 {
		fmt.Printf("\nAssumptions:\n")
		for i, assumption := range p.Assumptions {
			fmt.Printf("%d. %s\n", i+1, assumption)
		}
	}
	
	if len(p.Dependencies) > 0 {
		fmt.Printf("\nDependencies:\n")
		for i, dep := range p.Dependencies {
			fmt.Printf("%d. %s\n", i+1, dep)
		}
	}
	fmt.Printf("\n")
}

// Save saves the plan to a file
func (p *Plan) Save(filename string) error {
	content := fmt.Sprintf("# Plan: %s\n", p.Task)
	content += fmt.Sprintf("Created: %s\n\n", p.CreatedAt.Format("2006-01-02 15:04:05"))
	
	content += "## Steps\n"
	for i, step := range p.Steps {
		content += fmt.Sprintf("%d. %s\n", i+1, step)
	}
	
	content += "\n## Acceptance Criteria\n"
	for i, crit := range p.Acceptance {
		content += fmt.Sprintf("%d. %s\n", i+1, crit)
	}
	
	if len(p.Assumptions) > 0 {
		content += "\n## Assumptions\n"
		for i, assumption := range p.Assumptions {
			content += fmt.Sprintf("%d. %s\n", i+1, assumption)
		}
	}
	
	if len(p.Dependencies) > 0 {
		content += "\n## Dependencies\n"
		for i, dep := range p.Dependencies {
			content += fmt.Sprintf("%d. %s\n", i+1, dep)
		}
	}
	
	return os.WriteFile(filename, []byte(content), 0644)
}