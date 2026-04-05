package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
)

// extractTextToolCalls parses tool calls embedded in LLM text output.
// Some models (especially open-source via Ollama) output tool calls as text
// markers instead of structured JSON in the API response. This parser handles
// common formats:
//
// Format 1 (triple-dash separated):
//
//	---
//	tool_call
//	---
//	file_read
//	---
//	{"path": "foo.py"}
//	---
//
// Format 2 (markdown code block):
//
//	```tool_call
//	file_read({"path": "foo.py"})
//	```
//
// Returns parsed tool calls and the remaining text content (with tool call
// markers removed). Returns nil if no text tool calls are found.
func extractTextToolCalls(content string) ([]map[string]interface{}, string) {
	var toolCalls []map[string]interface{}

	// Format 1: ---\ntool_call\n---\nTOOL_NAME\n---\n{ARGS_JSON}\n---
	remaining := parseDashFormat(content, &toolCalls)

	// Format 2: ```tool_call\nTOOL_NAME(ARGS_JSON)\n```
	remaining = parseCodeBlockFormat(remaining, &toolCalls)

	// Format 3: [tool_calls][{...}][/tool_calls] — XML-like tags with JSON array
	remaining = parseXMLTagFormat(remaining, &toolCalls)

	// Format 4: tool_call\n```json\n{"command":"ls"}\n``` — narrated tool call with JSON block
	remaining = parseNarratedJSONFormat(remaining, &toolCalls)

	// Format 5: tool_call_name=file_read\ntool_call_arguments={...} — key=value format
	remaining = parseKeyValueFormat(remaining, &toolCalls)

	if len(toolCalls) == 0 {
		return nil, content
	}

	log.Printf("[Agent] parsed %d text-based tool call(s) from LLM output", len(toolCalls))
	return toolCalls, strings.TrimSpace(remaining)
}

// parseDashFormat handles the ---\ntool_call\n---\nname\n---\n{args}\n--- format.
// The JSON args can be multi-line with nested braces, so we match greedily up to the closing ---.
var dashToolCallRe = regexp.MustCompile(`(?s)---\s*\ntool_call\s*\n---\s*\n(\w+)\s*\n---\s*\n(\{.+?\})\s*\n---`)

func parseDashFormat(content string, toolCalls *[]map[string]interface{}) string {
	matches := dashToolCallRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	for _, match := range matches {
		toolName := content[match[2]:match[3]]
		argsStr := content[match[4]:match[5]]

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			log.Printf("[Agent] text tool call parse error for %s: %v", toolName, err)
			continue
		}

		*toolCalls = append(*toolCalls, map[string]interface{}{
			"id":   fmt.Sprintf("text-tc-%s-%d", toolName, len(*toolCalls)),
			"type": "function",
			"function": map[string]interface{}{
				"name":      toolName,
				"arguments": argsStr,
			},
		})
	}

	// Remove matched tool call markers from content
	return dashToolCallRe.ReplaceAllString(content, "")
}

// parseCodeBlockFormat handles ```tool_call\nname({args})\n``` format.
var codeBlockToolCallRe = regexp.MustCompile("(?s)```tool_call\\s*\\n(\\w+)\\(([^)]+)\\)\\s*\\n```")

func parseCodeBlockFormat(content string, toolCalls *[]map[string]interface{}) string {
	matches := codeBlockToolCallRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	for _, match := range matches {
		toolName := content[match[2]:match[3]]
		argsStr := content[match[4]:match[5]]

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			log.Printf("[Agent] text tool call parse error for %s: %v", toolName, err)
			continue
		}

		*toolCalls = append(*toolCalls, map[string]interface{}{
			"id":   fmt.Sprintf("text-tc-%s-%d", toolName, len(*toolCalls)),
			"type": "function",
			"function": map[string]interface{}{
				"name":      toolName,
				"arguments": argsStr,
			},
		})
	}

	return codeBlockToolCallRe.ReplaceAllString(content, "")
}

// parseXMLTagFormat handles [tool_calls][...JSON array...][/tool_calls] format.
var xmlTagToolCallRe = regexp.MustCompile(`(?s)\[tool_calls\]\s*(\[.+?\])\s*\[/tool_calls\]`)

