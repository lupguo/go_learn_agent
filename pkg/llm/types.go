// Package llm defines the core types and interfaces for LLM provider abstraction.
//
// The key design decision: all providers speak a unified message format.
// Anthropic, OpenAI, etc. each have their own wire format, but the agent loop
// only sees these types. Provider implementations handle the translation.
package llm

import "context"

// Role represents the sender of a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// ContentType distinguishes text from tool interactions in a message.
type ContentType string

const (
	ContentTypeText       ContentType = "text"
	ContentTypeToolUse    ContentType = "tool_use"
	ContentTypeToolResult ContentType = "tool_result"
)

// ContentBlock is one piece of a message — text, tool call, or tool result.
type ContentBlock struct {
	Type ContentType `json:"type"`

	// Text content (when Type == ContentTypeText)
	Text string `json:"text,omitempty"`

	// Tool use (when Type == ContentTypeToolUse)
	ID    string         `json:"id,omitempty"`
	Name  string         `json:"name,omitempty"`
	Input map[string]any `json:"input,omitempty"`

	// Tool result (when Type == ContentTypeToolResult)
	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
}

// Message is one turn in the conversation.
type Message struct {
	Role    Role           `json:"role"`
	Content []ContentBlock `json:"content"`
}

// NewTextMessage creates a simple text message.
func NewTextMessage(role Role, text string) Message {
	return Message{
		Role: role,
		Content: []ContentBlock{
			{Type: ContentTypeText, Text: text},
		},
	}
}

// NewToolResultMessage creates a message carrying tool execution results.
func NewToolResultMessage(results []ContentBlock) Message {
	return Message{
		Role:    RoleUser,
		Content: results,
	}
}

// ExtractText returns all text content from a message, joined by newlines.
func (m Message) ExtractText() string {
	var texts []string
	for _, block := range m.Content {
		if block.Type == ContentTypeText && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	if len(texts) == 0 {
		return ""
	}
	result := texts[0]
	for _, t := range texts[1:] {
		result += "\n" + t
	}
	return result
}

// ToolCalls returns all tool_use blocks from a message.
func (m Message) ToolCalls() []ContentBlock {
	var calls []ContentBlock
	for _, block := range m.Content {
		if block.Type == ContentTypeToolUse {
			calls = append(calls, block)
		}
	}
	return calls
}

// StopReason indicates why the model stopped generating.
type StopReason string

const (
	StopReasonEndTurn   StopReason = "end_turn"
	StopReasonToolUse   StopReason = "tool_use"
	StopReasonMaxTokens StopReason = "max_tokens"
)

// ToolDef describes a tool the model can call.
type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
}

// Request is what the agent loop sends to the LLM.
type Request struct {
	System    string    `json:"system"`
	Messages  []Message `json:"messages"`
	Tools     []ToolDef `json:"tools,omitempty"`
	MaxTokens int       `json:"max_tokens"`
}

// Response is what the LLM returns.
type Response struct {
	Content    []ContentBlock `json:"content"`
	StopReason StopReason     `json:"stop_reason"`
}

// HasToolUse returns true if the response includes any tool calls.
func (r Response) HasToolUse() bool {
	for _, block := range r.Content {
		if block.Type == ContentTypeToolUse {
			return true
		}
	}
	return false
}

// Provider is the core abstraction — any LLM backend implements this.
type Provider interface {
	SendMessage(ctx context.Context, req *Request) (*Response, error)
}
