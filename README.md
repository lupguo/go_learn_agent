# Go Agent 开发学习项目

基于 shareAI-Lab 开源社区的 [learn-claude-code](https://github.com/shareAI-lab/learn-claude-code) Python 教程的 Go 语言扩展重实现，4 阶段 19 步，系统掌握 Agent 开发全链路。

## 参考项目

本项目参考学习了 shareAI-Lab 开源社区的 **[learn-claude-code](https://github.com/shareAI-lab/learn-claude-code)** Python 教程。该教程通过 12 个递进 Session，拆解了 Claude Code 的 Harness 工程——即让 AI Agent 在特定领域高效运作所需的基础设施（工具、权限、上下文、多 Agent 协作等），涵盖从核心 Agent 循环到多 Agent 团队协作的完整链路。本项目在其基础上扩展为 19 步，补充了生产加固（权限、Hook、记忆、Prompt 管道、错误恢复）和后台执行（Cron、Worktree 隔离、MCP & Plugin）等内容。

本项目使用 Go 语言对其进行重新实现，在保持教学脉络一致的基础上，充分利用 Go 的特性进行改进：

| Python 原版 | Go 重实现 |
|---|---|
| dict 消息结构 | 强类型 struct + interface，编译期类型安全 |
| 函数式 tool dispatch map | `Tool` interface + `Registry`，可扩展可测试 |
| threading + lock | goroutine + channel，原生并发更轻量 |
| Anthropic-only | `Provider` interface + 工厂模式，支持 Anthropic/OpenAI/Gemini |
| 全局变量管理状态 | 依赖注入 / 显式传参 |
| 每个文件重复 env 加载 | `llm.LoadEnvFile` + `llm.NewProviderFromEnv` 统一 3 行创建 |

## 项目结构

```
├── pkg/                    # 共享库
│   ├── llm/                # LLM 抽象层（Provider interface + 3 个适配器）
│   │   ├── anthropic/      # Anthropic Messages API
│   │   ├── openai/         # OpenAI Chat Completions API
│   │   └── gemini/         # Gemini generateContent API
│   └── tool/               # Tool interface + Registry
├── internal/               # 各步骤实现（19 个包）
│   ├── s01_loop/           # Agent 循环
│   ├── ...
│   └── s19_mcp_plugin/     # MCP & Plugin
├── cmd/                    # 各步骤独立入口（19 个 main.go）
├── docs/                   # 设计文档
└── .env.example            # 环境变量模板
```

## 快速开始

```bash
# 1. 复制环境变量模板
cp .env.example .env

# 2. 编辑 .env，填入 API Key
vi .env

# 3. 运行任意步骤
go run ./cmd/s01_agent_loop
go run ./cmd/s02_tool_use
go run ./cmd/s19_mcp_plugin
```

## Provider 切换

通过 `PROVIDER_TYPE` 环境变量选择 LLM 后端：

```bash
# Anthropic (默认)
PROVIDER_TYPE=anthropic
ANTHROPIC_API_KEY=sk-ant-xxx
MODEL_ID=claude-sonnet-4-6

# OpenAI
PROVIDER_TYPE=openai
OPENAI_API_KEY=sk-xxx
MODEL_ID=gpt-4o

# Gemini
PROVIDER_TYPE=gemini
GEMINI_API_KEY=AIza-xxx
MODEL_ID=gemini-2.0-flash
```

也支持 Anthropic 兼容的第三方服务（MiniMax、GLM、Kimi、DeepSeek），通过 `ANTHROPIC_BASE_URL` 覆盖端点即可。

## 学习路线

### 阶段 1：核心 Agent 引擎（s01-s06）

| 步骤 | 主题 | 核心概念 |
|---|---|---|
| s01 | Agent Loop | call LLM → check stop_reason → execute tools → loop |
| s02 | Tool Use | Tool interface + Registry，消息规范化 |
| s03 | Planning | 会话级任务跟踪，Tool 管理"工作记忆" |
| s04 | Subagent | 上下文隔离，子 Agent 干净消息历史 |
| s05 | Skill Loading | 两层技能模型：目录扫描 + 按需全文加载 |
| s06 | Context Compaction | 三层压缩策略防止上下文爆炸 |

### 阶段 2：生产加固（s07-s11）

| 步骤 | 主题 | 核心概念 |
|---|---|---|
| s07 | Permission | 安全管道：deny → mode check → allow → ask |
| s08 | Hook System | 外部脚本扩展，退出码约定 |
| s09 | Memory | 4 类持久记忆（user/feedback/project/reference） |
| s10 | System Prompt | 6 段 Prompt 管道，静态/动态边界 |
| s11 | Error Recovery | max_tokens 续写、prompt_too_long 压缩、指数退避 |

### 阶段 3：工作持久化与后台执行（s12-s14）

| 步骤 | 主题 | 核心概念 |
|---|---|---|
| s12 | Task System | 持久任务图，双向依赖管理 |
| s13 | Background Tasks | goroutine 非阻塞执行 + 通知队列 |
| s14 | Cron Scheduler | 5 字段 cron 解析，recurring/one-shot 调度 |

### 阶段 4：多 Agent 平台与外部集成（s15-s19）

| 步骤 | 主题 | 核心概念 |
|---|---|---|
| s15 | Agent Teams | JSONL 收件箱通信，teammate goroutine |
| s16 | Team Protocols | 结构化请求-响应 FSM（shutdown/plan approval） |
| s17 | Autonomous Agents | WORK→IDLE 自主循环，任务自动认领 |
| s18 | Worktree Isolation | Git worktree 执行隔离 + EventBus 可观测 |
| s19 | MCP & Plugin | MCP stdio JSON-RPC，统一权限门控 |

## 构建验证

```bash
go build ./...   # 全量编译
go vet ./...     # 静态检查
```

## 设计文档

- [学习计划](docs/plans/2025-04-09-go-agent-learning-plan.md) — 4 阶段 19 步详细规划
- [实现总结](docs/go-agent-learning-summary.md) — 每步的文件清单与关键设计决策

## 致谢

- [learn-claude-code](https://github.com/shareAI-lab/learn-claude-code) — shareAI-Lab 开源社区的 Agent Harness 工程教程，本项目的 Python 参考原版
