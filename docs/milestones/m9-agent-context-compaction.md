# M9 Agent Context Compaction 与 Sandbox 文件工具 — 里程碑

**状态**：M9.1、M9.2、M9.3、M9.4、M9.5 已完成（2026-06-30）
**日期**：2026-06-30
**目标**：为 Producer / Craftsman / Reviewer / Composer 引入共享上下文压缩能力，并提供通用 sandbox `read_file` / `edit_file` 工具，让长任务中的历史工具结果、调研记录、计划草稿和媒体元信息都能被压缩、恢复和审计。

参考文档：

- [Agent Context Compaction 上下文压缩方案设计](../superpowers/specs/2026-06-30-agent-context-compaction-design.md)
- [M9.1 实施计划](../superpowers/plans/2026-06-30-m9-1-contextcompact-file-tools.md)
- [M9.2 实施计划](../superpowers/plans/2026-06-30-m9-2-micro-compact.md)
- [M9.3 实施计划](../superpowers/plans/2026-06-30-m9-3-four-agent-contextcompact.md)
- [M9.4 实施计划](../superpowers/plans/2026-06-30-m9-4-full-compact.md)
- [M9.5 实施计划](../superpowers/plans/2026-06-30-m9-5-media-e2e.md)
- [M8 Agent Skill Runtime 与 OpenMontage 本地化](./m8-agent-skill-runtime.md)
- [Agent MultiAgent 架构现状](../engineering/agent-multiagent-architecture.md)
- [数据库工程说明](../engineering/database.md)

## Codex Goal 建议

按阶段完成 M9 Agent Context Compaction。每个阶段必须先写该阶段实施计划，再开发、验证、记录验收结果；上一阶段未通过验收时，不进入下一阶段。

第一版只交付 **共享上下文压缩中间件 + 通用 sandbox 文本文件读写 + PostgreSQL 文本搜索恢复**。不要做向量数据库、用户可编辑压缩摘要 UI、远程文件管理器、复杂目录 ACL，也不要改变 Producer pending signal 的 claim / drain 时机。

## 已确认口径

- 四个 Agent 当前是 Eino Graph / native tool loop，第一版不强行迁移成 Eino ADK `ChatModelAgent`。
- 压缩发生在 `before_model -> call_model` 之间，由共享 `ContextCompactMiddleware` 改写模型可见 messages。
- Micro compact 优先压缩旧 tool result、长 tool call args、ffprobe / ffmpeg 日志、旧 skill 正文结果。
- Full compact 生成可 handoff 的结构化摘要，但不能覆盖 DB 事实源。
- 媒体文件本体不进入压缩摘要；图片、视频、音频保留稳定 media ref、media card、probe summary 和必要 sandbox path。
- 已知 `detail_file` 时，Agent 用 `read_file` 分段恢复原文；不知道路径时，用 `search_agent_history` 定位 compaction record 和 `detail_file`。
- 压缩只影响模型输入投影，不影响 Agent 对话框消息列表展示；历史消息列表仍展示原始 `agent_message` 内容。
- 通用文件工具只提供 `read_file` / `edit_file`；目录枚举、`find`、`grep`、`rg` 由 exec shell 工具实现。
- sandbox 已提供生命周期和持久化能力，第一版不为通用文件工具新增单独文件索引表。
- `read_file` / `edit_file` 只强制 `/workspace` 边界、文本大小限制和路径安全；目录语义通过 Agent prompt 和工具说明约束。
- Agent 写到 sandbox 的 notes / plans 是工作笔记，不是业务事实源；会影响产品状态的内容仍必须通过结构化工具落库。

## 阶段里程碑

