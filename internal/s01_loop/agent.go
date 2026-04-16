// Package s01_loop implements the core agent loop — the "heartbeat" of any agent.
//
// The pattern:
//
//	user message → LLM reply → tool_use? → execute tools → write results → continue
//
// This is the smallest useful coding agent. Everything else (tools, memory, teams)
// is built on top of this loop.
package s01_loop

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// LoopState tracks the agent loop's state across turns.
// Made explicit (not hidden in closures) so later chapters can extend it.
type LoopState struct {
	Messages         []llm.Message
	TurnCount        int
	TransitionReason string // why we entered the current turn ("tool_result", "")
}

// Agent is the s01 minimal agent.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	system    string
	maxTokens int
}

// New creates a new s01 agent.
func New(provider llm.Provider, registry *tool.Registry) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider: provider,
		registry: registry,
		system: fmt.Sprintf(
			"You are a coding agent at %s. "+
				"Use bash to inspect and change the workspace. Act first, then report clearly.",
			cwd,
		),
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM, executes any tool calls, and returns
// whether the loop should continue (true = more tool calls to process).
func (a *Agent) RunOneTurn(ctx context.Context, state *LoopState) (bool, error) {
	resp, err := a.provider.SendMessage(ctx, &llm.Request{
		System:    a.system,
		Messages:  state.Messages,
		Tools:     a.registry.ToolDefs(),
		MaxTokens: a.maxTokens,
	})
	if err != nil {
		return false, fmt.Errorf("LLM call failed: %w", err)
	}

	// Append assistant response to history
	state.Messages = append(state.Messages, llm.Message{
		Role:    llm.RoleAssistant,
		Content: resp.Content,
	})

	// If the model didn't request tool use, the turn is done
	if resp.StopReason != llm.StopReasonToolUse {
		state.TransitionReason = ""
		return false, nil
	}

	// Execute all tool calls and collect results
	var results []llm.ContentBlock
	for _, call := range resp.Content {
		if call.Type != llm.ContentTypeToolUse {
			continue
		}
		// Print the tool call for visibility
		fmt.Printf("\033[33m[tool: %s]\033[0m", call.Name)
		if cmd, ok := call.Input["command"]; ok {
			fmt.Printf(" \033[33m$ %s\033[0m", cmd)
		}
		fmt.Println()

		result := a.registry.Execute(ctx, call)
		// Print truncated output
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

	// Append tool results as a user message
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
