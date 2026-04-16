package s15_teams

import (
	"context"
	"encoding/json"
	"fmt"
)

// --- SpawnTeammateTool ---

type SpawnTeammateTool struct {
	manager *TeammateManager
}

func NewSpawnTeammateTool(m *TeammateManager) *SpawnTeammateTool {
	return &SpawnTeammateTool{manager: m}
}

func (t *SpawnTeammateTool) Name() string        { return "spawn_teammate" }
func (t *SpawnTeammateTool) Description() string {
	return "Spawn a persistent teammate that runs in its own goroutine."
}
func (t *SpawnTeammateTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":   map[string]any{"type": "string", "description": "Teammate name"},
			"role":   map[string]any{"type": "string", "description": "Teammate role (e.g. coder, reviewer)"},
			"prompt": map[string]any{"type": "string", "description": "Initial task prompt"},
		},
		"required": []string{"name", "role", "prompt"},
	}
}

func (t *SpawnTeammateTool) Execute(_ context.Context, input map[string]any) (string, error) {
	name, _ := input["name"].(string)
	role, _ := input["role"].(string)
	prompt, _ := input["prompt"].(string)
	if name == "" || role == "" || prompt == "" {
		return "", fmt.Errorf("name, role, and prompt are required")
	}
	return t.manager.Spawn(name, role, prompt), nil
}

// --- ListTeammatesTool ---

type ListTeammatesTool struct {
	manager *TeammateManager
}

func NewListTeammatesTool(m *TeammateManager) *ListTeammatesTool {
	return &ListTeammatesTool{manager: m}
}

func (t *ListTeammatesTool) Name() string        { return "list_teammates" }
func (t *ListTeammatesTool) Description() string  { return "List all teammates with name, role, status." }
func (t *ListTeammatesTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *ListTeammatesTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return t.manager.ListAll(), nil
}

// --- LeadSendMessageTool (bound to "lead") ---

type LeadSendMessageTool struct {
	bus *MessageBus
}

func NewLeadSendMessageTool(bus *MessageBus) *LeadSendMessageTool {
	return &LeadSendMessageTool{bus: bus}
}

func (t *LeadSendMessageTool) Name() string        { return "send_message" }
func (t *LeadSendMessageTool) Description() string  { return "Send a message to a teammate's inbox." }
func (t *LeadSendMessageTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":       map[string]any{"type": "string", "description": "Teammate name"},
			"content":  map[string]any{"type": "string", "description": "Message content"},
			"msg_type": map[string]any{"type": "string", "enum": []string{"message", "broadcast", "shutdown_request", "shutdown_response", "plan_approval", "plan_approval_response"}},
		},
		"required": []string{"to", "content"},
	}
}

func (t *LeadSendMessageTool) Execute(_ context.Context, input map[string]any) (string, error) {
	to, _ := input["to"].(string)
	content, _ := input["content"].(string)
	msgType, _ := input["msg_type"].(string)
	return t.bus.Send("lead", to, content, msgType), nil
}

// --- LeadReadInboxTool (bound to "lead") ---

type LeadReadInboxTool struct {
	bus *MessageBus
}

func NewLeadReadInboxTool(bus *MessageBus) *LeadReadInboxTool {
	return &LeadReadInboxTool{bus: bus}
}

func (t *LeadReadInboxTool) Name() string        { return "read_inbox" }
func (t *LeadReadInboxTool) Description() string  { return "Read and drain the lead's inbox." }
func (t *LeadReadInboxTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}

func (t *LeadReadInboxTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	msgs := t.bus.ReadInbox("lead")
	data, _ := json.MarshalIndent(msgs, "", "  ")
	return string(data), nil
}

// --- BroadcastTool ---

type BroadcastTool struct {
	bus     *MessageBus
	manager *TeammateManager
}

func NewBroadcastTool(bus *MessageBus, manager *TeammateManager) *BroadcastTool {
	return &BroadcastTool{bus: bus, manager: manager}
}

func (t *BroadcastTool) Name() string        { return "broadcast" }
func (t *BroadcastTool) Description() string  { return "Send a message to all teammates." }
func (t *BroadcastTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{"type": "string", "description": "Message to broadcast"},
		},
		"required": []string{"content"},
	}
}

func (t *BroadcastTool) Execute(_ context.Context, input map[string]any) (string, error) {
	content, _ := input["content"].(string)
	if content == "" {
		return "", fmt.Errorf("content is required")
	}
	return t.bus.Broadcast("lead", content, t.manager.MemberNames()), nil
}
