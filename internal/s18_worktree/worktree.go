package s18_worktree

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WorktreeEntry is one worktree record in the index.
type WorktreeEntry struct {
	Name      string  `json:"name"`
	Path      string  `json:"path"`
	Branch    string  `json:"branch"`
	TaskID    *int    `json:"task_id"`
	Status    string  `json:"status"` // active, removed, kept
	CreatedAt float64 `json:"created_at"`
	// Optional tracking fields
	RemovedAt      float64     `json:"removed_at,omitempty"`
	KeptAt         float64     `json:"kept_at,omitempty"`
	LastEnteredAt  float64     `json:"last_entered_at,omitempty"`
	LastCommandAt  float64     `json:"last_command_at,omitempty"`
	LastCmdPreview string      `json:"last_command_preview,omitempty"`
	Closeout       *CloseoutInfo `json:"closeout,omitempty"`
}

// WorktreeIndex is the .worktrees/index.json structure.
type WorktreeIndex struct {
	Worktrees []WorktreeEntry `json:"worktrees"`
}

// WorktreeManager manages git worktrees for task isolation.
type WorktreeManager struct {
	repoRoot     string
	dir          string
	indexPath    string
	tasks        *TaskManager
	events       *EventBus
	gitAvailable bool
	mu           sync.Mutex
}

func NewWorktreeManager(repoRoot string, tasks *TaskManager, events *EventBus) *WorktreeManager {
	dir := filepath.Join(repoRoot, ".worktrees")
	_ = os.MkdirAll(dir, 0755)
	indexPath := filepath.Join(dir, "index.json")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		_ = os.WriteFile(indexPath, []byte(`{"worktrees":[]}`), 0644)
	}
	wm := &WorktreeManager{
		repoRoot:  repoRoot,
		dir:       dir,
		indexPath: indexPath,
		tasks:     tasks,
		events:    events,
	}
	wm.gitAvailable = wm.checkGit()
	return wm
}

func (wm *WorktreeManager) GitAvailable() bool { return wm.gitAvailable }

