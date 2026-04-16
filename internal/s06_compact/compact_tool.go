package s06_compact

import (
	"context"

	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// CompactTool lets the LLM manually trigger conversation compaction.
type CompactTool struct{}

var _ tool.Tool = (*CompactTool)(nil)

func NewCompactTool() *CompactTool { return &CompactTool{} }

func (t *CompactTool) Name() string { return "compact" }

func (t *CompactTool) Description() string {
	return "Summarize earlier conversation so work can continue in a smaller context."
}

func (t *CompactTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"focus": map[string]any{
				"type":        "string",
				"description": "What to preserve in the summary.",
			},
		},
	}
}

func (t *CompactTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	// The actual compaction is handled in the agent loop after detecting this tool was called.
	return "Compacting conversation...", nil
}
