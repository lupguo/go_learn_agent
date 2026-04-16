// Package s08_hooks implements an extension point system around the agent loop.
//
// Hook events: SessionStart, PreToolUse, PostToolUse
// Exit code contract: 0=continue, 1=block, 2=inject message
//
// Key insight: "Extend the agent without touching the loop."
package s08_hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const hookTimeout = 30 * time.Second

// HookEvent names the points where hooks can fire.
type HookEvent string

const (
	EventSessionStart HookEvent = "SessionStart"
	EventPreToolUse   HookEvent = "PreToolUse"
	EventPostToolUse  HookEvent = "PostToolUse"
)

// HookDef is one hook entry from the config file.
type HookDef struct {
	Matcher string `json:"matcher"` // tool name filter ("*" or specific name)
	Command string `json:"command"` // shell command to run
}

// HookResult is the aggregate outcome of running all hooks for an event.
type HookResult struct {
	Blocked     bool
	BlockReason string
	Messages    []string
}

// HookContext carries information passed to hook scripts via environment variables.
type HookContext struct {
	ToolName   string
	ToolInput  map[string]any
	ToolOutput string // only set for PostToolUse
}

// HookManager loads and executes hooks from .hooks.json configuration.
type HookManager struct {
	hooks   map[HookEvent][]HookDef
	workDir string
	sdkMode bool
}

// NewHookManager loads hook config from the given path (defaults to .hooks.json in workDir).
func NewHookManager(workDir string, configPath string) *HookManager {
	hm := &HookManager{
		hooks: map[HookEvent][]HookDef{
			EventSessionStart: {},
			EventPreToolUse:   {},
			EventPostToolUse:  {},
		},
		workDir: workDir,
	}

	if configPath == "" {
		configPath = filepath.Join(workDir, ".hooks.json")
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return hm
	}

	var config struct {
		Hooks map[string][]HookDef `json:"hooks"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		fmt.Printf("[Hook config error: %v]\n", err)
		return hm
	}

	for _, event := range []HookEvent{EventSessionStart, EventPreToolUse, EventPostToolUse} {
		if defs, ok := config.Hooks[string(event)]; ok {
			hm.hooks[event] = defs
		}
	}
	fmt.Printf("[Hooks loaded from %s]\n", configPath)
	return hm
}

// RunHooks executes all hooks for the given event and returns the aggregate result.
func (hm *HookManager) RunHooks(event HookEvent, hctx *HookContext) HookResult {
	result := HookResult{}

	// Trust gate: only run hooks if workspace is trusted
	if !hm.isWorkspaceTrusted() {
		return result
	}

	defs := hm.hooks[event]
	for _, def := range defs {
		// Check matcher (tool name filter)
		if def.Matcher != "" && def.Matcher != "*" && hctx != nil {
			if def.Matcher != hctx.ToolName {
				continue
			}
		}
		if def.Command == "" {
			continue
		}

		hr := hm.runOneHook(event, def, hctx)
		if hr.Blocked {
			result.Blocked = true
			result.BlockReason = hr.BlockReason
		}
		result.Messages = append(result.Messages, hr.Messages...)
	}

	return result
}

func (hm *HookManager) runOneHook(event HookEvent, def HookDef, hctx *HookContext) HookResult {
	result := HookResult{}

	ctx, cancel := context.WithTimeout(context.Background(), hookTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", def.Command)
	cmd.Dir = hm.workDir

	// Build environment with hook context
	cmd.Env = append(os.Environ(), "HOOK_EVENT="+string(event))
	if hctx != nil {
		cmd.Env = append(cmd.Env, "HOOK_TOOL_NAME="+hctx.ToolName)
		if hctx.ToolInput != nil {
			inputJSON, _ := json.Marshal(hctx.ToolInput)
			s := string(inputJSON)
			if len(s) > 10000 {
				s = s[:10000]
			}
			cmd.Env = append(cmd.Env, "HOOK_TOOL_INPUT="+s)
		}
		if hctx.ToolOutput != "" {
			out := hctx.ToolOutput
			if len(out) > 10000 {
				out = out[:10000]
			}
			cmd.Env = append(cmd.Env, "HOOK_TOOL_OUTPUT="+out)
		}
	}

	stdout, stderr := &strings.Builder{}, &strings.Builder{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		fmt.Printf("  [hook:%s] Timeout (%s)\n", event, hookTimeout)
		return result
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			fmt.Printf("  [hook:%s] Error: %v\n", event, err)
			return result
		}
	}

	switch exitCode {
	case 0:
		// Continue — optionally print stdout
		if s := strings.TrimSpace(stdout.String()); s != "" {
			fmt.Printf("  [hook:%s] %s\n", event, truncate(s, 100))
		}
	case 1:
		// Block
		result.Blocked = true
		result.BlockReason = strings.TrimSpace(stderr.String())
		if result.BlockReason == "" {
			result.BlockReason = "Blocked by hook"
		}
		fmt.Printf("  [hook:%s] BLOCKED: %s\n", event, truncate(result.BlockReason, 200))
	case 2:
		// Inject message
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			result.Messages = append(result.Messages, msg)
			fmt.Printf("  [hook:%s] INJECT: %s\n", event, truncate(msg, 200))
		}
	}

	return result
}

// isWorkspaceTrusted checks for the trust marker file.
func (hm *HookManager) isWorkspaceTrusted() bool {
	if hm.sdkMode {
		return true
	}
	trustMarker := filepath.Join(hm.workDir, ".claude", ".claude_trusted")
	_, err := os.Stat(trustMarker)
	return err == nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