func (wm *WorktreeManager) checkGit() bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = wm.repoRoot
	out, err := cmd.CombinedOutput()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func (wm *WorktreeManager) runGit(args ...string) (string, error) {
	if !wm.gitAvailable {
		return "", fmt.Errorf("not in a git repository")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = wm.repoRoot
	out, err := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if err != nil {
		if result == "" {
			result = fmt.Sprintf("git %s failed", strings.Join(args, " "))
		}
		return "", fmt.Errorf("%s", result)
	}
	if result == "" {
		result = "(no output)"
	}
	return result, nil
}

func (wm *WorktreeManager) loadIndex() WorktreeIndex {
	data, _ := os.ReadFile(wm.indexPath)
	var idx WorktreeIndex
	_ = json.Unmarshal(data, &idx)
	return idx
}

func (wm *WorktreeManager) saveIndex(idx WorktreeIndex) {
	data, _ := json.MarshalIndent(idx, "", "  ")
	_ = os.WriteFile(wm.indexPath, data, 0644)
}

func (wm *WorktreeManager) find(name string) *WorktreeEntry {
	idx := wm.loadIndex()
	for i := range idx.Worktrees {
		if idx.Worktrees[i].Name == name {
			return &idx.Worktrees[i]
		}
	}
	return nil
}

func (wm *WorktreeManager) updateEntry(name string, fn func(*WorktreeEntry)) error {
	idx := wm.loadIndex()
	found := false
	for i := range idx.Worktrees {
		if idx.Worktrees[i].Name == name {
			fn(&idx.Worktrees[i])
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("worktree '%s' not found in index", name)
	}
	wm.saveIndex(idx)
	return nil
}

// Create creates a git worktree and optionally binds it to a task.
func (wm *WorktreeManager) Create(name string, taskID *int, baseRef string) (string, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	if err := safeName(name); err != nil {
		return "", err
	}
	if wm.find(name) != nil {
		return "", fmt.Errorf("worktree '%s' already exists", name)
	}
	if taskID != nil && !wm.tasks.Exists(*taskID) {
		return "", fmt.Errorf("task %d not found", *taskID)
	}
	if baseRef == "" {
		baseRef = "HEAD"
	}

	path := filepath.Join(wm.dir, name)
	branch := "wt/" + name

	wm.events.Emit("worktree.create.before", taskID, name, "", nil)

	if _, err := wm.runGit("worktree", "add", "-b", branch, path, baseRef); err != nil {
		wm.events.EmitError("worktree.create.failed", taskID, name, err.Error())
		return "", err
	}

	entry := WorktreeEntry{
		Name: name, Path: path, Branch: branch,
		TaskID: taskID, Status: "active", CreatedAt: float64(time.Now().Unix()),
	}
	idx := wm.loadIndex()
	idx.Worktrees = append(idx.Worktrees, entry)
	wm.saveIndex(idx)

	if taskID != nil {
		wm.tasks.BindWorktree(*taskID, name, "")
	}

	wm.events.Emit("worktree.create.after", taskID, name, "", nil)
	return toJSON(entry), nil
}

// ListAll returns all worktrees in the index.
func (wm *WorktreeManager) ListAll() string {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	idx := wm.loadIndex()
	if len(idx.Worktrees) == 0 {
		return "No worktrees in index."
	}
	var lines []string
	for _, wt := range idx.Worktrees {
		line := fmt.Sprintf("[%s] %s -> %s (%s)", wt.Status, wt.Name, wt.Path, wt.Branch)
		if wt.TaskID != nil {
			line += fmt.Sprintf(" task=%d", *wt.TaskID)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// Status shows git status for a worktree.
func (wm *WorktreeManager) Status(name string) string {
	wm.mu.Lock()
	wt := wm.find(name)
	wm.mu.Unlock()
	if wt == nil {
		return fmt.Sprintf("Error: Unknown worktree '%s'", name)
	}
	if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
		return fmt.Sprintf("Error: Worktree path missing: %s", wt.Path)
	}
	cmd := exec.Command("git", "status", "--short", "--branch")
	cmd.Dir = wt.Path
	out, _ := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))
	if result == "" {
		return "Clean worktree"
	}
	return result
}

// Enter marks a worktree as entered.
func (wm *WorktreeManager) Enter(name string) string {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	wt := wm.find(name)
	if wt == nil {
		return fmt.Sprintf("Error: Unknown worktree '%s'", name)
	}
	if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
		return fmt.Sprintf("Error: Worktree path missing: %s", wt.Path)
	}
	_ = wm.updateEntry(name, func(e *WorktreeEntry) {
		e.LastEnteredAt = float64(time.Now().Unix())
	})
	wm.events.Emit("worktree.enter", wt.TaskID, name, "", map[string]any{"path": wt.Path})
	return toJSON(wm.find(name))
}

// Run executes a command inside a worktree directory.
func (wm *WorktreeManager) Run(name, command string) string {
	dangerous := []string{"rm -rf /", "sudo", "shutdown", "reboot", "> /dev/"}
	for _, d := range dangerous {
		if strings.Contains(command, d) {
			return "Error: Dangerous command blocked"
		}
	}

	wm.mu.Lock()
	wt := wm.find(name)
	wm.mu.Unlock()
	if wt == nil {
		return fmt.Sprintf("Error: Unknown worktree '%s'", name)
	}
	if _, err := os.Stat(wt.Path); os.IsNotExist(err) {
		return fmt.Sprintf("Error: Worktree path missing: %s", wt.Path)
	}

	preview := command
	if len(preview) > 120 {
		preview = preview[:120]
	}

	wm.mu.Lock()
	_ = wm.updateEntry(name, func(e *WorktreeEntry) {
		e.LastEnteredAt = float64(time.Now().Unix())
		e.LastCommandAt = float64(time.Now().Unix())
		e.LastCmdPreview = preview
	})
	wm.events.EmitExtra("worktree.run.before", wt.TaskID, name, map[string]any{"command": preview})
	wm.mu.Unlock()

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = wt.Path
	out, _ := cmd.CombinedOutput()
	result := strings.TrimSpace(string(out))

	wm.events.Emit("worktree.run.after", wt.TaskID, name, "", nil)

	if result == "" {
		return "(no output)"
	}
	if len(result) > 50000 {
		result = result[:50000]
	}
	return result
}

// Remove removes a worktree and optionally completes its task.
func (wm *WorktreeManager) Remove(name string, force, completeTask bool, reason string) (string, error) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wt := wm.find(name)
	if wt == nil {
		return fmt.Sprintf("Error: Unknown worktree '%s'", name), nil
	}
	taskID := wt.TaskID

	wm.events.Emit("worktree.remove.before", taskID, name, "", nil)

	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, wt.Path)

	if _, err := wm.runGit(args...); err != nil {
		wm.events.EmitError("worktree.remove.failed", taskID, name, err.Error())
		return "", err
	}

	if completeTask && taskID != nil {
		s := "completed"
		wm.tasks.Update(*taskID, &s, nil)
		wm.events.EmitTask("task.completed", *taskID, name)
	}
	if taskID != nil {
		wm.tasks.RecordCloseout(*taskID, "removed", reason, false)
	}

	_ = wm.updateEntry(name, func(e *WorktreeEntry) {
		e.Status = "removed"
		e.RemovedAt = float64(time.Now().Unix())
		e.Closeout = &CloseoutInfo{Action: "remove", Reason: reason, At: float64(time.Now().Unix())}
	})

	wm.events.Emit("worktree.remove.after", taskID, name, "", nil)
	return fmt.Sprintf("Removed worktree '%s'", name), nil
}

