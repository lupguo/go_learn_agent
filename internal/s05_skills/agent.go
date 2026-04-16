package s05_skills

import (
	"context"
	"fmt"
	"os"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s05 agent with skill loading support.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	skills    *SkillRegistry
	system    string
	maxTokens int
}

// New creates a new s05 agent.
func New(provider llm.Provider, registry *tool.Registry, skills *SkillRegistry) *Agent {
	cwd, _ := os.Getwd()
	system := fmt.Sprintf("You are a coding agent at %s.\nUse load_skill when a task needs specialized instructions before you act.\n\nSkills available:\n%s",
		cwd, skills.DescribeAvailable())

	return &Agent{
		provider:  provider,
		registry:  registry,
		skills:    skills,
		system:    system,
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM and executes tool calls.
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
