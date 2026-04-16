package s15_teams

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

const maxTeammateTurns = 50

// MemberConfig is a teammate's persistent state in config.json.
type MemberConfig struct {
	Name   string `json:"name"`
	Role   string `json:"role"`
	Status string `json:"status"` // working, idle, shutdown
}

// TeamConfig is the persistent team configuration.
type TeamConfig struct {
	TeamName string         `json:"team_name"`
	Members  []MemberConfig `json:"members"`
}

// TeammateManager manages persistent named agents with goroutine-based worker loops.
type TeammateManager struct {
	dir        string
	configPath string
	config     TeamConfig
	bus        *MessageBus
	provider   llm.Provider
	workDir    string
	mu         sync.Mutex
}

// NewTeammateManager creates a manager backed by .team/config.json.
func NewTeammateManager(teamDir, workDir string, bus *MessageBus, provider llm.Provider) (*TeammateManager, error) {
	if err := os.MkdirAll(teamDir, 0755); err != nil {
		return nil, fmt.Errorf("create team dir: %w", err)
	}
	m := &TeammateManager{
		dir:        teamDir,
		configPath: filepath.Join(teamDir, "config.json"),
		bus:        bus,
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

// Spawn starts a teammate in its own goroutine.
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
			Name:   name,
			Role:   role,
			Status: "working",
		})
	}
	m.saveConfig()

	go m.teammateLoop(name, role, prompt)
	return fmt.Sprintf("Spawned '%s' (role: %s)", name, role)
}

// teammateLoop is the goroutine target for each teammate.
func (m *TeammateManager) teammateLoop(name, role, prompt string) {
	sysPrompt := fmt.Sprintf("You are '%s', role: %s, at %s. "+
		"Use send_message to communicate. Complete your task.", name, role, m.workDir)

	// Build teammate tool defs
	toolDefs := m.teammateToolDefs()

	// Build teammate registry for execution
	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(m.workDir))
	registry.Register(s02_tools.NewReadFileTool(m.workDir))
	registry.Register(s02_tools.NewWriteFileTool(m.workDir))
	registry.Register(s02_tools.NewEditFileTool(m.workDir))
	registry.Register(&sendMessageTool{bus: m.bus, sender: name})
	registry.Register(&readInboxTool{bus: m.bus, owner: name})

	messages := []llm.Message{llm.NewTextMessage(llm.RoleUser, prompt)}
	ctx := context.Background()

	for turn := 0; turn < maxTeammateTurns; turn++ {
		// Drain inbox
		inbox := m.bus.ReadInbox(name)
		for _, msg := range inbox {
			data, _ := json.Marshal(msg)
			messages = append(messages, llm.NewTextMessage(llm.RoleUser, string(data)))
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
		}

		if len(results) == 0 {
			break
		}
		messages = append(messages, llm.NewToolResultMessage(results))
	}

	// Mark idle when done
	m.mu.Lock()
	member := m.findMember(name)
	if member != nil && member.Status != "shutdown" {
		member.Status = "idle"
		m.saveConfig()
	}
	m.mu.Unlock()
}

func (m *TeammateManager) teammateToolDefs() []llm.ToolDef {
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
				"to":       map[string]any{"type": "string"},
				"content":  map[string]any{"type": "string"},
				"msg_type": map[string]any{"type": "string", "enum": []string{"message", "broadcast", "shutdown_request", "shutdown_response", "plan_approval", "plan_approval_response"}},
			}, "required": []string{"to", "content"}}},
		{Name: "read_inbox", Description: "Read and drain your inbox.", InputSchema: map[string]any{
			"type": "object", "properties": map[string]any{}}},
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
	return joinLines(lines)
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

func joinLines(lines []string) string {
	result := ""
	for i, l := range lines {
		if i > 0 {
			result += "\n"
		}
		result += l
	}
	return result
}

// --- Teammate-specific tools (bound to a specific sender/owner) ---

type sendMessageTool struct {
	bus    *MessageBus
	sender string
}

func (t *sendMessageTool) Name() string        { return "send_message" }
func (t *sendMessageTool) Description() string  { return "Send message to a teammate." }
func (t *sendMessageTool) Schema() any          { return nil } // defs provided inline
func (t *sendMessageTool) Execute(_ context.Context, input map[string]any) (string, error) {
	to, _ := input["to"].(string)
	content, _ := input["content"].(string)
	msgType, _ := input["msg_type"].(string)
	return t.bus.Send(t.sender, to, content, msgType), nil
}

type readInboxTool struct {
	bus   *MessageBus
	owner string
}

func (t *readInboxTool) Name() string        { return "read_inbox" }
func (t *readInboxTool) Description() string  { return "Read and drain your inbox." }
func (t *readInboxTool) Schema() any          { return nil }
func (t *readInboxTool) Execute(_ context.Context, _ map[string]any) (string, error) {
	msgs := t.bus.ReadInbox(t.owner)
	data, _ := json.MarshalIndent(msgs, "", "  ")
	return string(data), nil
}
