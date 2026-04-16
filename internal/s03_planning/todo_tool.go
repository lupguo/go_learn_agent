package s03_planning

import (
	"context"
	"fmt"

	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// todoToolName is the tool name used for plan detection in the agent loop.
const todoToolName = "todo"

// TodoTool implements tool.Tool to let the LLM rewrite the session plan.
type TodoTool struct {
	plan *PlanManager
}

// Verify interface compliance at compile time.
var _ tool.Tool = (*TodoTool)(nil)

// NewTodoTool creates a TodoTool backed by the given PlanManager.
func NewTodoTool(plan *PlanManager) *TodoTool {
	return &TodoTool{plan: plan}
}

func (t *TodoTool) Name() string { return todoToolName }

func (t *TodoTool) Description() string {
	return "Rewrite the current session plan for multi-step work."
}

func (t *TodoTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"content": map[string]any{"type": "string"},
						"status": map[string]any{
							"type": "string",
							"enum": []string{"pending", "in_progress", "completed"},
						},
						"activeForm": map[string]any{
							"type":        "string",
							"description": "Optional present-continuous label.",
						},
					},
					"required": []string{"content", "status"},
				},
			},
		},
		"required": []string{"items"},
	}
}

func (t *TodoTool) Execute(_ context.Context, input map[string]any) (string, error) {
	rawItems, ok := input["items"]
	if !ok {
		return "", fmt.Errorf("missing required field: items")
	}

	// items comes from JSON unmarshal as []any
	itemSlice, ok := rawItems.([]any)
	if !ok {
		return "", fmt.Errorf("items must be an array")
	}

	items := make([]map[string]any, 0, len(itemSlice))
	for i, raw := range itemSlice {
		m, ok := raw.(map[string]any)
		if !ok {
			return "", fmt.Errorf("item %d: expected object", i)
		}
		items = append(items, m)
	}

	return t.plan.Update(items)
}
