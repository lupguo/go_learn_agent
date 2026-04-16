package s08_hooks

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s08 agent with hook-aware tool execution.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	hooks     *HookManager
	system    string
	maxTokens int
}

// New creates a new s08 agent.
func New(provider llm.Provider, registry *tool.Registry, hooks *HookManager) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		hooks:     hooks,
		system:    fmt.Sprintf("You are a coding agent at %s. Use tools to solve tasks.", cwd),
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM, fires hooks around tool execution.
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

		hctx := &HookContext{
			ToolName:  call.Name,
			ToolInput: call.Input,
		}

		// --- PreToolUse hooks ---
		preResult := a.hooks.RunHooks(EventPreToolUse, hctx)

		// Inject hook messages
		for _, msg := range preResult.Messages {
			results = append(results, llm.ContentBlock{
				Type:      llm.ContentTypeToolResult,
				ToolUseID: call.ID,
				Content:   fmt.Sprintf("[Hook message]: %s", msg),
			})
		}

		if preResult.Blocked {
			results = append(results, llm.ContentBlock{
				Type:      llm.ContentTypeToolResult,
				ToolUseID: call.ID,
				Content:   fmt.Sprintf("Tool blocked by PreToolUse hook: %s", preResult.BlockReason),
			})
			continue
		}

		// --- Execute tool ---
		fmt.Printf("\033[33m> %s:\033[0m\n", call.Name)
		result := a.registry.Execute(ctx, call)
		output := result.Content
		if len(output) > 200 {
			fmt.Println(output[:200] + "...")
		} else {
			fmt.Println(output)
		}

		// --- PostToolUse hooks ---
		hctx.ToolOutput = result.Content
		postResult := a.hooks.RunHooks(EventPostToolUse, hctx)

		// Append post-hook messages to output
		for _, msg := range postResult.Messages {
			result.Content += fmt.Sprintf("\n[Hook note]: %s", msg)
		}

		results = append(results, result)
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
