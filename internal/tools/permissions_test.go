package tools

import "testing"

func TestPermissionReadOnlyAlwaysAllowed(t *testing.T) {
	pc := NewPermissionChecker(ModeReadOnly)
	tool := &FileReadTool{}
	result := pc.Check(tool, map[string]interface{}{"path": "anything"})
	if !result.Allowed {
		t.Error("read-only tools should always be allowed")
	}
}

func TestPermissionReadOnlyBlocksWrite(t *testing.T) {
	pc := NewPermissionChecker(ModeReadOnly)
	tool := &FileWriteTool{}
	result := pc.Check(tool, map[string]interface{}{"path": "file.go"})
	if result.Allowed {
		t.Error("write tools should be blocked in read-only mode")
	}
}

func TestPermissionAutoApproveAllowsWrite(t *testing.T) {
	pc := NewPermissionChecker(ModeAutoApprove)
	tool := &FileWriteTool{}
	result := pc.Check(tool, map[string]interface{}{"path": "file.go"})
	if !result.Allowed {
		t.Error("write tools should be allowed in auto-approve mode")
	}
}

func TestPermissionAutoApproveDenyPattern(t *testing.T) {
	pc := NewPermissionChecker(ModeAutoApprove)
	pc.DenyPatterns = []string{".env"}
	tool := &FileWriteTool{}
	result := pc.Check(tool, map[string]interface{}{"path": ".env"})
	if result.Allowed {
		t.Error("denied patterns should block even in auto-approve")
	}
}

func TestPermissionInteractivePromptsForWrite(t *testing.T) {
	pc := NewPermissionChecker(ModeInteractive)
	tool := &BashTool{}
	result := pc.Check(tool, map[string]interface{}{"command": "rm file"})
	if result.Allowed {
		t.Error("write tools should need approval in interactive mode")
	}
	if !result.Prompt {
		t.Error("should prompt user in interactive mode")
	}
}

func TestPermissionInteractiveAutoApprovePattern(t *testing.T) {
	pc := NewPermissionChecker(ModeInteractive)
	pc.AutoApproveGlob = []string{"*.go"}
	tool := &FileWriteTool{}
	result := pc.Check(tool, map[string]interface{}{"path": "main.go"})
	if !result.Allowed {
		t.Error("auto-approve pattern should allow matching files")
	}
}

func TestPermissionInteractiveAutoApproveNoMatch(t *testing.T) {
	pc := NewPermissionChecker(ModeInteractive)
	pc.AutoApproveGlob = []string{"*.go"}
	tool := &FileWriteTool{}
	result := pc.Check(tool, map[string]interface{}{"path": "secret.env"})
	if result.Allowed {
		t.Error("non-matching files should not be auto-approved")
	}
}
