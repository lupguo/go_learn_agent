package s02_tools

import (
	"context"
	"testing"

	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

type mockProvider struct {
	responses []*llm.Response
	callIndex int
}

func (m *mockProvider) SendMessage(_ context.Context, _ *llm.Request) (*llm.Response, error) {
	if m.callIndex >= len(m.responses) {
		return &llm.Response{StopReason: llm.StopReasonEndTurn}, nil
	}
	resp := m.responses[m.callIndex]
	m.callIndex++
	return resp, nil
}

func TestAgent_TextOnly(t *testing.T) {
	p := &mockProvider{responses: []*llm.Response{
		{Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "Hello!"}}, StopReason: llm.StopReasonEndTurn},
	}}
	agent := New(p, tool.NewRegistry())
	state := &LoopState{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")}, TurnCount: 1}
	if err := agent.Run(context.Background(), state); err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(state.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(state.Messages))
	}
}

func TestAgent_ToolUse(t *testing.T) {
	p := &mockProvider{responses: []*llm.Response{
		{Content: []llm.ContentBlock{
			{Type: llm.ContentTypeToolUse, ID: "t1", Name: "bash", Input: map[string]any{"command": "echo hi"}},
		}, StopReason: llm.StopReasonToolUse},
		{Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "Done"}}, StopReason: llm.StopReasonEndTurn},
	}}
	registry := tool.NewRegistry()
	registry.Register(NewBashTool(t.TempDir()))
	agent := New(p, registry)
	state := &LoopState{Messages: []llm.Message{llm.NewTextMessage(llm.RoleUser, "run echo")}, TurnCount: 1}
	if err := agent.Run(context.Background(), state); err != nil {
		t.Fatalf("error: %v", err)
	}
	// user, assistant(tool_use), user(tool_result), assistant(text)
	if len(state.Messages) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(state.Messages))
	}
	if state.TurnCount != 2 {
		t.Fatalf("expected turn count 2, got %d", state.TurnCount)
	}
}
