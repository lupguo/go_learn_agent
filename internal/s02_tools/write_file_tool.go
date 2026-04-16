package s02_tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteFileTool writes content to a file, creating parent directories as needed.
type WriteFileTool struct{ workDir string }

func NewWriteFileTool(workDir string) *WriteFileTool {
	return &WriteFileTool{workDir: workDir}
}

func (w *WriteFileTool) Name() string       { return "write_file" }
func (w *WriteFileTool) Description() string { return "Write content to a file." }
func (w *WriteFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
		},
		"required": []string{"path", "content"},
	}
}

func (w *WriteFileTool) Execute(_ context.Context, input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing or empty 'path' parameter")
	}
	content, ok := input["content"].(string)
	if !ok {
		return "", fmt.Errorf("missing 'content' parameter")
	}
	absPath, err := SafePath(w.workDir, path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	return fmt.Sprintf("Wrote %d bytes to %s", len(content), path), nil
}
