package s05_skills

import (
	"context"
	"fmt"

	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// LoadSkillTool lets the LLM load a skill's full body on demand.
type LoadSkillTool struct {
	registry *SkillRegistry
}

var _ tool.Tool = (*LoadSkillTool)(nil)

// NewLoadSkillTool creates a LoadSkillTool backed by the given registry.
func NewLoadSkillTool(registry *SkillRegistry) *LoadSkillTool {
	return &LoadSkillTool{registry: registry}
}

func (t *LoadSkillTool) Name() string { return "load_skill" }

func (t *LoadSkillTool) Description() string {
	return "Load the full body of a named skill into the current context."
}

func (t *LoadSkillTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{"type": "string"},
		},
		"required": []string{"name"},
	}
}

func (t *LoadSkillTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, ok := input["name"].(string)
	if !ok || name == "" {
		return "", fmt.Errorf("missing required field: name")
	}
	return t.registry.LoadFullText(name), nil
}
