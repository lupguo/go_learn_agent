package s17_autonomous

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var claimLock sync.Mutex

// TaskRecord mirrors s12's task structure for scanning.
type TaskRecord struct {
	ID          int    `json:"id"`
	Subject     string `json:"subject"`
	Description string `json:"description"`
	Status      string `json:"status"`
	Owner       string `json:"owner"`
	BlockedBy   []int  `json:"blockedBy"`
	ClaimRole   string `json:"claim_role,omitempty"`
	ClaimedAt   float64 `json:"claimed_at,omitempty"`
	ClaimSource string  `json:"claim_source,omitempty"`
}

// IsClaimable checks if a task can be claimed by the given role.
func IsClaimable(task *TaskRecord, role string) bool {
	if task.Status != "pending" {
		return false
	}
	if task.Owner != "" {
		return false
	}
	if len(task.BlockedBy) > 0 {
		return false
	}
	if task.ClaimRole != "" && role != "" && task.ClaimRole != role {
		return false
	}
	return true
}

// ScanUnclaimedTasks finds all claimable tasks in the tasks directory.
func ScanUnclaimedTasks(tasksDir, role string) []TaskRecord {
	_ = os.MkdirAll(tasksDir, 0755)
	entries, _ := os.ReadDir(tasksDir)

	var tasks []TaskRecord
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "task_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tasksDir, e.Name()))
		if err != nil {
			continue
		}
		var t TaskRecord
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		if IsClaimable(&t, role) {
			tasks = append(tasks, t)
		}
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	return tasks
}

// ClaimTask atomically claims a task for the given owner.
func ClaimTask(tasksDir string, taskID int, owner, role, source string) string {
	claimLock.Lock()
	defer claimLock.Unlock()

	path := filepath.Join(tasksDir, fmt.Sprintf("task_%d.json", taskID))
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Sprintf("Error: Task %d not found", taskID)
	}

	var task TaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return fmt.Sprintf("Error: parse task %d: %v", taskID, err)
	}

	if !IsClaimable(&task, role) {
		return fmt.Sprintf("Error: Task %d is not claimable for role=%s", taskID, role)
	}

	task.Owner = owner
	task.Status = "in_progress"
	task.ClaimedAt = float64(time.Now().Unix())
	task.ClaimSource = source

	out, _ := json.MarshalIndent(task, "", "  ")
	_ = os.WriteFile(path, out, 0644)

	// Append claim event
	appendClaimEvent(tasksDir, map[string]any{
		"event":   "task.claimed",
		"task_id": taskID,
		"owner":   owner,
		"role":    role,
		"source":  source,
		"ts":      time.Now().Unix(),
	})

	return fmt.Sprintf("Claimed task #%d for %s via %s", taskID, owner, source)
}

func appendClaimEvent(tasksDir string, payload map[string]any) {
	_ = os.MkdirAll(tasksDir, 0755)
	path := filepath.Join(tasksDir, "claim_events.jsonl")
	data, _ := json.Marshal(payload)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(data, '\n'))
}

// ListTasks returns a formatted task board listing.
func ListTasks(tasksDir string) string {
	_ = os.MkdirAll(tasksDir, 0755)
	entries, _ := os.ReadDir(tasksDir)

	var tasks []TaskRecord
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "task_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(tasksDir, e.Name()))
		if err != nil {
			continue
		}
		var t TaskRecord
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}
		tasks = append(tasks, t)
	}

	if len(tasks) == 0 {
		return "No tasks."
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })
	var lines []string
	for _, t := range tasks {
		marker := map[string]string{
			"pending": "[ ]", "in_progress": "[>]", "completed": "[x]",
		}[t.Status]
		if marker == "" {
			marker = "[?]"
		}
		line := fmt.Sprintf("  %s #%d: %s", marker, t.ID, t.Subject)
		if t.Owner != "" {
			line += fmt.Sprintf(" @%s", t.Owner)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
