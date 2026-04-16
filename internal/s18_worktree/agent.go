package s18_worktree

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s18 worktree-isolation agent.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	system    string
	maxTokens int
}

func New(provider llm.Provider, registry *tool.Registry) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		system:    fmt.Sprintf("You are a coding agent at %s. You can create isolated git worktrees for each task.", cwd),
		maxTokens: 8000,
	}
}

func (a *Agent) Run(ctx context.Context, state *s02_tools.LoopState) error {
	for {
		normalized := s02_tools.NormalizeMessages(state.Messages)
		resp, err := a.provider.SendMessage(ctx, &llm.Request{
			System: a.system, Messages: normalized,
			Tools: a.registry.ToolDefs(), MaxTokens: a.maxTokens,
		})
		if err != nil {
			return fmt.Errorf("API error: %w", err)
		}

		state.Messages = append(state.Messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		if resp.StopReason != llm.StopReasonToolUse {
			return nil
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
			return nil
		}
		state.Messages = append(state.Messages, llm.NewToolResultMessage(results))
		state.TurnCount++
		state.TransitionReason = "tool_result"
	}
}