| 阶段 | 里程碑 | 可验收标准 |
|---|---|---|
| M9.1 Sandbox 文件工具与压缩规划基座 | 新增 config、token counter、media card builder、compaction planner，并实现通用 `read_file` / `edit_file` native tools。 | ✅ 已完成；四个 Agent 都注册 `read_file` / `edit_file`；`read_file` 支持 offset / limit / truncated / next_offset；`edit_file` 支持 create / create_or_overwrite / append / replace；路径逃逸 `/workspace` 被拒绝；planner 能记录 token 统计和候选压缩计划但不改 prompt。验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`。 |
| M9.2 Micro Compact 与可恢复原文 | 新增压缩记录表、sqlc、micro compact，实现大型原文落 `/workspace/.clipanvil/context/`，prompt 用占位符替换旧工具结果。 | ✅ 已完成；新增 `agent_context_compaction` / `agent_message_compaction` sidecar 表和 sqlc；Producer / Composer 在小阈值下可触发 micro compact；四个 Agent 注册 `search_agent_history`；占位符只进入模型输入 projection；原始 `agent_message` 不删除、不覆盖；当前 same-turn tool loop、pending reminder、最新用户消息保留。验证：`make sqlc-generate`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。 |
| M9.3 四 Agent 统一接入 | Craftsman / Reviewer 接入同一中间件，统一 GraphConfig / responder message override，补齐四角色保留策略。 | ✅ 已完成；Producer、Craftsman、Reviewer、Composer 都走同一 `contextcompact` 实现；Craftsman 当前任务文本、Reviewer 当前图片 review message、Producer pending reminder、Composer 最新 same-turn assistant/tool 对均被保护；历史 source `agent_message.id` 写入 sidecar link；压缩只影响模型输入 projection，不改 Agent 对话框消息列表。验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。 |
| M9.4 Full Compact 与 Handoff Summary | 新增 summary model runner 和 handoff summary prompt，超过 full 阈值时生成结构化摘要，并支持 context overflow 后一次性 retry。 | ✅ 已完成；full compact 在模型调用前生成结构化 handoff summary，写入 `mode=full` sidecar record 和 sandbox detail file；summary 输入包含角色 facts / media cards，不只总结 `agent_message`；四角色支持 context overflow 一次性 `ForceFullCompact` retry；搜索工具返回 full record `mode`。验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。 |
| M9.5 媒体策略与真实 E2E | 完善 image / video / audio media cards，压缩 Composer probe/stage/ffmpeg 结果，并跑真实营销视频生成流程验证长上下文。 | ✅ 已完成；真实营销视频流程生成最终视频并完成 Reviewer quality gate；Reviewer rejected 是对 shot_03 静态 fallback 的合理质量判断；Producer 通过 `review_completed` signal 闭环唤醒并进入 HITL；真实 DB 中 Producer / Craftsman / Composer / Reviewer 均有可审计压缩链路；Agent 对话框消息列表仍展示原始 `agent_message` 内容。验证：`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。 |

## 阶段验收建议

### M9.1 Sandbox 文件工具与压缩规划基座

✅ 已完成（2026-06-30）。目标是先把 Agent 的通用文件能力和压缩规划能力做稳，不改变现有模型 prompt。

必须交付：

- `apps/server/internal/agent/contextcompact` 包的 config、token counter、media card builder、planner。
- sandbox `read_file` native tool。
- sandbox `edit_file` native tool。
- 四个 Agent native registry 注册 `read_file` / `edit_file`。
- planner diagnostics：记录 token_before、candidate messages、candidate estimated savings。
- path normalize 和 `/workspace` 边界校验。

验收命令：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

重点测试：

- `read_file` 分段读取 UTF-8 文本，超过 limit 返回 `truncated=true` 和 `next_offset`。
- `edit_file` 的 create 在文件存在时失败。
- `edit_file` 的 create_or_overwrite 可创建或覆盖文本。
- `edit_file` 的 append 只追加文本。
- `edit_file` 的 replace 要求 `old_text` 唯一匹配。
- `../`、绝对路径逃逸、symlink escape 被拒绝。
- planner 只输出候选计划，不改写模型消息。

