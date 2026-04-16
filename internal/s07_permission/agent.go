package s07_permission

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s07 agent with permission-gated tool execution.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	perms     *PermissionManager
	system    string
	maxTokens int
}

// New creates a new s07 agent.
func New(provider llm.Provider, registry *tool.Registry, perms *PermissionManager) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		perms:     perms,
		system:    fmt.Sprintf("You are a coding agent at %s. Use tools to solve tasks.\nThe user controls permissions. Some tool calls may be denied.", cwd),
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM, permission-checks each tool call, then executes.
func (a *Agent) RunOneTurn(ctx context.Context, state *s02_tools.LoopState) (bool, error) {
	normalized := s02_tools.NormalizeMessages(state.Messages)

	resp, err := a.provider.SendMessage(ctx, &llm.Request{
		System:    a.system,
		Messages:  normalized,
		Tools:     a.registry.ToolDefs(),
		MaxTokens: a.maxTokens,
	})
	if err != nil {
		return false, fmt.Errorf("LLM call failed: %w", err)
	}

	state.Messages = append(state.Messages, llm.Message{
		Role:    llm.RoleAssistant,
		Content: resp.Content,
	})

	if resp.StopReason != llm.StopReasonToolUse {
		state.TransitionReason = ""
		return false, nil
	}

	var results []llm.ContentBlock
	for _, call := range resp.Content {
		if call.Type != llm.ContentTypeToolUse {
			continue
		}

		output := a.executeWithPermission(ctx, call)
		results = append(results, llm.ContentBlock{
			Type:      llm.ContentTypeToolResult,
			ToolUseID: call.ID,
			Content:   output,
		})
	}

	if len(results) == 0 {
		state.TransitionReason = ""
		return false, nil
	}

	state.Messages = append(state.Messages, llm.NewToolResultMessage(results))
	state.TurnCount++
	state.TransitionReason = "tool_result"
	return true, nil
}

// executeWithPermission runs the permission pipeline then executes or denies the tool.
func (a *Agent) executeWithPermission(ctx context.Context, call llm.ContentBlock) string {
	decision := a.perms.Check(call.Name, call.Input)

	switch decision.Behavior {
	case "deny":
		fmt.Printf("  \033[31m[DENIED]\033[0m %s: %s\n", call.Name, decision.Reason)
		return fmt.Sprintf("Permission denied: %s", decision.Reason)

	case "ask":
		if askUser(call.Name, call.Input) {
			a.perms.RecordApproval()
			return a.executeTool(ctx, call)
		}
		if warning := a.perms.RecordDenial(); warning != "" {
			fmt.Println("  " + warning)
		}
		fmt.Printf("  \033[33m[USER DENIED]\033[0m %s\n", call.Name)
		return fmt.Sprintf("Permission denied by user for %s", call.Name)

	default: // allow
		return a.executeTool(ctx, call)
	}
}

// executeTool runs the tool and prints output.
func (a *Agent) executeTool(ctx context.Context, call llm.ContentBlock) string {
	fmt.Printf("\033[33m> %s:\033[0m\n", call.Name)
	result := a.registry.Execute(ctx, call)
	output := result.Content
	if len(output) > 200 {
		fmt.Println(output[:200] + "...")
	} else {
		fmt.Println(output)
	}
	return result.Content
}

// Run executes the agent loop.
func (a *Agent) Run(ctx context.Context, state *s02_tools.LoopState) error {
	for {
		cont, err := a.RunOneTurn(ctx, state)
		if err != nil {
			return err
		}
		if !cont {
			return nil
		}
	}
}

// askUser prompts the user interactively to approve a tool call.
func askUser(toolName string, toolInput map[string]any) bool {
	preview, _ := json.Marshal(toolInput)
	previewStr := string(preview)
	if len(previewStr) > 200 {
		previewStr = previewStr[:200] + "..."
	}
	fmt.Printf("\n  [Permission] %s: %s\n", toolName, previewStr)
	fmt.Print("  Allow? (y/n/always): ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(scanner.Text()))

	switch answer {
	case "always":
		return true
	case "y", "yes":
		return true
	default:
		return false
	}
}
