---
layout: default
title: Go Agent 学习总结
nav_order: 3
---

# Go Agent 开发学习总结

> 基于 Python 教程 `learn-claude-code` 的 Go 语言重新实现，4 阶段 19 步，完整覆盖 Agent 从单循环到多 Agent 平台的全链路。

## 项目信息

| 项 | 值 |
|---|---|
| 模块 | `github.com/lupguo/go_learn_agent` |
| Go 版本 | 1.26.1 |
| 源文件 | 96 个 `.go` 文件 |
| 代码行数 | ~12,400 行 |
| Python 参考 | `source/learn-claude-code/agents/s01-s19` |

## 架构概览

```
go_learn_agent/
├── pkg/                          # 共享库（7 个文件）
│   ├── llm/                      # LLM 抽象层
│   │   ├── types.go              # Provider interface, Message, ContentBlock, Request/Response
│   │   ├── provider.go           # 工厂：RegisterProvider, NewProvider, NewProviderFromEnv
│   │   ├── env.go                # 公共配置：LoadEnvFile, LoadConfigFromEnv
│   │   ├── anthropic/client.go   # Anthropic Messages API 适配
│   │   ├── openai/client.go      # OpenAI Chat Completions API 适配
│   │   └── gemini/client.go      # Gemini generateContent API 适配
│   └── tool/
│       └── registry.go           # Tool interface + Registry（注册/查找/分发）
├── internal/                     # 各步骤实现（19 个包）
│   ├── s01_loop/ ~ s19_mcp_plugin/
└── cmd/                          # 各步骤独立入口（19 个 main.go）
    ├── s01_agent_loop/ ~ s19_mcp_plugin/
```

### 核心抽象

```go
// LLM Provider — 任何 LLM 后端实现此接口
type Provider interface {
    SendMessage(ctx context.Context, req *Request) (*Response, error)
}

// Tool — 每个 Agent 工具实现此接口
type Tool interface {
    Name() string
    Description() string
    Schema() any
    Execute(ctx context.Context, input map[string]any) (string, error)
}
```

### 相较 Python 的改进

| Python 设计 | Go 改进 | 原因 |
|---|---|---|
| dict 消息结构 | 强类型 struct + interface | 编译期类型安全 |
| 函数式 tool dispatch | `Tool` interface + `Registry` | 可扩展、可测试 |
| threading + lock | goroutine + channel | Go 原生并发，更轻量 |
| Anthropic-only | `Provider` interface + 工厂模式 | 支持 Anthropic/OpenAI/Gemini 多后端 |
| 全局变量 | 依赖注入 / 显式传参 | 更好的可测试性 |
| 每个文件重复 env 加载 | `llm.LoadEnvFile` + `llm.NewProviderFromEnv` | 统一 3 行创建 |

---

## 阶段 1：核心 Agent 引擎（s01-s06）

### s01 — Agent Loop（Agent 循环）

**核心概念**：Agent 的心跳 — `call LLM → check stop_reason → execute tools → loop`

| 文件 | 职责 |
|---|---|
| `internal/s01_loop/agent.go` | 最小 Agent 循环：LoopState + Run() |
| `internal/s01_loop/bash_tool.go` | 单一 bash 工具 |
| `cmd/s01_agent_loop/main.go` | 交互式 REPL |

**关键设计**：`LoopState{Messages, TurnCount, TransitionReason}` — 贯穿所有后续步骤的状态容器。

---

### s02 — Tool Use（工具系统）

**核心概念**：Tool 注册、分发、消息规范化

| 文件 | 职责 |
|---|---|
| `internal/s02_tools/agent.go` | 标准 Agent 循环（被后续步骤复用） |
| `internal/s02_tools/bash_tool.go` | bash 工具（带危险命令过滤） |
| `internal/s02_tools/read_file_tool.go` | 文件读取 |
| `internal/s02_tools/write_file_tool.go` | 文件写入 |
| `internal/s02_tools/edit_file_tool.go` | 精确文本替换 |
| `internal/s02_tools/safepath.go` | 路径安全校验（防逃逸） |
| `internal/s02_tools/normalize.go` | 消息规范化：孤儿 tool_result 修复、同角色合并 |

**关键设计**：`NormalizeMessages()` — 修复因截断或错误产生的消息不一致，被所有后续 Agent 使用。4 个基础工具（bash/read/write/edit）也被后续步骤复用。

---

### s03 — Planning（会话级计划）

