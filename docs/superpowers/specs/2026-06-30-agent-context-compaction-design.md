# Agent Context Compaction 上下文压缩方案设计

**状态**：待评审
**日期**：2026-06-30
**适用范围**：ClipAnvil Agent 模式，Producer / Craftsman / Reviewer / Composer，Eino native tool loop，长任务上下文管理，媒体资源引用与检索

## 结论

ClipAnvil 需要新增一套共享的 Agent 上下文压缩基础设施，但第一版不应把压缩做成某个 Agent 的私有补丁，也不应把媒体文件当作普通消息文本裁剪。

推荐方案：

```text
四个 Agent 统一在 before_model -> call_model 之间进入 ContextCompactMiddleware
  -> TokenCounter 统计 messages + tool infos + 媒体引用摘要
  -> 超过 micro 阈值时，优先压缩最早的 tool_result / tool_call 参数
  -> 超过 full 阈值时，生成可 handoff 的全量摘要
  -> 压缩后的 prompt 保留最近用户消息、当前任务上下文、稳定语义引用和恢复指引
  -> 原始消息、媒体资产、工具结果和压缩记录继续持久化
  -> 已知路径时 Agent 通过 read_file 恢复压缩原文；不知道路径时通过 search_agent_history 检索
```

核心原则：

- 压缩历史消息，不压缩媒体本体。
- 压缩可重复读取的工具结果，不压缩当前判断必需的媒体输入。
- 所有图片、视频、音频都必须变成稳定语义引用、结构化摘要和可检索索引。
- Producer、Craftsman、Reviewer、Composer 复用同一套中间件，但保留不同角色的保留策略。
- 压缩只影响下一次模型调用看到的 prompt，不影响 Agent 对话框消息列表展示。
- 压缩失败必须 fail closed：不写半成品摘要，不破坏当前消息历史，不改变 Producer signal drain / claim 时机。

## 背景与当前代码事实

当前四个 Agent 已经是 Eino Graph，但不是直接使用 Eino ADK `ChatModelAgent`：

- Producer：`apps/server/internal/agent/producer/graph.go`
- Craftsman：`apps/server/internal/agent/craftsman/native_tool_loop.go`
- Reviewer：`apps/server/internal/agent/reviewer/native_tool_loop.go`
- Composer：`apps/server/internal/agent/composer/graph.go`

它们共同的结构是：

```text
load_context
  -> prepare_turn_state
  -> before_model
  -> call_model
  -> execute_tools
  -> append_tool_results
  -> before_model ...
```

因此第一版不强行改造成 ADK Agent，而是在现有 Eino native tool loop 中新增共享消息改写层。该层对齐 Eino ADK `ChatModelAgentMiddleware` 的语义，尤其是 `BeforeModelRewriteState`：模型调用前改写 messages / tools，并让改写后的状态继续参与后续 tool loop。

当前持久层已有：

- `agent_thread`
- `agent_task`
- `agent_message`
- `agent_event`
- `eino_checkpoint`
- `producer_pending_signal`
- `media_node`
- `media_asset`
- `artifact_version`
- `generation_job`
- `render_plan`
- `review_record`
- `timeline_plan`

但 `agent_message` 目前没有压缩状态、token 统计、摘要记录和原文恢复索引。因此上下文压缩不能只在 prompt 构造时做一次性裁剪，还需要新增持久化结构。

## 外部参考

参考资料：

- Anthropic API Compaction：https://platform.claude.com/docs/en/build-with-claude/compaction
- Claude Code Hooks `PreCompact` / `PostCompact`：https://docs.anthropic.com/en/docs/claude-code/hooks
- Codex CLI `/compact`、`model_context_window`、`model_auto_compact_token_limit`、`compact_prompt`：Codex 官方手册
- Eino ADK ChatModelAgentMiddleware：https://www.cloudwego.io/docs/eino/core_modules/eino_adk/eino_adk_chatmodelagentmiddleware/
- Eino Summarization Middleware：https://www.cloudwego.io/docs/eino/core_modules/eino_adk/eino_adk_chatmodelagentmiddleware/middleware_summarization/
- Eino Tool-reduction Middleware：https://www.cloudwego.io/docs/eino/core_modules/eino_adk/eino_adk_chatmodelagentmiddleware/middleware_toolreduction/

可借鉴点：

- Anthropic compaction 强调阈值触发、生成摘要、丢弃旧上下文。
- Claude Code 暴露 `PreCompact` / `PostCompact`，说明压缩应当是可观测的运行时事件。
- Codex 使用 context window、auto compact token limit、compact prompt 这些显式配置。
- Eino 已有 summarization 和 tool-reduction 两类 middleware，正好对应 full-compact 和 micro-compact。

ClipAnvil 的取舍：

- 不照搬“服务端直接丢弃旧消息”。ClipAnvil 有 DB 事实源和媒体资产，必须可追溯。
- 不让压缩摘要覆盖业务事实。真实状态仍以 DB、RenderPlan、artifact、review、timeline 为准。
- 不让大模型凭摘要推断媒体内容。当前关键媒体应以真实输入、关键帧、probe 结果或工具读取结果为准。

## 第一性原理

模型下一步只能使用本轮 prompt 中可见的信息。上下文压缩不是为了让 prompt 看起来短，而是为了让模型在长任务里仍能正确行动。

Agent 需要的信息分三类：

1. **当前行动必需信息**
   - 本轮用户目标。
   - 当前任务 scope。
   - 当前要评审或剪辑的媒体。
   - 待处理 signal。
   - 最近几轮工具调用结果。

2. **可摘要但必须保留的信息**
   - 已确认的创作决策。
   - 已生成资产和 winner 状态。
   - 历史 review 结论。
   - 已失败的尝试和避免重复的原因。
   - 用户明确偏好。

