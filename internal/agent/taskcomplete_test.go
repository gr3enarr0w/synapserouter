package agent

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/gr3enarr0w/synapserouter/internal/providers"
	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestTaskCompleteSummary(t *testing.T) {
	exec := &mockExecutor{
		responses: []providers.ChatResponse{
			{
				Choices: []providers.Choice{{
					Message: providers.Message{
						Role:    "assistant",
						Content: "",
						ToolCalls: []map[string]interface{}{{
							"id":       "call1",
							"function": map[string]interface{}{"name": "file_write", "arguments": `{"path": "test.txt", "content": "hello"}`},
						}},
					},
				}},
			},
			{
				Choices: []providers.Choice{{
					Message: providers.Message{
						Role:    "assistant",
						Content: "Task complete",
					},
				}},
			},
		},
	}

	registry := tools.NewRegistry()
	registry.Register(&tools.FileWriteTool{})
	registry.Register(&tools.FileReadTool{})
	
	// Use EventBus to capture events
	bus := NewEventBus()
	var emittedEvents []AgentEvent
	sub := bus.Subscribe()
	done := make(chan struct{})
	go func() {
		for e := range sub {
			emittedEvents = append(emittedEvents, e)
		}
		close(done)
	}()
	
	renderer := NewRenderer(&bytes.Buffer{})
	
	config := DefaultConfig()
	config.WorkDir = t.TempDir()
	config.EventBus = bus

	agent := New(exec, registry, renderer, config)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = agent.Run(ctx, "Write hello to test.txt")

	// Close subscriber and wait
	bus.Close()
	<-done

	// Find EventTaskComplete
	var taskCompleteFound bool
	for _, e := range emittedEvents {
		if e.Type == EventTaskComplete {
			taskCompleteFound = true
			if _, ok := e.Data["files_created"]; !ok {
				t.Error("EventTaskComplete missing files_created key")
			}
			if _, ok := e.Data["commands_total"]; !ok {
				t.Error("EventTaskComplete missing commands_total key")
			}
			break
		}
	}

	if !taskCompleteFound {
		t.Error("EventTaskComplete was not emitted")
	}
}
