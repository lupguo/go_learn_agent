package s16_protocols

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s16 lead agent with team protocol support.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	bus       *MessageBus
	system    string
	maxTokens int
}

// New creates a new s16 lead agent.
func New(provider llm.Provider, registry *tool.Registry, bus *MessageBus) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		bus:       bus,
		system:    fmt.Sprintf("You are a team lead at %s. Manage teammates with shutdown and plan approval protocols.", cwd),
		maxTokens: 8000,
	}
}

// Run executes the lead agent loop with inbox drain.
func (a *Agent) Run(ctx context.Context, state *s02_tools.LoopState) error {
	for {
		// Drain lead's inbox
		inbox := a.bus.ReadInbox("lead")
		if len(inbox) > 0 {
			data, _ := json.MarshalIndent(inbox, "", "  ")
			state.Messages = append(state.Messages,
				llm.NewTextMessage(llm.RoleUser, fmt.Sprintf("<inbox>%s</inbox>", string(data))))
			fmt.Printf("\033[35m[inbox] %d message(s) received\033[0m\n", len(inbox))
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
