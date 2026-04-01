package agent

import "testing"

func TestIsCompletionSignal(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		// Positive cases — should detect completion
		{"task complete", "The task complete.", true},
		{"task is complete", "The task is complete and verified.", true},
		{"task has been completed", "The task has been completed successfully.", true},
		{"successfully completed", "I have successfully completed the implementation.", true},
		{"all done", "All done! The tests pass.", true},
		{"the fix is complete", "The fix is complete.", true},
		{"changes are complete", "All changes are complete.", true},
		{"implementation is complete", "The implementation is complete.", true},

		// Case insensitive
		{"uppercase", "TASK COMPLETE", true},
		{"mixed case", "Task Is Complete", true},

		// Embedded in longer text
		{"embedded", "After running tests, the task is complete and ready for review.", true},

		// Negative cases — should NOT detect completion
		{"greeting", "hello", false},
		{"partial match task", "I'll work on the task", false},
		{"partial match complete", "I need to complete this step first", false},
		{"question about completion", "Is the task nearly done?", false},
		{"empty", "", false},
		{"code snippet", "func complete() { return }", false},
		{"incomplete phrase", "the changes are not yet ready", false},
		{"similar but different", "this completes the setup", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCompletionSignal(tt.content)
			if got != tt.want {
				t.Errorf("isCompletionSignal(%q) = %v, want %v", tt.content, got, tt.want)
			}
		})
	}
}