3. **可恢复的原始证据**
   - 长工具结果。
   - provider response。
   - ffprobe / ffmpeg 日志。
   - 大段 project context。
   - 早期工具调用参数和结果。

上下文压缩的目标是把第 2 类变成可靠摘要，把第 3 类变成可读取的文件引用和可搜索引用，同时绝不误删第 1 类。

## 范围

范围内：

- 新增共享 `contextcompact` 包。
- 新增 token counter 和阈值配置。
- 新增 micro-compact 策略。
- 新增 full-compact 策略。
- 新增媒体资源上下文策略。
- 新增压缩记录持久化。
- 新增 `search_agent_history` native tool。
- 新增通用 sandbox `read_file` / `edit_file` native tool。
- 四个 Agent 接入统一上下文压缩中间件。
- 单测覆盖 Producer、Craftsman、Reviewer、Composer 的一致行为。
- DB / sqlc / config / smoke 更新。

范围外：

- 不做向量数据库第一版。
- 不做用户可编辑的压缩摘要 UI。
- 不新增 `list_files` native tool；目录枚举、`find`、`grep` 由 exec shell 工具完成。
- 不删除原始 `agent_message`。
- 不覆盖 `agent_message.content` / `agent_message.raw_message` / `agent_message.message_type` 来保存压缩占位符。
- 不改变 Agent 对话框历史消息列表 API 的默认展示内容。
- 不改变 Producer pending signal 的 claim / drain 时机。
- 不把压缩作为修复模型上下文窗口之外所有问题的万能方案。
- 不让 skill、memory、project context 替代压缩系统。

## 核心概念

### Context Window

模型可接受的最大上下文窗口。第一版默认按配置读取：

```yaml
agent:
  context_compaction:
    enabled: true
    model_context_window_tokens: 256000
```

### Micro Compact

轻量压缩。它不重写整段历史，只从最早的可压缩工具结果开始，把长内容替换为稳定占位符。较短原文可直接放入压缩记录；较长原文写入 sandbox 文件，压缩记录保存摘要、hash、文件路径和来源 message 关系。

默认阈值：

```yaml
micro_trigger_tokens: 180000
micro_target_tokens: 150000
micro_min_reduction_tokens: 8000
```

### Full Compact

全量压缩。它读取当前消息历史和结构化事实，生成一条可交接摘要，并把早期消息替换成 summary + 最近上下文。

默认阈值：

```yaml
full_trigger_tokens: 200000
full_target_tokens: 140000
preserve_recent_user_messages: 6
preserve_recent_total_messages: 40
```

### Media Ref

媒体资源在模型上下文中的稳定引用。第一版统一使用人类可读语义引用：

```text
media_asset/{semantic_key}
artifact_version/{semantic_key}
media_node/{semantic_key}
render_plan/{semantic_key}
timeline_plan/{semantic_key}
```

如果对象尚无 semantic key，使用类型化 UUID 引用作为降级：

```text
artifact_version/550e8400-e29b-41d4-a716-446655440000
```

### Compaction Ref

压缩记录引用：

```text
agent_context_compaction/{semantic_key}
```

占位符中必须出现 compaction ref，方便工具恢复原始内容。
如果原文被落成 sandbox 文件，占位符还必须出现 `detail_file`，方便 Agent 用 `read_file` 直接恢复。

## Token 统计策略

第一版新增 `TokenCounter` 接口：

```go
type TokenCounter interface {
    CountMessages(ctx context.Context, input CountMessagesInput) (CountMessagesResult, error)
}
```

输入包括：

- `[]*schema.Message`
- `[]*schema.ToolInfo`
- 当前角色
- 当前模型 ID
- 媒体引用统计信息

实现建议：

- 默认用 `github.com/pkoukk/tiktoken-go` 估算文本和 tool schema。
- 对 Doubao / Volcengine 模型先采用 tiktoken 近似，不声称精确。
- 图片、视频、音频不要按 URL 或 base64 字符数粗暴统计。
- 媒体本体 token 使用 conservative media weight：
  - image input：按模型输入估算固定权重，默认每张 1500 tokens，可配置。
  - video URL：不直接进 prompt，按媒体摘要文本统计。
  - audio URL：不直接进 prompt，按音频摘要文本统计。
- 如果 provider response 返回真实 usage，记录到 diagnostics，用作后续校准，但第一版不依赖它触发压缩。

## 媒体资源处理策略

媒体资源是 ClipAnvil 的业务资产，不是对话历史字符串。上下文压缩必须遵守：

- 图片、视频、音频文件不进入 compaction summary。
- 原始媒体继续由 `media_asset`、`artifact_version`、MinIO / TOS / sandbox job 管理。
- 模型 prompt 中保留结构化 media card。
- 当前任务必需媒体可以保留真实模型输入。
- 历史媒体默认替换成可搜索引用。

### 图片

图片分三类：

| 类型 | Prompt 处理 | 压缩处理 |
|---|---|---|
| 当前用户参考图 | 保留真实 image input 或模型可访问 URL | 不压缩本体；保留 attachment / asset ref |
| 当前 Reviewer 评审图 | 保留真实 image input 或 signed URL | 不压缩本体；保留 review target ref |
| 历史生成图 | 使用 media card 摘要 | 压缩旧工具结果，只保留 artifact / node / render_plan ref |

media card 示例：

```text
[media:image]
ref: artifact_version/shot_01.preview.v2
node_ref: media_node/shot_01_preview
role: selected_preview
status: winner
mime: image/png
width: 1080
height: 1920
summary: 银灰色硬壳行李箱位于画面中心，机场候机厅背景，光线干净，产品边缘清晰。
source_ref: render_plan/shot_01.preview
```

图片处理规则：

