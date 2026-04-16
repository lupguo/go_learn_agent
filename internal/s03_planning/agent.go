package s03_planning

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

const planReminderInterval = 3

// Agent is the s03 agent — extends s02 with session-level planning.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	plan      *PlanManager
	system    string
	maxTokens int
}

// New creates a new s03 agent with planning support.
func New(provider llm.Provider, registry *tool.Registry, plan *PlanManager) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		plan:      plan,
		system:    fmt.Sprintf("You are a coding agent at %s.\nUse the todo tool for multi-step work.\nKeep exactly one step in_progress when a task has multiple steps.\nRefresh the plan as work advances. Prefer tools over prose.", cwd),
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM, executes tool calls, and tracks plan usage.
func (a *Agent) RunOneTurn(ctx context.Context, state *s02_tools.LoopState) (bool, error) {
	// Build system prompt: base + current plan state
	systemPrompt := a.system
	if a.plan.HasItems() {
		systemPrompt += "\n\nCurrent plan:\n" + a.plan.Render()
	}

	normalized := s02_tools.NormalizeMessages(state.Messages)

	resp, err := a.provider.SendMessage(ctx, &llm.Request{
		System:    systemPrompt,
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

	// Execute tools and track whether todo was used this turn
	var results []llm.ContentBlock
	usedTodo := false
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

		if call.Name == todoToolName {
			usedTodo = true
		}
	}

	if len(results) == 0 {
		state.TransitionReason = ""
		return false, nil
	}

	// Plan freshness tracking: nudge the model if it forgets to update
	if usedTodo {
		// Update() already reset rounds_since_update inside PlanManager
	} else {
		a.plan.NoteRoundWithoutUpdate()
		if reminder := a.plan.Reminder(); reminder != "" {
			// Inject reminder as a text block before tool results
			results = append([]llm.ContentBlock{
				{Type: llm.ContentTypeText, Text: reminder},
			}, results...)
		}
	}

	state.Messages = append(state.Messages, llm.NewToolResultMessage(results))
	state.TurnCount++
	state.TransitionReason = "tool_result"
	return true, nil
}

// Run executes the agent loop until the model stops requesting tools.
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
