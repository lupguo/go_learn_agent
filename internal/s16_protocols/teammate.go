package s16_protocols

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

const maxTeammateTurns = 50

// MemberConfig is a teammate's persistent state in config.json.
type MemberConfig struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

// TeamConfig is the persistent team configuration.
type TeamConfig struct {
	TeamName string         `json:"team_name"`
	Members  []MemberConfig `json:"members"`
}

// TeammateManager manages persistent named agents with protocol support.
type TeammateManager struct {
	dir        string
	configPath string
	config     TeamConfig
	bus        *MessageBus
	reqStore   *RequestStore
	provider   llm.Provider
	workDir    string
	mu         sync.Mutex
	nextReqSeq int
}

// NewTeammateManager creates a manager with protocol support.
func NewTeammateManager(teamDir, workDir string, bus *MessageBus, reqStore *RequestStore, provider llm.Provider) (*TeammateManager, error) {
	if err := os.MkdirAll(teamDir, 0755); err != nil {
		return nil, fmt.Errorf("create team dir: %w", err)
	}
	m := &TeammateManager{
		dir:        teamDir,
		configPath: filepath.Join(teamDir, "config.json"),
		bus:        bus,
		reqStore:   reqStore,
		provider:   provider,
		workDir:    workDir,
	}
	m.loadConfig()
	return m, nil
}

func (m *TeammateManager) loadConfig() {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		m.config = TeamConfig{TeamName: "default", Members: []MemberConfig{}}
		return
	}
	if err := json.Unmarshal(data, &m.config); err != nil {
		m.config = TeamConfig{TeamName: "default", Members: []MemberConfig{}}
	}
}

func (m *TeammateManager) saveConfig() {
	data, _ := json.MarshalIndent(m.config, "", "  ")
	_ = os.WriteFile(m.configPath, data, 0644)
}

func (m *TeammateManager) findMember(name string) *MemberConfig {
	for i := range m.config.Members {
		if m.config.Members[i].Name == name {
			return &m.config.Members[i]
		}
	}
	return nil
}

func (m *TeammateManager) nextRequestID() string {
	m.nextReqSeq++
	return fmt.Sprintf("req_%04d", m.nextReqSeq)
}

// Spawn starts a teammate with protocol-aware tools.
func (m *TeammateManager) Spawn(name, role, prompt string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	member := m.findMember(name)
	if member != nil {
		if member.Status != "idle" && member.Status != "shutdown" {
			return fmt.Sprintf("Error: '%s' is currently %s", name, member.Status)
		}
		member.Status = "working"
		member.Role = role
	} else {
		m.config.Members = append(m.config.Members, MemberConfig{
			Name: name, Role: role, Status: "working",
		})
	}
	m.saveConfig()

	go m.teammateLoop(name, role, prompt)
	return fmt.Sprintf("Spawned '%s' (role: %s)", name, role)
}