- 用户最新上传的产品图、角色图、参考图在 Producer 第一轮规划和 Craftsman 生成参考时优先保留。
- 同一个 asset 多次出现在历史消息中，只保留一次 media card。
- base64 图片不得持久进入压缩摘要。
- 如果图片只有 storage URL，没有视觉摘要，压缩层只写结构化引用，不编造画面内容。
- 视觉摘要只能来自已存在的模型输出、review、用户文本、人工标注或后续专门的 media_inspect 工具。

### 视频

视频不作为长历史消息直接进入模型。视频上下文应拆成：

- asset / artifact ref
- duration
- resolution
- fps
- mime
- poster frame ref
- sampled keyframe refs
- source render_plan
- generation_job status
- winner status
- review verdict
- timeline usage

media card 示例：

```text
[media:video]
ref: artifact_version/shot_02.video.v1
node_ref: media_node/shot_02_video
role: shot_video_winner
status: winner
duration_sec: 4.8
resolution: 720x1280
fps: 24
audio_stream: absent
summary: 行李箱经过机场安检通道，轮子顺滑推进，镜头以产品侧面和拉杆细节为主。
review_ref: review_record/shot_02_video.accepted
source_ref: render_plan/shot_02.video
```

视频处理规则：

- Producer 通常只看视频状态摘要，不看视频本体。
- Craftsman 通常只看参考视频摘要、关键帧和约束，不看完整视频。
- Reviewer 对 shot video / final video 评审时，需要真实媒体 URL、关键帧或 probe 结果。
- Composer 需要最精确的可剪辑信息：duration、stream、sandbox path、timeline in/out、audio presence。
- ffprobe / ffmpeg 原始输出优先 micro-compact，但保留 `probe_summary` 和 `sandbox_file_ref`。

### 音频

音频上下文应拆成：

- asset / artifact ref
- duration
- codec
- sample rate
- channels
- role：voiceover / bgm / sfx / final_mix
- voiceover script
- BGM mood / bpm / loop / ducking
- linked audio_plan / timeline_plan

media card 示例：

```text
[media:audio]
ref: artifact_version/audio.voiceover.v1
role: voiceover
status: generated
duration_sec: 14.8
codec: mp3
sample_rate: 48000
script: 轻装出发，悦行一路从容。
linked_plan_ref: audio_plan/main
```

音频处理规则：

- Producer 重点看 script、style、generated status 和与成片时长是否匹配。
- Craftsman 重点看 AudioPlan 和 provider 参数，不需要读取音频本体。
- Reviewer final video review 重点看成片音轨，不单独读 voiceover / BGM 本体，除非诊断音频问题。
- Composer 必须保留 voiceover / BGM 的 duration、sandbox path、volume、fade、ducking、timeline binding。
- 音频生成 tool result 中的 base64 或临时 URL 必须 micro-compact，只保留入库后的 asset ref。

## Micro Compact 详细策略

### 触发条件

本轮 `before_model` 中统计 token：

```text
if total_tokens >= micro_trigger_tokens:
    run micro compact until total_tokens <= micro_target_tokens
```

如果 token 已超过 `full_trigger_tokens`，优先考虑 full compact；但可以先执行 micro compact，若仍无法降到 full 以下，再进入 full compact。

### 候选消息

按 `agent_message.seq` 从早到晚选择：

1. tool result，尤其是：
   - `read_project_context`
   - `read_project_memory`
   - `load_agent_skill`
   - `probe_media`
   - `stage_media_inputs`
   - `render_timeline_template`
   - `run_ffmpeg_command`
   - provider / production 状态读取类工具
2. tool call arguments，尤其是大型 JSON payload。
3. assistant reasoning content。
4. 重复的 status / progress 消息。

第一版不 micro-compact：

- 最近 N 条消息。
- 当前 same-turn tool loop 消息。
- 当前 task 需要执行的 tool_call 与紧随其后的 tool_result。
- 用户最近明确指令。
- `request_user_decision` 的选项和用户选择结果。
- Reviewer 当前评审目标的媒体输入。
- Composer 当前 timeline 渲染必需的 sandbox path 和 probe summary。

### 工具调用协议保护

tool_call 和 tool_result 必须成对处理。不能只删除 result，保留一个指向巨大旧结果的 tool_call。推荐替换为：

```text
assistant tool_call arguments:
{"compacted": true, "compact_ref": "agent_context_compaction/...", "tool": "read_project_context"}

tool result:
工具结果已压缩。
- compact_ref: agent_context_compaction/...
- original_tool: read_project_context
- source_message_seq: 42
- summary: 读取了项目上下文，包含 6 个 shot、9 个 render_plan、5 个 winner artifact、2 个 pending signals。
- detail_file: /workspace/.clipanvil/context/read-project-context-0042.md
- 恢复方式: 优先调用 read_file(path="/workspace/.clipanvil/context/read-project-context-0042.md")；不知道路径时调用 search_agent_history(compact_ref="agent_context_compaction/...")
```

如果 provider 对历史 tool_call 参数格式敏感，第一版可只压缩 tool result content，不改 tool_call arguments。但压缩记录仍必须绑定 tool_call_id。

### 压缩记录

每次 micro compact 写入：

- workspace_id
- thread_id
- task_id
- role
- mode = `micro`
- trigger = `token_threshold`
- source_message_ids
- source_seq_start / source_seq_end
- original_token_estimate
- compacted_token_estimate
- summary
- detail_files：写入 sandbox 的原文路径列表，如 `/workspace/.clipanvil/context/read-project-context-0042.md`
- payload：原始 tool name、arguments_digest、result_digest、short_excerpt、media refs、detail_file metadata
- created_at

原始 `agent_message` 不删除、不覆盖。prompt 构造阶段根据 compaction record 决定给模型展示原文还是占位符，但这个占位符只能进入模型输入、trace diagnostics 或 compaction record，不能回写成用户可见的历史消息内容。

