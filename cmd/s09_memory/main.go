package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s09_memory"
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
	memoryDir := filepath.Join(cwd, ".memory")
	memMgr := s09_memory.NewMemoryManager(memoryDir)
	memMgr.LoadAll()

	if count := len(memMgr.Memories); count > 0 {
		fmt.Printf("[%d memories loaded into context]\n", count)
	} else {
		fmt.Println("[No existing memories. The agent can create them with save_memory.]")
	}

	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))
	registry.Register(s09_memory.NewSaveMemoryTool(memMgr))

	agent := s09_memory.New(provider, registry, memMgr)

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	fmt.Println("s09 Memory System — type 'q' to quit, '/memories' to list")
	for {
		fmt.Print("\033[36ms09 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}

		// /memories command
		if query == "/memories" {
			if len(memMgr.Memories) == 0 {
				fmt.Println("  (no memories)")
			} else {
				for name, mem := range memMgr.Memories {
					fmt.Printf("  [%s] %s: %s\n", mem.Type, name, mem.Description)
				}
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
