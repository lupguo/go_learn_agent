# Go Agent 开发学习计划

> 基于 source/learn-claude-code Python 教程，使用 Go 语言重新实现，系统掌握 Agent 开发。

## 设计理念

### 为什么不完全镜像 Python 的 s01-s19 顺序？

Python 版本的教学顺序总体优秀，但有几个可以优化的地方：

1. **s03 (Todo/Planning)** 放在 s04 (Subagent) 之前，但实际上 Planning 是建立在 tool use 之上的一个简单扩展，而 Subagent 才是架构层面的跳跃。建议保持原顺序但在 Go 实现中强调 s03 是"tool use 的应用"而非新概念。

2. **s10 (System Prompt)** 被放在 Phase 2 末尾，但实际上 prompt 工程是贯穿始终的。Go 版本中建议从 s01 就建立 `prompt.Builder` 的雏形，s10 做最终的模块化整合。

3. **Phase 3-4 的线程模型**在 Python 中使用 threading，Go 天然支持 goroutine + channel，这是 Go 的优势。可以更优雅地实现背景任务和多 Agent 通信。

### Go 实现的改进点

| 原 Python 设计 | Go 改进方案 | 原因 |
|---|---|---|
| dict 作为消息/工具结构 | 强类型 struct + interface | 编译期类型安全，避免运行时 key 错误 |
| 函数式 tool dispatch map | `Tool` interface + registry | 更易于扩展和测试 |
| threading + lock | goroutine + channel | Go 原生并发模型，更安全 |
| JSON 文件直接读写 | 封装 Store interface | 可插拔存储后端 |
| 全局变量管理状态 | 依赖注入 / 显式传参 | 更好的可测试性 |
| Anthropic-only | `LLMProvider` interface | 支持多 LLM 后端 |

---

## 项目结构

```
go_learn_agent/
├── go.mod
├── go.sum
├── pkg/                          # 共享库包
│   ├── llm/                      # LLM 抽象层
│   │   ├── provider.go           # Provider interface
│   │   ├── anthropic/            # Claude 实现
│   │   ├── openai/               # OpenAI 实现
│   │   └── message.go            # 统一消息类型
│   ├── tool/                     # Tool 抽象层
│   │   ├── tool.go               # Tool interface
│   │   ├── registry.go           # Tool registry
│   │   └── result.go             # Tool result types
│   └── util/                     # 通用工具
│       ├── frontmatter.go        # Frontmatter 解析
│       └── jsonl.go              # JSONL 读写
├── cmd/                          # 每步独立可执行文件
│   ├── s01_agent_loop/
│   ├── s02_tool_use/
│   ├── ...
│   └── s19_mcp_plugin/
├── internal/                     # 各步骤内部实现
│   ├── s01_loop/
│   ├── s02_tools/
│   ├── ...
│   └── s19_mcp/
├── docs/
│   └── plans/
└── testdata/                     # 测试数据
```

---

## 学习路线（4 阶段 19 步）

### 阶段 1：核心 Agent 引擎 (s01-s06)

> 目标：构建一个能读写文件、管理任务、加载技能、控制上下文的单 Agent 系统

#### Step 01 — Agent Loop（Agent 循环）
- **对应 Python**: `s01_agent_loop.py`
- **核心概念**: Agent 的心跳 — `call LLM → check stop_reason → execute tools → loop`
- **Go 实现要点**:
  - 定义 `LLMProvider` interface（`SendMessage(ctx, messages, tools) (Response, error)`）
  - 定义 `Message`, `ToolCall`, `ToolResult` 强类型结构
  - 实现最小 agent loop：`for { response := llm.Send(); if !response.HasToolUse() { break }; executeTools(); }`
  - 支持 Anthropic Claude API（HTTP client + JSON 序列化）
- **交付物**: 可运行的 CLI，能与 LLM 对话并执行 bash 命令
- **学习收获**: 理解 Agent 的本质就是一个 tool-augmented 对话循环

#### Step 02 — Tool Use（工具系统）
- **对应 Python**: `s02_tool_use.py`
- **核心概念**: Tool 注册、分发、消息规范化
- **Go 实现要点**:
  - `Tool` interface: `Name()`, `Description()`, `Schema()`, `Execute(ctx, params) (Result, error)`
  - `ToolRegistry`: 注册/查找/列出 tools
  - 实现核心工具：`bash`, `read_file`, `write_file`, `edit_file`, `glob`, `grep`
  - 消息规范化：孤儿 tool_result 检测、同角色消息合并
