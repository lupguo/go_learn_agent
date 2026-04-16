package s18_worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// EventBus provides append-only JSONL lifecycle events for observability.
type EventBus struct {
	path string
	mu   sync.Mutex
}

func NewEventBus(path string) *EventBus {
	dir := path[:strings.LastIndex(path, "/")]
	_ = os.MkdirAll(dir, 0755)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		_ = os.WriteFile(path, nil, 0644)
	}
	return &EventBus{path: path}
}

func (e *EventBus) Emit(event string, taskID *int, wtName string, errMsg string, extra map[string]any) {
	payload := map[string]any{"event": event, "ts": time.Now().Unix()}
	if taskID != nil {
		payload["task_id"] = *taskID
	}
	if wtName != "" {
		payload["worktree"] = wtName
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	for k, v := range extra {
		payload[k] = v
	}

	data, _ := json.Marshal(payload)

	e.mu.Lock()
	defer e.mu.Unlock()
	f, err := os.OpenFile(e.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}

func (e *EventBus) ListRecent(limit int) string {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	e.mu.Lock()
	raw, _ := os.ReadFile(e.path)
	e.mu.Unlock()

	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}

	var items []any
	for _, line := range lines {
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			items = append(items, map[string]any{"event": "parse_error", "raw": line})
		} else {
			items = append(items, obj)
		}
	}

	out, _ := json.MarshalIndent(items, "", "  ")
	return string(out)
}

// helper for emitting with task ID
func intPtr(v int) *int { return &v }

// EmitSimple is a convenience for common events.
func (e *EventBus) EmitSimple(event, wtName string) {
	e.Emit(event, nil, wtName, "", nil)
}

func (e *EventBus) EmitTask(event string, taskID int, wtName string) {
	e.Emit(event, intPtr(taskID), wtName, "", nil)
}

func (e *EventBus) EmitError(event string, taskID *int, wtName, errMsg string) {
	e.Emit(event, taskID, wtName, errMsg, nil)
}

func (e *EventBus) EmitExtra(event string, taskID *int, wtName string, extra map[string]any) {
	e.Emit(event, taskID, wtName, "", extra)
}

// FormatTaskID returns a pointer or nil for optional task IDs.
func FormatTaskID(taskID int, hasTask bool) *int {
	if !hasTask {
		return nil
	}
	return &taskID
}

// safeName validates a worktree name.
func safeName(name string) error {
	if len(name) == 0 || len(name) > 40 {
		return fmt.Errorf("invalid worktree name: must be 1-40 chars")
	}
	for _, c := range name {
		if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == '-') {
			return fmt.Errorf("invalid worktree name: only letters, digits, ., _, - allowed")
		}
	}
	return nil
}
