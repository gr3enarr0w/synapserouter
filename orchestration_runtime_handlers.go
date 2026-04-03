package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"


	"github.com/gr3enarr0w/synapserouter/internal/orchestration"
)

func orchestrationAgentsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agents := orchestrator.ListAgentsFiltered(strings.TrimSpace(r.URL.Query().Get("filter")))
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": len(agents),
			"data":  agents,
		})
	case http.MethodPost:
		var req orchestration.AgentSpawnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		agent, err := orchestrator.SpawnAgent(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(agent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func orchestrationAgentHandler(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	agent, err := orchestrator.GetAgent(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_metrics")), "true") {
		metrics, err := orchestrator.AgentMetrics(agentID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"agent":   agent,
			"metrics": metrics,
			"health":  orchestrator.AgentHealth(),
		})
		return
	}
	json.NewEncoder(w).Encode(agent)
}

func orchestrationAgentStatusHandler(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	agent, err := orchestrator.GetAgent(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	metrics, err := orchestrator.AgentMetrics(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"agent":   agent,
		"metrics": metrics,
		"health":  orchestrator.AgentHealth(),
	})
}

func orchestrationAgentStopHandler(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	agent, err := orchestrator.StopAgent(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agent)
}

func orchestrationAgentMetricsHandler(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	metrics, err := orchestrator.AgentMetrics(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func orchestrationAgentHealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(orchestrator.AgentHealth())
}

func orchestrationAgentLogsHandler(w http.ResponseWriter, r *http.Request) {
	agentID := r.PathValue("agent_id")
	logs, err := orchestrator.AgentLogs(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(logs),
		"data":  logs,
	})
}

func orchestrationSwarmsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		swarms := orchestrator.ListSwarms()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": len(swarms),
			"data":  swarms,
		})
	case http.MethodPost:
		var req orchestration.SwarmRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		swarm, err := orchestrator.InitSwarm(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(swarm)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func orchestrationSwarmHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	w.Header().Set("Content-Type", "application/json")
	if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_metrics")), "true") ||
		strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("include_agents")), "true") {
		status, err := orchestrator.SwarmStatus(swarmID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(status)
		return
	}
	swarm, err := orchestrator.GetSwarm(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(swarm)
}

func orchestrationSwarmStatusHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	status, err := orchestrator.SwarmStatus(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

func orchestrationSwarmStartHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	sessionID := strings.TrimSpace(r.Header.Get("X-Session-ID"))
	task, err := orchestrator.StartSwarm(r.Context(), swarmID, sessionID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(task)
}

func orchestrationSwarmStopHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	swarm, err := orchestrator.StopSwarm(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(swarm)
}

func orchestrationSwarmPauseHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	swarm, err := orchestrator.PauseSwarm(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(swarm)
}

func orchestrationSwarmResumeHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	swarm, err := orchestrator.ResumeSwarm(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(swarm)
}

func orchestrationSwarmScaleHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	var req orchestration.SwarmScaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	swarm, err := orchestrator.ScaleSwarm(r.Context(), swarmID, req.Count)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(swarm)
}

func orchestrationSwarmCoordinateHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	var req orchestration.SwarmCoordinateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	swarm, err := orchestrator.CoordinateSwarm(r.Context(), swarmID, req.Agents)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(swarm)
}

func orchestrationSwarmLoadHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	load, err := orchestrator.SwarmLoad(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(load)
}

func orchestrationSwarmImbalanceHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	report, err := orchestrator.DetectImbalance(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(report)
}

func orchestrationSwarmRebalancePreviewHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	preview, err := orchestrator.PreviewRebalance(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(preview)
}

func orchestrationSwarmStealableTasksHandler(w http.ResponseWriter, r *http.Request) {
	swarmID := r.PathValue("swarm_id")
	stealable, err := orchestrator.ListStealableTasks(swarmID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(stealable),
		"data":  stealable,
	})
}

func orchestrationWorkflowsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		templates := orchestrator.ListWorkflowTemplates()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"count": len(templates),
			"data":  templates,
		})
	case http.MethodPost:
		var template orchestration.WorkflowTemplate
		if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		saved, err := orchestrator.SaveWorkflowTemplate(template)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(saved)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func orchestrationWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	templateID := r.PathValue("template_id")
	switch r.Method {
	case http.MethodGet:
		template, ok := orchestrator.GetWorkflowTemplate(templateID)
		if !ok {
			http.Error(w, "workflow template not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(template)
	case http.MethodPut:
		var template orchestration.WorkflowTemplate
		if err := json.NewDecoder(r.Body).Decode(&template); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		template.ID = templateID
		saved, err := orchestrator.SaveWorkflowTemplate(template)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(saved)
	case http.MethodDelete:
		if err := orchestrator.DeleteWorkflowTemplate(templateID); err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func orchestrationWorkflowRunHandler(w http.ResponseWriter, r *http.Request) {
	templateID := r.PathValue("template_id")
	var req orchestration.WorkflowRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	swarm, task, err := orchestrator.RunWorkflowTemplate(r.Context(), templateID, req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	response := map[string]interface{}{"swarm": swarm}
	if task != nil {
		response["task"] = task
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)
}

func orchestrationExecutionStateHandler(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("workflow_id")
	state, err := orchestrator.WorkflowState(workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(state)
}

func orchestrationExecutionMetricsHandler(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("workflow_id")
	metrics, err := orchestrator.WorkflowMetrics(workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

func orchestrationExecutionDebugHandler(w http.ResponseWriter, r *http.Request) {
	workflowID := r.PathValue("workflow_id")
	debugInfo, err := orchestrator.WorkflowDebugInfo(workflowID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(debugInfo)
}

func orchestrationTaskEventsHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	startRequested := strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("start")), "true")

	stream, cancel, err := orchestrator.SubscribeTask(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer cancel()

	if startRequested {
		if err := orchestrator.StartTask(taskID); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-stream:
			if !ok {
				fmt.Fprint(w, "data: [DONE]\n\n")
				flusher.Flush()
				return
			}
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}

func orchestrationTaskAssignHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	var req orchestration.TaskAssignRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	task, err := orchestrator.AssignTask(taskID, strings.TrimSpace(req.AgentID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

func orchestrationTaskStealHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	var req orchestration.TaskStealRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
	}
	result, err := orchestrator.StealTask(r.Context(), taskID, strings.TrimSpace(req.SwarmID), strings.TrimSpace(req.StealerID))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func orchestrationTaskContestHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	var req orchestration.TaskContestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	contest, err := orchestrator.ContestTaskSteal(taskID, strings.TrimSpace(req.OriginalAgentID), strings.TrimSpace(req.Reason))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contest)
}

func orchestrationTaskContestResolveHandler(w http.ResponseWriter, r *http.Request) {
	taskID := r.PathValue("task_id")
	var req orchestration.TaskContestResolutionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	contest, err := orchestrator.ResolveTaskStealContest(taskID, strings.TrimSpace(req.WinnerAgentID), strings.TrimSpace(req.Reason))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(contest)
}
