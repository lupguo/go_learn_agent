package s17_autonomous

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

const (
	maxTeammateTurns = 50
	pollInterval     = 5 * time.Second
	idleTimeout      = 60 * time.Second
)

type MemberConfig struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"`
}

type TeamConfig struct {
	TeamName string         `json:"team_name"`
	Members  []MemberConfig `json:"members"`
}

type TeammateManager struct {
	dir        string
	configPath string
	config     TeamConfig
	bus        *MessageBus
	reqStore   *RequestStore
	provider   llm.Provider
	workDir    string
	tasksDir   string
	mu         sync.Mutex
	nextReqSeq int
}

func NewTeammateManager(teamDir, workDir, tasksDir string, bus *MessageBus, reqStore *RequestStore, provider llm.Provider) (*TeammateManager, error) {
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
		tasksDir:   tasksDir,
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
	_ = json.Unmarshal(data, &m.config)
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

func (m *TeammateManager) setStatus(name, status string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mem := m.findMember(name); mem != nil {
		mem.Status = status
		m.saveConfig()
	}
}

func (m *TeammateManager) nextRequestID() string {
	m.nextReqSeq++
	return fmt.Sprintf("req_%04d", m.nextReqSeq)
}

func (m *TeammateManager) memberRole(name string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if mem := m.findMember(name); mem != nil {
		return mem.Role
	}
	return ""
}

// Spawn starts an autonomous teammate.
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

	go m.autonomousLoop(name, role, prompt)
	return fmt.Sprintf("Spawned '%s' (role: %s)", name, role)
}

// autonomousLoop runs WORK → IDLE → auto-claim cycle.
func (m *TeammateManager) autonomousLoop(name, role, prompt string) {
	teamName := m.config.TeamName
	sysPrompt := fmt.Sprintf("You are '%s', role: %s, team: %s, at %s. "+
		"Use idle tool when you have no more work. You will auto-claim new tasks.", name, role, teamName, m.workDir)

	toolDefs := m.teammateToolDefs()
	registry := m.buildTeammateRegistry(name)
	messages := []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)}
	ctx := context.Background()

	for {
		// -- WORK PHASE --
		idleRequested := false
		for turn := 0; turn < maxTeammateTurns; turn++ {
			// Drain inbox
			inbox := m.bus.ReadInbox(name)
			for _, msg := range inbox {
				if msg.Type == "shutdown_request" {
					m.setStatus(name, "shutdown")
					return
				}
				data, _ := json.Marshal(msg)
				messages = append(messages, llm.NewTextMessage(llm.RoleUser, string(data)))
			}

			normalized := s02_tools.NormalizeMessages(messages)
			resp, err := m.provider.SendMessage(ctx, &llm.Request{
				System: sysPrompt, Messages: normalized,
				Tools: toolDefs, MaxTokens: 8000,
			})
			if err != nil {
				m.setStatus(name, "idle")
				return
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
				if call.Name == "idle" {
					idleRequested = true
					results = append(results, llm.ContentBlock{
						Type: llm.ContentTypeToolResult, ToolUseID: call.ID,
						Content: "Entering idle phase. Will poll for new tasks.",
					})
					fmt.Printf("  \033[35m[%s]\033[0m idle: entering idle phase\n", name)
				} else {
					result := registry.Execute(ctx, call)
					output := result.Content
					if len(output) > 120 {
						output = output[:120] + "..."
					}
					fmt.Printf("  \033[35m[%s]\033[0m %s: %s\n", name, call.Name, output)
					results = append(results, result)
				}
			}

			if len(results) > 0 {
				messages = append(messages, llm.NewToolResultMessage(results))
			}
			if idleRequested {
				break
			}
		}

		// -- IDLE PHASE: poll for inbox + unclaimed tasks --
		m.setStatus(name, "idle")
		resumed := false
		polls := int(idleTimeout / pollInterval)

		for i := 0; i < polls; i++ {
			time.Sleep(pollInterval)

			// Check inbox
			inbox := m.bus.ReadInbox(name)
			if len(inbox) > 0 {
				ensureIdentityContext(&messages, name, role, teamName)
				for _, msg := range inbox {
					if msg.Type == "shutdown_request" {
						m.setStatus(name, "shutdown")
						return
					}
					data, _ := json.Marshal(msg)
					messages = append(messages, llm.NewTextMessage(llm.RoleUser, string(data)))
				}
				resumed = true
				break
			}

			// Scan for unclaimed tasks
			unclaimed := ScanUnclaimedTasks(m.tasksDir, role)
			if len(unclaimed) > 0 {
				task := unclaimed[0]
				result := ClaimTask(m.tasksDir, task.ID, name, role, "auto")
				if strings.HasPrefix(result, "Error:") {
					continue
				}
				taskPrompt := fmt.Sprintf("<auto-claimed>Task #%d: %s\n%s</auto-claimed>",
					task.ID, task.Subject, task.Description)
				ensureIdentityContext(&messages, name, role, teamName)
				messages = append(messages, llm.NewTextMessage(llm.RoleUser, taskPrompt))
				messages = append(messages, llm.Message{
					Role:    llm.RoleAssistant,
					Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: result + ". Working on it."}},
				})
				fmt.Printf("  \033[35m[%s]\033[0m auto-claimed task #%d\n", name, task.ID)
				resumed = true
				break
			}
		}

		if !resumed {
			m.setStatus(name, "shutdown")
			fmt.Printf("  \033[35m[%s]\033[0m idle timeout, shutting down\n", name)
			return
		}
		m.setStatus(name, "working")
	}
}

