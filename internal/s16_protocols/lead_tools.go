package s16_protocols

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// --- SpawnTeammateTool ---

type SpawnTeammateTool struct{ manager *TeammateManager }

func NewSpawnTeammateTool(m *TeammateManager) *SpawnTeammateTool {
	return &SpawnTeammateTool{manager: m}
}
func (t *SpawnTeammateTool) Name() string        { return "spawn_teammate" }
func (t *SpawnTeammateTool) Description() string  { return "Spawn a persistent teammate." }
func (t *SpawnTeammateTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":   map[string]any{"type": "string"},
			"role":   map[string]any{"type": "string"},
			"prompt": map[string]any{"type": "string"},
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

type ListTeammatesTool struct{ manager *TeammateManager }

func NewListTeammatesTool(m *TeammateManager) *ListTeammatesTool {
	return &ListTeammatesTool{manager: m}
}
func (t *ListTeammatesTool) Name() string        { return "list_teammates" }
func (t *ListTeammatesTool) Description() string  { return "List all teammates." }
func (t *ListTeammatesTool) Schema() any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (t *ListTeammatesTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	return t.manager.ListAll(), nil
}

// --- LeadSendMessageTool ---

type LeadSendMessageTool struct{ bus *MessageBus }

func NewLeadSendMessageTool(bus *MessageBus) *LeadSendMessageTool {
	return &LeadSendMessageTool{bus: bus}
}
func (t *LeadSendMessageTool) Name() string        { return "send_message" }
func (t *LeadSendMessageTool) Description() string  { return "Send a message to a teammate." }
func (t *LeadSendMessageTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"to":       map[string]any{"type": "string"},
			"content":  map[string]any{"type": "string"},
			"msg_type": map[string]any{"type": "string", "enum": []string{"message", "broadcast", "shutdown_request", "shutdown_response", "plan_approval", "plan_approval_response"}},
		},
		"required": []string{"to", "content"},
	}
}
func (t *LeadSendMessageTool) Execute(_ context.Context, input map[string]any) (string, error) {
	to, _ := input["to"].(string)
	content, _ := input["content"].(string)
	msgType, _ := input["msg_type"].(string)
	return t.bus.Send("lead", to, content, msgType, nil), nil
}

// --- LeadReadInboxTool ---

type LeadReadInboxTool struct{ bus *MessageBus }

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
		"properties": map[string]any{"content": map[string]any{"type": "string"}},
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

// --- ShutdownRequestTool (lead sends shutdown request to teammate) ---

type ShutdownRequestTool struct {
	bus      *MessageBus
	reqStore *RequestStore
	manager  *TeammateManager
}

func NewShutdownRequestTool(bus *MessageBus, reqStore *RequestStore, mgr *TeammateManager) *ShutdownRequestTool {
	return &ShutdownRequestTool{bus: bus, reqStore: reqStore, manager: mgr}
}
func (t *ShutdownRequestTool) Name() string { return "shutdown_request" }
func (t *ShutdownRequestTool) Description() string {
	return "Request a teammate to shut down gracefully. Returns a request_id."
}
func (t *ShutdownRequestTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"teammate": map[string]any{"type": "string", "description": "Teammate name to shut down"},
		},
		"required": []string{"teammate"},
	}
}
func (t *ShutdownRequestTool) Execute(_ context.Context, input map[string]any) (string, error) {
	teammate, _ := input["teammate"].(string)
	if teammate == "" {
		return "", fmt.Errorf("teammate is required")
	}

	t.manager.mu.Lock()
	reqID := t.manager.nextRequestID()
	t.manager.mu.Unlock()

	now := float64(time.Now().Unix())
	t.reqStore.Create(&RequestRecord{
		RequestID: reqID,
		Kind:      "shutdown",
		From:      "lead",
		To:        teammate,
		Status:    "pending",
		CreatedAt: now,
		UpdatedAt: now,
	})

	t.bus.Send("lead", teammate, "Please shut down gracefully.", "shutdown_request",
		&InboxMessage{RequestID: reqID})

	return fmt.Sprintf("Shutdown request %s sent to '%s' (status: pending)", reqID, teammate), nil
}

// --- CheckShutdownTool (lead checks shutdown request status) ---

type CheckShutdownTool struct{ reqStore *RequestStore }

func NewCheckShutdownTool(reqStore *RequestStore) *CheckShutdownTool {
	return &CheckShutdownTool{reqStore: reqStore}
}
func (t *CheckShutdownTool) Name() string { return "check_shutdown" }
func (t *CheckShutdownTool) Description() string {
	return "Check the status of a shutdown request by request_id."
}
func (t *CheckShutdownTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request_id": map[string]any{"type": "string"},
		},
		"required": []string{"request_id"},
	}
}
func (t *CheckShutdownTool) Execute(_ context.Context, input map[string]any) (string, error) {
	reqID, _ := input["request_id"].(string)
	record := t.reqStore.Get(reqID)
	if record == nil {
		return `{"error": "not found"}`, nil
	}
	data, _ := json.MarshalIndent(record, "", "  ")
	return string(data), nil
}

// --- PlanReviewTool (lead approves/rejects a teammate's plan) ---

type PlanReviewTool struct {
	bus      *MessageBus
	reqStore *RequestStore
}

func NewPlanReviewTool(bus *MessageBus, reqStore *RequestStore) *PlanReviewTool {
	return &PlanReviewTool{bus: bus, reqStore: reqStore}
}
func (t *PlanReviewTool) Name() string { return "plan_review" }
func (t *PlanReviewTool) Description() string {
	return "Approve or reject a teammate's plan. Provide request_id + approve + optional feedback."
}
func (t *PlanReviewTool) Schema() any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"request_id": map[string]any{"type": "string"},
			"approve":    map[string]any{"type": "boolean"},
			"feedback":   map[string]any{"type": "string"},
		},
		"required": []string{"request_id", "approve"},
	}
}
func (t *PlanReviewTool) Execute(_ context.Context, input map[string]any) (string, error) {
	reqID, _ := input["request_id"].(string)
	approve, _ := input["approve"].(bool)
	feedback, _ := input["feedback"].(string)

	req := t.reqStore.Get(reqID)
	if req == nil {
		return fmt.Sprintf("Error: Unknown plan request_id '%s'", reqID), nil
	}

	t.reqStore.Update(reqID, func(r *RequestRecord) {
		if approve {
			r.Status = "approved"
		} else {
			r.Status = "rejected"
		}
		r.ReviewedBy = "lead"
		r.ResolvedAt = float64(time.Now().Unix())
		r.Feedback = feedback
	})

	t.bus.Send("lead", req.From, feedback, "plan_approval_response", &InboxMessage{
		RequestID: reqID,
		Approve:   &approve,
		Feedback:  feedback,
	})

	result := "approved"
	if !approve {
		result = "rejected"
	}
	return fmt.Sprintf("Plan %s for '%s'", result, req.From), nil
}