- **交付物**: Agent 可以读写文件、搜索代码
- **学习收获**: 理解 Tool 是 Agent 与外部世界交互的唯一通道

#### Step 03 — Todo/Planning（会话级计划）
- **对应 Python**: `s03_todo_write.py`
- **核心概念**: 轻量级任务跟踪（会话内，非持久化）
- **Go 实现要点**:
  - `PlanManager` struct：管理 `PlanItem` 列表
  - 作为 Tool 注册：`plan_create`, `plan_update`, `plan_list`
  - 计划刷新提醒：N 轮未更新时注入提醒消息
- **交付物**: Agent 可以创建和跟踪多步任务
- **学习收获**: Tool 本身可以管理 Agent 的"工作记忆"

#### Step 04 — Subagent（子 Agent 隔离）
- **对应 Python**: `s04_subagent.py`
- **核心概念**: 上下文隔离 — 子 Agent 有干净的消息历史
- **Go 实现要点**:
  - `SubagentRunner`：启动一个新的 agent loop，空消息列表
  - 子 Agent 拥有父 Agent 工具的子集（不能递归 spawn）
  - 只返回最终文本摘要给父 Agent
  - `AgentTemplate`：从 `.md` frontmatter 解析 Agent 配置
- **交付物**: 父 Agent 可以委托探索任务给子 Agent
- **学习收获**: 理解 Agent 间的隔离模型和上下文管理

#### Step 05 — Skill Loading（技能加载）
- **对应 Python**: `s05_skill_loading.py`
- **核心概念**: 两层技能模型（目录 → 按需加载全文）
- **Go 实现要点**:
  - `SkillManifest`：name, description, body 的 frontmatter 结构
  - `SkillRegistry`：扫描目录，生成技能清单
  - 两层加载：Layer 1（名称+描述进 system prompt）→ Layer 2（按需读取全文）
  - 作为 Tool 暴露：`load_skill(name)`
- **交付物**: Agent 可以按需加载外部知识
- **学习收获**: 延迟加载 vs 预加载的权衡，类似 Go 的 lazy init 模式

#### Step 06 — Context Compaction（上下文压缩）
- **对应 Python**: `s06_context_compact.py`
- **核心概念**: 防止上下文爆炸的三层策略
- **Go 实现要点**:
  - 策略 1：大输出持久化到磁盘，消息中替换为摘要标记
  - 策略 2：旧 tool result 微压缩（截断 + 标记）
  - 策略 3：对话过长时 LLM 自动摘要 + 从摘要继续
  - `CompactManager`：跟踪 token 使用量，触发压缩
  - Transcript 保存（压缩前备份完整对话）
- **交付物**: Agent 可以处理长会话而不崩溃
- **学习收获**: Token 经济学，理解 LLM 的上下文窗口限制

**阶段 1 里程碑**: 一个完整的单 Agent 系统，能读写文件、管理任务、加载技能、控制上下文。

---

### 阶段 2：生产加固 (s07-s11)

> 目标：让 Agent 具备安全性、可扩展性、持久记忆和容错能力

#### Step 07 — Permission System（权限系统）
- **对应 Python**: `s07_permission_system.py`
- **核心概念**: 安全管道 — deny → mode check → allow → ask user
- **Go 实现要点**:
  - `PermissionManager` with pipeline pattern
  - `BashValidator`：检测危险命令（rm -rf, sudo, 管道注入等）
  - 三种模式：`default`（全部询问）、`plan`（只读）、`auto`（自动批准读操作）
  - 熔断器：连续 N 次拒绝后警告
- **学习收获**: Agent 安全不是附加功能，而是架构核心

#### Step 08 — Hook System（钩子系统）
- **对应 Python**: `s08_hook_system.py`
- **核心概念**: 不修改核心代码的扩展机制
- **Go 实现要点**:
  - Hook 事件：PreToolUse, PostToolUse, SessionStart
  - Go `os/exec` 运行外部脚本
  - 退出码约定：0=继续, 1=阻止, 2=注入消息
  - Hook 配置从 `.hooks.json` 加载
