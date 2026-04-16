package s04_subagent

import (
	"context"
	"fmt"

	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// TaskTool dispatches a subagent with fresh context.
// The child shares the filesystem but not conversation history.
type TaskTool struct {
	provider      llm.Provider
	childRegistry *tool.Registry
}

var _ tool.Tool = (*TaskTool)(nil)

// NewTaskTool creates a TaskTool that spawns subagents using the given provider
// and a filtered tool registry (no task tool — prevents recursion).
func NewTaskTool(provider llm.Provider, childRegistry *tool.Registry) *TaskTool {
	return &TaskTool{
		provider:      provider,
		childRegistry: childRegistry,
	}
}

func (t *TaskTool) Name() string { return "task" }

func (t *TaskTool) Description() string {
	return "Spawn a subagent with fresh context. It shares the filesystem but not conversation history."
}

func (t *TaskTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{"type": "string"},
			"description": map[string]any{
				"type":        "string",
				"description": "Short description of the task",
			},
		},
		"required": []string{"prompt"},
	}
}

func (t *TaskTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	prompt, ok := input["prompt"].(string)
	if !ok || prompt == "" {
		return "", fmt.Errorf("missing required field: prompt")
	}

	desc := "subtask"
	if d, ok := input["description"].(string); ok && d != "" {
		desc = d
	}
	fmt.Printf("  [subagent: %s]\n", desc)

	return RunSubagent(ctx, t.provider, t.childRegistry, prompt), nil
}
