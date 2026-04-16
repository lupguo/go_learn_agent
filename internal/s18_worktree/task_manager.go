package s18_worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WTaskRecord extends the s12 task with worktree binding fields.
type WTaskRecord struct {
	ID            int         `json:"id"`
	Subject       string      `json:"subject"`
	Description   string      `json:"description"`
	Status        string      `json:"status"`
	Owner         string      `json:"owner"`
	Worktree      string      `json:"worktree"`
	WorktreeState string      `json:"worktree_state"` // unbound, active, kept, removed
	LastWorktree  string      `json:"last_worktree"`
	Closeout      *CloseoutInfo `json:"closeout"`
	BlockedBy     []int       `json:"blockedBy"`
	CreatedAt     float64     `json:"created_at"`
	UpdatedAt     float64     `json:"updated_at"`
}

type CloseoutInfo struct {
	Action string  `json:"action"`
	Reason string  `json:"reason"`
	At     float64 `json:"at"`
}

// TaskManager handles persistent tasks with worktree binding.
type TaskManager struct {
	dir    string
	nextID int
	mu     sync.Mutex
}

func NewTaskManager(dir string) (*TaskManager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	m := &TaskManager{dir: dir}
	m.nextID = m.maxID() + 1
	return m, nil
}

func (m *TaskManager) maxID() int {
	entries, _ := os.ReadDir(m.dir)
	maxVal := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "task_") || !strings.HasSuffix(name, ".json") {
			continue
		}
		idStr := strings.TrimSuffix(strings.TrimPrefix(name, "task_"), ".json")
		if id, err := strconv.Atoi(idStr); err == nil && id > maxVal {
			maxVal = id
		}
	}
	return maxVal
}

func (m *TaskManager) taskPath(id int) string {
	return filepath.Join(m.dir, fmt.Sprintf("task_%d.json", id))
}

func (m *TaskManager) load(id int) (*WTaskRecord, error) {
	data, err := os.ReadFile(m.taskPath(id))
	if err != nil {
		return nil, fmt.Errorf("task %d not found", id)
	}
	var t WTaskRecord
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (m *TaskManager) save(t *WTaskRecord) {
	data, _ := json.MarshalIndent(t, "", "  ")
	_ = os.WriteFile(m.taskPath(t.ID), data, 0644)
}

func (m *TaskManager) Exists(id int) bool {
	_, err := os.Stat(m.taskPath(id))
	return err == nil
}

func (m *TaskManager) Create(subject, description string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := float64(time.Now().Unix())
	t := &WTaskRecord{
		ID: m.nextID, Subject: subject, Description: description,
		Status: "pending", WorktreeState: "unbound", BlockedBy: []int{},
		CreatedAt: now, UpdatedAt: now,
	}
	m.save(t)
	m.nextID++
	return toJSON(t), nil
}

func (m *TaskManager) Get(id int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.load(id)
	if err != nil {
		return "", err
	}
	return toJSON(t), nil
}

func (m *TaskManager) Update(id int, status, owner *string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.load(id)
	if err != nil {
		return "", err
	}
	if status != nil {
		t.Status = *status
	}
	if owner != nil {
		t.Owner = *owner
	}
	t.UpdatedAt = float64(time.Now().Unix())
	m.save(t)
	return toJSON(t), nil
}

func (m *TaskManager) BindWorktree(id int, worktree, owner string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.load(id)
	if err != nil {
		return "", err
	}
	t.Worktree = worktree
	t.LastWorktree = worktree
	t.WorktreeState = "active"
	if owner != "" {
		t.Owner = owner
	}
	if t.Status == "pending" {
		t.Status = "in_progress"
	}
	t.UpdatedAt = float64(time.Now().Unix())
	m.save(t)
	return toJSON(t), nil
}

func (m *TaskManager) RecordCloseout(id int, action, reason string, keepBinding bool) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, err := m.load(id)
	if err != nil {
		return "", err
	}
	t.Closeout = &CloseoutInfo{Action: action, Reason: reason, At: float64(time.Now().Unix())}
	t.WorktreeState = action
	if !keepBinding {
		t.Worktree = ""
	}
	t.UpdatedAt = float64(time.Now().Unix())
	m.save(t)
	return toJSON(t), nil
}

func (m *TaskManager) ListAll() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	entries, _ := os.ReadDir(m.dir)
	var tasks []WTaskRecord
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "task_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue
		}
		var t WTaskRecord
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
		marker := map[string]string{"pending": "[ ]", "in_progress": "[>]", "completed": "[x]", "deleted": "[-]"}[t.Status]
		if marker == "" {
			marker = "[?]"
		}
		line := fmt.Sprintf("%s #%d: %s", marker, t.ID, t.Subject)
		if t.Owner != "" {
			line += fmt.Sprintf(" owner=%s", t.Owner)
		}
		if t.Worktree != "" {
			line += fmt.Sprintf(" wt=%s", t.Worktree)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func toJSON(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}
