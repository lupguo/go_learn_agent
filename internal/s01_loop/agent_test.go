package s01_loop

import (
    "context"
    "testing"

    "github.com/lupguo/go_learn_agent/pkg/llm"
    "github.com/lupguo/go_learn_agent/pkg/tool"
)

// mockProvider is a test double for llm.Provider.
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

func TestAgentLoop_TextOnly(t *testing.T) {
    // LLM responds with text only (no tool use) — loop should run once and stop.
    provider := &mockProvider{
        responses: []*llm.Response{
            {
                Content:    []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "Hello!"}},
                StopReason: llm.StopReasonEndTurn,
            },
        },
    }

    registry := tool.NewRegistry()
    agent := New(provider, registry)

    state := &LoopState{
        Messages:  []llm.Message{llm.NewTextMessage(llm.RoleUser, "hi")},
        TurnCount: 1,
    }

    err := agent.Run(context.Background(), state)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Should have 2 messages: user + assistant
    if len(state.Messages) != 2 {
        t.Fatalf("expected 2 messages, got %d", len(state.Messages))
    }
    if state.Messages[1].ExtractText() != "Hello!" {
        t.Fatalf("expected 'Hello!', got %q", state.Messages[1].ExtractText())
    }
}

func TestAgentLoop_ToolUse(t *testing.T) {
    // LLM calls bash, then responds with text.
    provider := &mockProvider{
        responses: []*llm.Response{
            {
                Content: []llm.ContentBlock{
                    {Type: llm.ContentTypeToolUse, ID: "tool_1", Name: "bash", Input: map[string]any{"command": "echo hello"}},
                },
                StopReason: llm.StopReasonToolUse,
            },
            {
                Content:    []llm.ContentBlock{{Type: llm.ContentTypeText, Text: "Done!"}},
                StopReason: llm.StopReasonEndTurn,
            },
        },
    }

    registry := tool.NewRegistry()
    registry.Register(NewBashTool())
    agent := New(provider, registry)

    state := &LoopState{
        Messages:  []llm.Message{llm.NewTextMessage(llm.RoleUser, "run echo hello")},
        TurnCount: 1,
    }

    err := agent.Run(context.Background(), state)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }

    // Should have 4 messages: user, assistant(tool_use), user(tool_result), assistant(text)
    if len(state.Messages) != 4 {
        t.Fatalf("expected 4 messages, got %d", len(state.Messages))
    }
    if state.Messages[3].ExtractText() != "Done!" {
        t.Fatalf("expected 'Done!', got %q", state.Messages[3].ExtractText())
    }
    if state.TurnCount != 2 {
        t.Fatalf("expected turn count 2, got %d", state.TurnCount)
    }
}

func TestBashTool_DangerousCommand(t *testing.T) {
    bash := NewBashTool()
    result, err := bash.Execute(context.Background(), map[string]any{"command": "reboot"})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != "Error: Dangerous command blocked" {
        t.Fatalf("expected dangerous command blocked, got %q", result)
    }
}

func TestBashTool_Execute(t *testing.T) {
    bash := NewBashTool()
    result, err := bash.Execute(context.Background(), map[string]any{"command": "echo test123"})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != "test123" {
        t.Fatalf("expected 'test123', got %q", result)
    }
}
