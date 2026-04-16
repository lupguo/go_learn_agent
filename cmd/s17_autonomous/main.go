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
	"github.com/lupguo/go_learn_agent/internal/s17_autonomous"
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
	requestsDir := filepath.Join(teamDir, "requests")
	tasksDir := filepath.Join(cwd, ".tasks")

	bus, err := s17_autonomous.NewMessageBus(inboxDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	reqStore, err := s17_autonomous.NewRequestStore(requestsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	teamMgr, err := s17_autonomous.NewTeammateManager(teamDir, cwd, tasksDir, bus, reqStore, provider)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	registry := tool.NewRegistry()
	registry.Register(s02_tools.NewBashTool(cwd))
	registry.Register(s02_tools.NewReadFileTool(cwd))
	registry.Register(s02_tools.NewWriteFileTool(cwd))
	registry.Register(s02_tools.NewEditFileTool(cwd))
	registry.Register(s17_autonomous.NewSpawnTeammateTool(teamMgr))
	registry.Register(s17_autonomous.NewListTeammatesTool(teamMgr))
	registry.Register(s17_autonomous.NewLeadSendMessageTool(bus))
	registry.Register(s17_autonomous.NewLeadReadInboxTool(bus))
	registry.Register(s17_autonomous.NewBroadcastTool(bus, teamMgr))
	registry.Register(s17_autonomous.NewShutdownRequestTool(bus, reqStore, teamMgr))
	registry.Register(s17_autonomous.NewCheckShutdownTool(reqStore))
	registry.Register(s17_autonomous.NewPlanReviewTool(bus, reqStore))
	registry.Register(s17_autonomous.NewLeadClaimTaskTool(tasksDir))

	agent := s17_autonomous.New(provider, registry, bus)

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	fmt.Println("s17 Autonomous Agents — type 'q' to quit, '/team', '/inbox', '/tasks'")
	for {
		fmt.Print("\033[36ms17 >> \033[0m")
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
		if query == "/tasks" {
			fmt.Println(s17_autonomous.ListTasks(tasksDir))
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