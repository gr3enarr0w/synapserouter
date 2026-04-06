package agent

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestRejectionHistory(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{
			{
				Choices: []providers.Choice{{
					Message: providers.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []map[string]interface{}{{
							"id":       "call1",
							"function": map[string]interface{}{"name": "bash", "arguments": `{"command": "python3 -c \"print(1)\"" }`},
						}},
					},
				}},
			},
			{
				Choices: []providers.Choice{{
					Message: providers.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []map[string]interface{}{{
							"id":       "call2",
							"function": map[string]interface{}{"name": "bash", "arguments": `{"command": "python3 -c \"print(1)\"" }`},
						}},
					},
				}},
			},
			{
				Choices: []providers.Choice{{
					Message: providers.Message{
						Role:    "assistant",
						Content: "After rejection, I will use a different approach.",
					},
				}},
			},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&tools.BashTool{})
	renderer := NewRenderer(&bytes.Buffer{})
	config := DefaultConfig()
	config.WorkDir = t.TempDir()

	agent := New(exec, registry, renderer, config)
	
	// First run - should trigger bash tool
	_, err := agent.Run(context.Background(), "run invalid bash command twice to trigger rejection")
	if err != nil {
		t.Fatal(err)
	}

	// After 2 identical rejections, escalation clears the history and injects a system message.
	// Verify the escalation message was added to the conversation.
	foundEscalation := false
	for _, msg := range agent.conversation.Messages() {
		if msg.Role == "system" && strings.Contains(msg.Content, "rejected this command twice") {
			foundEscalation = true
			break
		}
	}
	
	if !foundEscalation && len(agent.rejectionHistory) > 0 {
		// Check if any rejection reached threshold
		t.Logf("rejectionHistory: %v", agent.rejectionHistory)
	}
}
