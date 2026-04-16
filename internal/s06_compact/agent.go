package s06_compact

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s06 agent with context compaction support.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	state     *CompactState
	workDir   string
	system    string
	maxTokens int
}

// New creates a new s06 agent.
func New(provider llm.Provider, registry *tool.Registry, state *CompactState, workDir string) *Agent {
	return &Agent{
		provider:  provider,
		registry:  registry,
		state:     state,
		workDir:   workDir,
		system:    fmt.Sprintf("You are a coding agent at %s.\nKeep working step by step, and use compact if the conversation gets too long.", workDir),
		maxTokens: 8000,
	}
}

// RunOneTurn sends messages to the LLM, executes tools, and manages compaction.
func (a *Agent) RunOneTurn(ctx context.Context, loopState *s02_tools.LoopState) (bool, error) {
	// Strategy 2: micro-compact older tool results
	loopState.Messages = MicroCompact(loopState.Messages)

	// Strategy 3: auto-compact if context too large
	if EstimateContextSize(loopState.Messages) > ContextLimit {
		fmt.Println("[auto compact]")
		compacted, err := a.compactHistory(ctx, loopState.Messages, "")
		if err != nil {
			return false, fmt.Errorf("auto-compact failed: %w", err)
		}
		loopState.Messages = loopState.Messages[:0]
		loopState.Messages = append(loopState.Messages, compacted...)
	}

	normalized := s02_tools.NormalizeMessages(loopState.Messages)

	resp, err := a.provider.SendMessage(ctx, &llm.Request{
		System:    a.system,
		Messages:  normalized,
		Tools:     a.registry.ToolDefs(),
		MaxTokens: a.maxTokens,
	})
	if err != nil {
		return false, fmt.Errorf("LLM call failed: %w", err)
	}

	loopState.Messages = append(loopState.Messages, llm.Message{
		Role:    llm.RoleAssistant,
		Content: resp.Content,
	})

	if resp.StopReason != llm.StopReasonToolUse {
		loopState.TransitionReason = ""
		return false, nil
	}

	// Execute tools, detect manual compact, and persist large outputs
	var results []llm.ContentBlock
	manualCompact := false
	compactFocus := ""

	for _, call := range resp.Content {
		if call.Type != llm.ContentTypeToolUse {
			continue
		}
		fmt.Printf("\033[33m> %s:\033[0m\n", call.Name)

		result := a.registry.Execute(ctx, call)

		// Strategy 1: persist large output to disk
		if call.Name == "bash" || call.Name == "read_file" {
			result.Content = PersistLargeOutput(a.workDir, call.ID, result.Content)
		}

		// Track file access for compact context
		if call.Name == "read_file" {
			if path, ok := call.Input["path"].(string); ok {
				a.state.TrackRecentFile(path)
			}
		}

		if call.Name == "compact" {
			manualCompact = true
			if f, ok := call.Input["focus"].(string); ok {
				compactFocus = f
			}
		}

		output := result.Content
		if len(output) > 200 {
			output = output[:200] + "..."
		}
		fmt.Println(output)
		results = append(results, result)
	}

	if len(results) == 0 {
		loopState.TransitionReason = ""
		return false, nil
	}

	loopState.Messages = append(loopState.Messages, llm.NewToolResultMessage(results))
	loopState.TurnCount++
	loopState.TransitionReason = "tool_result"

	// Handle manual compact after appending results
	if manualCompact {
		fmt.Println("[manual compact]")
		compacted, err := a.compactHistory(ctx, loopState.Messages, compactFocus)
		if err != nil {
			return false, fmt.Errorf("manual compact failed: %w", err)
		}
		loopState.Messages = loopState.Messages[:0]
		loopState.Messages = append(loopState.Messages, compacted...)
	}

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

// compactHistory saves a transcript, summarizes the conversation, and returns
// a fresh single-message history to continue from.
func (a *Agent) compactHistory(ctx context.Context, messages []llm.Message, focus string) ([]llm.Message, error) {
	// Save transcript before compacting
	transcriptPath := WriteTranscript(a.workDir, messages)
	if transcriptPath != "" {
		fmt.Printf("[transcript saved: %s]\n", transcriptPath)
	}

	// Ask LLM to summarize
	summary, err := a.summarizeHistory(ctx, messages)
	if err != nil {
		return nil, err
	}

	if focus != "" {
		summary += "\n\nFocus to preserve next: " + focus
	}
	if len(a.state.RecentFiles) > 0 {
		var lines []string
		for _, f := range a.state.RecentFiles {
			lines = append(lines, "- "+f)
		}
		summary += "\n\nRecent files to reopen if needed:\n" + strings.Join(lines, "\n")
	}

	a.state.HasCompacted = true
	a.state.LastSummary = summary

	return []llm.Message{
		llm.NewTextMessage(llm.RoleUser,
			"This conversation was compacted so the agent can continue working.\n\n"+summary),
	}, nil
}

// summarizeHistory asks the LLM to produce a compact summary of the conversation.
func (a *Agent) summarizeHistory(ctx context.Context, messages []llm.Message) (string, error) {
	data, _ := json.Marshal(messages)
	conversation := string(data)
	if len(conversation) > 80000 {
		conversation = conversation[:80000]
	}

	prompt := "Summarize this coding-agent conversation so work can continue.\n" +
		"Preserve:\n" +
		"1. The current goal\n" +
		"2. Important findings and decisions\n" +
		"3. Files read or changed\n" +
		"4. Remaining work\n" +
		"5. User constraints and preferences\n" +
		"Be compact but concrete.\n\n" +
		conversation

	resp, err := a.provider.SendMessage(ctx, &llm.Request{
		Messages:  []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)},
		MaxTokens: 2000,
	})
	if err != nil {
		return "", fmt.Errorf("summarize failed: %w", err)
	}

	for _, block := range resp.Content {
		if block.Type == llm.ContentTypeText && block.Text != "" {
			return strings.TrimSpace(block.Text), nil
		}
	}
	return "(empty summary)", nil
}
