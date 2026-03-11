package orchestration

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const workflowTemplatesSettingKey = "orchestration.workflow_templates"

type WorkflowTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Topology    string   `json:"topology"`
	Strategy    string   `json:"strategy"`
	AgentTypes  []string `json:"agent_types,omitempty"`
	Roles       []string `json:"roles,omitempty"`
}

type WorkflowRunRequest struct {
	Objective string `json:"objective"`
	SessionID string `json:"session_id,omitempty"`
	Execute   bool   `json:"execute,omitempty"`
}

func WorkflowTemplates() []WorkflowTemplate {
	return []WorkflowTemplate{
		{
			ID:          "development",
			Name:        "Development",
			Description: "Hierarchical coding swarm with architecture, implementation, testing, and review.",
			Topology:    "hierarchical",
			Strategy:    "development",
			AgentTypes:  []string{"hierarchical-coordinator", "architect", "coder", "coder", "tester", "reviewer"},
			Roles:       []string{"architect", "coder", "tester", "reviewer"},
		},
		{
			ID:          "research",
			Name:        "Research",
			Description: "Research-heavy swarm for investigating options and summarizing recommendations.",
			Topology:    "mesh",
			Strategy:    "research",
			AgentTypes:  []string{"mesh-coordinator", "researcher", "researcher", "architect", "documenter"},
			Roles:       []string{"researcher", "architect", "documenter"},
		},
		{
			ID:          "security",
			Name:        "Security Audit",
			Description: "Security-focused workflow combining architecture review, implementation checks, and reporting.",
			Topology:    "hierarchical",
			Strategy:    "security",
			AgentTypes:  []string{"hierarchical-coordinator", "security-architect", "reviewer", "tester", "documenter"},
			Roles:       []string{"architect", "reviewer", "tester", "documenter"},
		},
		{
			ID:          "debugging",
			Name:        "Debugging",
			Description: "Focused bug triage and remediation loop with verification.",
			Topology:    "hierarchical",
			Strategy:    "debugging",
			AgentTypes:  []string{"hierarchical-coordinator", "debugger", "coder", "tester"},
			Roles:       []string{"debugger", "coder", "tester"},
		},
		{
			ID:          "sparc",
			Name:        "SPARC",
			Description: "Specification-to-review workflow with refinement and validation coverage.",
			Topology:    "hierarchical-mesh",
			Strategy:    "sparc",
			AgentTypes:  []string{"hierarchical-coordinator", "architect", "coder", "tester", "reviewer", "documenter"},
			Roles:       []string{"researcher", "architect", "coder", "tester", "reviewer", "documenter"},
		},
	}
}

func FindWorkflowTemplate(templateID string) (WorkflowTemplate, bool) {
	templateID = strings.TrimSpace(strings.ToLower(templateID))
	for _, template := range WorkflowTemplates() {
		if strings.EqualFold(template.ID, templateID) {
			return template, true
		}
	}
	return WorkflowTemplate{}, false
}

func (m *Manager) ListWorkflowTemplates() []WorkflowTemplate {
	templates := append([]WorkflowTemplate(nil), WorkflowTemplates()...)
	custom := m.loadCustomWorkflowTemplates()
	if len(custom) == 0 {
		return templates
	}

	index := make(map[string]int, len(templates))
	for i, template := range templates {
		index[strings.ToLower(template.ID)] = i
	}
	for _, template := range custom {
		key := strings.ToLower(strings.TrimSpace(template.ID))
		if idx, ok := index[key]; ok {
			templates[idx] = template
			continue
		}
		templates = append(templates, template)
	}
	return templates
}

func (m *Manager) GetWorkflowTemplate(templateID string) (WorkflowTemplate, bool) {
	templateID = strings.TrimSpace(strings.ToLower(templateID))
	for _, template := range m.ListWorkflowTemplates() {
		if strings.EqualFold(template.ID, templateID) {
			return template, true
		}
	}
	return WorkflowTemplate{}, false
}

func (m *Manager) SaveWorkflowTemplate(template WorkflowTemplate) (WorkflowTemplate, error) {
	template.ID = strings.TrimSpace(strings.ToLower(template.ID))
	template.Name = strings.TrimSpace(template.Name)
	template.Description = strings.TrimSpace(template.Description)
	template.Topology = strings.TrimSpace(template.Topology)
	template.Strategy = strings.TrimSpace(template.Strategy)
	if template.ID == "" {
		return WorkflowTemplate{}, fmt.Errorf("workflow template id is required")
	}
	if template.Name == "" {
		template.Name = template.ID
	}
	if template.Topology == "" {
		template.Topology = DefaultSwarmTopology()
	}
	if template.Strategy == "" {
		template.Strategy = DefaultSwarmStrategy()
	}
	if len(template.AgentTypes) == 0 {
		template.AgentTypes = AgentTypesForTopology(template.Topology)
	}

	custom := m.loadCustomWorkflowTemplates()
	updated := false
	for i, existing := range custom {
		if strings.EqualFold(existing.ID, template.ID) {
			custom[i] = template
			updated = true
			break
		}
	}
	if !updated {
		custom = append(custom, template)
	}
	if err := m.persistCustomWorkflowTemplates(custom); err != nil {
		return WorkflowTemplate{}, err
	}
	return template, nil
}