func parseXMLTagFormat(content string, toolCalls *[]map[string]interface{}) string {
	matches := xmlTagToolCallRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	for _, match := range matches {
		jsonArr := content[match[2]:match[3]]

		var calls []map[string]interface{}
		if err := json.Unmarshal([]byte(jsonArr), &calls); err != nil {
			log.Printf("[Agent] text tool call parse error (xml-tag format): %v", err)
			continue
		}

		*toolCalls = append(*toolCalls, calls...)
	}

	return xmlTagToolCallRe.ReplaceAllString(content, "")
}

// parseNarratedJSONFormat handles models that narrate a tool call then provide
// a JSON code block with arguments. Matches patterns like:
//
//	tool_call
//	```json
//	{"command": "ls -la"}
//	```
//
// The tool name is inferred from the JSON keys (e.g., "command" → bash, "path" → file_read).
var narratedJSONRe = regexp.MustCompile("(?s)tool_call\\s*\\n```(?:json)?\\s*\\n(\\{.+?\\})\\s*\\n```")

// toolNameFromArgs infers the tool name from the argument keys.
// Priority-ordered slice to ensure correct tool routing (specific keys before generic).
var argKeyPriority = []struct {
	key  string
	tool string
}{
	{"old_string", "file_edit"},
	{"command", "bash"},
	{"pattern", "grep"},
	{"query", "web_search"},
	{"subcommand", "git"},
	{"cell", "notebook_edit"},
	{"content", "file_write"},
	{"url", "web_fetch"},
	{"path", "file_read"},
}

func parseNarratedJSONFormat(content string, toolCalls *[]map[string]interface{}) string {
	matches := narratedJSONRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	for _, match := range matches {
		argsStr := content[match[2]:match[3]]

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			log.Printf("[Agent] text tool call parse error (narrated-json): %v", err)
			continue
		}

		// Infer tool name from argument keys (priority-ordered)
		toolName := ""
		for _, pair := range argKeyPriority {
			if _, ok := args[pair.key]; ok {
				toolName = pair.tool
				break
			}
		}
		if toolName == "" {
			continue // can't determine tool
		}

		// file_write needs both path and content; file_read only needs path
		if toolName == "file_read" {
			if _, hasContent := args["content"]; hasContent {
				toolName = "file_write"
			}
		}

		*toolCalls = append(*toolCalls, map[string]interface{}{
			"id":   fmt.Sprintf("text-tc-%s-%d", toolName, len(*toolCalls)),
			"type": "function",
			"function": map[string]interface{}{
				"name":      toolName,
				"arguments": argsStr,
			},
		})
	}

	return narratedJSONRe.ReplaceAllString(content, "")
}

// isCompletionSignal detects when the model signals task completion.
// Used to exit the agent loop when the model says the task is done,
// preventing infinite re-confirmation loops (#333).
func isCompletionSignal(content string) bool {
	lower := strings.ToLower(content)
	signals := []string{
		"task complete",
		"task is complete",
		"task has been completed",
		"successfully completed",
		"the fix is complete",
		"changes are complete",
		"implementation is complete",
		"i've completed the task",
		"work is finished",
	}
	for _, sig := range signals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

// parseKeyValueFormat handles tool_call_name=NAME\ntool_call_arguments={...} format.
var keyValueToolCallRe = regexp.MustCompile(`(?s)tool_call_name\s*=\s*(\w+)\s*\ntool_call_arguments\s*=\s*(\{.+?\})`)

func parseKeyValueFormat(content string, toolCalls *[]map[string]interface{}) string {
	matches := keyValueToolCallRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}

	for _, match := range matches {
		toolName := content[match[2]:match[3]]
		argsStr := content[match[4]:match[5]]

		var args map[string]interface{}
		if err := json.Unmarshal([]byte(argsStr), &args); err != nil {
			log.Printf("[Agent] text tool call parse error (key-value): %v", err)
			continue
		}

		*toolCalls = append(*toolCalls, map[string]interface{}{
			"id":   fmt.Sprintf("text-tc-%s-%d", toolName, len(*toolCalls)),
			"type": "function",
			"function": map[string]interface{}{
				"name":      toolName,
				"arguments": argsStr,
			},
		})
	}

	return keyValueToolCallRe.ReplaceAllString(content, "")
}