// teammateLoop runs the teammate's agent loop with protocol handling.
func (m *TeammateManager) teammateLoop(name, role, prompt string) {
	sysPrompt := fmt.Sprintf("You are '%s', role: %s, at %s. "+
		"Submit plans via plan_approval before major work. "+
		"Respond to shutdown_request with shutdown_response.", name, role, m.workDir)

	toolDefs := m.teammateToolDefs()

	// Build registry with protocol tools
	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(m.workDir))
	registry.Register(s02_tools.NewReadFileTool(m.workDir))
	registry.Register(s02_tools.NewWriteFileTool(m.workDir))
	registry.Register(s02_tools.NewEditFileTool(m.workDir))
	registry.Register(&teammateSendTool{bus: m.bus, sender: name})
	registry.Register(&teammateReadInboxTool{bus: m.bus, owner: name})
	registry.Register(&teammateShutdownResponseTool{bus: m.bus, reqStore: m.reqStore, sender: name})
	registry.Register(&teammatePlanApprovalTool{bus: m.bus, reqStore: m.reqStore, sender: name, mgr: m})

	messages := []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)}
	ctx := context.Background()
	shouldExit := false

	for turn := 0; turn < maxTeammateTurns; turn++ {
		// Drain inbox
		inbox := m.bus.ReadInbox(name)
		for _, msg := range inbox {
			data, _ := json.Marshal(msg)
			messages = append(messages, llm.NewTextMessage(llm.RoleUser, string(data)))
		}

		if shouldExit {
			break
		}

		normalized := s02_tools.NormalizeMessages(messages)
		resp, err := m.provider.SendMessage(ctx, &llm.Request{
			System:    sysPrompt,
			Messages:  normalized,
			Tools:     toolDefs,
			MaxTokens: 8000,
		})
		if err != nil {
			break
		}

		messages = append(messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		if resp.StopReason != llm.StopReasonToolUse {
			break
		}

		var results []llm.ContentBlock
		for _, call := range resp.Content {
			if call.Type != llm.ContentTypeToolUse {
				continue
			}
			result := registry.Execute(ctx, call)
			output := result.Content
			if len(output) > 120 {
				output = output[:120] + "..."
			}
			fmt.Printf("  \033[35m[%s]\033[0m %s: %s\n", name, call.Name, output)
			results = append(results, result)

			// Check if teammate approved shutdown
			if call.Name == "shutdown_response" {
				if approve, ok := call.Input["approve"].(bool); ok && approve {
					shouldExit = true
				}
			}
		}

		if len(results) == 0 {
			break
		}
		messages = append(messages, llm.NewToolResultMessage(results))
	}

	// Update status
	m.mu.Lock()
	member := m.findMember(name)
	if member != nil {
		if shouldExit {
			member.Status = "shutdown"
		} else {
			member.Status = "idle"
		}
		m.saveConfig()
	}
	m.mu.Unlock()
}

func (m *TeammateManager) teammateToolDefs() []llm.ToolDef {
	msgTypeEnum := []string{"message", "broadcast", "shutdown_request", "shutdown_response", "plan_approval", "plan_approval_response"}
	return []llm.ToolDef{
		{Name: "bash", Description: "Run a shell command.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"command": map[string]any{"type": "string"}}, "required": []string{"command"}}},
		{Name: "read_file", Description: "Read file contents.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}}, "required": []string{"path"}}},
		{Name: "write_file", Description: "Write content to file.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"}}, "required": []string{"path", "content"}}},
		{Name: "edit_file", Description: "Replace exact text in file.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"path": map[string]any{"type": "string"}, "old_text": map[string]any{"type": "string"}, "new_text": map[string]any{"type": "string"}}, "required": []string{"path", "old_text", "new_text"}}},
		{Name: "send_message", Description: "Send message to a teammate.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{
				"to": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
				"msg_type": map[string]any{"type": "string", "enum": msgTypeEnum},
			}, "required": []string{"to", "content"}}},
		{Name: "read_inbox", Description: "Read and drain your inbox.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{}}},
		{Name: "shutdown_response", Description: "Respond to a shutdown request. Approve to shut down, reject to keep working.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{
				"request_id": map[string]any{"type": "string"}, "approve": map[string]any{"type": "boolean"},
				"reason": map[string]any{"type": "string"},
			}, "required": []string{"request_id", "approve"}}},
		{Name: "plan_approval", Description: "Submit a plan for lead approval.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"plan": map[string]any{"type": "string"}}, "required": []string{"plan"}}},
	}
}

// ListAll returns a formatted listing of all team members.
func (m *TeammateManager) ListAll() string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.config.Members) == 0 {
		return "No teammates."
	}
	lines := []string{fmt.Sprintf("Team: %s", m.config.TeamName)}
	for _, mem := range m.config.Members {
		lines = append(lines, fmt.Sprintf("  %s (%s): %s", mem.Name, mem.Role, mem.Status))
	}
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

