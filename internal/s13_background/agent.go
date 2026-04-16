package s13_background

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s13 agent with background task execution.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	bgManager *BackgroundManager
	system    string
	maxTokens int
}

// New creates a new s13 agent.
func New(provider llm.Provider, registry *tool.Registry, bgManager *BackgroundManager) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		bgManager: bgManager,
		system:    fmt.Sprintf("You are a coding agent at %s. Use background_run for long-running commands.", cwd),
		maxTokens: 8000,
	}
}

// Run executes the agent loop with background notification drain.
func (a *Agent) Run(ctx context.Context, state *s02_tools.LoopState) error {
	for {
		// Drain background notifications and inject before LLM call
		notifs := a.bgManager.DrainNotifications()
		if len(notifs) > 0 && len(state.Messages) > 0 {
			var lines []string
			for _, n := range notifs {
				lines = append(lines, fmt.Sprintf("[bg:%s] %s: %s (output_file=%s)",
					n.TaskID, n.Status, n.Preview, n.OutputFile))
			}
			notifText := fmt.Sprintf("<background-results>\n%s\n</background-results>",
				strings.Join(lines, "\n"))
			state.Messages = append(state.Messages,
				llm.NewTextMessage(llm.RoleUser, notifText))
			fmt.Printf("\033[35m[bg] %d notification(s) injected\033[0m\n", len(notifs))
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
