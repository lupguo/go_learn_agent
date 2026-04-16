package s09_memory

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

const memoryGuidance = `
When to save memories:
- User states a preference ("I like tabs", "always use pytest") -> type: user
- User corrects you ("don't do X", "that was wrong because...") -> type: feedback
- You learn a project fact not easy to infer from current code alone -> type: project
- You learn where an external resource lives (ticket board, dashboard) -> type: reference

When NOT to save:
- Anything easily derivable from code (function signatures, file structure)
- Temporary task state (current branch, open PR numbers)
- Secrets or credentials
`

// Agent is the s09 agent with memory-aware system prompt.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	memory    *MemoryManager
	maxTokens int
}

// New creates a new s09 agent.
func New(provider llm.Provider, registry *tool.Registry, memory *MemoryManager) *Agent {
	return &Agent{
		provider:  provider,
		registry:  registry,
		memory:    memory,
		maxTokens: 8000,
	}
}

// buildSystemPrompt assembles the system prompt with current memory content.
// Rebuilt each turn so newly saved memories are visible immediately.
func (a *Agent) buildSystemPrompt() string {
	cwd, _ := os.Getwd()
	parts := fmt.Sprintf("You are a coding agent at %s. Use tools to solve tasks.", cwd)

	if memSection := a.memory.MemoryPrompt(); memSection != "" {
		parts += "\n\n" + memSection
	}

	parts += "\n" + memoryGuidance
	return parts
}

// RunOneTurn sends messages to the LLM and executes tool calls.
func (a *Agent) RunOneTurn(ctx context.Context, state *s02_tools.LoopState) (bool, error) {
	normalized := s02_tools.NormalizeMessages(state.Messages)

	resp, err := a.provider.SendMessage(ctx, &llm.Request{
		System:    a.buildSystemPrompt(),
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
