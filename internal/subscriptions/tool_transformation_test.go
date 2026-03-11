package subscriptions

import (
	"encoding/json"
	"testing"
)

// TestTransformToAnthropicTools tests the OpenAI → Anthropic tool transformation
// This test validates the fix for BUG-CLAUDE-TOOLS-001
func TestTransformToAnthropicTools(t *testing.T) {
	tests := []struct {
		name     string
		input    []map[string]interface{}
		expected []map[string]interface{}
	}{
		{
			name: "BUG-CLAUDE-TOOLS-001: OpenAI format tool with all fields",
			input: []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "get_weather",
						"description": "Get current weather",
						"parameters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"location": map[string]interface{}{
									"type":        "string",
									"description": "City name",
								},
							},
							"required": []interface{}{"location"},
						},
					},
				},
			},
			expected: []map[string]interface{}{
				{
					"name":        "get_weather",
					"description": "Get current weather",
					"input_schema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"location": map[string]interface{}{
								"type":        "string",
								"description": "City name",
							},
						},
						"required": []interface{}{"location"},
					},
				},
			},
		},
		{
			name: "Multiple OpenAI format tools",
			input: []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "get_weather",
						"description": "Get weather",
						"parameters": map[string]interface{}{
							"type": "object",
						},
					},
				},
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "calculate",
						"description": "Do math",
						"parameters": map[string]interface{}{
							"type": "object",
						},
					},
				},
			},
			expected: []map[string]interface{}{
				{
					"name":         "get_weather",
					"description":  "Get weather",
					"input_schema": map[string]interface{}{"type": "object"},
				},
				{
					"name":         "calculate",
					"description":  "Do math",
					"input_schema": map[string]interface{}{"type": "object"},
				},
			},
		},
		{
			name: "Already in Anthropic format (pass through)",
			input: []map[string]interface{}{
				{
					"name":         "get_weather",
					"description":  "Get weather",
					"input_schema": map[string]interface{}{"type": "object"},
				},
			},
			expected: []map[string]interface{}{
				{
					"name":         "get_weather",
					"description":  "Get weather",
					"input_schema": map[string]interface{}{"type": "object"},
				},
			},
		},
		{
			name: "Tool without description",
			input: []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name": "simple_tool",
						"parameters": map[string]interface{}{
							"type": "object",
						},
					},
				},
			},
			expected: []map[string]interface{}{
				{
					"name":         "simple_tool",
					"input_schema": map[string]interface{}{"type": "object"},
				},
			},
		},
		{
			name:     "Empty tools array",
			input:    []map[string]interface{}{},
			expected: nil,
		},
		{
			name:     "Nil tools array",
			input:    nil,
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TransformToAnthropicTools(tt.input)

			if !equalTools(result, tt.expected) {
				resultJSON, _ := json.MarshalIndent(result, "", "  ")
				expectedJSON, _ := json.MarshalIndent(tt.expected, "", "  ")
				t.Errorf("TransformToAnthropicTools() mismatch:\nGot:\n%s\n\nExpected:\n%s", resultJSON, expectedJSON)
			}
		})
	}
}

// TestTransformToAnthropicTools_ComplexSchema tests transformation with complex parameter schemas
func TestTransformToAnthropicTools_ComplexSchema(t *testing.T) {
	input := []map[string]interface{}{
		{
			"type": "function",
			"function": map[string]interface{}{
				"name":        "search_database",
				"description": "Search a database with filters",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search query",
						},
						"filters": map[string]interface{}{
							"type": "object",
							"properties": map[string]interface{}{
								"category": map[string]interface{}{
									"type": "string",
									"enum": []interface{}{"books", "electronics", "clothing"},
								},
								"price_range": map[string]interface{}{
									"type": "object",
									"properties": map[string]interface{}{
										"min": map[string]interface{}{"type": "number"},
										"max": map[string]interface{}{"type": "number"},
									},
								},
							},
						},
					},
					"required": []interface{}{"query"},
				},
			},
		},
	}

	result := TransformToAnthropicTools(input)

	if len(result) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(result))
	}

	tool := result[0]

	// Verify basic structure
	if tool["name"] != "search_database" {
		t.Errorf("Expected name 'search_database', got %v", tool["name"])
	}

	// Verify input_schema exists (not parameters)
	if _, hasInputSchema := tool["input_schema"]; !hasInputSchema {
		t.Error("Tool should have 'input_schema' field")
	}

	if _, hasParameters := tool["parameters"]; hasParameters {
		t.Error("Tool should NOT have 'parameters' field (OpenAI format)")
	}

	// Verify complex nested structure is preserved
	inputSchema, ok := tool["input_schema"].(map[string]interface{})
	if !ok {
		t.Fatal("input_schema should be a map")
	}

	properties, ok := inputSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("input_schema.properties should be a map")
	}

	filters, ok := properties["filters"].(map[string]interface{})
	if !ok {
		t.Fatal("filters property should exist")
	}

	filterProps, ok := filters["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("filters.properties should be a map")
	}

	if _, hasPriceRange := filterProps["price_range"]; !hasPriceRange {
		t.Error("Nested price_range structure should be preserved")
	}
}

// Helper function to compare tool arrays
func equalTools(a, b []map[string]interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}