**核心概念**：轻量级任务跟踪（会话内，非持久化）

| 文件 | 职责 |
|---|---|
| `internal/s03_planning/plan.go` | PlanManager：PlanItem 列表管理 |
| `internal/s03_planning/todo_tool.go` | TodoTool：plan_create/plan_update/plan_list |
| `internal/s03_planning/agent.go` | Agent + 计划过期刷新提醒 |

**关键设计**：Tool 本身管理 Agent 的"工作记忆" — 计划不持久化，生命周期 = 会话。

---

### s04 — Subagent（子 Agent 隔离）

**核心概念**：上下文隔离 — 子 Agent 有干净的消息历史

| 文件 | 职责 |
|---|---|
| `internal/s04_subagent/agent.go` | 父 Agent 循环 |
| `internal/s04_subagent/task_tool.go` | TaskTool：spawn 子 Agent，返回摘要 |
| `internal/s04_subagent/template.go` | AgentTemplate：从 frontmatter 解析配置 |

**关键设计**：子 Agent = 新 LoopState + 工具子集，只返回文本摘要。不能递归 spawn。

---

### s05 — Skill Loading（技能加载）

**核心概念**：两层技能模型 — 目录级扫描 + 按需加载全文

| 文件 | 职责 |
|---|---|
| `internal/s05_skills/registry.go` | SkillRegistry：扫描 `.skills/`，frontmatter 解析 |
| `internal/s05_skills/load_skill_tool.go` | LoadSkillTool：按需读取技能全文 |
| `internal/s05_skills/agent.go` | Agent + Layer 1 清单注入 system prompt |

**关键设计**：Layer 1 = 名称+描述（进 system prompt），Layer 2 = 按需全文加载（tool 调用）。

---

### s06 — Context Compaction（上下文压缩）

**核心概念**：防止上下文爆炸的三层策略

| 文件 | 职责 |
|---|---|
| `internal/s06_compact/compact.go` | CompactManager：token 估算、旧消息微压缩、LLM 摘要 |
| `internal/s06_compact/compact_tool.go` | CompactTool：手动触发压缩 |
| `internal/s06_compact/agent.go` | Agent + 自动压缩触发 |

**关键设计**：策略 1（大输出持久化）→ 策略 2（旧 tool result 截断）→ 策略 3（LLM 摘要 + transcript 备份）。

---

## 阶段 2：生产加固（s07-s11）

### s07 — Permission System（权限系统）

**核心概念**：安全管道 — deny → mode check → allow → ask user

| 文件 | 职责 |
|---|---|
| `internal/s07_permission/permission.go` | PermissionManager：pipeline 模式，BashValidator |
| `internal/s07_permission/agent.go` | Agent + 权限检查拦截 |

**关键设计**：三种模式（default/plan/auto），危险命令检测（rm -rf, sudo, 管道注入），熔断器。

---

### s08 — Hook System（钩子系统）

**核心概念**：不修改核心代码的扩展机制

| 文件 | 职责 |
|---|---|
| `internal/s08_hooks/hooks.go` | HookManager：PreToolUse/PostToolUse/SessionStart |
| `internal/s08_hooks/agent.go` | Agent + hook 执行（退出码约定：0=继续, 1=阻止, 2=注入） |

**关键设计**：Hook = 外部脚本（`os/exec`），通过退出码和 stdout 控制 Agent 行为。

---

### s09 — Memory System（记忆系统）

**核心概念**：区分临时上下文和持久知识

| 文件 | 职责 |
|---|---|
| `internal/s09_memory/memory.go` | MemoryManager：4 类记忆（user/feedback/project/reference），MEMORY.md 索引 |
| `internal/s09_memory/save_memory_tool.go` | SaveMemoryTool：创建/更新记忆文件 |
| `internal/s09_memory/agent.go` | Agent + 记忆上下文注入 |

**关键设计**：每条记忆 = 独立 `.md` 文件 + frontmatter，`MEMORY.md` 作为索引（≤200 行）。

---

### s10 — System Prompt（系统提示工程）

**核心概念**：Prompt 是管道，不是字符串拼接

| 文件 | 职责 |
|---|---|
| `internal/s10_prompt/builder.go` | PromptBuilder：6 个 section 有序组装，动态边界 |
| `internal/s10_prompt/agent.go` | Agent + builder 驱动的 system prompt |

**关键设计**：6 段管道（identity → env → tools → rules → memory → dynamic），静态/动态边界分离。

---