## Full Compact 详细策略

### 触发条件

本轮 `before_model` 中统计 token：

```text
if total_tokens >= full_trigger_tokens:
    run full compact
```

full compact 应在模型调用前完成。不能等模型因 context overflow 失败后再补救，因为这会造成用户可见失败和任务状态不一致。

### 输入

full compact 的摘要模型输入应包含：

- 当前角色 system-level summary instruction。
- 当前完整消息历史的压缩候选部分。
- 最近用户消息和当前任务上下文。
- 当前 DB facts：
  - project brief
  - project memory
  - storyboard
  - key elements
  - shots
  - render plans
  - generation jobs
  - artifact winners
  - review records
  - audio plan
  - timeline plans
  - pending signals
- media cards。

不要只把 `agent_message` 喂给摘要模型。ClipAnvil 的真实事实源在 DB，历史消息只是对话与工具轨迹。

### 输出格式

full summary 必须是结构化 Markdown：

```markdown
# Compacted Agent Handoff Summary

## User Goal

## Confirmed Decisions

## Current Project State

## Media Assets

## Shot / RenderPlan Status

## Review Findings

## Audio / Timeline State

## Pending Signals And Tasks

## Known Failures And Avoidances

## Recent User Instructions To Preserve Verbatim

## Next Recommended Actions

## Recovery References
```

要求：

- 用户最近关键指令必须尽量原文保留。
- 所有业务对象都使用 semantic ref。
- 不得编造媒体视觉内容。
- 对不确定事实写“未确认”，并指向恢复工具。
- 下一步行动必须和当前 Agent 角色边界一致。

### Prompt 重建

full compact 后，本轮模型看到的消息顺序：

```text
system prompt
system/user message: compacted handoff summary
recent preserved messages
current runtime trigger / current task context
same-turn tool messages
pending reminders
```

最近消息保留策略：

- 至少保留最近 6 条用户消息。
- 至少保留最近 40 条总消息。
- 保留当前 task 的 same-turn messages。
- 保留未完成 tool loop 的必要消息。
- 保留最近一次 user decision 和选择结果。

## Agent 对话框展示边界

上下文压缩不能影响用户在 Agent 对话框中看到的消息列表。Agent 对话框的历史消息展示仍以原始 `agent_message` 记录为准。

硬约束：

- 压缩后的 `CompactedMessages` 是一次模型调用的临时输入，不是新的对话历史事实源。
- `agent_context_compaction` 和 `agent_message_compaction` 是 sidecar metadata，不能替代 `agent_message.content`。
- 历史消息列表 API 默认不得把原始消息替换成 compaction placeholder、summary 或 `detail_file`。
- 不得为了节省前端展示体积而覆盖、截断或重写已有 `agent_message.raw_message`。
- 如果未来 UI 需要展示“此消息已参与压缩”，只能新增独立 metadata / diagnostics 标识，不能改变原消息正文。
- 当前 turn 的 streaming message、tool call、tool result 展示不进入压缩改写路径。

因此，同一条历史消息会有两种投影：

```text
用户可见投影:
  agent_message 原文 -> Agent 对话框消息列表

模型输入投影:
  agent_message 原文 + compaction sidecar -> prompt builder -> ContextCompactMiddleware -> compacted messages
```

验收时必须同时检查：模型输入 token 被压缩，且 Agent 对话框历史消息没有出现压缩占位符。

## 新增工具：search_agent_history

### 目的

压缩后 Agent 必须能恢复原始事实。`search_agent_history` 是四个角色都可用的 native tool，用于搜索：

- 原始 agent_message。
- compaction record。
- tool result 原文。
- media card。
- summary。

### 工具输入

```json
{
  "query": "shot_02 suitcase wheel issue",
  "compact_ref": "agent_context_compaction/...",
  "media_ref": "artifact_version/shot_02.video.v1",
  "role": "producer",
  "message_type": "tool_result",
  "limit": 10
}
```

字段规则：

| 字段 | 必需 | 说明 |
|---|---|---|
| `query` | 否 | 文本搜索关键词。 |
| `compact_ref` | 否 | 精确恢复某条压缩记录。 |
| `media_ref` | 否 | 搜索关联某个媒体对象的历史。 |
| `role` | 否 | 限定来源角色；默认当前 role 可见范围。 |
| `message_type` | 否 | `text`、`tool_call`、`tool_result`、`ui_card`、`status`。 |
| `limit` | 否 | 默认 10，最大 50。 |

必须至少提供 `query`、`compact_ref`、`media_ref` 之一。

### 工具输出

```text
历史搜索结果
- result_ref: agent_message/producer/42
  type: tool_result
  tool: read_project_context
  created_at: 2026-06-30T10:20:00Z
  summary: 包含当前 6 个 shot 和 preview/video winner 状态。
  excerpt: ...
- result_ref: agent_context_compaction/context_micro_0003
  type: compacted_tool_result
  source_seq: 40-42
  summary: ...
```

默认只返回摘要和短 excerpt。需要完整原文时，可用 `compact_ref` 精确定位对应压缩记录。
如果结果已经落到 sandbox 文件，`search_agent_history` 返回 `detail_file` 路径，不直接返回完整原文；Agent 再调用 `read_file` 分段读取。这样搜索工具只负责定位历史，文件工具负责恢复内容。

### 权限边界

- Producer 可搜索 workspace 内所有 Agent 线程。
- Craftsman 默认只能搜索自己的 thread 和同 scope 的 Producer 摘要。
- Reviewer 默认只能搜索自己的 thread、当前 review target、Producer 项目摘要。
- Composer 默认只能搜索 composer thread、final_output scope、Producer 项目摘要和相关 media refs。

第一版如果权限实现复杂，可以先用 workspace + role + scope 过滤，禁止跨 workspace。

## 新增工具：通用 Sandbox 文件读写

