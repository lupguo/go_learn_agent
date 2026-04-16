package s02_tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// BashTool executes shell commands in the workspace.
type BashTool struct {
	workDir string
	timeout time.Duration
}

func NewBashTool(workDir string) *BashTool {
	return &BashTool{workDir: workDir, timeout: 120 * time.Second}
}

func (b *BashTool) Name() string       { return "bash" }
func (b *BashTool) Description() string { return "Run a shell command in the current workspace." }
func (b *BashTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
		},
		"required": []string{"command"},
	}
}

var dangerousPatterns = []string{"rm -rf /", "sudo", "shutdown", "reboot", "> /dev/"}

func (b *BashTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("missing or empty 'command' parameter")
	}
	for _, p := range dangerousPatterns {
		if strings.Contains(command, p) {
			return "Error: Dangerous command blocked", nil
		}
	}
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = b.workDir
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "Error: Timeout (120s)", nil
	}
	if err != nil {
		if len(output) > 0 {
			return strings.TrimSpace(string(output)), nil
		}
		return fmt.Sprintf("Error: %v", err), nil
	}
	result := strings.TrimSpace(string(output))
	if result == "" {
		return "(no output)", nil
	}
	if len(result) > 50000 {
		return result[:50000], nil
	}
	return result, nil
}
