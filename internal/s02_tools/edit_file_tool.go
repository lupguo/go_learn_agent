package s02_tools

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// EditFileTool replaces the first occurrence of old_text with new_text in a file.
type EditFileTool struct{ workDir string }

func NewEditFileTool(workDir string) *EditFileTool {
	return &EditFileTool{workDir: workDir}
}

func (e *EditFileTool) Name() string       { return "edit_file" }
func (e *EditFileTool) Description() string { return "Replace exact text in a file." }
func (e *EditFileTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path":     map[string]any{"type": "string"},
			"old_text": map[string]any{"type": "string"},
			"new_text": map[string]any{"type": "string"},
		},
		"required": []string{"path", "old_text", "new_text"},
	}
}

func (e *EditFileTool) Execute(_ context.Context, input map[string]any) (string, error) {
	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "", fmt.Errorf("missing or empty 'path' parameter")
	}
	oldText, ok := input["old_text"].(string)
	if !ok {
		return "", fmt.Errorf("missing 'old_text' parameter")
	}
	newText, _ := input["new_text"].(string)
	absPath, err := SafePath(e.workDir, path)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	content := string(data)
	if !strings.Contains(content, oldText) {
		return fmt.Sprintf("Error: Text not found in %s", path), nil
	}
	newContent := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(absPath, []byte(newContent), 0o644); err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	return fmt.Sprintf("Edited %s", path), nil
}