完成记录：

- 新增 `apps/server/internal/agent/contextcompact`，包含 config defaults、token counter、media card skeleton 和只读 planner。
- token counter 使用 `github.com/pkoukk/tiktoken-go`，未知模型使用保守 heuristic fallback。
- planner 只记录 token_before 和候选压缩计划，不改写任何 `schema.Message`。
- 新增 sandbox 文本文件 path 校验和 `ApplyTextEdit`，覆盖 create / create_or_overwrite / append / replace。
- 新增 `read_file` / `edit_file` native tools，基于当前 workspace sandbox 的 `Download` / `Upload` / `Exec` 工作。
- Producer、Craftsman、Reviewer、Composer 四个 native registry 均已注册 `read_file` / `edit_file`。
- 本阶段不新增 DB migration，不实现 `search_agent_history`，不改 Agent 对话框消息列表投影。
- 已执行验证：`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/sandbox ./internal/agent/tools ./internal/config -run 'ContextCompaction|AgentContext|TokenCounter|Planner|WorkspaceTextPath|ApplyTextEdit|ReadFile|EditFile|SandboxFile|ComposerNativeToolsRegisterExpectedNames' -count=1`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`。

### M9.2 Micro Compact 与可恢复原文

✅ 已完成（2026-06-30）。目标是先把可恢复 micro compact 跑通，并严格保持 Agent 对话框消息列表展示原始 `agent_message`。

目标是先解决最常见的上下文膨胀：旧 tool result、长 tool call args、ffprobe / ffmpeg 输出和旧 skill tool result。

必须交付：

- `agent_context_compaction` migration。
- `agent_message_compaction` migration。
- sqlc queries。
- micro compact planner execution。
- detail file 写入 `/workspace/.clipanvil/context/`。
- prompt 占位符包含 `compact_ref`、summary、`detail_file` 和恢复方式。
- `search_agent_history` native tool。
- Producer / Composer 接入 micro compact。
- 历史消息列表 API 仍返回原始 `agent_message` 内容。

验收命令：

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

重点测试：

- 小阈值 fixture 中，旧 `read_project_context` 结果被替换成占位符。
- 大型原文被写入 sandbox detail file，DB 只保存 metadata、hash、bytes、source ids。
- `read_file(path=detail_file)` 能分段恢复原文。
- `search_agent_history(compact_ref=...)` 能定位 compaction record 和 detail file。
- tool_call / tool_result 的 ID 和顺序保持有效。
- 当前 same-turn tool loop 不被压缩。
- Agent 对话框消息列表不展示 micro compact 占位符、summary 或 detail file 替换文本。

完成记录：

- 新增 `apps/server/migrations/036_agent_context_compaction.sql`，包含 `agent_context_compaction` 与 `agent_message_compaction` sidecar 表。
- 新增 `apps/server/sqlc/queries/agent_context_compaction.sql` 并重新生成 sqlc 代码。
- `contextcompact.SQLStore` 负责创建 compaction record、按 `semantic_key` 查询、搜索历史记录、写入 `agent_message_compaction` 关联。
- `ContextCompactMiddleware` 只返回模型输入 projection，不修改传入的原始 `schema.Message`，也不调用 `UpdateAgentMessage`。
- 长原文通过 `DetailFileWriter` 写入 `/workspace/.clipanvil/context/`；压缩占位符包含 `compact_ref`、summary、`detail_file` 和恢复方式。
- Producer 接入 micro compact，并把可映射的历史 `agent_message.id` 传给 middleware 写 sidecar link。
- Composer 接入 micro compact；保护最新 same-turn tool loop，允许更早的大型 Composer tool result 被压缩。
- 新增 `search_agent_history` native tool，并注册给 Producer、Craftsman、Reviewer、Composer。
- `search_agent_history(compact_ref=...)` 返回 `summary`、`detail_files`、source message/media refs；完整原文继续通过 `read_file` 分段恢复。
- 新增回归测试，断言 compaction 包不引用 `UpdateAgentMessage` / `UpdateMessage` / `ListAgentMessagesByThread` 改写路径，保护 Agent 对话框历史列表展示原始消息。
- 已执行验证：`make sqlc-generate`、`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/tools ./internal/agent/producer ./internal/agent/composer -run 'Store|DetailFile|Projection|MicroCompact|SearchAgentHistory|ContextCompaction|MessageProjection|ComposerNativeToolsRegisterExpectedNames' -count=1`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。

### M9.3 四 Agent 统一接入

✅ 已完成（2026-06-30）。目标是消除角色漂移，让 Producer、Craftsman、Reviewer、Composer 都复用同一中间件，但保留各自不能压缩的上下文。

必须交付：

- Craftsman 接入 `ContextCompactMiddleware`。
- Reviewer 接入 `ContextCompactMiddleware`。
- 四角色 GraphConfig 或 runtime context 统一传入 role、thread、task、scope、model id。
- responder 支持使用 middleware 输出的 compacted messages。
- 四角色保留策略测试。

验收命令：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

重点测试：

- Producer 不压缩最近用户意图、decision card、pending signal reminder。
- Craftsman 不压缩当前 scope media refs 和当前 RenderPlan 关键参数。
- Reviewer 不压缩当前 review target 的真实媒体输入、keyframe 或 probe summary。
- Composer 不压缩当前 timeline target、sandbox path、duration、codec、audio stream、timeline plan。
- 旧 skill / memory tool result 可被压缩。

完成记录：

- Craftsman / Reviewer 的 Volcengine responder 已新增 `ContextCompactor` 注入，并在调用模型前执行 `ContextCompactMiddleware.Project`。
- Producer、Craftsman、Reviewer、Composer 四角色均使用同一 `contextcompact.Middleware` 做模型输入 projection；压缩后的占位符不会写回 `agent_message`。
- 新增共享 `CurrentToolLoopFromIndex` / `PendingReminderTargetIndex`，统一 latest same-turn assistant/tool 对和 pending reminder 的保护边界。
- Craftsman / Reviewer 在从 `agentprompt.HistoryMessages` 构造模型消息时保留 prompt index 到 source `agent_message.id` 的映射，用于写入 `agent_message_compaction`。
- `ContextCompactMiddleware` 在复用已有 `semantic_key` record 时也会补写 source message link，避免 sidecar 引用缺失。
- 新增 Craftsman / Reviewer 压缩投影测试，覆盖旧长 tool result 被压缩、当前任务文本或当前图片 review message 保留、原始 `agent_message.Content` 不变。
- 新增 role name 稳定性测试，并保留 compaction 包不引用 `UpdateAgentMessage` / `UpdateMessage` / `ListAgentMessagesByThread` 的边界测试，保护 Agent 对话框历史列表继续展示原始消息。
- 已执行验证：`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer ./internal/agent/tools -run 'RoleNames|AgentMessageUpdatePath|ContextCompaction|Projection|SearchAgentHistory|PendingReminders|ToolPromptMessages|PromptMessages|ComposerNativeToolsRegisterExpectedNames' -count=1`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`。