### 设计原则

这部分不做复杂的文件管理系统，直接参考 Claude Code / Codex 的文件工具心智模型：

- `read_file` 负责读取 sandbox 内文本文件。
- `edit_file` 负责创建、覆盖、追加或局部编辑文本文件。
- `list`、`find`、`grep`、目录探索由 exec shell 工具完成。
- sandbox 已负责生命周期、隔离和持久化存储，第一版不再为通用文件工具新增单独的文件索引表。

上下文压缩可以把较长原文写成 sandbox 文件，然后在摘要里留下可读路径。Agent 需要恢复细节时，优先通过 `read_file` 读取对应文件；需要按语义找历史时，再调用 `search_agent_history`。

### 可访问范围

文件工具只允许访问当前 workspace sandbox 内的路径，不允许访问宿主机文件系统。第一版工具层只强制 `/workspace` 边界、文本大小限制和路径安全，不做复杂目录 ACL。

推荐目录约定：

```text
/workspace/.clipanvil/context/
  系统写入的压缩归档、长 tool result、长 tool call args。
  Agent 可读；不建议 Agent 主动编辑。

/workspace/.clipanvil/notes/
  Agent 自主写入的调研记录、计划、草稿、阶段性判断。
  Agent 可读写。

/workspace/input/
  已 stage 的输入素材。
  Agent 可读，媒体内容通常通过 probe/metadata 工具理解。

/workspace/output/
  渲染输出和可提交产物。
  Agent 可读；媒体产物提交仍走 submit artifact / media asset 流程。
```

这些目录是 Agent 使用规范，不是第一版工具 ACL。最低要求是 path normalize 后必须仍在 `/workspace` 内，禁止 `..`、symlink escape 和绝对路径逃逸。通用 `edit_file` 可以写文本文件，但 Agent prompt 应要求：业务事实用结构化工具落库，媒体产物用 artifact 工具提交，压缩归档通常只读。

### read_file

输入示例：

```json
{
  "path": "/workspace/.clipanvil/context/producer-full-compact-0004.md",
  "offset": 0,
  "limit": 20000
}
```

行为：

- 读取 UTF-8 文本文件。
- 默认限量返回，避免一次性把大文件重新塞满上下文。
- 超过 limit 时返回 `truncated=true`、`next_offset`、`bytes_total`。
- 读取媒体文件本体不是第一版目标；媒体细节应通过现有 probe / artifact / media metadata 工具读取。

### edit_file

输入示例：

```json
{
  "path": "/workspace/.clipanvil/notes/craftsman-shot-02-research.md",
  "mode": "create_or_overwrite",
  "content": "# Shot 02 调研记录\n\n...",
  "reason": "记录本轮素材分析和生成计划"
}
```

推荐 mode：

| mode | 说明 |
|---|---|
| `create` | 文件不存在时创建，已存在则失败。 |
| `create_or_overwrite` | 创建或整体覆盖。 |
| `append` | 追加内容。 |
| `replace` | 用 `old_text` / `new_text` 做局部替换。 |

`replace` 建议要求 `old_text` 唯一匹配；不唯一或找不到时失败，避免误改。后续如果需要更强能力，再引入 patch / checksum。

### Agent 使用规范

- Producer 可写需求澄清、商业判断、全局 campaign plan 草稿。
- Craftsman 可写素材分析、prompt 实验记录、单镜头生成计划。
- Reviewer 可写评审依据、问题清单、返工建议。
- Composer 可写 timeline 推导、音频对齐记录、ffmpeg 调试记录。

这些文件是工作笔记，不是业务事实源。凡是会影响产品状态的内容，仍必须通过结构化工具落库，例如 `upsert_project_brief`、`upsert_storyboard`、`upsert_render_plan`、`create_timeline_plan`、`submit_artifact`。

### 与上下文压缩的关系

micro compact 和 full compact 可以把以下内容写入 `/workspace/.clipanvil/context/`：

- 被压缩的 tool result 原文。
- 被压缩的长 tool call args。
- full compact 的 handoff summary。
- 媒体引用清单和关键 metadata 快照。

压缩后消息只保留短摘要和文件路径，例如：

```text
历史内容已压缩。
- summary: 读取了项目上下文，包含 6 个 shot、9 个 render_plan、5 个 winner artifact。
- detail_file: /workspace/.clipanvil/context/producer-tool-result-0042.md
- 恢复方式: 调用 read_file(path="/workspace/.clipanvil/context/producer-tool-result-0042.md")
```

这样可以把“可恢复原文”交给通用文件读取工具，而不是为每类压缩内容设计专用读取接口。`search_agent_history` 仍保留，用来在 Agent 不知道具体文件路径时做语义检索。

## 数据模型

新增表建议：

```sql
CREATE TABLE agent_context_compaction (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL REFERENCES agent_thread(id) ON DELETE CASCADE,
    task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    role TEXT NOT NULL,
    mode TEXT NOT NULL,
    trigger TEXT NOT NULL,
    semantic_key TEXT NOT NULL,
    source_seq_start BIGINT,
    source_seq_end BIGINT,
    source_message_ids JSONB NOT NULL DEFAULT '[]',
    source_media_refs JSONB NOT NULL DEFAULT '[]',
    original_token_estimate BIGINT NOT NULL DEFAULT 0,
    compacted_token_estimate BIGINT NOT NULL DEFAULT 0,
    summary TEXT NOT NULL DEFAULT '',
    detail_files JSONB NOT NULL DEFAULT '[]',
    payload JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT agent_context_compaction_mode_check CHECK (mode IN ('micro', 'full')),
    CONSTRAINT agent_context_compaction_trigger_check CHECK (trigger IN ('manual', 'auto', 'token_threshold', 'model_error'))
);

CREATE UNIQUE INDEX idx_agent_context_compaction_workspace_semantic
    ON agent_context_compaction(workspace_id, semantic_key);

CREATE INDEX idx_agent_context_compaction_thread_created
    ON agent_context_compaction(thread_id, created_at DESC);
```