// MemberNames returns all teammate names.
func (m *TeammateManager) MemberNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	names := make([]string, len(m.config.Members))
	for i, mem := range m.config.Members {
		names[i] = mem.Name
	}
	return names
}

// --- Teammate-specific protocol tools ---

type teammateSendTool struct {
	bus    *MessageBus
	sender string
}

func (t *teammateSendTool) Name() string        { return "send_message" }
func (t *teammateSendTool) Description() string  { return "Send message to a teammate." }
func (t *teammateSendTool) Schema() any          { return nil }
func (t *teammateSendTool) Execute(_ context.Context, input map[string]any) (string, error) {
	to, _ := input["to"].(string)
	content, _ := input["content"].(string)
	msgType, _ := input["msg_type"].(string)
	return t.bus.Send(t.sender, to, content, msgType, nil), nil
}

type teammateReadInboxTool struct {
	bus   *MessageBus
	owner string
}

func (t *teammateReadInboxTool) Name() string        { return "read_inbox" }
func (t *teammateReadInboxTool) Description() string  { return "Read and drain your inbox." }
func (t *teammateReadInboxTool) Schema() any          { return nil }
func (t *teammateReadInboxTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	msgs := t.bus.ReadInbox(t.owner)
	data, _ := json.MarshalIndent(msgs, "", "  ")
	return string(data), nil
}

type teammateShutdownResponseTool struct {
	bus      *MessageBus
	reqStore *RequestStore
	sender   string
}

func (t *teammateShutdownResponseTool) Name() string        { return "shutdown_response" }
func (t *teammateShutdownResponseTool) Description() string  { return "Respond to a shutdown request." }
func (t *teammateShutdownResponseTool) Schema() any          { return nil }
func (t *teammateShutdownResponseTool) Execute(_ context.Context, input map[string]any) (string, error) {
	reqID, _ := input["request_id"].(string)
	approve, _ := input["approve"].(bool)
	reason, _ := input["reason"].(string)

	updated := t.reqStore.Update(reqID, func(r *RequestRecord) {
		if approve {
			r.Status = "approved"
		} else {
			r.Status = "rejected"
		}
		r.ResolvedBy = t.sender
		r.ResolvedAt = float64(time.Now().Unix())
	})
	if updated == nil {
		return fmt.Sprintf("Error: Unknown shutdown request %s", reqID), nil
	}

	t.bus.Send(t.sender, "lead", reason, "shutdown_response", &InboxMessage{
		RequestID: reqID,
		Approve:   &approve,
	})

	if approve {
		return "Shutdown approved", nil
	}
	return "Shutdown rejected", nil
}

type teammatePlanApprovalTool struct {
	bus      *MessageBus
	reqStore *RequestStore
	sender   string
	mgr      *TeammateManager
}

func (t *teammatePlanApprovalTool) Name() string        { return "plan_approval" }
func (t *teammatePlanApprovalTool) Description() string  { return "Submit a plan for lead approval." }
func (t *teammatePlanApprovalTool) Schema() any          { return nil }
func (t *teammatePlanApprovalTool) Execute(_ context.Context, input map[string]any) (string, error) {
	planText, _ := input["plan"].(string)

	t.mgr.mu.Lock()
	reqID := t.mgr.nextRequestID()
	t.mgr.mu.Unlock()

	now := float64(time.Now().Unix())
	t.reqStore.Create(&RequestRecord{
		RequestID: reqID,
		Kind:      "plan_approval",
		From:      t.sender,
		To:        "lead",
		Status:    "pending",
		Plan:      planText,
		CreatedAt: now,
		UpdatedAt: now,
	})

	t.bus.Send(t.sender, "lead", planText, "plan_approval", &InboxMessage{
		RequestID: reqID,
		Plan:      planText,
	})

	return fmt.Sprintf("Plan submitted (request_id=%s). Waiting for lead approval.", reqID), nil
}
