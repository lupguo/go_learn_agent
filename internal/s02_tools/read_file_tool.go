package s02_tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// ReadFileTool reads file contents with optional line limit.
type ReadFileTool struct{ workDir string }

func NewReadFileTool(workDir string) *ReadFileTool {
	return &ReadFileTool{workDir: workDir}
}

func (r *ReadFileTool) Name() string       { return "read_file" }
func (r *ReadFileTool) Description() string { return "Read file contents." }
func (r *ReadFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":  map[string]any{"type": "string"},
			"limit": map[string]any{"type": "integer", "description": "Max lines to read"},
		},
		"required": []string{"path"},
	}
}

func (r *ReadFileTool) Execute(_ context.Context, input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing or empty 'path' parameter")
	}
	absPath, err := SafePath(r.workDir, path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	lines := strings.Split(string(data), "\n")
	var limit int
	if l, ok := input["limit"].(float64); ok && int(l) > 0 {
		limit = int(l)
	}
	if limit > 0 && limit < len(lines) {
		lines = append(lines[:limit], fmt.Sprintf("... (%d more lines)", len(lines)-limit))
	}
	result := strings.Join(lines, "\n")
	if len(result) > 50000 {
		result = result[:50000]
	}
	return result, nil
}