可选新增关联表：

```sql
CREATE TABLE agent_message_compaction (
    message_id UUID PRIMARY KEY REFERENCES agent_message(id) ON DELETE CASCADE,
    compaction_id UUID NOT NULL REFERENCES agent_context_compaction(id) ON DELETE CASCADE,
    compacted_role TEXT NOT NULL,
    compacted_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

如果不新增关联表，也可以把 `source_message_ids` 放在 `agent_context_compaction.payload` 中。但为了查询和测试清晰，推荐第一版新增关联表。

`payload` 不应无条件塞入完整长原文。推荐规则：

- 小于配置阈值的原文可进入 `payload.short_excerpt` 或 `payload.inline_original`。
- 大型 tool result、长 tool call args、ffprobe JSON、ffmpeg stderr 写入 `/workspace/.clipanvil/context/`。
- `detail_files` 保存可读路径、sha256、bytes、source_message_id、tool_call_id。
- `search_agent_history` 读取 DB metadata；`read_file` 读取 sandbox 文件内容。

## Eino Native Middleware 设计

新增包：

```text
apps/server/internal/agent/contextcompact/
  config.go
  token_counter.go
  media_cards.go
  planner.go
  micro_compact.go
  full_compact.go
  middleware.go
  store.go
  search_tool.go
  sandbox_file_tools.go
