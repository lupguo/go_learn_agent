package s18_worktree

import (
	"context"
	"fmt"
)

// Helper to extract int from JSON input (float64 from JSON).
func intFromInput(input map[string]any, key string) (int, bool) {
	v, ok := input[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	}
	return 0, false
}

func optionalIntFromInput(input map[string]any, key string) *int {
	v, ok := intFromInput(input, key)
	if !ok {
		return nil
	}
	return &v
}

// --- Task Tools ---

type TaskCreateTool struct{ tasks *TaskManager }

func NewTaskCreateTool(t *TaskManager) *TaskCreateTool { return &TaskCreateTool{tasks: t} }
func (t *TaskCreateTool) Name() string                  { return "task_create" }
func (t *TaskCreateTool) Description() string           { return "Create a new task on the shared task board." }
func (t *TaskCreateTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"subject": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"},
	}, "required": []string{"subject"}}
}
func (t *TaskCreateTool) Execute(_ context.Context, input map[string]any) (string, error) {
	subject, _ := input["subject"].(string)
	desc, _ := input["description"].(string)
	return t.tasks.Create(subject, desc)
}

type TaskListTool struct{ tasks *TaskManager }

func NewTaskListTool(t *TaskManager) *TaskListTool { return &TaskListTool{tasks: t} }
func (t *TaskListTool) Name() string                { return "task_list" }
func (t *TaskListTool) Description() string         { return "List all tasks with status, owner, and worktree binding." }
func (t *TaskListTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *TaskListTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return t.tasks.ListAll(), nil
}

type TaskGetTool struct{ tasks *TaskManager }

func NewTaskGetTool(t *TaskManager) *TaskGetTool { return &TaskGetTool{tasks: t} }
func (t *TaskGetTool) Name() string               { return "task_get" }
func (t *TaskGetTool) Description() string        { return "Get task details by ID." }
func (t *TaskGetTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"task_id": map[string]any{"type": "integer"},
	}, "required": []string{"task_id"}}
}
func (t *TaskGetTool) Execute(_ context.Context, input map[string]any) (string, error) {
	id, ok := intFromInput(input, "task_id")
	if !ok {
		return "", fmt.Errorf("task_id is required")
	}
	return t.tasks.Get(id)
}

type TaskUpdateTool struct{ tasks *TaskManager }

func NewTaskUpdateTool(t *TaskManager) *TaskUpdateTool { return &TaskUpdateTool{tasks: t} }
func (t *TaskUpdateTool) Name() string                  { return "task_update" }
func (t *TaskUpdateTool) Description() string           { return "Update task status or owner." }
func (t *TaskUpdateTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"task_id": map[string]any{"type": "integer"},
		"status":  map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "deleted"}},
		"owner":   map[string]any{"type": "string"},
	}, "required": []string{"task_id"}}
}
func (t *TaskUpdateTool) Execute(_ context.Context, input map[string]any) (string, error) {
	id, ok := intFromInput(input, "task_id")
	if !ok {
		return "", fmt.Errorf("task_id is required")
	}
	var status, owner *string
	if s, ok := input["status"].(string); ok {
		status = &s
	}
	if o, ok := input["owner"].(string); ok {
		owner = &o
	}
	return t.tasks.Update(id, status, owner)
}

type TaskBindWorktreeTool struct{ tasks *TaskManager }

func NewTaskBindWorktreeTool(t *TaskManager) *TaskBindWorktreeTool {
	return &TaskBindWorktreeTool{tasks: t}
}
func (t *TaskBindWorktreeTool) Name() string        { return "task_bind_worktree" }
func (t *TaskBindWorktreeTool) Description() string { return "Bind a task to a worktree name." }
func (t *TaskBindWorktreeTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"task_id":  map[string]any{"type": "integer"},
		"worktree": map[string]any{"type": "string"},
		"owner":    map[string]any{"type": "string"},
	}, "required": []string{"task_id", "worktree"}}
}
func (t *TaskBindWorktreeTool) Execute(_ context.Context, input map[string]any) (string, error) {
	id, ok := intFromInput(input, "task_id")
	if !ok {
		return "", fmt.Errorf("task_id is required")
	}
	wt, _ := input["worktree"].(string)
	owner, _ := input["owner"].(string)
	return t.tasks.BindWorktree(id, wt, owner)
}

// --- Worktree Tools ---

type WTCreateTool struct{ wm *WorktreeManager }

func NewWTCreateTool(wm *WorktreeManager) *WTCreateTool { return &WTCreateTool{wm: wm} }
func (t *WTCreateTool) Name() string                     { return "worktree_create" }
func (t *WTCreateTool) Description() string              { return "Create a git worktree and optionally bind it to a task." }
func (t *WTCreateTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "task_id": map[string]any{"type": "integer"},
		"base_ref": map[string]any{"type": "string"},
	}, "required": []string{"name"}}
}
func (t *WTCreateTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	taskID := optionalIntFromInput(input, "task_id")
	baseRef, _ := input["base_ref"].(string)
	return t.wm.Create(name, taskID, baseRef)
}

