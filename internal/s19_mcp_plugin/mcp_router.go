package s19_mcp_plugin

import (
	"fmt"
	"strings"

	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// MCPToolRouter routes tool calls to the correct MCP server.
// MCP tools use the prefix mcp__{server}__{tool} and live alongside
// native tools in the same tool pool.
type MCPToolRouter struct {
	clients map[string]*MCPClient
}

func NewMCPToolRouter() *MCPToolRouter {
	return &MCPToolRouter{clients: make(map[string]*MCPClient)}
}

// RegisterClient adds an MCP client to the router.
func (r *MCPToolRouter) RegisterClient(client *MCPClient) {
	r.clients[client.ServerName] = client
}

// IsMCPTool returns true if the tool name is an MCP-prefixed tool.
func (r *MCPToolRouter) IsMCPTool(toolName string) bool {
	return strings.HasPrefix(toolName, "mcp__")
}

// Call routes an MCP tool call to the correct server.
func (r *MCPToolRouter) Call(toolName string, arguments map[string]any) (string, error) {
	parts := strings.SplitN(toolName, "__", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid MCP tool name: %s", toolName)
	}
	serverName := parts[1]
	actualTool := parts[2]
	client, ok := r.clients[serverName]
	if !ok {
		return "", fmt.Errorf("MCP server not found: %s", serverName)
	}
	return client.CallTool(actualTool, arguments)
}

// GetAllTools collects tool definitions from all connected MCP servers.
func (r *MCPToolRouter) GetAllTools() []AgentToolDef {
	var all []AgentToolDef
	for _, client := range r.clients {
		all = append(all, client.GetAgentTools()...)
	}
	return all
}

// ClientCount returns the number of connected MCP servers.
func (r *MCPToolRouter) ClientCount() int {
	return len(r.clients)
}

// ListServers returns a summary of all connected servers.
func (r *MCPToolRouter) ListServers() string {
	if len(r.clients) == 0 {
		return "(no MCP servers connected)"
	}
	var lines []string
	for name, c := range r.clients {
		lines = append(lines, fmt.Sprintf("  %s: %d tools", name, len(c.tools)))
	}
	return strings.Join(lines, "\n")
}

// DisconnectAll shuts down all MCP server processes.
func (r *MCPToolRouter) DisconnectAll() {
	for _, c := range r.clients {
		c.Disconnect()
	}
}

// BuildToolPool assembles the complete tool pool: native registry + MCP tools.
// Native tools take precedence on name conflicts.
func BuildToolPool(registry *tool.Registry, router *MCPToolRouter) []llm.ToolDef {
	// Start with native tools.
	all := registry.ToolDefs()
	nativeNames := make(map[string]bool)
	for _, d := range all {
		nativeNames[d.Name] = true
	}

	// Add MCP tools that don't conflict.
	for _, mcpTool := range router.GetAllTools() {
		if !nativeNames[mcpTool.Name] {
			all = append(all, llm.ToolDef{
				Name:        mcpTool.Name,
				Description: mcpTool.Description,
				InputSchema: mcpTool.InputSchema,
			})
		}
	}
	return all
}
