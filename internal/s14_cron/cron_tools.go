package s14_cron

import (
	"context"
	"fmt"
)

// --- CronCreateTool ---

type CronCreateTool struct {
	scheduler *CronScheduler
}

func NewCronCreateTool(s *CronScheduler) *CronCreateTool {
	return &CronCreateTool{scheduler: s}
}

func (t *CronCreateTool) Name() string        { return "cron_create" }
func (t *CronCreateTool) Description() string {
	return "Schedule a recurring or one-shot task with a cron expression."
}
func (t *CronCreateTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"cron":      map[string]any{"type": "string", "description": "5-field cron expression: 'min hour dom month dow'"},
			"prompt":    map[string]any{"type": "string", "description": "The prompt to inject when the task fires"},
			"recurring": map[string]any{"type": "boolean", "description": "true=repeat, false=fire once then delete. Default true."},
			"durable":   map[string]any{"type": "boolean", "description": "true=persist to disk, false=session-only. Default false."},
		},
		"required": []string{"cron", "prompt"},
	}
}

func (t *CronCreateTool) Execute(_ context.Context, input map[string]any) (string, error) {
	cronExpr, _ := input["cron"].(string)
	if cronExpr == "" {
		return "", fmt.Errorf("cron expression is required")
	}
	prompt, _ := input["prompt"].(string)
	if prompt == "" {
		return "", fmt.Errorf("prompt is required")
	}

	recurring := true
	if v, ok := input["recurring"].(bool); ok {
		recurring = v
	}
	durable := false
	if v, ok := input["durable"].(bool); ok {
		durable = v
	}

	return t.scheduler.Create(cronExpr, prompt, recurring, durable), nil
}

// --- CronDeleteTool ---

type CronDeleteTool struct {
	scheduler *CronScheduler
}

func NewCronDeleteTool(s *CronScheduler) *CronDeleteTool {
	return &CronDeleteTool{scheduler: s}
}

func (t *CronDeleteTool) Name() string        { return "cron_delete" }
func (t *CronDeleteTool) Description() string  { return "Delete a scheduled task by ID." }
func (t *CronDeleteTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "string", "description": "Task ID to delete"},
		},
		"required": []string{"id"},
	}
}

func (t *CronDeleteTool) Execute(_ context.Context, input map[string]any) (string, error) {
	id, _ := input["id"].(string)
	if id == "" {
		return "", fmt.Errorf("id is required")
	}
	return t.scheduler.Delete(id), nil
}

// --- CronListTool ---

type CronListTool struct {
	scheduler *CronScheduler
}

func NewCronListTool(s *CronScheduler) *CronListTool {
	return &CronListTool{scheduler: s}
}

func (t *CronListTool) Name() string        { return "cron_list" }
func (t *CronListTool) Description() string  { return "List all scheduled tasks." }
func (t *CronListTool) Schema() any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *CronListTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return t.scheduler.ListTasks(), nil
}
