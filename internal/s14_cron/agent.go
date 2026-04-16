package s14_cron

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s14 agent with cron scheduling support.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	scheduler *CronScheduler
	system    string
	maxTokens int
}

// New creates a new s14 agent.
func New(provider llm.Provider, registry *tool.Registry, scheduler *CronScheduler) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		scheduler: scheduler,
		system: fmt.Sprintf("You are a coding agent at %s. Use tools to solve tasks.\n\n"+
			"You can schedule future work with cron_create. Tasks fire automatically "+
			"and their prompts are injected into the conversation.", cwd),
		maxTokens: 8000,
	}
}

// Run executes the agent loop with cron notification drain.
func (a *Agent) Run(ctx context.Context, state *s02_tools.LoopState) error {
	for {
		// Drain cron notifications and inject before LLM call
		notifs := a.scheduler.DrainNotifications()
		for _, note := range notifs {
			preview := note
			if len(preview) > 100 {
				preview = preview[:100]
			}
			fmt.Printf("\033[35m[Cron notification] %s\033[0m\n", preview)
			state.Messages = append(state.Messages,
				llm.NewTextMessage(llm.RoleUser, note))
		}

		normalized := s02_tools.NormalizeMessages(state.Messages)

		resp, err := a.provider.SendMessage(ctx, &llm.Request{
			System:    a.system,
			Messages:  normalized,
			Tools:     a.registry.ToolDefs(),
			MaxTokens: a.maxTokens,
		})
		if err != nil {
			return fmt.Errorf("API error: %w", err)
		}

		state.Messages = append(state.Messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		})

		if resp.StopReason != llm.StopReasonToolUse {
			return nil
		}

		// Execute tools
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