### M9.4 Full Compact 与 Handoff Summary

✅ 已完成（2026-06-30）。目标是在 micro compact 不足以降到安全阈值时，生成可交接的全量摘要，并让 Agent 在下一步仍能正确行动。

必须交付：

- summary model runner。
- full compact prompt。
- full compact record。
- full compact 后的 prompt 重建。
- `model_error` trigger 的一次性 context overflow retry。
- full compact diagnostics 和 agent event。

验收命令：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

重点测试：

- full summary 包含 `User Goal`、`Confirmed Decisions`、`Current Project State`、`Media Assets`、`Shot / RenderPlan Status`、`Review Findings`、`Audio / Timeline State`、`Pending Signals And Tasks`、`Recovery References`。
- 最近用户关键指令原文保留。
- 当前 DB facts 进入 summary 输入，不能只总结 agent messages。
- 没有来源的媒体描述写“未生成视觉摘要”，不能编造画面或音频。
- summary 失败时不写半成品、不改写历史、不继续 retry 循环。

完成记录：

- 新增 `FullSummaryFact`、`FullSummaryInput`、`FullSummarizer`、`BuildFullSummaryPrompt` 和 `ValidateFullSummaryMarkdown`。
- 新增 `StaticFullSummarizer` fallback 和 `VolcengineFullSummarizer`，真实模式使用 Volcengine text model 生成 handoff summary，非 real 模式使用 deterministic fallback。
- `ContextCompactMiddleware` 支持 `ForceFullCompact`、`Facts`、`MediaCards` 和 full compact projection；full compact 会插入 `# Compacted Agent Handoff Summary`，并保留最近用户消息、最近总消息、same-turn tool loop、pending reminders 和当前任务上下文。
- full compact 写入既有 `agent_context_compaction` sidecar 表，`mode=full`，并把 summary detail file 写入 `/workspace/.clipanvil/context/`。
- Producer 新增 PSS-based facts provider，`RuntimeContextLoader` 会把 DB-derived project facts 注入 `ProducerContext`。
- Craftsman、Reviewer、Composer 新增本地 facts/media-card builders；媒体 summary 缺少可信来源时写“未生成视觉摘要”，不编造画面或音频。
- Producer、Craftsman、Reviewer、Composer 均支持 context overflow 后一次性 `ForceFullCompact` retry；不会无限重试。
- `search_agent_history` 输出新增 `mode` 字段，可区分 `micro` / `full` compaction records。
- 已执行验证：`cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/contextcompact ./internal/agent/tools ./internal/agent/producer ./internal/agent/craftsman ./internal/agent/reviewer ./internal/agent/composer ./cmd/server -run 'FullCompact|ContextOverflow|ContextCompaction|SearchAgentHistory|AgentMessageUpdatePath|ContextFullSummarizer|FullSummarizer|FullCompactFacts' -count=1`、`GOCACHE=/private/tmp/clipanvil-go-build make server-test`、`git diff --check`、`rg -n "UpdateAgentMessage|UpdateMessage\\(|ListAgentMessagesByThread" apps/server/internal/agent/contextcompact apps/server/internal/agent/producer/model_responder.go apps/server/internal/agent/craftsman/model_responder.go apps/server/internal/agent/reviewer/model_responder.go apps/server/internal/agent/composer/model_responder.go`（只命中 boundary test）。

