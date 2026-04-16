package s12_tasks

import (
	"context"
	"fmt"
)

// --- TaskCreateTool ---

type TaskCreateTool struct {
	manager *TaskManager
}

func NewTaskCreateTool(m *TaskManager) *TaskCreateTool {
	return &TaskCreateTool{manager: m}
}

func (t *TaskCreateTool) Name() string        { return "task_create" }
func (t *TaskCreateTool) Description() string  { return "Create a new task." }
func (t *TaskCreateTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"subject":     map[string]any{"type": "string", "description": "Brief title for the task"},
			"description": map[string]any{"type": "string", "description": "Detailed description of what needs to be done"},
		},
		"required": []string{"subject"},
	}
}

func (t *TaskCreateTool) Execute(_ context.Context, input map[string]any) (string, error) {
	subject, _ := input["subject"].(string)
	if subject == "" {
		return "", fmt.Errorf("subject is required")
	}
	desc, _ := input["description"].(string)
	return t.manager.Create(subject, desc)
}

// --- TaskGetTool ---

type TaskGetTool struct {
	manager *TaskManager
}

func NewTaskGetTool(m *TaskManager) *TaskGetTool {
	return &TaskGetTool{manager: m}
}

func (t *TaskGetTool) Name() string        { return "task_get" }
func (t *TaskGetTool) Description() string  { return "Get full details of a task by ID." }
func (t *TaskGetTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id": map[string]any{"type": "integer", "description": "The task ID to retrieve"},
		},
		"required": []string{"task_id"},
	}
}

func (t *TaskGetTool) Execute(_ context.Context, input map[string]any) (string, error) {
	taskID, err := intFromInput(input, "task_id")
	if err != nil {
		return "", err
	}
	return t.manager.Get(taskID)
}

// --- TaskUpdateTool ---

type TaskUpdateTool struct {
	manager *TaskManager
}

func NewTaskUpdateTool(m *TaskManager) *TaskUpdateTool {
	return &TaskUpdateTool{manager: m}
}

func (t *TaskUpdateTool) Name() string        { return "task_update" }
func (t *TaskUpdateTool) Description() string  { return "Update a task's status, owner, or dependencies." }
func (t *TaskUpdateTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"task_id":      map[string]any{"type": "integer", "description": "The task ID to update"},
			"status":       map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "deleted"}},
			"owner":        map[string]any{"type": "string", "description": "Set when a teammate claims the task"},
			"addBlockedBy": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
			"addBlocks":    map[string]any{"type": "array", "items": map[string]any{"type": "integer"}},
		},
		"required": []string{"task_id"},
	}
}

func (t *TaskUpdateTool) Execute(_ context.Context, input map[string]any) (string, error) {
	taskID, err := intFromInput(input, "task_id")
	if err != nil {
		return "", err
	}

	var status *TaskStatus
	if s, ok := input["status"].(string); ok {
		st := TaskStatus(s)
		status = &st
	}

	var owner *string
	if o, ok := input["owner"].(string); ok {
		owner = &o
	}

	addBlockedBy := intSliceFromInput(input, "addBlockedBy")
	addBlocks := intSliceFromInput(input, "addBlocks")

	return t.manager.Update(taskID, status, owner, addBlockedBy, addBlocks)
}

// --- TaskListTool ---

type TaskListTool struct {
	manager *TaskManager
}

func NewTaskListTool(m *TaskManager) *TaskListTool {
	return &TaskListTool{manager: m}
}

func (t *TaskListTool) Name() string        { return "task_list" }
func (t *TaskListTool) Description() string  { return "List all tasks with status summary." }
func (t *TaskListTool) Schema() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *TaskListTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return t.manager.ListAll(), nil
}

// --- input helpers ---

func intFromInput(input map[string]any, key string) (int, error) {
	v, ok := input[key]
	if !ok {
		return 0, fmt.Errorf("%s is required", key)
	}
	// JSON numbers come as float64
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	default:
		return 0, fmt.Errorf("%s must be an integer", key)
	}
}

func intSliceFromInput(input map[string]any, key string) []int {
	v, ok := input[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]int, 0, len(arr))
	for _, item := range arr {
		switch n := item.(type) {
		case float64:
			result = append(result, int(n))
		case int:
			result = append(result, n)
		}
	}
	return result
}
