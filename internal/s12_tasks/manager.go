package s12_tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	StatusPending    TaskStatus = "pending"
	StatusInProgress TaskStatus = "in_progress"
	StatusCompleted  TaskStatus = "completed"
	StatusDeleted    TaskStatus = "deleted"
)

// TaskRecord is a single durable work item stored as JSON on disk.
type TaskRecord struct {
	ID          int        `json:"id"`
	Subject     string     `json:"subject"`
	Description string     `json:"description"`
	Status      TaskStatus `json:"status"`
	BlockedBy   []int      `json:"blockedBy"`
	Blocks      []int      `json:"blocks"`
	Owner       string     `json:"owner"`
}

// TaskManager provides CRUD for a persistent task graph in .tasks/ directory.
type TaskManager struct {
	dir    string
	nextID int
	mu     sync.Mutex
}

// NewTaskManager creates a TaskManager, ensuring the directory exists.
func NewTaskManager(dir string) (*TaskManager, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create tasks dir: %w", err)
	}
	m := &TaskManager{dir: dir}
	m.nextID = m.maxID() + 1
	return m, nil
}

// maxID scans existing task files to find the highest ID.
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

func (m *TaskManager) load(id int) (*TaskRecord, error) {
	data, err := os.ReadFile(m.taskPath(id))
	if err != nil {
		return nil, fmt.Errorf("task %d not found", id)
	}
	var task TaskRecord
	if err := json.Unmarshal(data, &task); err != nil {
		return nil, fmt.Errorf("parse task %d: %w", id, err)
	}
	return &task, nil
}

func (m *TaskManager) save(task *TaskRecord) error {
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.taskPath(task.ID), data, 0644)
}

// Create adds a new task and returns its JSON representation.
func (m *TaskManager) Create(subject, description string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task := &TaskRecord{
		ID:          m.nextID,
		Subject:     subject,
		Description: description,
		Status:      StatusPending,
		BlockedBy:   []int{},
		Blocks:      []int{},
		Owner:       "",
	}
	if err := m.save(task); err != nil {
		return "", err
	}
	m.nextID++
	return toJSON(task), nil
}

// Get returns the full JSON of a single task.
func (m *TaskManager) Get(taskID int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.load(taskID)
	if err != nil {
		return "", err
	}
	return toJSON(task), nil
}

// Update modifies a task's status, owner, or dependencies.
func (m *TaskManager) Update(taskID int, status *TaskStatus, owner *string,
	addBlockedBy []int, addBlocks []int) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	task, err := m.load(taskID)
	if err != nil {
		return "", err
	}

	if owner != nil {
		task.Owner = *owner
	}

	if status != nil {
		switch *status {
		case StatusPending, StatusInProgress, StatusCompleted, StatusDeleted:
			// valid
		default:
			return "", fmt.Errorf("invalid status: %s", *status)
		}
		task.Status = *status

		// When completed, remove this task from all other tasks' blockedBy
		if *status == StatusCompleted {
			m.clearDependency(taskID)
		}
	}

	if len(addBlockedBy) > 0 {
		task.BlockedBy = uniqueAppend(task.BlockedBy, addBlockedBy)
	}

	if len(addBlocks) > 0 {
		task.Blocks = uniqueAppend(task.Blocks, addBlocks)
		// Bidirectional: also update the blocked tasks' blockedBy lists
		for _, blockedID := range addBlocks {
			blocked, err := m.load(blockedID)
			if err != nil {
				continue
			}
			if !containsInt(blocked.BlockedBy, taskID) {
				blocked.BlockedBy = append(blocked.BlockedBy, taskID)
				_ = m.save(blocked)
			}
		}
	}

	if err := m.save(task); err != nil {
		return "", err
	}
	return toJSON(task), nil
}

// clearDependency removes completedID from all tasks' blockedBy lists.
func (m *TaskManager) clearDependency(completedID int) {
	entries, _ := os.ReadDir(m.dir)
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "task_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue
		}
		var task TaskRecord
		if err := json.Unmarshal(data, &task); err != nil {
			continue
		}
		if removeInt(&task.BlockedBy, completedID) {
			_ = m.save(&task)
		}
	}
}

// ListAll returns a formatted summary of all tasks.
func (m *TaskManager) ListAll() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, _ := os.ReadDir(m.dir)
	var tasks []TaskRecord
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), "task_") || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, e.Name()))
		if err != nil {
			continue
		}
		var task TaskRecord
		if err := json.Unmarshal(data, &task); err != nil {
			continue
		}
		tasks = append(tasks, task)
	}

	if len(tasks) == 0 {
		return "No tasks."
	}

	sort.Slice(tasks, func(i, j int) bool { return tasks[i].ID < tasks[j].ID })

	var lines []string
	for _, t := range tasks {
		marker := map[TaskStatus]string{
			StatusPending:    "[ ]",
			StatusInProgress: "[>]",
			StatusCompleted:  "[x]",
			StatusDeleted:    "[-]",
		}[t.Status]
		if marker == "" {
			marker = "[?]"
		}
		line := fmt.Sprintf("%s #%d: %s", marker, t.ID, t.Subject)
		if t.Owner != "" {
			line += fmt.Sprintf(" owner=%s", t.Owner)
		}
		if len(t.BlockedBy) > 0 {
			line += fmt.Sprintf(" (blocked by: %v)", t.BlockedBy)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// --- helpers ---

func toJSON(v any) string {
	data, _ := json.MarshalIndent(v, "", "  ")
	return string(data)
}

func uniqueAppend(base, add []int) []int {
	set := make(map[int]bool, len(base))
	for _, v := range base {
		set[v] = true
	}
	for _, v := range add {
		if !set[v] {
			base = append(base, v)
			set[v] = true
		}
	}
	return base
}

func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func removeInt(slice *[]int, val int) bool {
	for i, v := range *slice {
		if v == val {
			*slice = append((*slice)[:i], (*slice)[i+1:]...)
			return true
		}
	}
	return false
}