func (m *Manager) DeleteWorkflowTemplate(templateID string) error {
	templateID = strings.TrimSpace(strings.ToLower(templateID))
	if templateID == "" {
		return fmt.Errorf("workflow template id is required")
	}

	custom := m.loadCustomWorkflowTemplates()
	filtered := make([]WorkflowTemplate, 0, len(custom))
	removed := false
	for _, template := range custom {
		if strings.EqualFold(template.ID, templateID) {
			removed = true
			continue
		}
		filtered = append(filtered, template)
	}
	if !removed {
		return fmt.Errorf("workflow template not found: %s", templateID)
	}
	return m.persistCustomWorkflowTemplates(filtered)
}

func (m *Manager) loadCustomWorkflowTemplates() []WorkflowTemplate {
	if m == nil {
		return nil
	}
	if len(m.customWorkflowTemplates) > 0 {
		return append([]WorkflowTemplate(nil), m.customWorkflowTemplates...)
	}
	if m.db == nil {
		return nil
	}
	var raw string
	if err := m.db.QueryRow(`SELECT value FROM runtime_settings WHERE key = ?`, workflowTemplatesSettingKey).Scan(&raw); err != nil {
		return nil
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var templates []WorkflowTemplate
	if err := json.Unmarshal([]byte(raw), &templates); err != nil {
		return nil
	}
	m.customWorkflowTemplates = append([]WorkflowTemplate(nil), templates...)
	return templates
}

func (m *Manager) persistCustomWorkflowTemplates(templates []WorkflowTemplate) error {
	if m == nil {
		return nil
	}
	m.customWorkflowTemplates = append([]WorkflowTemplate(nil), templates...)
	if m.db == nil {
		return nil
	}
	raw, err := json.Marshal(templates)
	if err != nil {
		return err
	}
	_, err = m.db.Exec(`
		INSERT INTO runtime_settings (key, value, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = CURRENT_TIMESTAMP
	`, workflowTemplatesSettingKey, string(raw))
	return err
}

func (m *Manager) RunWorkflowTemplate(ctx context.Context, templateID string, req WorkflowRunRequest) (*Swarm, *Task, error) {
	template, ok := m.GetWorkflowTemplate(templateID)
	if !ok {
		return nil, nil, fmt.Errorf("workflow template not found: %s", templateID)
	}

	objective := strings.TrimSpace(req.Objective)
	if objective == "" {
		objective = template.Name
	}

	swarm, err := m.InitSwarm(ctx, SwarmRequest{
		Objective:  objective,
		Topology:   template.Topology,
		Strategy:   template.Strategy,
		MaxAgents:  len(template.AgentTypes),
		AgentTypes: append([]string(nil), template.AgentTypes...),
		Execute:    false,
		SessionID:  req.SessionID,
	})
	if err != nil {
		return nil, nil, err
	}

	if !req.Execute {
		return swarm, nil, nil
	}

	roles := append([]string(nil), template.Roles...)
	if len(roles) == 0 {
		for _, agentType := range template.AgentTypes {
			role := mapAgentTypeToRole(agentType)
			if role != "" {
				roles = append(roles, role)
			}
		}
	}
	roles = dedupe(roles)
	task, err := m.CreateTask(ctx, TaskRequest{
		Goal:      objective,
		SessionID: req.SessionID,
		Roles:     roles,
		Execute:   true,
		MaxSteps:  len(roles),
	})
	if err != nil {
		return swarm, nil, err
	}

	m.mu.Lock()
	now := time.Now()
	swarm.Status = SwarmStatusRunning
	swarm.StartedAt = &now
	swarm.TaskIDs = append(swarm.TaskIDs, task.ID)
	for _, agentID := range swarm.AgentIDs {
		agent, ok := m.agents[agentID]
		if !ok {
			continue
		}
		agent.Status = AgentStatusBusy
		agent.LastSeenAt = now
		m.persistAgent(agent)
	}
	if assignedAgentID := m.selectAgentForTaskLocked(swarm, task); assignedAgentID != "" {
		task.AssignedTo = assignedAgentID
		task.Status = TaskStatusAssigned
		if agent, ok := m.agents[assignedAgentID]; ok {
			agent.Status = AgentStatusBusy
			agent.LastSeenAt = time.Now()
			m.persistAgent(agent)
		}
		m.persistTask(task)
	}
	m.persistSwarm(swarm)
	m.mu.Unlock()

	return swarm, task, nil
}
