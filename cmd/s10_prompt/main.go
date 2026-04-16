package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s10_prompt"
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
	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))

	builder := s10_prompt.NewBuilder(cwd, registry.ToolDefs(), os.Getenv("MODEL_ID"))

	// Show assembled prompt stats at startup
	fullPrompt := builder.Build()
	sectionCount := strings.Count(fullPrompt, "\n# ")
	fmt.Printf("[System prompt assembled: %d chars, ~%d sections]\n", len(fullPrompt), sectionCount)

	agent := s10_prompt.New(provider, registry, builder)

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	fmt.Println("s10 System Prompt — type 'q' to quit, '/prompt' to view, '/sections' to list")
	for {
		fmt.Print("\033[36ms10 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}

		// /prompt — show full assembled prompt
		if query == "/prompt" {
			fmt.Println("--- System Prompt ---")
			fmt.Println(builder.Build())
			fmt.Println("--- End ---")
			continue
		}

		// /sections — show section headers only
		if query == "/sections" {
			prompt := builder.Build()
			for _, line := range strings.Split(prompt, "\n") {
				if strings.HasPrefix(line, "# ") || line == s10_prompt.DynamicBoundary {
					fmt.Printf("  %s\n", line)
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
