package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lupguo/go_learn_agent/internal/s02_tools"
	"github.com/lupguo/go_learn_agent/internal/s18_worktree"
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

	repoRoot := detectRepoRoot()
	tasksDir := filepath.Join(repoRoot, ".tasks")
	eventsPath := filepath.Join(repoRoot, ".worktrees", "events.jsonl")

	taskMgr, err := s18_worktree.NewTaskManager(tasksDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	events := s18_worktree.NewEventBus(eventsPath)
	wtMgr := s18_worktree.NewWorktreeManager(repoRoot, taskMgr, events)

	registry := tool.NewRegistry()
	// 4 base tools
	registry.Register(s02_tools.NewBashTool(repoRoot))
	registry.Register(s02_tools.NewReadFileTool(repoRoot))
	registry.Register(s02_tools.NewWriteFileTool(repoRoot))
	registry.Register(s02_tools.NewEditFileTool(repoRoot))
	// 5 task tools
	registry.Register(s18_worktree.NewTaskCreateTool(taskMgr))
	registry.Register(s18_worktree.NewTaskListTool(taskMgr))
	registry.Register(s18_worktree.NewTaskGetTool(taskMgr))
	registry.Register(s18_worktree.NewTaskUpdateTool(taskMgr))
	registry.Register(s18_worktree.NewTaskBindWorktreeTool(taskMgr))
	// 9 worktree tools
	registry.Register(s18_worktree.NewWTCreateTool(wtMgr))
	registry.Register(s18_worktree.NewWTListTool(wtMgr))
	registry.Register(s18_worktree.NewWTEnterTool(wtMgr))
	registry.Register(s18_worktree.NewWTStatusTool(wtMgr))
	registry.Register(s18_worktree.NewWTRunTool(wtMgr))
	registry.Register(s18_worktree.NewWTCloseoutTool(wtMgr))
	registry.Register(s18_worktree.NewWTRemoveTool(wtMgr))
	registry.Register(s18_worktree.NewWTKeepTool(wtMgr))
	registry.Register(s18_worktree.NewWTEventsTool(events))

	agent := s18_worktree.New(provider, registry)

	if wtMgr.GitAvailable() {
		fmt.Println("s18 Worktree Isolation — git detected")
	} else {
		fmt.Println("s18 Worktree Isolation — WARNING: not in a git repo, worktree commands will fail")
	}
	fmt.Println("Type 'q' to quit, '/tasks', '/worktrees', '/events'")

	var history []llm.Message
	scanner := bufio.NewScanner(os.Stdin)
	ctx := context.Background()

	for {
		fmt.Print("\033[36ms18 >> \033[0m")
		if !scanner.Scan() {
			break
		}
		query := strings.TrimSpace(scanner.Text())
		if query == "" || query == "q" || query == "exit" {
			break
		}
		if query == "/tasks" {
			fmt.Println(taskMgr.ListAll())
			fmt.Println()
			continue
		}
		if query == "/worktrees" {
			fmt.Println(wtMgr.ListAll())
			fmt.Println()
			continue
		}
		if query == "/events" {
			fmt.Println(events.ListRecent(20))
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

func detectRepoRoot() string {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(out))
	}
	cwd, _ := os.Getwd()
	return cwd
}