package s01_loop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// BashTool executes shell commands in the workspace.
type BashTool struct {
	workDir string
	timeout time.Duration
}

// NewBashTool creates a bash tool rooted at the current working directory.
func NewBashTool() *BashTool {
	cwd, _ := os.Getwd()
	return &BashTool{
		workDir: cwd,
		timeout: 120 * time.Second,
	}
}

func (b *BashTool) Name() string        { return "bash" }
func (b *BashTool) Description() string  { return "Run a shell command in the current workspace." }
func (b *BashTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
		},
		"required": []string{"command"},
	}
}

// dangerousPatterns is a minimal blocklist for obviously destructive commands.
var dangerousPatterns = []string{
	"rm -rf /",
	"sudo",
	"shutdown",
	"reboot",
	"> /dev/",
}

func (b *BashTool) Execute(ctx context.Context, input map[string]any) (string, error) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "", fmt.Errorf("missing or empty 'command' parameter")
	}

	// Basic safety check
	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return "Error: Dangerous command blocked", nil
		}
	}

	// Run with timeout
	ctx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = b.workDir

	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "Error: Timeout (120s)", nil
	}
	if err != nil {
		// Include the output even on non-zero exit — it often has the error message
		if len(output) > 0 {
			return strings.TrimSpace(string(output)), nil
		}
		return fmt.Sprintf("Error: %v", err), nil
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		return "(no output)", nil
	}
	// Truncate very large output
	if len(result) > 50000 {
		return result[:50000], nil
	}
	return result, nil
}
