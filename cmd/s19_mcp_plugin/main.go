package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s19_mcp_plugin"
	"github.com/lupguo/go_learn_agent/pkg/llm"
	_ "github.com/lupguo/go_learn_agent/pkg/llm/anthropic"
	_ "github.com/lupguo/go_learn_agent/pkg/llm/gemini"
	_ "github.com/lupguo/go_learn_agent/pkg/llm/openai"
	"github.com/lupguo/go_learn_agent/pkg/tool"
)

func main() {
	llm.LoadEnvFile(".env")
	provider, err := llm.NewProviderFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cwd, _ := os.Getwd()

	// Permission gate (default mode — asks for write/high-risk).
	gate := s19_mcp_plugin.NewPermissionGate(s19_mcp_plugin.ModeDefault)

	// MCP router and plugin loader.
	router := s19_mcp_plugin.NewMCPToolRouter()
	loader := s19_mcp_plugin.NewPluginLoader([]string{cwd})

	// Scan for plugins and connect MCP servers.
	found := loader.Scan()
	if len(found) > 0 {
		fmt.Printf("[Plugins loaded: %s]\n", strings.Join(found, ", "))
		for serverName, config := range loader.GetMCPServers() {
			client := s19_mcp_plugin.NewMCPClient(serverName, config.Command, config.Args, config.Env)
			if err := client.Connect(); err != nil {
				fmt.Fprintf(os.Stderr, "[MCP] Failed to connect %s: %v\n", serverName, err)
				continue
			}
			if _, err := client.ListTools(); err != nil {
				fmt.Fprintf(os.Stderr, "[MCP] Failed to list tools for %s: %v\n", serverName, err)
				client.Disconnect()
				continue
			}
			router.RegisterClient(client)
			fmt.Printf("[MCP] Connected to %s\n", serverName)
		}
	}
	defer router.DisconnectAll()

	// Native tool registry.
	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))

	// Report tool pool.
	toolPool := s19_mcp_plugin.BuildToolPool(registry, router)
	mcpTools := router.GetAllTools()
	fmt.Printf("[Tool pool: %d tools (%d from MCP)]\n", len(toolPool), len(mcpTools))

	agent := s19_mcp_plugin.New(provider, registry, router, gate)

	fmt.Println("s19 MCP & Plugin — type 'q' to quit, '/tools', '/mcp'")

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	for {
		fmt.Print("\033[36ms19 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}
		if query == "/tools" {
			for _, t := range s19_mcp_plugin.BuildToolPool(registry, router) {
				prefix := "       "
				if strings.HasPrefix(t.Name, "mcp__") {
					prefix = "[MCP] "
				}
				desc := t.Description
				if len(desc) > 60 {
					desc = desc[:60]
				}
				fmt.Printf("  %s%s: %s\n", prefix, t.Name, desc)
			}
			fmt.Println()
			continue
		}
		if query == "/mcp" {
			fmt.Println(router.ListServers())
			fmt.Println()
			continue
		}

		history = append(history, llm.NewTextMessage(llm.RoleUser, query))
		state := &s02_tools.LoopState{Messages: history, TurnCount: 1}
		if err := agent.Run(ctx, state); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
			continue
		}
		history = state.Messages
		if len(history) > 0 {
			if text := history[len(history)-1].ExtractText(); text != "" {
				fmt.Println(text)
			}
		}
		fmt.Println()
	}
}