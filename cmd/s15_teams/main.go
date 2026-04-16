package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s15_teams"
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
	teamDir := filepath.Join(cwd, ".team")
	inboxDir := filepath.Join(teamDir, "inbox")

	bus, err := s15_teams.NewMessageBus(inboxDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	teamMgr, err := s15_teams.NewTeammateManager(teamDir, cwd, bus, provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))
	registry.Register(s15_teams.NewSpawnTeammateTool(teamMgr))
	registry.Register(s15_teams.NewListTeammatesTool(teamMgr))
	registry.Register(s15_teams.NewLeadSendMessageTool(bus))
	registry.Register(s15_teams.NewLeadReadInboxTool(bus))
	registry.Register(s15_teams.NewBroadcastTool(bus, teamMgr))

	agent := s15_teams.New(provider, registry, bus)

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	fmt.Println("s15 Agent Teams — type 'q' to quit, '/team' to list, '/inbox' to read")
	for {
		fmt.Print("\033[36ms15 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}

		if query == "/team" {
			fmt.Println(teamMgr.ListAll())
			fmt.Println()
			continue
		}
		if query == "/inbox" {
			msgs := bus.ReadInbox("lead")
			data, _ := json.MarshalIndent(msgs, "", "  ")
			fmt.Println(string(data))
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