- **学习收获**: 开放-封闭原则在 Agent 架构中的应用

#### Step 09 — Memory System（记忆系统）
- **对应 Python**: `s09_memory_system.py`
- **核心概念**: 区分临时上下文和持久知识
- **Go 实现要点**:
  - 4 种记忆类型：user, feedback, project, reference
  - 每条记忆 = 独立 `.md` 文件 + frontmatter
  - `MEMORY.md` 索引文件（≤200 行）
  - `MemoryStore` interface：CRUD 操作
- **学习收获**: Memory ≠ Context — 记忆是持久的，上下文是临时的

#### Step 10 — System Prompt（系统提示工程）
- **对应 Python**: `s10_system_prompt.py`
- **核心概念**: Prompt 是管道，不是字符串拼接
- **Go 实现要点**:
  - `PromptBuilder`：6 个 section 有序组装
  - 静态/动态边界（DYNAMIC_BOUNDARY）
  - CLAUDE.md 链：global → project → subdirectory
  - System reminder 机制（独立 user message 注入高动态信息）
- **学习收获**: System prompt 的质量直接决定 Agent 的行为质量

#### Step 11 — Error Recovery（错误恢复）
- **对应 Python**: `s11_error_recovery.py`
- **核心概念**: 三条恢复路径
- **Go 实现要点**:
  - `max_tokens` → 注入 continuation 消息 → 重试（≤3次）
  - `prompt_too_long` → compact + 重试
  - 连接错误 → 指数退避 + 重试（≤3次）
  - Go 的 `context.Context` + `retry` 模式非常适合
- **学习收获**: 优雅降级 vs 直接崩溃

**阶段 2 里程碑**: Agent 现在是生产级的 — 安全、可扩展、有记忆、能容错。

---

### 阶段 3：工作持久化与后台执行 (s12-s14)

> 目标：Agent 的工作可以跨会话持久化，可以后台执行，可以自我调度

#### Step 12 — Task System（持久任务系统）
- **对应 Python**: `s12_task_system.py`
- **核心概念**: 持久化的任务图（与 s03 的会话级 todo 不同）
- **Go 实现要点**:
  - `TaskManager`：CRUD + 依赖图管理
  - `TaskRecord`：id, subject, status, owner, blockedBy, blocks
  - JSON 文件持久化到 `.tasks/`
  - 双向依赖：完成 A 时自动从 B 的 blockedBy 中移除
- **学习收获**: 任务图是协调复杂工作的关键数据结构

#### Step 13 — Background Tasks（后台任务）
- **对应 Python**: `s13_background_tasks.py`
- **核心概念**: 非阻塞执行 + 通知队列
- **Go 实现要点**:
  - `goroutine` 替代 Python threading — 更轻量、更安全
  - `channel` 作为通知队列（替代 Python 的 queue.Queue）
  - 输出持久化到 `.runtime-tasks/*.log`
  - 停滞检测：> 45s 的任务标记为 stalled
  - `select` + `context.WithTimeout` 实现优雅超时
- **学习收获**: Go 并发原语在 Agent 场景中的威力

#### Step 14 — Cron Scheduler（定时调度）
- **对应 Python**: `s14_cron_scheduler.py`
- **核心概念**: 基于 cron 语法的自调度
- **Go 实现要点**:
  - cron 解析器（5 字段标准格式）
  - `time.Ticker` 替代 Python 的 `sleep(1)` 循环
  - One-shot vs recurring 任务
  - PID 锁防止重复执行
  - 启动时检测漏执行的任务（≤24h 回溯）
- **学习收获**: Agent 可以安排自己的未来工作

**阶段 3 里程碑**: 工作系统现在是持久的、可后台的、可自调度的。

---

### 阶段 4：多 Agent 平台与外部集成 (s15-s19)

> 目标：从单 Agent 扩展到多 Agent 协作平台，集成外部工具

#### Step 15 — Agent Teams（Agent 团队）
- **对应 Python**: `s15_agent_teams.py`
- **核心概念**: 持久化的命名 Agent，JSONL 收件箱通信
- **Go 实现要点**:
  - `TeammateManager`：创建/管理/停止 teammates
  - 每个 teammate 运行在独立 goroutine 中
  - `MessageBus`：基于 JSONL 文件的 append-only 通信
  - `channel` 用于 goroutine 间实时通知，JSONL 用于持久化
