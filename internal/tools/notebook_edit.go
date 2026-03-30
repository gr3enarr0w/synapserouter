package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NotebookEditTool edits a Jupyter notebook cell by index.
type NotebookEditTool struct{}

func (t *NotebookEditTool) Name() string        { return "notebook_edit" }
func (t *NotebookEditTool) Description() string { return "Edit a Jupyter notebook cell by index" }
func (t *NotebookEditTool) Category() ToolCategory { return CategoryWrite }

func (t *NotebookEditTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Path to .ipynb file",
			},
			"cell": map[string]interface{}{
				"type":        "integer",
				"description": "Cell index (1-based)",
			},
			"source": map[string]interface{}{
				"type":        "string",
				"description": "New cell source content",
			},
			"cell_type": map[string]interface{}{
				"type":        "string",
				"description": "Cell type: code or markdown (default: keep existing)",
			},
		},
		"required": []string{"path", "cell", "source"},
	}
}

func (t *NotebookEditTool) Execute(ctx context.Context, args map[string]interface{}, workDir string) (*ToolResult, error) {
	path, err := resolveToolPathChecked(stringArg(args, "path"), workDir)
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	if !strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".ipynb") {
		return &ToolResult{Error: "notebook_edit only works on .ipynb files"}, nil
	}

	cellIdx := intArg(args, "cell", 0)
	source := stringArg(args, "source")
	cellType := stringArg(args, "cell_type")

	if cellIdx < 1 {
		return &ToolResult{Error: "cell index must be >= 1"}, nil
	}

	// Read and parse the notebook, preserving all fields
	data, err := os.ReadFile(path)
	if err != nil {
		return &ToolResult{Error: err.Error()}, nil
	}

	var nb map[string]json.RawMessage
	if err := json.Unmarshal(data, &nb); err != nil {
		return &ToolResult{Error: fmt.Sprintf("invalid notebook JSON: %v", err)}, nil
	}

	cellsRaw, ok := nb["cells"]
	if !ok {
		return &ToolResult{Error: "notebook has no cells array"}, nil
	}

	var cells []map[string]json.RawMessage
	if err := json.Unmarshal(cellsRaw, &cells); err != nil {
		return &ToolResult{Error: fmt.Sprintf("invalid cells array: %v", err)}, nil
	}

	if cellIdx > len(cells) {
		return &ToolResult{Error: fmt.Sprintf("cell index %d out of range (notebook has %d cells)", cellIdx, len(cells))}, nil
	}

	cell := cells[cellIdx-1]

	// Convert source string to ipynb []string format (each line ends with \n except last)
	lines := strings.Split(source, "\n")
	sourceLines := make([]string, len(lines))
	for i, line := range lines {
		if i < len(lines)-1 {
			sourceLines[i] = line + "\n"
		} else {
			sourceLines[i] = line
		}
	}
	// Remove trailing empty string if source ended with newline
	if len(sourceLines) > 0 && sourceLines[len(sourceLines)-1] == "" {
		sourceLines = sourceLines[:len(sourceLines)-1]
	}

	sourceJSON, err := json.Marshal(sourceLines)
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("failed to marshal source: %v", err)}, nil
	}
	cell["source"] = sourceJSON

	// Optionally change cell type
	if cellType != "" {
		ct, _ := json.Marshal(cellType)
		cell["cell_type"] = ct
	}

	cells[cellIdx-1] = cell

	// Marshal cells back
	newCellsJSON, err := json.Marshal(cells)
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("failed to marshal cells: %v", err)}, nil
	}
	nb["cells"] = newCellsJSON

	// Write back with indentation matching ipynb standard
	output, err := json.MarshalIndent(nb, "", " ")
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("failed to marshal notebook: %v", err)}, nil
	}

	info, _ := os.Stat(path)
	mode := os.FileMode(0644)
	if info != nil {
		mode = info.Mode()
	}

	if err := os.WriteFile(path, output, mode); err != nil {
		return &ToolResult{Error: fmt.Sprintf("failed to write notebook: %v", err)}, nil
	}

	return &ToolResult{Output: fmt.Sprintf("updated cell %d in %s", cellIdx, path)}, nil
}
