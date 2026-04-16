package s13_background

import (
	"context"
	"fmt"
)

// --- BackgroundRunTool ---

type BackgroundRunTool struct {
	manager *BackgroundManager
}

func NewBackgroundRunTool(m *BackgroundManager) *BackgroundRunTool {
	return &BackgroundRunTool{manager: m}
}

func (t *BackgroundRunTool) Name() string        { return "background_run" }
func (t *BackgroundRunTool) Description() string  { return "Run command in background goroutine. Returns task_id immediately." }
func (t *BackgroundRunTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string", "description": "Shell command to run in background"},
		},
		"required": []string{"command"},
	}
}

func (t *BackgroundRunTool) Execute(_ context.Context, input map[string]any) (string, error) {
	command, _ := input["command"].(string)
	if command == "" {
		return "", fmt.Errorf("command is required")
	}
	return t.manager.Run(command), nil
}

// --- CheckBackgroundTool ---

type CheckBackgroundTool struct {
	manager *BackgroundManager
}

func NewCheckBackgroundTool(m *BackgroundManager) *CheckBackgroundTool {
	return &CheckBackgroundTool{manager: m}
}

func (t *CheckBackgroundTool) Name() string        { return "check_background" }
func (t *CheckBackgroundTool) Description() string  { return "Check background task status. Omit task_id to list all." }
func (t *CheckBackgroundTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{"type": "string", "description": "Specific task ID to check"},
		},
	}
}

func (t *CheckBackgroundTool) Execute(_ context.Context, input map[string]any) (string, error) {
	taskID, _ := input["task_id"].(string)
	return t.manager.Check(taskID), nil
}