type WTListTool struct{ wm *WorktreeManager }

func NewWTListTool(wm *WorktreeManager) *WTListTool { return &WTListTool{wm: wm} }
func (t *WTListTool) Name() string                   { return "worktree_list" }
func (t *WTListTool) Description() string            { return "List worktrees tracked in .worktrees/index.json." }
func (t *WTListTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *WTListTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return t.wm.ListAll(), nil
}

type WTEnterTool struct{ wm *WorktreeManager }

func NewWTEnterTool(wm *WorktreeManager) *WTEnterTool { return &WTEnterTool{wm: wm} }
func (t *WTEnterTool) Name() string                    { return "worktree_enter" }
func (t *WTEnterTool) Description() string             { return "Enter or reopen a worktree lane." }
func (t *WTEnterTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string"},
	}, "required": []string{"name"}}
}
func (t *WTEnterTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	return t.wm.Enter(name), nil
}

type WTStatusTool struct{ wm *WorktreeManager }

func NewWTStatusTool(wm *WorktreeManager) *WTStatusTool { return &WTStatusTool{wm: wm} }
func (t *WTStatusTool) Name() string                     { return "worktree_status" }
func (t *WTStatusTool) Description() string              { return "Show git status for one worktree." }
func (t *WTStatusTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string"},
	}, "required": []string{"name"}}
}
func (t *WTStatusTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	return t.wm.Status(name), nil
}

type WTRunTool struct{ wm *WorktreeManager }

func NewWTRunTool(wm *WorktreeManager) *WTRunTool { return &WTRunTool{wm: wm} }
func (t *WTRunTool) Name() string                  { return "worktree_run" }
func (t *WTRunTool) Description() string           { return "Run a shell command in a named worktree directory." }
func (t *WTRunTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "command": map[string]any{"type": "string"},
	}, "required": []string{"name", "command"}}
}
func (t *WTRunTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	command, _ := input["command"].(string)
	return t.wm.Run(name, command), nil
}

type WTCloseoutTool struct{ wm *WorktreeManager }

func NewWTCloseoutTool(wm *WorktreeManager) *WTCloseoutTool { return &WTCloseoutTool{wm: wm} }
func (t *WTCloseoutTool) Name() string                       { return "worktree_closeout" }
func (t *WTCloseoutTool) Description() string                { return "Close out a lane by keeping or removing it." }
func (t *WTCloseoutTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "action": map[string]any{"type": "string", "enum": []string{"keep", "remove"}},
		"reason": map[string]any{"type": "string"}, "force": map[string]any{"type": "boolean"},
		"complete_task": map[string]any{"type": "boolean"},
	}, "required": []string{"name", "action"}}
}
func (t *WTCloseoutTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	action, _ := input["action"].(string)
	reason, _ := input["reason"].(string)
	force, _ := input["force"].(bool)
	completeTask, _ := input["complete_task"].(bool)
	return t.wm.Closeout(name, action, reason, force, completeTask)
}

type WTRemoveTool struct{ wm *WorktreeManager }

func NewWTRemoveTool(wm *WorktreeManager) *WTRemoveTool { return &WTRemoveTool{wm: wm} }
func (t *WTRemoveTool) Name() string                     { return "worktree_remove" }
func (t *WTRemoveTool) Description() string              { return "Remove a worktree and optionally complete its task." }
func (t *WTRemoveTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string"}, "force": map[string]any{"type": "boolean"},
		"complete_task": map[string]any{"type": "boolean"}, "reason": map[string]any{"type": "string"},
	}, "required": []string{"name"}}
}
func (t *WTRemoveTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	force, _ := input["force"].(bool)
	completeTask, _ := input["complete_task"].(bool)
	reason, _ := input["reason"].(string)
	return t.wm.Remove(name, force, completeTask, reason)
}

type WTKeepTool struct{ wm *WorktreeManager }

func NewWTKeepTool(wm *WorktreeManager) *WTKeepTool { return &WTKeepTool{wm: wm} }
func (t *WTKeepTool) Name() string                   { return "worktree_keep" }
func (t *WTKeepTool) Description() string            { return "Mark a worktree as kept without removing it." }
func (t *WTKeepTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"name": map[string]any{"type": "string"},
	}, "required": []string{"name"}}
}
func (t *WTKeepTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	return t.wm.Keep(name), nil
}

type WTEventsTool struct{ events *EventBus }

func NewWTEventsTool(events *EventBus) *WTEventsTool { return &WTEventsTool{events: events} }
func (t *WTEventsTool) Name() string                  { return "worktree_events" }
func (t *WTEventsTool) Description() string           { return "List recent lifecycle events." }
func (t *WTEventsTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{
		"limit": map[string]any{"type": "integer"},
	}}
}
func (t *WTEventsTool) Execute(_ context.Context, input map[string]any) (string, error) {
	limit := 20
	if v, ok := intFromInput(input, "limit"); ok {
		limit = v
	}
	return t.events.ListRecent(limit), nil
}