### M9.5 媒体策略与真实 E2E

✅ 已完成（2026-06-30）。真实 smoke 已生成最终视频，完成 Reviewer final video quality gate，并证明上下文压缩、文件恢复、媒体引用、四角色协作和 Agent chat list 边界可以一起工作。

当前实测记录：

- 报告：`docs/superpowers/reports/2026-06-30-m9-5-media-e2e.md`
- Workspace：`6029230f-3944-4f5c-89e0-56c5b2fca65c`
- Final artifact：`composer.final_output.1879ab47.compose.artifact.v1`
- Final video asset：`c8a2ef05-edaf-4fd6-8226-c3da33b1a235`
- TimelinePlan：`1879ab47-d930-4462-9d9c-3c655df48911`
- Reviewer task：`08b1a3aa-87d8-4dab-bb13-6ac461be9838`
- Review record：`d21107fc-e407-4369-b6cc-6731803990d0`，`status=rejected`

目标是用真实营销视频生成流程证明上下文压缩、文件恢复、媒体引用、四角色协作都能一起工作。

完成记录：

- image / video / audio media card builder 已覆盖 stable refs、kind、status、trusted source、summary fallback、duration 和 sandbox path。
- Composer stage / probe / ffmpeg 大输出可被 micro compact，并保留当前 timeline path、duration、codec、sandbox refs。
- 真实流程中 `agent_context_compaction` 有 Producer / Craftsman / Composer / Reviewer 记录，`agent_message_compaction` 有 sidecar links，sandbox detail files 可读。
- DB 复核：真实 workspace `agent_message` 共 297 条，`content` 中 `compact_ref` / `Agent Context Detail` / `已压缩` 均为 0；压缩没有影响 Agent 对话框消息列表展示。
- Reviewer final video review 已终态 `rejected`，这是对最终成片质量的正确判断，不是压缩链路失败。
- `review_completed` signal 已被下一轮 Producer drain；没有修改 Producer pending signal claim / drain 时机。
- 真实流程暴露的 `dispatch_craftsman` 显式 shot scope 扩散问题已修复，并新增 `scope.id=shot_03` + `shot_refs=[]` 回归测试，保证只派发单个 shot。
- 修复前错误派发产生的仍会继续执行的 smoke 残留任务已标记失败；Producer 当前 `waiting_for_user` 是 shot_03 provider 拒绝后的合理 HITL 升级。
- 营销视频生成 smoke 报告：`docs/superpowers/reports/2026-06-30-m9-5-media-e2e.md`。

