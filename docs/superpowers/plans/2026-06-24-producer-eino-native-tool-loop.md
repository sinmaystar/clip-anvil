# Producer Eino Native Tool Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 Producer 从 `draft_response` 内部工具循环迁移到更显式的 Eino 工具循环结构，确保实际工具执行经过 Eino `ToolsNode`，工具调用、HITL interrupt/resume、以及 agent 运行期数据库数据都可验证。

**Architecture:** 保留现有 `draft_response` 兼容路径；新增显式工具循环路径，图节点表达为 `load_context -> prepare_turn_state -> call_model -> execute_tools -> append_tool_results -> call_model -> finalize_response`。`execute_tools` 节点负责调用 Eino `ToolsNode`，HITL 继续通过 Eino 原生 `StatefulInterrupt` 与 checkpoint resume 传播。

**Tech Stack:** Go 1.26, CloudWeGo Eino v0.9.9, Hertz, pgx/sqlc, PostgreSQL, existing agent runtime tables.

---

## Constraints

- 不改动无关文件，不回滚用户已有改动。
- 先写失败测试，再写生产代码。
- 保持默认 Producer 行为兼容；显式工具循环通过 `GraphConfig` 模式开启，验证后再切默认。
- `request_user_decision` 的 HITL 必须继续依赖 Eino interrupt/resume，而不是绕开为业务层状态机。
- 端到端验证必须检查 `agent_thread`、`agent_task`、`agent_event`、`agent_message` 的关键数据。

## Task 1: Add Failing Tests

- [ ] 在 `apps/server/internal/agent/producer/graph_test.go` 添加显式工具循环图结构测试：
  - 构造 `GraphConfig{Mode: ProducerGraphModeExplicitToolLoop}`。
  - 使用 `einoruntime.GraphInfoRegistry` 捕获图。
  - 断言存在 `call_model`、`execute_tools`、`append_tool_results`、`finalize_response`。
  - 断言存在 `execute_tools -> append_tool_results` 与 `append_tool_results -> call_model`。
- [ ] 添加显式工具循环工具调用测试：
  - 模型第一轮返回 `create_agent_text_node` native tool call。
  - 工具通过 Eino `ToolsNode` 执行。
  - 第二轮模型看到 same-turn tool result 后返回最终文本。
- [ ] 添加显式工具循环 HITL resume 测试：
  - 模型第一轮返回 `request_user_decision` tool call。
  - 第一次运行返回 Eino interrupt。
  - 使用同一 checkpoint key 与 resume data 第二次运行。
  - 断言最终 assistant 文本正确。
- [ ] 运行失败测试：

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'TestProducerGraphExplicit' -count=1
```

Expected: 因 `ProducerGraphModeExplicitToolLoop` 未实现而失败。

## Task 2: Implement Explicit Tool Loop

- [ ] 在 `apps/server/internal/agent/producer/graph.go` 增加：
  - `ProducerGraphMode` 类型。
  - `ProducerGraphModeInlineDraft` 默认模式。
  - `ProducerGraphModeExplicitToolLoop` 新模式。
  - `ProducerLoopState` 内部状态结构。
- [ ] 将现有图构建拆为 `newInlineDraftGraph`，保持默认图结构不变。
- [ ] 新增 `newExplicitToolLoopGraph`：
  - `load_context`
  - `prepare_turn_state`
  - `call_model`
  - branch 到 `prepare_tool_message` / `finalize_response` / `fail_turn`
  - `prepare_tool_message`
  - `execute_tools`
  - `append_tool_results`
  - `finalize_response`
  - `fail_turn`
- [ ] `execute_tools` 必须创建并调用现有 `newEinoProducerToolNode`，不能直接调用 `ToolExecutor`。
- [ ] `append_tool_results` 复用 `appendNativeSameTurnMessages`，保证下一轮模型能看到 assistant tool call 与 tool result。
- [ ] 抽取 `finalizeProducerOutput` 复用现有空文本/reasoning fallback 行为。

## Task 3: Runtime Wiring

- [ ] 在 Producer Runner/Server 初始化处打开显式工具循环模式。
- [ ] 保留测试或配置里默认模式，避免未迁移调用方被影响。
- [ ] 确认 `CheckPointStore`、`WithCheckPointID`、`BatchResumeWithData` 仍由 `Graph.Run` 统一传入。

## Task 4: E2E Database Verification

- [ ] 增加 `scripts/smoke-m6-producer-toolnode-hitl.sh`。
- [ ] 脚本通过 HTTP API 创建测试账号、agent workspace、发送 agent 消息。
- [ ] 使用确定性测试开关触发：
  - 一次 Eino ToolNode 工具调用。
  - 一次 HITL decision request。
  - 一次 decision resume。
- [ ] 脚本查询数据库，校验：
  - `agent_thread` 存在且 workspace/thread 对应。
  - `agent_task` 至少包含 `producer_turn` 与 `decision_resume`，状态正确。
  - `agent_event` 包含 decision requested/resolved 或 graph interrupted/resumed 相关事件。
  - `agent_message` 包含 user message、assistant/tool-visible message、最终 assistant message。
- [ ] 若现有 HTTP 路径无法稳定触发指定模型工具调用，添加仅测试/本地 smoke 可用的后端测试入口或 Go E2E test，仍必须落到真实数据库。

## Task 5: Verification

- [ ] 运行 Producer 单测：

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -count=1
```

- [ ] 运行后端相关测试：

```bash
cd apps/server
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/... -count=1
```

- [ ] 启动本地运行时并执行 smoke：

```bash
./scripts/dev-start.sh
./scripts/smoke-m6-producer-toolnode-hitl.sh
```

- [ ] 运行文档/空白检查：

```bash
git diff --check
```
