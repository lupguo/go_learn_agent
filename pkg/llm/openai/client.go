// Package openai implements the LLM Provider interface for the OpenAI Chat Completions API.
//
// Wire format reference: https://platform.openai.com/docs/api-reference/chat
package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/lupguo/go_learn_agent/pkg/llm"
)

func init() {
	llm.RegisterProvider("openai", func(cfg *llm.Config) (llm.Provider, error) {
		var opts []Option
		if cfg.BaseURL != "" {
			opts = append(opts, WithBaseURL(cfg.BaseURL))
		}
		return New(cfg.APIKey, cfg.Model, opts...), nil
	})
}

const defaultBaseURL = "https://api.openai.com"

// Client talks to the OpenAI Chat Completions API (or compatible providers).
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API endpoint.
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// New creates an OpenAI client.
func New(apiKey, model string, opts ...Option) *Client {
	c := &Client{
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
		model:   model,
		http:    http.DefaultClient,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// --- Wire types (OpenAI-specific JSON format) ---

type wireRequest struct {
	Model    string        `json:"model"`
	Messages []wireMessage `json:"messages"`
	Tools    []wireTool    `json:"tools,omitempty"`
}

type wireMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content,omitempty"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function wireFunction `json:"function"`
}

type wireFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON string, not object
}

type wireTool struct {
	Type     string             `json:"type"`
	Function wireFunctionSchema `json:"function"`
}

type wireFunctionSchema struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters"`
}

type wireResponse struct {
	Choices []wireChoice `json:"choices"`
	Error   *wireError   `json:"error,omitempty"`
}

type wireChoice struct {
	Message      wireMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type wireError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

// --- Convert between our types and wire types ---

func toWireMessages(msgs []llm.Message) []wireMessage {
	var out []wireMessage
	for _, m := range msgs {
		// Check if this message contains tool results → split into separate role=tool messages.
		var toolResults []llm.ContentBlock
		var textParts []string
		var toolCalls []wireToolCall

		for _, b := range m.Content {
			switch b.Type {
			case llm.ContentTypeText:
				textParts = append(textParts, b.Text)
			case llm.ContentTypeToolUse:
				argsJSON, _ := json.Marshal(b.Input)
				toolCalls = append(toolCalls, wireToolCall{
					ID:   b.ID,
					Type: "function",
					Function: wireFunction{
						Name:      b.Name,
						Arguments: string(argsJSON),
					},
				})
			case llm.ContentTypeToolResult:
				toolResults = append(toolResults, b)
			}
		}

		// Emit assistant message with text and/or tool_calls.
		if m.Role == llm.RoleAssistant {
			wm := wireMessage{Role: "assistant"}
			if len(textParts) > 0 {
				wm.Content = joinStrings(textParts)
			}
			if len(toolCalls) > 0 {
				wm.ToolCalls = toolCalls
			}
			out = append(out, wm)
			continue
		}

		// Emit tool result messages as separate role=tool messages.
		if len(toolResults) > 0 {
			for _, tr := range toolResults {
				out = append(out, wireMessage{
					Role:       "tool",
					Content:    tr.Content,
					ToolCallID: tr.ToolUseID,
				})
			}
			continue
		}

		// Regular user message.
		out = append(out, wireMessage{
			Role:    string(m.Role),
			Content: joinStrings(textParts),
		})
	}
	return out
}

func fromWireResponse(wr *wireResponse) *llm.Response {
	if len(wr.Choices) == 0 {
		return &llm.Response{StopReason: llm.StopReasonEndTurn}
	}
	choice := wr.Choices[0]

	var blocks []llm.ContentBlock
	if choice.Message.Content != "" {
		blocks = append(blocks, llm.ContentBlock{
			Type: llm.ContentTypeText,
			Text: choice.Message.Content,
		})
	}
	for _, tc := range choice.Message.ToolCalls {
		var input map[string]any
		_ = json.Unmarshal([]byte(tc.Function.Arguments), &input)
		blocks = append(blocks, llm.ContentBlock{
			Type:  llm.ContentTypeToolUse,
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: input,
		})
	}

	stopReason := mapFinishReason(choice.FinishReason)
	return &llm.Response{Content: blocks, StopReason: stopReason}
}

func mapFinishReason(reason string) llm.StopReason {
	switch reason {
	case "stop":
		return llm.StopReasonEndTurn
	case "tool_calls":
		return llm.StopReasonToolUse
	case "length":
		return llm.StopReasonMaxTokens
	default:
		return llm.StopReasonEndTurn
	}
}

// --- Provider implementation ---

// SendMessage implements llm.Provider.
func (c *Client) SendMessage(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	wireMsgs := toWireMessages(req.Messages)

	// Prepend system message if provided.
	if req.System != "" {
		wireMsgs = append([]wireMessage{{Role: "system", Content: req.System}}, wireMsgs...)
	}

	wireTools := make([]wireTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		wireTools = append(wireTools, wireTool{
			Type: "function",
			Function: wireFunctionSchema{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		})
	}

	body := wireRequest{
		Model:    c.model,
		Messages: wireMsgs,
		Tools:    wireTools,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var wireResp wireResponse
	if err := json.Unmarshal(respBody, &wireResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	if wireResp.Error != nil {
		return nil, fmt.Errorf("API error: [%s] %s", wireResp.Error.Type, wireResp.Error.Message)
	}

	return fromWireResponse(&wireResp), nil
}

func joinStrings(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	result := parts[0]
	for _, p := range parts[1:] {
		result += "\n" + p
	}
	return result
}