- **学习收获**: 多 Agent 的核心是通信协议，不是 Agent 本身

#### Step 16 — Team Protocols（团队协议）
- **对应 Python**: `s16_team_protocols.py`
- **核心概念**: 结构化的请求-响应协议，FSM 状态机
- **Go 实现要点**:
  - 协议类型：shutdown_request, plan_approval
  - `RequestStore`：每个请求 = `.team/requests/` 下的 JSON 文件
  - request_id 关联请求和响应
  - FSM: pending → approved | rejected
- **学习收获**: 自由文本聊天 vs 结构化协议的权衡

#### Step 17 — Autonomous Agents（自主 Agent）
- **对应 Python**: `s17_autonomous_agents.py`
- **核心概念**: Agent 自主发现和认领工作
- **Go 实现要点**:
  - 空闲轮询：WORK → IDLE（每 5s 轮询 → 60s 后关闭）
  - 扫描 `.tasks/` 找未认领的任务
  - 角色匹配：`task.claim_role` 过滤
  - 身份重注入：上下文压缩后重新注入 `<identity>` 块
  - Go 的 `time.Ticker` + `select` 天然适合轮询模式
- **学习收获**: 自主性 = 感知（扫描）+ 决策（认领）+ 执行（工作）

#### Step 18 — Worktree Isolation（工作树隔离）
- **对应 Python**: `s18_worktree_task_isolation.py`
- **核心概念**: Git worktree 作为执行隔离环境
- **Go 实现要点**:
  - `WorktreeManager`：创建/绑定/清理 git worktrees
  - `EventBus`：append-only 事件日志 (`.worktrees/events.jsonl`)
  - 任务绑定到 worktree：`task.worktree = "auth-refactor"`
  - 收尾动作：keep（保留）或 remove（删除 worktree + 完成任务）
  - Go `os/exec` 调用 git 命令
- **学习收获**: 并行任务需要资源隔离，Git worktree 是代码级隔离方案

#### Step 19 — MCP & Plugin（外部工具集成）
- **对应 Python**: `s19_mcp_plugin.py`
- **核心概念**: Model Context Protocol — 外部工具的标准化接入
- **Go 实现要点**:
  - MCP 客户端：stdio 协议（JSON-RPC over stdin/stdout）
  - 工具命名：`mcp__servername__toolname`
  - `CapabilityGate`：统一权限检查（native + MCP 工具）
  - Plugin manifest：声明外部 server 启动方式
  - Go 的 `io.Pipe` + `json.Decoder` 适合 stdio 通信
- **学习收获**: 开放生态 = 标准协议 + 统一权限

**阶段 4 里程碑**: 完整的多 Agent 平台 — 自主协作、隔离执行、外部集成。

---

## 前置准备

在开始 Step 01 之前，需要完成：

1. **初始化 Go module**
   ```bash
   cd go_learn_agent && go mod init github.com/lupguo/go_learn_agent
   ```

2. **获取 API Key**
   - Anthropic Claude API Key（主要使用）
   - OpenAI API Key（可选，用于测试 pluggable 接口）

3. **设计 LLM Provider interface**（s01 的一部分）
   ```go
   type Provider interface {
       SendMessage(ctx context.Context, req *Request) (*Response, error)
   }
   ```

4. **设计统一消息类型**（s01 的一部分）
   ```go
   type Message struct {
       Role    Role          `json:"role"`
       Content []ContentBlock `json:"content"`
   }
   ```

---

## 每步实施节奏建议

每一步建议按以下流程：

1. **读**: 先读对应的 Python 源码，理解其设计意图
2. **思**: 思考 Go 的惯用实现方式，哪些可以直译、哪些需要重设计
3. **写**: 实现 Go 版本
4. **测**: 编写测试验证核心逻辑
5. **跑**: 实际运行，与 Python 版本对比行为
6. **记**: 记录学到的关键 insight 和 Go vs Python 的差异

---

## 附加学习资源

在实现过程中，可以参考 `source/OpenHands/` 项目的以下部分：
- Agent 循环实现：`openhands/agenthub/`
- 工具系统：`openhands/runtime/`
- 多 Agent 协作：`openhands/controller/`
- MCP 集成模式

这些是生产级参考，可以在完成对应步骤后对照学习。
