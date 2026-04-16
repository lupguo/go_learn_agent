package s19_mcp_plugin

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// MCPClient is a minimal MCP client over stdio (JSON-RPC 2.0).
type MCPClient struct {
	ServerName string
	command    string
	args       []string
	env        []string
	process    *exec.Cmd
	stdin      *json.Encoder
	stdout     *bufio.Reader
	requestID  int
	tools      []MCPToolSpec
	mu         sync.Mutex
}

// MCPToolSpec is a tool definition from an MCP server.
type MCPToolSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"inputSchema"`
}

func NewMCPClient(serverName, command string, args []string, env map[string]string) *MCPClient {
	// Merge extra env with current env.
	merged := os.Environ()
	for k, v := range env {
		merged = append(merged, k+"="+v)
	}
	return &MCPClient{
		ServerName: serverName,
		command:    command,
		args:       args,
		env:        merged,
	}
}

// Connect starts the MCP server process and performs handshake.
func (c *MCPClient) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.process = exec.Command(c.command, c.args...)
	c.process.Env = c.env
	c.process.Stderr = os.Stderr

	stdinPipe, err := c.process.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdoutPipe, err := c.process.StdoutPipe()
	if err != nil {
		return fmt.Errorf("stdout pipe: %w", err)
	}
	if err := c.process.Start(); err != nil {
		return fmt.Errorf("start %s: %w", c.command, err)
	}

	c.stdin = json.NewEncoder(stdinPipe)
	c.stdout = bufio.NewReader(stdoutPipe)

	// Initialize handshake.
	resp, err := c.sendRecv("initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "teaching-agent", "version": "1.0"},
	})
	if err != nil {
		c.kill()
		return fmt.Errorf("initialize: %w", err)
	}
	if _, ok := resp["result"]; !ok {
		c.kill()
		return fmt.Errorf("initialize: no result in response")
	}

	// Send initialized notification (no response expected).
	c.send("notifications/initialized", nil)

	return nil
}

// ListTools fetches available tools from the server.
func (c *MCPClient) ListTools() ([]MCPToolSpec, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	resp, err := c.sendRecv("tools/list", map[string]any{})
	if err != nil {
		return nil, err
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("tools/list: invalid result")
	}
	toolsRaw, _ := result["tools"].([]any)
	c.tools = nil
	for _, raw := range toolsRaw {
		data, _ := json.Marshal(raw)
		var spec MCPToolSpec
		if json.Unmarshal(data, &spec) == nil {
			c.tools = append(c.tools, spec)
		}
	}
	return c.tools, nil
}

// CallTool executes a tool on the server.
func (c *MCPClient) CallTool(toolName string, arguments map[string]any) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	resp, err := c.sendRecv("tools/call", map[string]any{
		"name":      toolName,
		"arguments": arguments,
	})
	if err != nil {
		return "", err
	}
	if errObj, ok := resp["error"].(map[string]any); ok {
		msg, _ := errObj["message"].(string)
		return "", fmt.Errorf("MCP Error: %s", msg)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		return "MCP Error: no result", nil
	}
	contentRaw, _ := result["content"].([]any)
	var parts []string
	for _, c := range contentRaw {
		if m, ok := c.(map[string]any); ok {
			if text, ok := m["text"].(string); ok {
				parts = append(parts, text)
			} else {
				data, _ := json.Marshal(m)
				parts = append(parts, string(data))
			}
		}
	}
	if len(parts) == 0 {
		return "(no output)", nil
	}
	return strings.Join(parts, "\n"), nil
}

// GetAgentTools returns MCP tools converted to prefixed agent tool definitions.
func (c *MCPClient) GetAgentTools() []AgentToolDef {
	var defs []AgentToolDef
	for _, t := range c.tools {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		defs = append(defs, AgentToolDef{
			Name:        fmt.Sprintf("mcp__%s__%s", c.ServerName, t.Name),
			Description: t.Description,
			InputSchema: schema,
			MCPServer:   c.ServerName,
			MCPTool:     t.Name,
		})
	}
	return defs
}

// Disconnect shuts down the server process.
func (c *MCPClient) Disconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.kill()
}

func (c *MCPClient) kill() {
	if c.process != nil && c.process.Process != nil {
		_ = c.process.Process.Kill()
		_ = c.process.Wait()
		c.process = nil
	}
}

func (c *MCPClient) send(method string, params map[string]any) {
	c.requestID++
	envelope := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.requestID,
		"method":  method,
	}
	if params != nil {
		envelope["params"] = params
	}
	_ = c.stdin.Encode(envelope)
}

func (c *MCPClient) sendRecv(method string, params map[string]any) (map[string]any, error) {
	c.requestID++
	envelope := map[string]any{
		"jsonrpc": "2.0",
		"id":      c.requestID,
		"method":  method,
	}
	if params != nil {
		envelope["params"] = params
	}
	if err := c.stdin.Encode(envelope); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	line, err := c.stdout.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("recv: %w", err)
	}
	var resp map[string]any
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return resp, nil
}

// AgentToolDef is an MCP tool converted to agent format.
type AgentToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema any    `json:"input_schema"`
	MCPServer   string `json:"_mcp_server"`
	MCPTool     string `json:"_mcp_tool"`
}
