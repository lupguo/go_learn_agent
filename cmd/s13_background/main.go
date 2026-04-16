package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s13_background"
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
	runtimeDir := filepath.Join(cwd, ".runtime-tasks")

	bgManager, err := s13_background.NewBackgroundManager(cwd, runtimeDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))
	registry.Register(s13_background.NewBackgroundRunTool(bgManager))
	registry.Register(s13_background.NewCheckBackgroundTool(bgManager))

	agent := s13_background.New(provider, registry, bgManager)

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	fmt.Println("s13 Background Tasks — type 'q' to quit, '/bg' to list background tasks")
	for {
		fmt.Print("\033[36ms13 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}

		// Local command: /bg
		if query == "/bg" {
			fmt.Println(bgManager.Check(""))
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