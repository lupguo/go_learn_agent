package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s12_tasks"
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
	tasksDir := filepath.Join(cwd, ".tasks")

	taskMgr, err := s12_tasks.NewTaskManager(tasksDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))
	registry.Register(s12_tasks.NewTaskCreateTool(taskMgr))
	registry.Register(s12_tasks.NewTaskUpdateTool(taskMgr))
	registry.Register(s12_tasks.NewTaskListTool(taskMgr))
	registry.Register(s12_tasks.NewTaskGetTool(taskMgr))

	agent := s12_tasks.New(provider, registry)

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	fmt.Println("s12 Task System — type 'q' to quit, '/tasks' to list tasks")
	for {
		fmt.Print("\033[36ms12 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}

		// Local command: /tasks
		if query == "/tasks" {
			fmt.Println(taskMgr.ListAll())
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