// ensureIdentityContext injects identity block if missing (after compression).
func ensureIdentityContext(messages *[]llm.Message, name, role, teamName string) {
	if len(*messages) > 0 {
		first := (*messages)[0]
		if len(first.Content) > 0 && strings.Contains(first.Content[0].Text, "<identity>") {
			return
		}
	}
	identity := llm.NewTextMessage(llm.RoleUser,
		fmt.Sprintf("<identity>You are '%s', role: %s, team: %s. Continue your work.</identity>", name, role, teamName))
	ack := llm.Message{
		Role:    llm.RoleAssistant,
		Content: []llm.ContentBlock{{Type: llm.ContentTypeText, Text: fmt.Sprintf("I am %s. Continuing.", name)}},
	}
	*messages = append([]llm.Message{identity, ack}, *messages...)
}

func (m *TeammateManager) buildTeammateRegistry(name string) *tool.Registry {
	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(m.workDir))
	registry.Register(s02_tools.NewReadFileTool(m.workDir))
	registry.Register(s02_tools.NewWriteFileTool(m.workDir))
	registry.Register(s02_tools.NewEditFileTool(m.workDir))
	registry.Register(&teammateSendTool{bus: m.bus, sender: name})
	registry.Register(&teammateReadInboxTool{bus: m.bus, owner: name})
	registry.Register(&teammateShutdownResponseTool{bus: m.bus, reqStore: m.reqStore, sender: name})
	registry.Register(&teammatePlanApprovalTool{bus: m.bus, reqStore: m.reqStore, sender: name, mgr: m})
	registry.Register(&teammateClaimTaskTool{tasksDir: m.tasksDir, owner: name, mgr: m})
	return registry
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
			"type": "object", "properties": map[string]any{"to": map[string]any{"type": "string"}, "content": map[string]any{"type": "string"},
				"msg_type": map[string]any{"type": "string", "enum": msgTypeEnum}}, "required": []string{"to", "content"}}},
		{Name: "read_inbox", Description: "Read and drain your inbox.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{}}},
		{Name: "shutdown_response", Description: "Respond to a shutdown request.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"request_id": map[string]any{"type": "string"}, "approve": map[string]any{"type": "boolean"}, "reason": map[string]any{"type": "string"}}, "required": []string{"request_id", "approve"}}},
		{Name: "plan_approval", Description: "Submit a plan for lead approval.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"plan": map[string]any{"type": "string"}}, "required": []string{"plan"}}},
		{Name: "idle", Description: "Signal no more work. Enters idle polling phase.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{}}},
		{Name: "claim_task", Description: "Claim a task from the task board by ID.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{"task_id": map[string]any{"type": "integer"}}, "required": []string{"task_id"}}},
	}
}

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
	return strings.Join(lines, "\n")
}

func (m *TeammateManager) MemberNames() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, len(m.config.Members))
	for i, mem := range m.config.Members {
		names[i] = mem.Name
	}
	return names
}

// --- Teammate-specific tools ---

type teammateSendTool struct {
	bus    *MessageBus
	sender string
}

func (t *teammateSendTool) Name() string       { return "send_message" }
func (t *teammateSendTool) Description() string { return "Send message to a teammate." }
func (t *teammateSendTool) Schema() any         { return nil }
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

func (t *teammateReadInboxTool) Name() string       { return "read_inbox" }
func (t *teammateReadInboxTool) Description() string { return "Read and drain your inbox." }
func (t *teammateReadInboxTool) Schema() any         { return nil }
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

func (t *teammateShutdownResponseTool) Name() string       { return "shutdown_response" }
func (t *teammateShutdownResponseTool) Description() string { return "Respond to a shutdown request." }
func (t *teammateShutdownResponseTool) Schema() any         { return nil }
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
	t.bus.Send(t.sender, "lead", reason, "shutdown_response", &InboxMessage{RequestID: reqID, Approve: &approve})
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

func (t *teammatePlanApprovalTool) Name() string       { return "plan_approval" }
func (t *teammatePlanApprovalTool) Description() string { return "Submit a plan for lead approval." }
func (t *teammatePlanApprovalTool) Schema() any         { return nil }
func (t *teammatePlanApprovalTool) Execute(_ context.Context, input map[string]any) (string, error) {
	planText, _ := input["plan"].(string)
	t.mgr.mu.Lock()
	reqID := t.mgr.nextRequestID()
	t.mgr.mu.Unlock()
	now := float64(time.Now().Unix())
	t.reqStore.Create(&RequestRecord{
		RequestID: reqID, Kind: "plan_approval", From: t.sender, To: "lead",
		Status: "pending", Plan: planText, CreatedAt: now, UpdatedAt: now,
	})
	t.bus.Send(t.sender, "lead", planText, "plan_approval", &InboxMessage{RequestID: reqID, Plan: planText})
	return fmt.Sprintf("Plan submitted (request_id=%s). Waiting for approval.", reqID), nil
}

type teammateClaimTaskTool struct {
	tasksDir string
	owner    string
	mgr      *TeammateManager
}

func (t *teammateClaimTaskTool) Name() string       { return "claim_task" }
func (t *teammateClaimTaskTool) Description() string { return "Claim a task from the task board." }
func (t *teammateClaimTaskTool) Schema() any         { return nil }
func (t *teammateClaimTaskTool) Execute(_ context.Context, input map[string]any) (string, error) {
	taskIDf, _ := input["task_id"].(float64)
	role := t.mgr.memberRole(t.owner)
	return ClaimTask(t.tasksDir, int(taskIDf), t.owner, role, "manual"), nil
}