### s11 — Error Recovery（错误恢复）

**核心概念**：三条恢复路径

| 文件 | 职责 |
|---|---|
| `internal/s11_recovery/recovery.go` | RecoveryManager：max_tokens 续写、prompt_too_long 压缩、连接错误退避 |
| `internal/s11_recovery/agent.go` | Agent + 自动恢复循环 |

**关键设计**：`max_tokens` → 注入 continuation 重试（≤3次），`prompt_too_long` → compact + 重试，连接错误 → 指数退避。

---

## 阶段 3：工作持久化与后台执行（s12-s14）

### s12 — Task System（持久任务系统）

**核心概念**：持久化的任务图，带依赖关系

| 文件 | 职责 |
|---|---|
| `internal/s12_tasks/manager.go` | TaskManager：`.tasks/` JSON 持久化，依赖图（blockedBy/blocks） |
| `internal/s12_tasks/task_tools.go` | 4 个 tool：task_create/get/update/list |
| `internal/s12_tasks/agent.go` | Agent 循环 |

**关键设计**：双向依赖同步 — `addBlocks` 自动更新被阻塞任务的 `blockedBy`；任务完成时 `clearDependency` 清理所有引用。

---

### s13 — Background Tasks（后台任务）

**核心概念**：非阻塞执行 + 通知队列

| 文件 | 职责 |
|---|---|
| `internal/s13_background/background.go` | BackgroundManager：goroutine 执行，Notification 队列，停滞检测 |
| `internal/s13_background/bg_tools.go` | BackgroundRunTool, CheckBackgroundTool |
| `internal/s13_background/agent.go` | Agent + drain-before-LLM 模式 |

**关键设计**：后台命令在 goroutine 中执行，完成后推入通知队列。Agent 每轮 LLM 调用前先 drain 通知，注入 `<background-results>` 消息。

---

### s14 — Cron Scheduler（定时调度）

**核心概念**：基于 cron 语法的自调度

| 文件 | 职责 |
|---|---|
| `internal/s14_cron/cron.go` | CronScheduler：5 字段解析器，后台 goroutine 1s tick，jitter，自动过期 |
| `internal/s14_cron/cron_tools.go` | CronCreateTool, CronDeleteTool, CronListTool |
| `internal/s14_cron/agent.go` | Agent + cron 通知 drain |

**关键设计**：完整 5 字段 cron 解析（`*`, `*/N`, `N`, `N-M`, 逗号列表），FNV hash 确定性 jitter，recurring/one-shot + durable/session 模式。

---

## 阶段 4：多 Agent 平台与外部集成（s15-s19）

### s15 — Agent Teams（Agent 团队）

**核心概念**：持久化命名 Agent + JSONL 通信

| 文件 | 职责 |
|---|---|
| `internal/s15_teams/message_bus.go` | MessageBus：JSONL 收件箱，Send/ReadInbox/Broadcast |
| `internal/s15_teams/teammate.go` | TeammateManager：spawn goroutine，独立 tool 集，≤50 turn |
| `internal/s15_teams/lead_tools.go` | 5 个 lead 工具：spawn/list/send/read/broadcast |
| `internal/s15_teams/agent.go` | Lead Agent + inbox drain |

**关键设计**：每个 teammate = 独立 goroutine + 绑定 sender 的专属 tool 集，防止跨 Agent 干扰。

---

### s16 — Team Protocols（团队协议）

**核心概念**：结构化请求-响应协议 + FSM

| 文件 | 职责 |
|---|---|
| `internal/s16_protocols/message_bus.go` | 扩展 InboxMessage（RequestID/Approve/Plan/Feedback） |
| `internal/s16_protocols/request_store.go` | RequestStore：`.team/requests/` JSON 持久化 |
| `internal/s16_protocols/teammate.go` | Teammate + 协议 tool（shutdown response, plan approval） |
| `internal/s16_protocols/lead_tools.go` | 12 个 lead 工具（+3 协议：shutdown/check/plan_review） |
| `internal/s16_protocols/agent.go` | Lead Agent |

**关键设计**：两种协议 — shutdown（request→response FSM）和 plan approval（submit→review FSM），通过 RequestStore 持久化状态。

---

### s17 — Autonomous Agents（自主 Agent）

**核心概念**：Agent 自主发现和认领工作