```

核心接口：

```go
type Middleware interface {
    RewriteBeforeModel(ctx context.Context, input RewriteInput) (RewriteOutput, error)
}
```

`RewriteInput`：

- role
- workspace_id
- thread_id
- task_id
- graph_name
- model_id
- messages
- tool_infos
- current task metadata
- scope type / scope id

`RewriteOutput`：

- messages
- tool_infos
- token_before
- token_after
- applied_compactions
- warnings

四个 Agent 接入方式：

- 在 `before_model` 中调用 `ContextCompactMiddleware.RewriteBeforeModel`。
- 将改写后的 messages 或 context compact state 传给 responder。
- responder 不再各自私有决定是否裁剪历史。
- prompt 组装函数继续由各角色维护，但最终 `[]*schema.Message` 必须经过统一中间件。
- `read_file` / `edit_file` 注册为 Producer、Craftsman、Reviewer、Composer 共用 native tools。
- 目录枚举不新增专用 tool；需要查看文件列表时，Agent 使用现有 exec shell 工具执行 `ls`、`find`、`grep`、`rg`。

因为当前 responder 接口接收的是 role-specific Context，而不是 `[]*schema.Message`，第一版可以采用较小改造：

1. 给每个 Context 增加 `Compaction *contextcompact.State` 或 `CompactedMessages []*schema.Message`。
2. `before_model` 阶段先用角色的 prompt builder 预构造 messages，交给 middleware。
3. responder 如果发现 `CompactedMessages` 非空，则直接使用它；否则走原 prompt builder。

后续如果迁移到 Eino ADK Agent，可把同一套逻辑适配成真正的 `ChatModelAgentMiddleware`。

## 四个 Agent 的保留策略

### Producer

Producer 需要 workspace 级全局状态。

必须保留：

- 最近用户目标和修改意见。
- 最近 decision card 与用户选择。
- pending signals reminder。
- active project brief / memory / storyboard / audio plan。
- 当前 winner assets summary。
- 未处理 RenderPlan / review / composer 状态。

优先压缩：

- 早期 `read_project_context` tool result。
- 早期 skill 正文 tool result。
- 重复 dispatch / status 工具结果。
- 已处理 signal 的长列表。

媒体策略：

- 最新用户参考图可保留真实 image input。
- 历史媒体只保留 media card。
- 不在 Producer prompt 中塞入完整视频或音频 URL，除非当前任务明确要求检查该媒体。

### Craftsman

Craftsman 需要把创作事实转成 RenderPlan。

必须保留：

- 当前 scope：shot / key_element_state / audio_plan。
- 当前 target phase。
- 当前可用 reference media refs。
- Producer 给出的 execution_policy。
- 最近 Reviewer 修复建议。

优先压缩：

- 早期 `read_project_memory` 长结果。
- 已提交 RenderPlan 的旧参数。
- load_agent_skill 的完整正文。

媒体策略：

- 对 preview / video 生成，保留当前参考图和关键 media refs。
- 对 audio 生成，保留 AudioPlan script 和 voice / BGM 参数，不保留音频本体。

### Reviewer

Reviewer 是最不能过度压缩的角色，因为它要基于真实 artifact 判断质量。

必须保留：

- 当前 review target 的真实媒体引用。
- artifact_version / generation_job / render_plan ref。
- prior review summary。
- rubric。
- 当前 repair / retry context。

优先压缩：

- 旧 review 轮次的长解释。
- 旧项目上下文。
- 与当前 target 无关的工具结果。

媒体策略：

- 图片评审保留当前图片 input。
- 视频评审保留当前视频 URL / keyframe / probe summary。
- final video 评审保留最终视频媒体引用和音轨摘要。
- 不凭 full summary 判断画面或音频质量。

### Composer

Composer 最容易被 stage/probe/render 日志撑爆上下文。

必须保留：

- 当前 timeline target。
- staged media sandbox path。
- video duration / stream summary。
- audio duration / stream summary。
- timeline plan。
- final render result。

优先压缩：

- `stage_media_inputs` 的完整文件列表原文。
- `probe_media` 的完整 ffprobe JSON。
- `run_ffmpeg_command` stdout/stderr。
- 旧 timeline render attempts。

媒体策略：

- 不丢 sandbox path。
- 不丢 duration、codec、resolution、audio_stream。
- 大日志压缩成摘要和 compaction ref。
- final artifact 只保留 artifact ref、asset ref、timeline ref 和 review state。

## 对抗性审查与处理方案

### 风险 1：压缩摘要编造媒体内容

问题：模型可能在 full compact 时把“可能是行李箱”写成确认事实。

处理：

- 摘要 prompt 明确禁止编造媒体视觉 / 音频内容。
- 没有来源的媒体描述只能写“未生成视觉摘要”。
- media card 的 `summary` 必须带来源：user / model_output / review / probe / manual。
- Reviewer 当前目标不允许只用摘要替代真实媒体。

### 风险 2：tool_call / tool_result 配对被破坏

问题：历史 tool call 协议不完整会让模型误以为工具仍在等待结果。

处理：

- micro compact 必须按 tool_call_id 成对处理。
- 如果找不到 result，跳过该 tool_call。
- 如果 provider 对历史 tool_call 参数敏感，第一版只压缩 result content。
- 单测覆盖 assistant tool_call + tool result 的顺序和 ID 保持。

### 风险 3：最近用户意图被 full compact 吞掉

问题：长任务中用户最新要求比早期目标更重要。

处理：

- 最近 6 条用户消息原文保留。
- full summary 里专门有 `Recent User Instructions To Preserve Verbatim`。
- 如果最近用户消息超过预算，优先裁剪旧工具结果，而不是用户消息。

### 风险 4：Producer signal 被压缩后漏处理

问题：signal 是工程事件，不是普通聊天消息。

处理：

- 不改变 Producer signal claim / drain 时机。
- pending signal reminder 不写入持久历史，也不被当作普通历史压缩对象。
- claimed signals 相关上下文只在当前 Producer turn 保留。
- full summary 可描述“已有 pending signals”，但真实处理仍以 DB 中 `producer_pending_signal` 为准。

### 风险 5：压缩后无法恢复原始证据

问题：摘要不够时 Agent 无法追问历史。

处理：

- 每个占位符必须包含 `compact_ref`。
- 已知 `detail_file` 时，Agent 用 `read_file` 分段读取原文。
- 不知道路径时，`search_agent_history` 支持按 compact_ref、query、media_ref 定位压缩记录和 `detail_file`。
- compaction record 保存 source_message_ids、detail_files、payload metadata。
- 原始 `agent_message` 不删除。

### 风险 6：token counter 不准

问题：Doubao / Volcengine tokenizer 与 tiktoken 不完全一致。

处理：

- 第一版采用保守估算。
- 触发阈值低于真实 context window，留足安全边界。
- 记录 provider usage，与估算差异进入 diagnostics。
- 如果模型返回 context overflow，允许进入 `model_error` trigger 的 full compact retry，但不能无限重试。

### 风险 7：压缩破坏 prompt cache 或推理稳定性

问题：频繁 micro compact 会让 prompt 变化过大。

处理：

- `micro_min_reduction_tokens` 小于阈值时不应用压缩。
- 已压缩消息不重复压缩。
- 优先压缩旧消息，不动 system prompt、skill index、最近消息。
- 压缩事件写入 diagnostics，便于追踪。

### 风险 8：Composer 丢失 sandbox 路径导致无法渲染

问题：Composer 的路径是可执行事实，不是普通日志。

处理：

- `sandbox_path`、`staged_input_ref`、`timeline_plan_ref` 永不压缩为摘要-only。
- 大的 ffprobe JSON 可压缩，但保留 `probe_summary`。
- `run_ffmpeg_command` stderr 可压缩，但错误摘要和 exit code 必须保留。

### 风险 9：skill 正文和 context compaction 相互打架

问题：刚加载的 skill 正文可能很长，压缩后又要求重新加载。

处理：

- 最近一次 `load_agent_skill` 结果在同一 turn 内不 micro-compact。
- 跨 turn 的 skill 正文可以压缩成 skill name / version / hash / loaded sections。
- 若 Agent 需要完整 skill，重新调用 `load_agent_skill`。

### 风险 10：Agent 对话框历史被压缩占位符污染

问题：如果实现时直接改写 `agent_message`，用户会在对话框中看到“工具结果已压缩”而不是原始交互历史，体验和审计都会变差。

处理：

- 压缩改写只发生在 model input projection。
- `agent_message` 原始内容只追加、不覆盖、不删除。
- `agent_message_compaction` 只记录关联关系，不参与默认消息列表正文渲染。
- 对话消息列表 API 增加回归测试，断言压缩后仍返回原始消息正文。
- compaction placeholder 只允许出现在模型输入快照、trace diagnostics、测试断言和 compaction record 中。

## 配置

新增配置：

```yaml
agent:
  context_compaction:
    enabled: true
    model_context_window_tokens: 256000
    micro_trigger_tokens: 180000
    micro_target_tokens: 150000
    micro_min_reduction_tokens: 8000
    full_trigger_tokens: 200000
    full_target_tokens: 140000
    preserve_recent_user_messages: 6
    preserve_recent_total_messages: 40
    search_max_results: 50
    media_image_input_token_weight: 1500
    media_card_max_chars: 1200
