// Package gemini implements the LLM Provider interface for the Google Gemini API.
//
// Wire format reference: https://ai.google.dev/api/generate-content
package gemini

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
	llm.RegisterProvider("gemini", func(cfg *llm.Config) (llm.Provider, error) {
		var opts []Option
		if cfg.BaseURL != "" {
			opts = append(opts, WithBaseURL(cfg.BaseURL))
		}
		return New(cfg.APIKey, cfg.Model, opts...), nil
	})
}

const defaultBaseURL = "https://generativelanguage.googleapis.com"

// Client talks to the Gemini generateContent API.
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

// New creates a Gemini client.
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

// --- Wire types (Gemini-specific JSON format) ---

type wireRequest struct {
	Contents          []wireContent          `json:"contents"`
	Tools             []wireToolDecl         `json:"tools,omitempty"`
	SystemInstruction *wireContent           `json:"systemInstruction,omitempty"`
}

type wireContent struct {
	Role  string     `json:"role,omitempty"`
	Parts []wirePart `json:"parts"`
}

// wirePart uses pointer fields so zero values are omitted.
type wirePart struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *wireFunctionCall `json:"functionCall,omitempty"`
	FunctionResponse *wireFuncResponse `json:"functionResponse,omitempty"`
}

type wireFunctionCall struct {
	Name string         `json:"name"`
	Args map[string]any `json:"args,omitempty"`
}

type wireFuncResponse struct {
	Name     string         `json:"name"`
	Response map[string]any `json:"response"`
}

type wireToolDecl struct {
	FunctionDeclarations []wireFuncDecl `json:"functionDeclarations"`
}

type wireFuncDecl struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  any    `json:"parameters,omitempty"`
}

type wireResponse struct {
	Candidates []wireCandidate `json:"candidates"`
	Error      *wireError      `json:"error,omitempty"`
}

type wireCandidate struct {
	Content      wireContent `json:"content"`
	FinishReason string      `json:"finishReason"`
}

type wireError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- Convert between our types and wire types ---

func toWireContents(msgs []llm.Message) []wireContent {
	var out []wireContent
	for _, m := range msgs {
		role := "user"
		if m.Role == llm.RoleAssistant {
			role = "model"
		}

		var parts []wirePart
		for _, b := range m.Content {
			switch b.Type {
			case llm.ContentTypeText:
				parts = append(parts, wirePart{Text: b.Text})
			case llm.ContentTypeToolUse:
				parts = append(parts, wirePart{
					FunctionCall: &wireFunctionCall{Name: b.Name, Args: b.Input},
				})
			case llm.ContentTypeToolResult:
				// Gemini uses function name, not ID. We store name in Content field
				// as "name:content" for round-trip, or fall back to ToolUseID.
				name := b.ToolUseID // use as fallback identifier
				resp := map[string]any{"result": b.Content}
				parts = append(parts, wirePart{
					FunctionResponse: &wireFuncResponse{Name: name, Response: resp},
				})
			}
		}
		if len(parts) > 0 {
			out = append(out, wireContent{Role: role, Parts: parts})
		}
	}
	return out
}

func fromWireResponse(wr *wireResponse) *llm.Response {
	if len(wr.Candidates) == 0 {
		return &llm.Response{StopReason: llm.StopReasonEndTurn}
	}
	cand := wr.Candidates[0]

	var blocks []llm.ContentBlock
	hasFunctionCall := false
	for _, p := range cand.Content.Parts {
		if p.Text != "" {
			blocks = append(blocks, llm.ContentBlock{
				Type: llm.ContentTypeText,
				Text: p.Text,
			})
		}
		if p.FunctionCall != nil {
			hasFunctionCall = true
			blocks = append(blocks, llm.ContentBlock{
				Type:  llm.ContentTypeToolUse,
				ID:    p.FunctionCall.Name, // Gemini uses name as ID
				Name:  p.FunctionCall.Name,
				Input: p.FunctionCall.Args,
			})
		}
	}

	stopReason := mapFinishReason(cand.FinishReason, hasFunctionCall)
	return &llm.Response{Content: blocks, StopReason: stopReason}
}

func mapFinishReason(reason string, hasFunctionCall bool) llm.StopReason {
	// Gemini signals tool use via functionCall parts, not finish_reason.
	if hasFunctionCall {
		return llm.StopReasonToolUse
	}
	switch reason {
	case "STOP":
		return llm.StopReasonEndTurn
	case "MAX_TOKENS":
		return llm.StopReasonMaxTokens
	default:
		return llm.StopReasonEndTurn
	}
}

// --- Provider implementation ---

// SendMessage implements llm.Provider.
func (c *Client) SendMessage(ctx context.Context, req *llm.Request) (*llm.Response, error) {
	wireContents := toWireContents(req.Messages)

	body := wireRequest{
		Contents: wireContents,
	}

	// System prompt goes into systemInstruction.
	if req.System != "" {
		body.SystemInstruction = &wireContent{
			Parts: []wirePart{{Text: req.System}},
		}
	}

	// Convert tools.
	if len(req.Tools) > 0 {
		var decls []wireFuncDecl
		for _, t := range req.Tools {
			decls = append(decls, wireFuncDecl{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			})
		}
		body.Tools = []wireToolDecl{{FunctionDeclarations: decls}}
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

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
		return nil, fmt.Errorf("API error: [%d] %s", wireResp.Error.Code, wireResp.Error.Message)
	}

	return fromWireResponse(&wireResp), nil
}