已执行验收命令：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

本阶段没有前端文件改动，因此未跑 web build / lint。

真实 smoke 需要检查：

1. 上传产品参考图。
2. Producer 生成 brief / storyboard / audio plan。
3. Craftsman 生成 preview / video / audio RenderPlan。
4. Worker 生成图片、视频、音频。
5. Reviewer 评审。
6. Composer 成片。
7. DB 中存在可审计的 `agent_context_compaction`、`agent_message_compaction`、`agent_message`、`artifact_version`、`producer_pending_signal`。
8. sandbox 中存在可读取的 `/workspace/.clipanvil/context/` detail files。
9. `read_file` 能恢复至少一个被压缩的长 tool result。
10. `search_agent_history` 能从 compact_ref 找到对应 detail file。
11. 浏览器 Agent 对话框历史消息仍展示原始消息，不展示压缩占位符。

## 完成定义

- M9.1-M9.5 全部通过各自验收，且验收结果写入对应阶段总结或 PR 描述。
- 四个 Agent 都复用同一套 `contextcompact` middleware，不出现四份独立裁剪逻辑。
- `read_file` / `edit_file` 成为四个 Agent 的通用 sandbox 文本文件工具。
- 目录探索通过 exec shell 工具完成，不新增 `list_files` native tool。
- Micro compact 能稳定压缩旧 tool result 和长 tool call args，并通过 detail file 恢复原文。
- Full compact 能生成可 handoff 的结构化摘要，并保留最近用户意图和当前任务上下文。
- 压缩只影响模型输入，不改变 Agent 对话框消息列表展示。
- 媒体本体不被塞进压缩摘要；当前评审或剪辑必需媒体不被摘要替代。
- Producer pending signal 的 claim / drain 时机不变。
- 至少一条真实浏览器营销视频流程通过，并验证 DB、tool trace、sandbox detail files 和最终成片状态。

## 暂不做

- 向量数据库或 embedding 检索。
- 用户可编辑压缩摘要 UI。
- 远程文件管理器或 sandbox 文件浏览 UI。
- 复杂目录 ACL。
- `list_files` native tool。
- 删除原始 `agent_message`。
- 覆盖、截断或重写已有 `agent_message.content` / `agent_message.raw_message`。
- 让压缩摘要覆盖 DB 事实源。
- 用文件笔记替代 `upsert_project_brief`、`upsert_storyboard`、`upsert_render_plan`、`create_timeline_plan`、`submit_artifact` 等结构化工具。
- 改变 Producer pending signal 的 claim / drain 时机。
