package s19_mcp_plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

// Agent is the s19 agent with unified native + MCP tool pool and permission gate.
type Agent struct {
	provider llm.Provider
	registry *tool.Registry
	router   *MCPToolRouter
	gate     *CapabilityPermissionGate
	system   string
	maxTok   int
}

func New(provider llm.Provider, registry *tool.Registry, router *MCPToolRouter, gate *CapabilityPermissionGate) *Agent {
	cwd, _ := os.Getwd()
	return &Agent{
		provider: provider,
		registry: registry,
		router:   router,
		gate:     gate,
		system: fmt.Sprintf(
			"You are a coding agent at %s. Use tools to solve tasks.\n"+
				"You have both native tools and MCP tools available.\n"+
				"MCP tools are prefixed with mcp__{server}__{tool}.\n"+
				"All capabilities pass through the same permission gate before execution.", cwd),
		maxTok: 8000,
	}
}

func (a *Agent) Run(ctx context.Context, state *s02_tools.LoopState) error {
	toolPool := BuildToolPool(a.registry, a.router)

	for {
		normalized := s02_tools.NormalizeMessages(state.Messages)
		resp, err := a.provider.SendMessage(ctx, &llm.Request{
			System: a.system, Messages: normalized,
			Tools: toolPool, MaxTokens: a.maxTok,
		})
		if err != nil {
			return fmt.Errorf("API error: %w", err)
		}

		state.Messages = append(state.Messages, llm.Message{Role: llm.RoleAssistant, Content: resp.Content})
		if resp.StopReason != llm.StopReasonToolUse {
			return nil
		}

		var results []llm.ContentBlock
		for _, call := range resp.Content {
			if call.Type != llm.ContentTypeToolUse {
				continue
			}

			// Permission gate check.
			decision := a.gate.Check(call.Name, call.Input)
			var output string

			switch decision.Behavior {
			case BehaviorDeny:
				output = fmt.Sprintf("Permission denied: %s", decision.Reason)
			case BehaviorAsk:
				if !askUser(a.gate, decision.Intent, call.Input) {
					output = fmt.Sprintf("Permission denied by user: %s", decision.Reason)
				} else {
					output = a.executeTool(ctx, call)
				}
			default:
				output = a.executeTool(ctx, call)
			}

			// Normalize result with source info.
			normalized := normalizeToolResult(call.Name, output, &decision.Intent)
			fmt.Printf("\033[33m> %s:\033[0m %s\n", call.Name, truncate(output, 200))

			results = append(results, llm.ContentBlock{
				Type:      llm.ContentTypeToolResult,
				ToolUseID: call.ID,
				Content:   normalized,
			})
		}
		if len(results) == 0 {
			return nil
		}
		state.Messages = append(state.Messages, llm.NewToolResultMessage(results))
		state.TurnCount++
		state.TransitionReason = "tool_result"
	}
}

// executeTool dispatches to native registry or MCP router.
func (a *Agent) executeTool(ctx context.Context, call llm.ContentBlock) string {
	if a.router.IsMCPTool(call.Name) {
		result, err := a.router.Call(call.Name, call.Input)
		if err != nil {
			return fmt.Sprintf("Error: %v", err)
		}
		return result
	}
	result := a.registry.Execute(ctx, call)
	return result.Content
}

func askUser(gate *CapabilityPermissionGate, intent CapabilityIntent, input map[string]any) bool {
	prompt := gate.FormatAskPrompt(intent, input)
	fmt.Printf("\n  %s\n", prompt)
	fmt.Print("  Allow? (y/n): ")
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	answer := strings.TrimSpace(strings.ToLower(line))
	return answer == "y" || answer == "yes"
}

func normalizeToolResult(toolName, output string, intent *CapabilityIntent) string {
	status := "ok"
	if strings.Contains(output, "Error:") || strings.Contains(output, "MCP Error:") {
		status = "error"
	}
	preview := output
	if len(preview) > 500 {
		preview = preview[:500]
	}
	payload := map[string]any{
		"source": intent.Source,
		"server": intent.Server,
		"tool":   intent.Tool,
		"risk":   intent.Risk,
		"status": status,
		"output": preview,
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