// Keep marks a worktree as kept without removing it.
func (wm *WorktreeManager) Keep(name string) string {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wt := wm.find(name)
	if wt == nil {
		return fmt.Sprintf("Error: Unknown worktree '%s'", name)
	}
	if wt.TaskID != nil {
		wm.tasks.RecordCloseout(*wt.TaskID, "kept", "", true)
	}
	_ = wm.updateEntry(name, func(e *WorktreeEntry) {
		e.Status = "kept"
		e.KeptAt = float64(time.Now().Unix())
		e.Closeout = &CloseoutInfo{Action: "keep", At: float64(time.Now().Unix())}
	})
	wm.events.Emit("worktree.keep", wt.TaskID, name, "", nil)
	return toJSON(wm.find(name))
}

// Closeout provides a unified keep-or-remove decision.
func (wm *WorktreeManager) Closeout(name, action, reason string, force, completeTask bool) (string, error) {
	switch action {
	case "keep":
		wm.mu.Lock()
		wt := wm.find(name)
		wm.mu.Unlock()
		if wt == nil {
			return fmt.Sprintf("Error: Unknown worktree '%s'", name), nil
		}
		if wt.TaskID != nil {
			wm.tasks.RecordCloseout(*wt.TaskID, "kept", reason, true)
			if completeTask {
				s := "completed"
				wm.tasks.Update(*wt.TaskID, &s, nil)
			}
		}
		wm.mu.Lock()
		_ = wm.updateEntry(name, func(e *WorktreeEntry) {
			e.Status = "kept"
			e.KeptAt = float64(time.Now().Unix())
			e.Closeout = &CloseoutInfo{Action: "keep", Reason: reason, At: float64(time.Now().Unix())}
		})
		wm.events.EmitExtra("worktree.closeout.keep", wt.TaskID, name, map[string]any{"reason": reason})
		result := toJSON(wm.find(name))
		wm.mu.Unlock()
		return result, nil
	case "remove":
		wm.events.EmitExtra("worktree.closeout.remove", nil, name, map[string]any{"reason": reason})
		return wm.Remove(name, force, completeTask, reason)
	default:
		return "", fmt.Errorf("action must be 'keep' or 'remove'")
	}
}
