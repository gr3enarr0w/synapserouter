package agent

import (
	"strings"
	"testing"

	"github.com/gr3enarr0w/synapserouter/internal/tools"
)

func TestGenerateToolBlock(t *testing.T) {
	registry := tools.DefaultRegistry()
	block := generateToolBlock(registry)
	
	// Verify the block contains all registered tools
	registeredTools := registry.List()
	for _, toolName := range registeredTools {
		if !strings.Contains(block, toolName) {
			t.Errorf("generateToolBlock() missing tool: %s", toolName)
		}
	}
	
	// Verify it contains expected sections
	if !strings.Contains(block, "AVAILABLE TOOLS") {
		t.Error("generateToolBlock() missing 'AVAILABLE TOOLS' header")
	}
	if !strings.Contains(block, "Args:") {
		t.Error("generateToolBlock() missing 'Args:' for tool parameter docs")
	}
}
