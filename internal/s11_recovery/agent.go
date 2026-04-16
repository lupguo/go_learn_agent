package s11_recovery

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s11 agent with error recovery.
type Agent struct {
	provider  llm.Provider
	registry  *tool.Registry
	system    string
	maxTokens int
}

// New creates a new s11 agent.
func New(provider llm.Provider, registry *tool.Registry) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider:  provider,
		registry:  registry,
		system:    fmt.Sprintf("You are a coding agent at %s. Use tools to solve tasks.", cwd),
		maxTokens: 8000,
	}
}

// Run executes the agent loop with all three recovery strategies.
func (a *Agent) Run(ctx context.Context, state *s02_tools.LoopState) error {
	maxOutputRecoveryCount := 0

	for {
		// Proactive auto-compact if context is getting large
		if EstimateTokens(state.Messages) > TokenThreshold {
			fmt.Println("[Recovery] Token estimate exceeds threshold. Auto-compacting...")
			state.Messages = AutoCompact(ctx, a.provider, state.Messages)
		}

		// --- API call with retry ---
		normalized := s02_tools.NormalizeMessages(state.Messages)
		var resp *llm.Response

		for attempt := 0; attempt <= MaxRecoveryAttempts; attempt++ {
			var err error
			resp, err = a.provider.SendMessage(ctx, &llm.Request{
				System:    a.system,
				Messages:  normalized,
				Tools:     a.registry.ToolDefs(),
				MaxTokens: a.maxTokens,
			})

			if err == nil {
				break // success
			}

			// Strategy 2: prompt_too_long → compact and retry
			if IsPromptTooLongError(err) {
				fmt.Printf("[Recovery] Prompt too long. Compacting... (attempt %d)\n", attempt+1)
				state.Messages = AutoCompact(ctx, a.provider, state.Messages)
				normalized = s02_tools.NormalizeMessages(state.Messages)
				continue
			}

			// Strategy 3: retryable errors → backoff
			if IsRetryableError(err) && attempt < MaxRecoveryAttempts {
				delay := BackoffDelay(attempt)
				fmt.Printf("[Recovery] API error: %v. Retrying in %v (attempt %d/%d)\n",
					err, delay.Round(100*time.Millisecond), attempt+1, MaxRecoveryAttempts)
				time.Sleep(delay)
				continue
			}

			// Non-retryable or exhausted
			return fmt.Errorf("API call failed after %d retries: %w", attempt, err)
		}

		if resp == nil {
			return fmt.Errorf("no response received")
		}

		state.Messages = append(state.Messages, llm.Message{
			Role:    llm.RoleAssistant,
			Content: resp.Content,
		})

		// --- Strategy 1: max_tokens recovery ---
		if resp.StopReason == llm.StopReasonMaxTokens {
			maxOutputRecoveryCount++
			if maxOutputRecoveryCount <= MaxRecoveryAttempts {
				fmt.Printf("[Recovery] max_tokens hit (%d/%d). Injecting continuation...\n",
					maxOutputRecoveryCount, MaxRecoveryAttempts)
				state.Messages = append(state.Messages,
					llm.NewTextMessage(llm.RoleUser, continuationMessage))
				continue
			}
			fmt.Printf("[Error] max_tokens recovery exhausted (%d attempts). Stopping.\n", MaxRecoveryAttempts)
			return nil
		}

		// Reset on non-max_tokens response
		maxOutputRecoveryCount = 0

		// --- Normal end_turn ---
		if resp.StopReason != llm.StopReasonToolUse {
			return nil
		}

		// --- Execute tools ---
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
