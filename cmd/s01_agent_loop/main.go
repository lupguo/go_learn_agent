// s01_agent_loop is the minimal agent — demonstrates the core loop pattern.
//
// Usage:
//
//	go run ./cmd/s01_agent_loop
//
// Requires .env file (or environment variables):
//
//	ANTHROPIC_API_KEY=sk-ant-xxx
//	MODEL_ID=claude-sonnet-4-6
//	ANTHROPIC_BASE_URL=https://api.anthropic.com  (optional)
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s01_loop"
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

	// Create tool registry with just bash
	registry := tool.NewRegistry()
	registry.Register(s01_loop.NewBashTool())

	// Create agent
	agent := s01_loop.New(provider, registry)

	// Interactive REPL
	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	fmt.Println("s01 Agent Loop — type 'q' to quit")
	for {
		fmt.Print("\033[36ms01 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}

		history = append(history, llm.NewTextMessage(llm.RoleUser, query))
		state := &s01_loop.LoopState{
			Messages:  history,
			TurnCount: 1,
		}

		if err := agent.Run(ctx, state); err != nil {
			fmt.Fprintf(os.Stderr, "\033[31mError: %v\033[0m\n", err)
			continue
		}

		// Print the final assistant text
		if len(history) > 0 {
			last := history[len(history)-1]
			if text := last.ExtractText(); text != "" {
				fmt.Println(text)
			}
		}
		fmt.Println()
	}
}
