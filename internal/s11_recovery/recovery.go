// Package s11_recovery implements three error recovery paths for the agent loop.
//
// Strategy 1: max_tokens → inject continuation message, retry (≤3 times)
// Strategy 2: prompt_too_long → auto-compact conversation, retry
// Strategy 3: connection/rate errors → exponential backoff with jitter (≤3 retries)
//
// Key insight: "A robust agent recovers instead of crashing."
package s11_recovery

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"time"

	"github.com/lupguo/go_learn_agent/pkg/llm"
)

const (
	MaxRecoveryAttempts = 3
	BackoffBaseDelay    = 1.0  // seconds
	BackoffMaxDelay     = 30.0 // seconds
	TokenThreshold      = 50000
)

const continuationMessage = "Output limit hit. Continue directly from where you stopped -- " +
	"no recap, no repetition. Pick up mid-sentence if needed."

// EstimateTokens returns a rough token count (~4 chars per token).
func EstimateTokens(messages []llm.Message) int {
	data, _ := json.Marshal(messages)
	return len(data) / 4
}

// BackoffDelay calculates exponential backoff with jitter.
func BackoffDelay(attempt int) time.Duration {
	delay := math.Min(BackoffBaseDelay*math.Pow(2, float64(attempt)), BackoffMaxDelay)
	jitter := rand.Float64()
	return time.Duration((delay + jitter) * float64(time.Second))
}

// AutoCompact asks the LLM to summarize the conversation for continuation.
func AutoCompact(ctx context.Context, provider llm.Provider, messages []llm.Message) []llm.Message {
	data, _ := json.Marshal(messages)
	conversation := string(data)
	if len(conversation) > 80000 {
		conversation = conversation[:80000]
	}

	prompt := "Summarize this conversation for continuity. Include:\n" +
		"1) Task overview and success criteria\n" +
		"2) Current state: completed work, files touched\n" +
		"3) Key decisions and failed approaches\n" +
		"4) Remaining next steps\n" +
		"Be concise but preserve critical details.\n\n" +
		conversation

	var summary string
	resp, err := provider.SendMessage(ctx, &llm.Request{
		Messages:  []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)},
		MaxTokens: 4000,
	})
	if err != nil {
		summary = fmt.Sprintf("(compact failed: %v). Previous context lost.", err)
	} else {
		for _, block := range resp.Content {
			if block.Type == llm.ContentTypeText && block.Text != "" {
				summary = strings.TrimSpace(block.Text)
				break
			}
		}
		if summary == "" {
			summary = "(empty summary)"
		}
	}

	continuation := "This session continues from a previous conversation that was compacted. " +
		"Summary of prior context:\n\n" + summary + "\n\n" +
		"Continue from where we left off without re-asking the user."

	return []llm.Message{llm.NewTextMessage(llm.RoleUser, continuation)}
}

// IsPromptTooLongError checks if an error indicates the prompt exceeded the model's limit.
func IsPromptTooLongError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "overlong_prompt") ||
		(strings.Contains(msg, "prompt") && strings.Contains(msg, "long")) ||
		strings.Contains(msg, "too many tokens")
}

// IsRetryableError checks if an error is a transient connection/rate issue.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "status 500") ||
		strings.Contains(msg, "status 502") ||
		strings.Contains(msg, "status 503") ||
		strings.Contains(msg, "status 529") ||
		strings.Contains(msg, "connection") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "eof")
}
