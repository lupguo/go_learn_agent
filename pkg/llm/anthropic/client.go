// Package anthropic implements the LLM Provider interface for the Anthropic Messages API.
//
// Wire format reference: https://docs.anthropic.com/en/api/messages
//
// This client also works with Anthropic-compatible providers (DeepSeek, GLM, Kimi, MiniMax)
// by overriding the BaseURL.
package anthropic

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
	llm.RegisterProvider("anthropic", func(cfg *llm.Config) (llm.Provider, error) {
		var opts []Option
		if cfg.BaseURL != "" {
			opts = append(opts, WithBaseURL(cfg.BaseURL))
		}
		return New(cfg.APIKey, cfg.Model, opts...), nil
	})
}

const defaultBaseURL = "https://api.anthropic.com"
const apiVersion = "2023-06-01"

// Client talks to the Anthropic Messages API (or compatible providers).
type Client struct {
	apiKey  string
	baseURL string
	model   string
	http    *http.Client
}

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the API endpoint (for compatible providers).
func WithBaseURL(url string) Option {
	return func(c *Client) { c.baseURL = url }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.http = hc }
}

// New creates an Anthropic client.
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

// --- Wire types (Anthropic-specific JSON format) ---

type wireRequest struct {
	Model     string        `json:"model"`
	System    string        `json:"system,omitempty"`
	Messages  []wireMessage `json:"messages"`
	Tools     []wireTool    `json:"tools,omitempty"`
	MaxTokens int           `json:"max_tokens"`
}

type wireMessage struct {
	Role    string            `json:"role"`
	Content json.RawMessage   `json:"content"` // string or []wireContentBlock
}

type wireContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Content   any            `json:"content,omitempty"` // string or structured
}

type wireTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

type wireResponse struct {
	Content    []wireContentBlock `json:"content"`
	StopReason string             `json:"stop_reason"`
	Error      *wireError         `json:"error,omitempty"`
}

type wireError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// --- Convert between our types and wire types ---

func toWireMessages(msgs []llm.Message) ([]wireMessage, error) {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		blocks := make([]wireContentBlock, 0, len(m.Content))
		for _, b := range m.Content {
			wb := wireContentBlock{Type: string(b.Type)}
			switch b.Type {
			case llm.ContentTypeText:
				wb.Text = b.Text
			case llm.ContentTypeToolUse:
				wb.ID = b.ID
				wb.Name = b.Name
				wb.Input = b.Input
			case llm.ContentTypeToolResult:
				wb.ToolUseID = b.ToolUseID
				wb.Content = b.Content
			}
			blocks = append(blocks, wb)
		}
		raw, err := json.Marshal(blocks)
		if err != nil {
			return nil, fmt.Errorf("marshal content: %w", err)
		}
		out = append(out, wireMessage{
			Role:    string(m.Role),
			Content: raw,
		})
	}
	return out, nil
}

func fromWireResponse(wr *wireResponse) *llm.Response {
	blocks := make([]llm.ContentBlock, 0, len(wr.Content))
	for _, wb := range wr.Content {
		b := llm.ContentBlock{Type: llm.ContentType(wb.Type)}
		switch llm.ContentType(wb.Type) {
		case llm.ContentTypeText:
			b.Text = wb.Text
		case llm.ContentTypeToolUse:
			b.ID = wb.ID
			b.Name = wb.Name
			b.Input = wb.Input
		}
		blocks = append(blocks, b)
	}
	return &llm.Response{
		Content:    blocks,
		StopReason: llm.StopReason(wr.StopReason),
	}
}

// --- Provider implementation ---

// SendMessage implements llm.Provider.
func (c *Client) SendMessage(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	wireMsgs, err := toWireMessages(req.Messages)
	if err != nil {
		return nil, err
	}

	wireTools := make([]wireTool, 0, len(req.Tools))
	for _, t := range req.Tools {
		wireTools = append(wireTools, wireTool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	body := wireRequest{
		Model:     c.model,
		System:    req.System,
		Messages:  wireMsgs,
		Tools:     wireTools,
		MaxTokens: req.MaxTokens,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

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
