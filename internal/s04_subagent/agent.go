package s04_subagent

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

const maxSubagentTurns = 30

// Agent is the s04 parent agent that can dispatch subagents.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	system    string
	maxTokens int
}

// New creates a new s04 parent agent.
func New(provider llm.Provider, registry *tool.Registry) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		system:    fmt.Sprintf("You are a coding agent at %s. Use the task tool to delegate exploration or subtasks.", cwd),
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM, executes tool calls including subagent dispatch.
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
		fmt.Printf("\033[33m> %s:\033[0m\n", call.Name)
		result := a.registry.Execute(ctx, call)
		output := result.Content
		if len(output) > 200 {
			output = output[:200] + "..."
		}
		fmt.Println(output)
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

// Run executes the parent agent loop.
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

// RunSubagent executes a subagent with fresh context and filtered tools.
// It returns only the final text summary — child context is discarded.
func RunSubagent(ctx context.Context, provider llm.Provider, childRegistry *tool.Registry, prompt string) string {
	cwd, _ := os.Getwd()
	system := fmt.Sprintf("You are a coding subagent at %s. Complete the given task, then summarize your findings.", cwd)

	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, prompt),
	}

	for turn := 0; turn < maxSubagentTurns; turn++ {
		resp, err := provider.SendMessage(ctx, &llm.Request{
			System:    system,
			Messages:  messages,
			Tools:     childRegistry.ToolDefs(),
			MaxTokens: 8000,
		})
		if err != nil {
			return fmt.Sprintf("Subagent error: %v", err)
		}

		messages = append(messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		})

		if resp.StopReason != llm.StopReasonToolUse {
			break
		}

		var results []llm.ContentBlock
		for _, call := range resp.Content {
			if call.Type != llm.ContentTypeToolUse {
				continue
			}
			fmt.Printf("\033[35m  sub> %s\033[0m\n", call.Name)
			result := childRegistry.Execute(ctx, call)
			results = append(results, result)
		}
		if len(results) == 0 {
			break
		}
		messages = append(messages, llm.NewToolResultMessage(results))
	}

	// Extract final text summary — only this returns to the parent
	if len(messages) > 0 {
		if text := messages[len(messages)-1].ExtractText(); text != "" {
			return text
		}
	}
	return "(no summary)"
}
