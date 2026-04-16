package s02_tools

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// LoopState tracks the agent loop state across turns.
type LoopState struct {
	Messages         []llm.Message
	TurnCount        int
	TransitionReason string
}

// Agent is the s02 agent with expanded tools and message normalization.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	system    string
	maxTokens int
}

// New creates a new s02 agent.
func New(provider llm.Provider, registry *tool.Registry) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		system:    fmt.Sprintf("You are a coding agent at %s. Use tools to solve tasks. Act, don't explain.", cwd),
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM, executes tool calls, returns whether to continue.
func (a *Agent) RunOneTurn(ctx context.Context, state *LoopState) (bool, error) {
	// Key s02 addition: normalize messages before every API call
	normalized := NormalizeMessages(state.Messages)

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

// Run executes the agent loop until the model stops requesting tools.
func (a *Agent) Run(ctx context.Context, state *LoopState) error {
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