```

环境变量：

```text
CLIPANVIL_AGENT_CONTEXT_COMPACTION_ENABLED=true
CLIPANVIL_AGENT_CONTEXT_COMPACTION_MODEL_CONTEXT_WINDOW_TOKENS=256000
CLIPANVIL_AGENT_CONTEXT_COMPACTION_MICRO_TRIGGER_TOKENS=180000
CLIPANVIL_AGENT_CONTEXT_COMPACTION_MICRO_TARGET_TOKENS=150000
CLIPANVIL_AGENT_CONTEXT_COMPACTION_FULL_TRIGGER_TOKENS=200000
```

默认启用建议：

- 本地开发默认开启，但可用 env 关闭。
- 测试 fixture 可用小阈值强制触发。
- production 默认开启。

## 可观测性

每次压缩写入：

- agent event：`context_compaction_applied`
- task metadata diagnostics
- Coze Loop callback metadata，如已启用 tracing

事件 payload：

```json
{
  "mode": "micro",
  "role": "producer",
  "token_before": 187000,
  "token_after": 149000,
  "compaction_refs": ["agent_context_compaction/context_micro_0003"],
  "source_seq_start": 12,
  "source_seq_end": 51
}
```

用户 UI 第一版不需要展示所有细节，但后端日志和测试必须可查。

## 测试策略

### 单元测试

`contextcompact`：

- token counter 对 messages + tools 稳定计数。
- micro compact 优先压缩最早 tool result。
- micro compact 不压缩最近用户消息。
- micro compact 保持 tool_call_id 配对。
- full compact 保留最近用户消息。
- media card 不编造缺失 summary。
- compaction placeholder 包含 compact_ref、detail_file 和恢复指引。
- compaction rewrite 只作用于 model input，不修改原始 `agent_message`。

Sandbox file tools：

- `read_file` 只能读取 `/workspace` 内文件。
- `read_file` 支持 offset / limit / truncated / next_offset。
- `edit_file` 支持 create / create_or_overwrite / append / replace。
- `edit_file` 的 replace 在 `old_text` 不存在或不唯一时失败。
- path normalize 后逃逸 `/workspace` 的路径被拒绝。

Producer：

- pending signal reminder 不被持久化也不作为普通历史压缩。
- 最新用户参考图保留。
- 旧 read_project_context 被 micro compact。

Craftsman：

- 当前 scope media refs 保留。
- 旧 skill / memory tool result 可压缩。

Reviewer：

- 当前 review target 媒体不压缩。
- prior review 旧消息可压缩。

Composer：

- sandbox path / probe summary 保留。
- ffprobe 原始 JSON / ffmpeg stderr 可压缩。

### 集成测试

- 构造超长 agent_message 历史，跑 Producer task，断言模型输入 token 降到阈值以下。
- 构造包含图片、视频、音频 artifact 的 workspace，断言 media cards 包含 refs，不包含 base64。
- 已知 `detail_file` 时调用 `read_file` 能分段恢复原始 tool result。
- 不知道路径时调用 `search_agent_history(compact_ref=...)` 能定位对应 `detail_file`。
- 压缩后读取 Agent 对话框消息列表，历史消息正文仍是原始 `agent_message` 内容，不出现 compaction placeholder。
- 四个 Agent 都能在小阈值下触发压缩且不破坏 tool loop。

### Smoke

复用营销视频生成流程：

1. 上传产品参考图。
2. Producer 生成 brief / storyboard / audio plan。
3. Craftsman 生成 preview / video / audio RenderPlan。
4. Worker 生成图片、视频、音频。
5. Reviewer 评审。
6. Composer 成片。
7. 人工检查 DB：
   - `agent_context_compaction`
   - `agent_message_compaction`
   - `agent_message`
   - `artifact_version`
   - `producer_pending_signal`
8. 检查最终 Producer 不再因 context overflow 失败。

## 分阶段落地

### Milestone 1：基础包与只读规划

- 新增 config。
- 新增 token counter。
- 新增 media card builder。
- 新增 compaction planner。
- 实现 sandbox `read_file` / `edit_file` native tools。
- 补齐 path 校验、offset / limit、append / replace 单测。
- 不改 prompt，只记录 token 和候选压缩计划。
- 单测覆盖 planner。

### Milestone 2：Micro Compact

- 新增 DB 表和 sqlc。
- 实现 micro compact。
- 接入 Producer 和 Composer。
- micro compact 将大型原文写入 `/workspace/.clipanvil/context/`，占位符保留 `detail_file`。
- 新增 `search_agent_history` 按 compact_ref 定位压缩记录和 detail file。
- 用小阈值单测证明 tool result 压缩有效。

### Milestone 3：四 Agent 统一接入

- Craftsman / Reviewer 接入。
- 统一 GraphConfig / responder message override。
- 补齐四角色一致性测试。

### Milestone 4：Full Compact

- 新增 summary model runner。
- 新增 handoff summary prompt。
- 写 full compaction record。
- 接入最近消息保留策略。
- 支持 `model_error` trigger 的一次性 retry。

### Milestone 5：媒体与真实 E2E

- 完善 image / video / audio media cards。
- Composer probe/stage/ffmpeg 结果压缩。
- 跑真实营销视频生成流程，验证最终 Producer / Composer 不再爆上下文。

## 验收标准

- 长营销视频流程在 256k context window 模型下不会因为历史 tool result 膨胀而失败。
- 压缩后 Producer 能继续处理 pending signals、review results、audio success 和 composer result。
- Reviewer 当前评审媒体不被摘要替代。
- Composer 不丢失 sandbox path、duration、codec、timeline refs。
- `read_file` 能根据 detail_file 分段恢复压缩原文。
- `search_agent_history` 能根据 compact_ref 找到对应压缩记录和 detail_file。
- 原始 `agent_message` 和媒体资产不被删除。
- Agent 对话框消息列表不展示压缩占位符，仍展示原始历史消息。
- 四个 Agent 的压缩策略共享实现，不出现四份漂移逻辑。
- 单测和至少一条真实浏览器 smoke 通过。

## 待用户确认的产品取舍

推荐默认值是：

- 开启 context compaction。
- micro 180k -> 150k。
- full 200k -> 140k。
- 保留最近 6 条用户消息和最近 40 条总消息。
- 第一版搜索用 PostgreSQL 文本搜索，不接向量库。

如果实际模型 context window 不是 256k，应按模型配置调整阈值。阈值配置必须支持 per environment 覆盖。
