package s02_tools

import (
	"testing"

	"github.com/lupguo/go_learn_agent/pkg/llm"
)

func TestNormalize_OrphanToolUse(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "do something"),
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			{Type: llm.ContentTypeToolUse, ID: "orphan_1", Name: "bash", Input: map[string]any{"command": "ls"}},
		}},
	}
	result := NormalizeMessages(messages)
	if len(result) != 3 {
		t.Fatalf("expected 3, got %d", len(result))
	}
	if result[2].Content[0].Content != "(cancelled)" {
		t.Fatalf("expected cancelled, got %+v", result[2].Content)
	}
}

func TestNormalize_MatchedToolUse(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "run ls"),
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			{Type: llm.ContentTypeToolUse, ID: "match_1", Name: "bash", Input: map[string]any{"command": "ls"}},
		}},
		{Role: llm.RoleUser, Content: []llm.ContentBlock{
			{Type: llm.ContentTypeToolResult, ToolUseID: "match_1", Content: "file.txt"},
		}},
	}
	result := NormalizeMessages(messages)
	if len(result) != 3 {
		t.Fatalf("expected 3 (no extra placeholders), got %d", len(result))
	}
}

func TestNormalize_MergeSameRole(t *testing.T) {
	messages := []llm.Message{
		llm.NewTextMessage(llm.RoleUser, "first"),
		llm.NewTextMessage(llm.RoleUser, "second"),
	}
	result := NormalizeMessages(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 merged, got %d", len(result))
	}
	if len(result[0].Content) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(result[0].Content))
	}
}

func TestNormalize_Empty(t *testing.T) {
	result := NormalizeMessages(nil)
	if result != nil {
		t.Fatalf("expected nil, got %v", result)
	}
}
