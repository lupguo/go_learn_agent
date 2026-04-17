---
layout: default
title: 首页
nav_order: 1
---

# Go Agent 开发学习

基于 Python 教程 `learn-claude-code` 的 Go 语言重新实现，4 阶段 19 步，完整覆盖 Agent 从单循环到多 Agent 平台的全链路。

| 项目 | 信息 |
|---|---|
| 模块 | `github.com/lupguo/go_learn_agent` |
| 源文件 | 96 个 `.go` 文件 |
| 代码行数 | ~12,400 行 |

---

## 文档导航

- [**Go Agent 开发完整指南**](go-agent-comprehensive-guide.html) — 19 个 Agent 的架构设计与交互流程（含 Mermaid 图）
- [**Go Agent 学习总结**](go-agent-learning-summary.html) — 项目实现总结与文件清单

---

## 学习路线

| 阶段 | 步骤 | 主题 |
|---|---|---|
| **核心引擎** | s01-s06 | Agent 循环、工具系统、计划、子 Agent、技能、上下文压缩 |
| **生产加固** | s07-s11 | 权限、钩子、记忆、提示工程、错误恢复 |
| **工作持久化** | s12-s14 | 持久任务、后台执行、定时调度 |
| **多 Agent 平台** | s15-s19 | 团队协作、协议、自主 Agent、工作树隔离、MCP 插件 |

---

## 快速开始

```bash
# 配置环境
cat > .env << 'EOF'
PROVIDER_TYPE=anthropic
ANTHROPIC_API_KEY=sk-ant-xxx
MODEL_ID=claude-sonnet-4-6
EOF

# 运行任意步骤
go run ./cmd/s01_agent_loop
go run ./cmd/s19_mcp_plugin
```
