// Package tool defines the Tool interface and registry for agent tool dispatch.
//
// Key design: Tools implement an interface (not a function map like Python).
// This gives us compile-time type safety and makes testing easy via mock tools.
package tool

import (
	"context"
	"fmt"

	"github.com/lupguo/go_learn_agent/pkg/llm"
)

// Tool is the interface every agent tool implements.
type Tool interface {
	// Name returns the tool name (used in LLM tool_use blocks).
	Name() string
	// Description returns a human-readable description for the LLM.
	Description() string
	// Schema returns the JSON Schema for the tool's input parameters.
	Schema() any
	// Execute runs the tool with the given input and returns a text result.
	Execute(ctx context.Context, input map[string]any) (string, error)
}

// Registry manages tool registration and dispatch.
type Registry struct {
	tools map[string]Tool
	order []string // preserve registration order
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(t Tool) {
	name := t.Name()
	if _, exists := r.tools[name]; !exists {
		r.order = append(r.order, name)
	}
	r.tools[name] = t
}

// Get looks up a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// ToolDefs returns LLM-compatible tool definitions for all registered tools.
func (r *Registry) ToolDefs() []llm.ToolDef {
	defs := make([]llm.ToolDef, 0, len(r.order))
	for _, name := range r.order {
		t := r.tools[name]
		defs = append(defs, llm.ToolDef{
			Name:        t.Name(),
			Description: t.Description(),
			InputSchema: t.Schema(),
		})
	}
	return defs
}

// Execute dispatches a tool call and returns the result as a ContentBlock.
func (r *Registry) Execute(ctx context.Context, call llm.ContentBlock) llm.ContentBlock {
	t, ok := r.tools[call.Name]
	if !ok {
		return llm.ContentBlock{
			Type:      llm.ContentTypeToolResult,
			ToolUseID: call.ID,
			Content:   fmt.Sprintf("Error: unknown tool %q", call.Name),
		}
	}

	result, err := t.Execute(ctx, call.Input)
	if err != nil {
		result = fmt.Sprintf("Error: %v", err)
	}

	return llm.ContentBlock{
		Type:      llm.ContentTypeToolResult,
		ToolUseID: call.ID,
		Content:   result,
	}
}
