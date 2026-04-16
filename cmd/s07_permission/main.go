package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s07_permission"
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

	// Choose permission mode at startup
	fmt.Println("Permission modes: default, plan, auto")
	fmt.Print("Mode (default): ")
	scanner := bufio.NewScanner(os.Stdin)
	mode := s07_permission.ModeDefault
	if scanner.Scan() {
		modeStr := strings.TrimSpace(scanner.Text())
		switch s07_permission.Mode(modeStr) {
		case s07_permission.ModePlan:
			mode = s07_permission.ModePlan
		case s07_permission.ModeAuto:
			mode = s07_permission.ModeAuto
		}
	}

	perms := s07_permission.NewPermissionManager(mode)
	fmt.Printf("[Permission mode: %s]\n", mode)

	cwd, _ := os.Getwd()
	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))

	agent := s07_permission.New(provider, registry, perms)

	var history []llm.Message
	ctx := context.Background()

	fmt.Println("s07 Permission System — type 'q' to quit, '/mode <mode>' to switch, '/rules' to list rules")
	for {
		fmt.Print("\033[36ms07 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}

		// /mode command
		if strings.HasPrefix(query, "/mode") {
			parts := strings.Fields(query)
			if len(parts) == 2 {
				newMode := s07_permission.Mode(parts[1])
				switch newMode {
				case s07_permission.ModeDefault, s07_permission.ModePlan, s07_permission.ModeAuto:
					perms.Mode = newMode
					fmt.Printf("[Switched to %s mode]\n", newMode)
				default:
					fmt.Println("Usage: /mode <default|plan|auto>")
				}
			} else {
				fmt.Println("Usage: /mode <default|plan|auto>")
			}
			continue
		}

		// /rules command
		if query == "/rules" {
			for i, rule := range perms.Rules {
				fmt.Printf("  %d: %s\n", i, rule)
			}
			continue
		}

		history = append(history, llm.NewTextMessage(llm.RoleUser, query))
		state := &s02_tools.LoopState{Messages: history, TurnCount: 1}
		if err := agent.Run(ctx, state); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
			continue
		}
		if len(history) > 0 {
			if text := history[len(history)-1].ExtractText(); text != "" {
				fmt.Println(text)
			}
		}
		fmt.Println()
	}
}