| 文件 | 职责 |
|---|---|
| `internal/s17_autonomous/task_board.go` | ScanUnclaimedTasks, ClaimTask（互斥锁 + 事件日志） |
| `internal/s17_autonomous/teammate.go` | autonomousLoop：WORK→IDLE 周期，5s 轮询，60s 超时 |
| `internal/s17_autonomous/lead_tools.go` | 13 个 lead 工具（+LeadClaimTaskTool） |
| `internal/s17_autonomous/agent.go` | Lead Agent |
| `internal/s17_autonomous/message_bus.go` | 同 s16 |
| `internal/s17_autonomous/request_store.go` | 同 s16 |

**关键设计**：WORK→IDLE→auto-claim 循环；`ensureIdentityContext` 在上下文压缩后重新注入 `<identity>` 块；idle + claim_task 专用工具。

---

### s18 — Worktree Isolation（工作树隔离）

**核心概念**：Git worktree 作为执行隔离环境

| 文件 | 职责 |
|---|---|
| `internal/s18_worktree/event_bus.go` | EventBus：append-only JSONL 生命周期事件 |
| `internal/s18_worktree/task_manager.go` | WTaskRecord（扩展 worktree 绑定字段），TaskManager |
| `internal/s18_worktree/worktree.go` | WorktreeManager：git worktree create/enter/run/remove/keep/closeout |
| `internal/s18_worktree/tools.go` | 14 个 tool 结构体（5 task + 9 worktree） |
| `internal/s18_worktree/agent.go` | Agent 循环 |

**关键设计**：任务是控制平面，worktree 是执行平面。WorktreeManager 封装 git 命令，EventBus 提供全链路可观测性。

---

### s19 — MCP & Plugin（外部工具集成）

**核心概念**：Model Context Protocol — 外部工具标准化接入

| 文件 | 职责 |
|---|---|
| `internal/s19_mcp_plugin/permission.go` | CapabilityPermissionGate：统一权限（native + MCP），风险分级 |
| `internal/s19_mcp_plugin/mcp_client.go` | MCPClient：stdio JSON-RPC 2.0，connect/list_tools/call_tool |
| `internal/s19_mcp_plugin/plugin_loader.go` | PluginLoader：扫描 `.claude-plugin/plugin.json` manifest |
| `internal/s19_mcp_plugin/mcp_router.go` | MCPToolRouter：`mcp__{server}__{tool}` 前缀路由，BuildToolPool 合并 |
| `internal/s19_mcp_plugin/agent.go` | Agent + 权限门控 + 统一分发 |

**关键设计**：外部 MCP 工具进入与原生工具相同的 pipeline — 统一权限检查、规范化结果、单一 tool pool 发送给 LLM。

---

## 公共基础设施（pkg/）

### LLM 抽象层（`pkg/llm/`）

| 文件 | 职责 |
|---|---|
| `types.go` | Provider interface, Message, ContentBlock, Request/Response, StopReason |
| `provider.go` | 工厂模式：Config + RegisterProvider + NewProvider + NewProviderFromEnv |
| `env.go` | LoadEnvFile + LoadConfigFromEnv（PROVIDER_TYPE/API_KEY/MODEL_ID/BASE_URL） |
| `anthropic/client.go` | Anthropic Messages API 适配（wire format 转换，init 自注册） |
| `openai/client.go` | OpenAI Chat Completions API 适配（tool_calls JSON string, role=tool） |
| `gemini/client.go` | Gemini generateContent API 适配（functionCall/functionResponse, role=model） |

**Provider 切换**：通过 `PROVIDER_TYPE` 环境变量选择后端，各 Provider 在 `init()` 中自注册，19 个入口统一 3 行代码创建。

### 工具系统（`pkg/tool/`）

| 文件 | 职责 |
|---|---|
| `registry.go` | Tool interface + Registry：Register/Get/ToolDefs/Execute |

---

## 运行方式

```bash
# 1. 配置 .env
cat > .env << 'EOF'
PROVIDER_TYPE=anthropic
ANTHROPIC_API_KEY=sk-ant-xxx
MODEL_ID=claude-sonnet-4-6
EOF

# 2. 运行任意步骤
go run ./cmd/s01_agent_loop
go run ./cmd/s12_tasks
go run ./cmd/s19_mcp_plugin

# 3. 切换 Provider
PROVIDER_TYPE=openai OPENAI_API_KEY=sk-xxx MODEL_ID=gpt-4o go run ./cmd/s01_agent_loop
```

## 构建验证

```bash
go build ./...   # 全量编译
go vet ./...     # 静态检查
```
