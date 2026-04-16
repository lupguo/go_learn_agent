package s13_background

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

const (
	StallThreshold  = 45 * time.Second
	CommandTimeout  = 300 * time.Second
	OutputMaxLen    = 50000
	PreviewMaxLen   = 500
)

// Notification is a completion event pushed to the queue by a background goroutine.
type Notification struct {
	TaskID     string `json:"task_id"`
	Status     string `json:"status"`
	Command    string `json:"command"`
	Preview    string `json:"preview"`
	OutputFile string `json:"output_file"`
}

// TaskInfo tracks a single background execution.
type TaskInfo struct {
	ID            string   `json:"id"`
	Status        string   `json:"status"`
	Command       string   `json:"command"`
	StartedAt     float64  `json:"started_at"`
	FinishedAt    *float64 `json:"finished_at"`
	ResultPreview string   `json:"result_preview"`
	OutputFile    string   `json:"output_file"`
}

// BackgroundManager manages goroutine-based background command execution.
type BackgroundManager struct {
	dir     string
	workDir string
	tasks   map[string]*TaskInfo
	notifs  []Notification
	mu      sync.Mutex
	nextSeq int
}

// NewBackgroundManager creates a new manager with the given runtime directory.
func NewBackgroundManager(workDir, runtimeDir string) (*BackgroundManager, error) {
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return nil, fmt.Errorf("create runtime dir: %w", err)
	}
	return &BackgroundManager{
		dir:     runtimeDir,
		workDir: workDir,
		tasks:   make(map[string]*TaskInfo),
	}, nil
}

// Run starts a command in a background goroutine and returns immediately.
func (m *BackgroundManager) Run(command string) string {
	m.mu.Lock()
	m.nextSeq++
	taskID := fmt.Sprintf("bg_%04d", m.nextSeq)
	outputFile := filepath.Join(m.dir, taskID+".log")
	relOutput, _ := filepath.Rel(m.workDir, outputFile)

	info := &TaskInfo{
		ID:         taskID,
		Status:     "running",
		Command:    command,
		StartedAt:  float64(time.Now().Unix()),
		OutputFile: relOutput,
	}
	m.tasks[taskID] = info
	m.persistTask(taskID)
	m.mu.Unlock()

	go m.execute(taskID, command)

	cmdPreview := command
	if len(cmdPreview) > 80 {
		cmdPreview = cmdPreview[:80]
	}
	return fmt.Sprintf("Background task %s started: %s (output_file=%s)", taskID, cmdPreview, relOutput)
}

// execute is the goroutine target: run subprocess, capture output, push notification.
func (m *BackgroundManager) execute(taskID, command string) {
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = m.workDir

	var output string
	var status string

	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// Command ran but returned non-zero
			output = string(outBytes)
			status = "completed"
		} else if strings.Contains(err.Error(), "signal: killed") {
			output = "Error: Timeout (300s)"
			status = "timeout"
		} else {
			output = fmt.Sprintf("Error: %v\n%s", err, string(outBytes))
			status = "error"
		}
	} else {
		output = string(outBytes)
		status = "completed"
	}

	output = strings.TrimSpace(output)
	if output == "" {
		output = "(no output)"
	}
	if len(output) > OutputMaxLen {
		output = output[:OutputMaxLen]
	}

	preview := preview(output)

	// Write output to log file
	m.mu.Lock()
	info := m.tasks[taskID]
	outputPath := filepath.Join(m.dir, taskID+".log")
	_ = os.WriteFile(outputPath, []byte(output), 0644)

	now := float64(time.Now().Unix())
	info.Status = status
	info.FinishedAt = &now
	info.ResultPreview = preview
	m.persistTask(taskID)

	relOutput, _ := filepath.Rel(m.workDir, outputPath)
	cmdPreview := info.Command
	if len(cmdPreview) > 80 {
		cmdPreview = cmdPreview[:80]
	}
	m.notifs = append(m.notifs, Notification{
		TaskID:     taskID,
		Status:     status,
		Command:    cmdPreview,
		Preview:    preview,
		OutputFile: relOutput,
	})
	m.mu.Unlock()
}

// Check returns status of a single task (by ID) or all tasks.
func (m *BackgroundManager) Check(taskID string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if taskID != "" {
		info, ok := m.tasks[taskID]
		if !ok {
			return fmt.Sprintf("Error: Unknown task %s", taskID)
		}
		data, _ := json.MarshalIndent(map[string]any{
			"id":             info.ID,
			"status":         info.Status,
			"command":        info.Command,
			"result_preview": info.ResultPreview,
			"output_file":    info.OutputFile,
		}, "", "  ")
		return string(data)
	}

	if len(m.tasks) == 0 {
		return "No background tasks."
	}

	var lines []string
	for _, info := range m.tasks {
		rp := info.ResultPreview
		if rp == "" {
			rp = "(running)"
		}
		lines = append(lines, fmt.Sprintf("%s: [%s] %s -> %s",
			info.ID, info.Status, truncate(info.Command, 60), rp))
	}
	return strings.Join(lines, "\n")
}

// DrainNotifications returns all pending completion notifications and clears the queue.
func (m *BackgroundManager) DrainNotifications() []Notification {
	m.mu.Lock()
	defer m.mu.Unlock()

	notifs := make([]Notification, len(m.notifs))
	copy(notifs, m.notifs)
	m.notifs = m.notifs[:0]
	return notifs
}

// DetectStalled returns task IDs that have been running longer than StallThreshold.
func (m *BackgroundManager) DetectStalled() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := float64(time.Now().Unix())
	var stalled []string
	for id, info := range m.tasks {
		if info.Status != "running" {
			continue
		}
		if now-info.StartedAt > StallThreshold.Seconds() {
			stalled = append(stalled, id)
		}
	}
	return stalled
}

func (m *BackgroundManager) persistTask(taskID string) {
	info := m.tasks[taskID]
	data, _ := json.MarshalIndent(info, "", "  ")
	path := filepath.Join(m.dir, taskID+".json")
	_ = os.WriteFile(path, data, 0644)
}

// --- helpers ---

func preview(output string) string {
	compact := strings.Join(strings.Fields(output), " ")
	if len(compact) > PreviewMaxLen {
		compact = compact[:PreviewMaxLen]
	}
	return compact
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
