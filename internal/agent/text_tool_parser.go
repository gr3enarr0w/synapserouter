package agent

import (
	"encoding/json"
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
			"id":   "text-tc-" + toolName,
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
			"id":   "text-tc-" + toolName,
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
