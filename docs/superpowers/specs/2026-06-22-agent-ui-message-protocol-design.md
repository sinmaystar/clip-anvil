# Agent UI Message Protocol 设计方案

**状态**：待评审
**日期**：2026-06-22
**所属里程碑**：M6 MultiAgent Agent Mode
**阶段目标**：将 Agent 对话消息从 `message_type + content.text + raw_message` 的临时渲染结构升级为版本化 UI Message Protocol，并同步重构右侧 ClipAnvil 对话框的输入区、模型选择、思考选择和消息渲染能力。该协议作为未来 HITL、自定义卡片、工具轨迹、图片、视频和成片预览的终态基础。

## 1. 背景

当前 M6 已经具备：

- Agent runtime 持久化：`agent_thread`、`agent_message`、`agent_task`、`agent_event`、`eino_checkpoint`。
- Agent HTTP / WebSocket 对话通道。
- 右侧悬浮 ClipAnvil 对话框。
- 真实 Volcengine 模型流式输出，包括 content delta 和 reasoning delta。
- Agent 附件上传为 source material node。
- 模型选择和 thinking depth 选择。
- `ui_card` 决策卡片雏形。

当前问题：

- 消息正文仍按普通文本 `<p>` 渲染，模型返回 Markdown 时体验很差。
- HITL 卡片、tool call、tool result、thinking、附件都通过局部 `if/else` 拼在 `AgentWorkspacePage.tsx` 内，扩展成本高。
- 模型选择和思考选择在聊天框顶部，和用户输入动作分离；不支持 thinking 的模型仍暴露了无意义的思考控件。
- composer 是单行体验，不适合真实任务描述、分镜需求、批量反馈或多附件上下文。
- 未来 Agent 会返回图片、视频、成片、review rubric、工具调用记录、确认卡片和节点引用；继续叠加 `message_type` 分支会很快失控。

本阶段不再以历史消息兼容为约束。正确方向是建立终态协议，然后让后续能力沿协议扩展。

## 2. 设计结论

采用 **版本化 Agent UI Message Protocol + 前端 block renderer registry + 多行 composer 工具栏**。

核心变化：

1. 后端新写入的 `agent_message.content` 使用 `schema + blocks` 结构。
2. 前端消息列表不直接读取 `content.text` 渲染，而是将 `content.blocks` 交给 block renderer registry。
3. WebSocket streaming delta 也按 block 语义传输，至少区分 `markdown` 和 `thinking`。
4. HITL、工具状态、附件、媒体、错误、引用等都变成 block 类型。
5. 对话框顶部只保留 ClipAnvil 身份、连接状态和关闭按钮。
6. 模型选择和思考选择移到底部 composer 工具栏。
7. 不支持 thinking 的模型完全隐藏 thinking 控件。
8. composer 改为多行 auto-grow 输入框，保留 Enter 发送、Shift+Enter 换行。

目标形态：

```json
{
  "schema": "clipanvil.agent.message.v1",
  "blocks": [
    {
      "id": "blk_1",
      "type": "markdown",
      "text": "## 广告方向\n\n- 目标用户：城市跑者\n- 核心卖点：降噪、稳定佩戴"
    },
    {
      "id": "blk_2",
      "type": "decision_card",
      "decision_id": "uuid",
      "title": "确认广告方向",
      "message": "请选择下一步主打方向。",
      "options": [
        { "id": "performance", "label": "运动性能" },
        { "id": "lifestyle", "label": "生活方式" }
      ],
      "allow_free_text": true,
      "status": "pending"
    }
  ]
}
```

## 3. 范围

### 3.1 包含

- 定义 `clipanvil.agent.message.v1` 协议。
- 后端新增 Agent message content builder，所有新消息写入 `content.schema` 和 `content.blocks`。
- 后端 HTTP DTO 返回 blocks。
- WebSocket `agent.message.delta` 升级为 block delta。
- 前端新增 `AgentMessageRenderer`。
- 前端新增 block renderer registry。
- Markdown block 使用 `react-markdown + remark-gfm`，禁用 HTML。
- Thinking block 使用可折叠渲染，流式时显示光影动效。
- Decision card block 替代当前散落的 `decisionCardFromMessage` 直接分支。
- Tool status block 渲染工具调用和工具结果。
- Attachment / media block 支持图片、视频、文本附件和后续产物预览。
- Error block 渲染失败信息。
- Composer 多行输入和底部工具栏。
- 模型选择移动到 composer 工具栏。
- Thinking 选择移动到 composer 工具栏，并按模型能力显示/隐藏。
- 单测和浏览器 E2E 验收。

### 3.2 不包含

- 完整 MultiAgent Craftsman / Worker / Composer 输出协议。
- Studio / Agent 导入导出。
- 历史消息数据迁移和兼容渲染。
- 富文本编辑器。
- 语音输入。
- MCP 工具市场或工具授权 UI。
- 图片/视频生成真实执行链路的新能力；本阶段只定义和渲染 media block。

## 4. 协议定义

### 4.1 Message Content Envelope

`agent_message.content` 新结构：

```ts
interface AgentMessageContentV1 {
  schema: "clipanvil.agent.message.v1";
  blocks: AgentMessageBlock[];
  metadata?: {
    client_message_id?: string;
    source?: "user" | "agent" | "tool" | "system";
  };
}
```

规则：

- `blocks` 是 UI 事实源。
- `raw_message` 继续保存 provider 原始元数据、diagnostics、reasoning tokens 等调试信息，但前端正常渲染不依赖 `raw_message`。
- `message_type` 暂时保留为粗粒度索引和查询字段，但不再作为 UI 渲染主分支。
- 新写入消息必须有 `schema`。
- 不做旧历史兼容；旧消息如果没有 `schema`，可以显示为 unsupported message 或在开发环境清空测试数据。

### 4.2 Block Common Fields

```ts
interface BaseBlock {
  id: string;
  type: string;
  created_at?: string;
  visibility?: "user" | "debug" | "hidden";
}
```

规则：

- `id` 在单条消息内唯一。
- `visibility="hidden"` 的 block 不渲染，但可用于调试或后续引用。
- `visibility="debug"` 首期不渲染，后续可在 debug mode 展示。

### 4.3 Markdown Block

```ts
interface MarkdownBlock extends BaseBlock {
  type: "markdown";
  text: string;
}
```

渲染：

- 使用 GFM。
- 禁用 HTML。
- 支持标题、列表、表格、代码块、引用、链接。
- 外链首期正常打开；后续可增加 link safety。

用途：

- 用户文本消息。
- assistant 最终回复。
- 工具结果摘要。
- review rubric 摘要。

### 4.4 Thinking Block

```ts
interface ThinkingBlock extends BaseBlock {
  type: "thinking";
  text: string;
  status: "streaming" | "done";
  default_collapsed: boolean;
}
```

规则：

- thinking 模型流式输出时写入 streaming block delta。
- 完成后 status 改为 `done`，默认收起。
- 不支持 thinking 的模型不应产生 thinking block；如果 provider 返回了 reasoning delta 但模型 capability 关闭，后端仍可保存到 raw diagnostics，但不生成用户可见 thinking block。
- 后续如果需要调试展示，可由 debug visibility 控制，不污染普通用户体验。

### 4.5 Decision Card Block

```ts
interface DecisionCardBlock extends BaseBlock {
  type: "decision_card";
  decision_id: string;
  title: string;
  message: string;
  options: Array<{
    id: string;
    label: string;
    description?: string;
  }>;
  allow_free_text: boolean;
  status: "pending" | "handled" | "failed" | "cancelled";
  selected_option_id?: string;
  free_text?: string;
}
```

规则：

- 由 `request_user_decision` tool 生成。
- 与 `agent_event(type='decision_requested')` 关联。
- 用户提交后，后端更新相关 card block 状态或写入新的 resolved block。
- 前端只根据 block 渲染卡片，不再从零散 `content.card_type` 推断。

### 4.6 Tool Status Block

```ts
interface ToolStatusBlock extends BaseBlock {
  type: "tool_status";
  tool_call_id: string;
  tool_name: string;
  label: string;
  status: "running" | "succeeded" | "failed";
  summary?: string;
  error_message?: string;
}
```

用途：

- `read_workspace_context`：正在读取项目上下文 / 已读取项目上下文。
- `create_agent_text_node`：正在创建文本节点 / 已创建文本节点。
- 未来生成图片、视频、成片时展示任务状态。

规则：

- 工具调用和工具结果仍可分别持久化为 agent message，但 UI 渲染统一用 tool_status block。
- 失败时必须有 `error_message` 或可读 fallback。

### 4.7 Attachment Block

```ts
interface AttachmentBlock extends BaseBlock {
  type: "attachment";
  attachments: Array<{
    asset_id: string;
    node_id: string;
    kind: "image" | "video" | "text";
    name: string;
    mime: string;
    size_bytes: number;
  }>;
}
```

用途：

- 用户发送消息时携带上传素材。
- assistant 引用已有素材。

规则：

- 附件 chip 是 block 渲染的一部分。
- 附件上传后仍创建 Agent-owned source material node。

### 4.8 Media Block

```ts
interface MediaBlock extends BaseBlock {
  type: "media";
  asset_id: string;
  node_id?: string;
  kind: "image" | "video" | "text" | "final_video";
  title?: string;
  url?: string;
  thumbnail_url?: string;
  mime?: string;
}
```

用途：

- assistant 返回生成图。
- assistant 返回视频预览。
- Composer 返回成片。
- Worker 返回中间产物。

规则：

- 首期只做渲染容器和空状态，不新增生成能力。
- URL 必须来自后端授权/存储服务，前端不拼接存储路径。

### 4.9 Error Block

```ts
interface ErrorBlock extends BaseBlock {
  type: "error";
  title: string;
  message: string;
  code?: string;
  retryable?: boolean;
}
```

用途：

- 模型调用失败。
- 工具失败。
- HITL resume 失败。

## 5. WebSocket Delta

当前 `agent.message.delta` 需要升级为 block delta。

```ts
type AgentMessageDeltaEvent = {
  type: "agent.message.delta";
  payload: {
    workspace_id: string;
    thread_id: string;
    task_id: string;
    message_id?: string;
    block_id: string;
    block_type: "markdown" | "thinking";
    delta: string;
    sequence: number;
  };
};
```

规则：

- assistant content delta 写入 `block_type="markdown"`。
- reasoning delta 写入 `block_type="thinking"`，但只有当前模型 supports_thinking 且 reasoning effort 非关闭时才广播用户可见 thinking delta。
- 前端按 `task_id + block_id` 合并 streaming blocks。
- 最终 `agent.message.created` 到达后，用持久化 message 替换 streaming placeholder。
- WebSocket 断连后继续通过 REST 拉取持久化 messages，不要求 delta 补发。

## 6. 后端实现设计

### 6.1 新增包

建议新增：

```text
apps/server/internal/agent/uimessage/
  blocks.go
  builder.go
  stream.go
```

职责：

- 定义 Go 侧 block struct。
- 提供 `NewUserMarkdownMessage`、`NewAssistantMarkdownMessage`、`NewThinkingBlock`、`NewDecisionCardMessage`、`NewToolStatusMessage` 等 builder。
- 统一生成 block id。
- 统一 JSON marshal。

### 6.2 写入规则

用户消息：

```json
{
  "schema": "clipanvil.agent.message.v1",
  "blocks": [
    { "id": "blk_text", "type": "markdown", "text": "用户输入" },
    { "id": "blk_attachments", "type": "attachment", "attachments": [] }
  ],
  "metadata": {
    "client_message_id": "..."
  }
}
```

assistant 最终消息：

```json
{
  "schema": "clipanvil.agent.message.v1",
  "blocks": [
    {
      "id": "blk_thinking",
      "type": "thinking",
      "text": "...",
      "status": "done",
      "default_collapsed": true
    },
    {
      "id": "blk_answer",
      "type": "markdown",
      "text": "..."
    }
  ]
}
```

无 thinking 时不写 thinking block。

tool call / result：

- 可继续保留 `message_type='tool_call'` 和 `message_type='tool_result'` 作为数据库分类。
- `content.blocks` 内使用 `tool_status`。

HITL：

- `message_type='ui_card'` 可以保留作为粗分类。
- `content.blocks` 内使用 `decision_card`。

### 6.3 Prompt Context

Producer prompt 构建不应直接依赖 UI block 的全部结构。

规则：

- `markdown` block 可作为 user / assistant 文本上下文。
- `attachment` block 转为附件摘要和可用 image input。
- `decision_card` block 可转为“用户待选择/已选择”的摘要。
- `tool_status` block 默认不进入普通历史上下文；同轮 tool resume 仍通过工程层 same-turn tool messages 传给模型。
- `thinking` block 不进入普通历史上下文。
- `media` block 转为资产摘要，是否传 image input 由模型能力决定。

## 7. 前端实现设计

### 7.1 新增模块

建议新增：

```text
apps/web/src/lib/agentMessageBlocks.ts
apps/web/src/components/agent/AgentMessageRenderer.tsx
apps/web/src/components/agent/AgentComposer.tsx
apps/web/src/components/agent/AgentMarkdownBlock.tsx
apps/web/src/components/agent/AgentThinkingBlock.tsx
apps/web/src/components/agent/AgentDecisionCardBlock.tsx
apps/web/src/components/agent/AgentToolStatusBlock.tsx
apps/web/src/components/agent/AgentMediaBlock.tsx
```

`AgentWorkspacePage.tsx` 保留页面编排和数据订阅，不继续承担 block 解析和渲染细节。

### 7.2 Renderer Registry

```ts
type AgentBlockRenderer = (props: {
  block: AgentMessageBlock;
  message: AgentMessage;
  actions: AgentMessageActions;
}) => React.ReactNode;
```

规则：

- renderer registry 按 `block.type` 查找。
- 未知 block 渲染为轻量 unsupported block，不让页面崩溃。
- decision card 的提交动作通过 `actions.respondDecision` 注入。

### 7.3 Markdown Renderer

- 复用 `react-markdown` 和 `remark-gfm`。
- `skipHtml` 必须开启。
- 样式不能依赖 Studio 的 MarkdownPreview 卡片背景；Agent 气泡内 markdown 要轻量、可读、紧凑。
- 代码块支持横向滚动。
- 表格在窄宽度下横向滚动。

### 7.4 Composer

布局：

```text
composer shell
  attachment chips
  textarea auto-grow
  bottom toolbar
    left:
      add attachment
      model selector
      thinking selector only if supported
      future: MCP / Canvas / tools
    right:
      send button
```

交互：

- textarea 最小 2 行，最大约 8 行，超过后内部滚动。
- Enter 发送。
- Shift+Enter 换行。
- 发送时 trim 文本，但 textarea 可保留多行编辑体验。
- agent busy 时禁用发送和模型切换。
- attachment upload 时禁用发送。
- 不支持 thinking 的模型完全隐藏 thinking 控件。
- 支持 thinking 的模型显示 pill/dropdown：
  - `思考 关闭`
  - `思考 低`
  - `思考 中`
  - `思考 高`
- 模型 selector 可显示当前模型短名，例如 `Doubao Mini`、`Doubao Pro`。

### 7.5 Header

顶部 header 只保留：

- `ClipAnvil`
- 连接状态点。
- 关闭按钮。

不再放模型和思考控件。

## 8. 数据与迁移策略

本阶段不以旧历史消息兼容为目标。

策略：

- 新写入消息全部使用 `clipanvil.agent.message.v1`。
- 本地开发和测试数据可以清空或忽略旧消息。
- 如果页面遇到旧消息，显示 `Unsupported message format` 的开发态 fallback 即可。
- 不写一次性历史数据迁移。
- 不为旧 `content.text` 增加长期渲染分支。

原因：

- Agent 功能仍在 M6 开发期，协议正确性优先于历史测试数据保留。
- 未来 MultiAgent / HITL / media 扩展依赖统一 blocks。

## 9. 可交付标准

后端：

- `agent_message.content` 新消息使用 `schema + blocks`。
- user message、assistant message、tool status、decision card、error message 都通过 builder 生成。
- REST 返回 blocks。
- WS delta 使用 block delta。
- Producer prompt builder 从 blocks 提取语义上下文，不读取旧 `content.text`。
- 不支持 thinking 的模型不生成用户可见 thinking block。

前端：

- 顶部模型和思考控件移除。
- composer 多行输入、工具栏模型选择、条件 thinking 选择完成。
- 模型不支持 thinking 时不展示 thinking 控件。
- Markdown 正常渲染。
- Thinking block 可折叠，流式时有光影动效。
- Decision card 通过 block renderer 渲染。
- Tool status 通过 block renderer 渲染。
- Attachment/media/error/unknown block 有可用 UI。
- 现有右侧悬浮、resize、折叠小球、自动滚动不回归。

## 10. 验收测试标准

### 10.1 后端单测

必须覆盖：

- UI message builder 输出合法 schema。
- user markdown + attachment blocks。
- assistant markdown + thinking blocks。
- no-thinking 模型不会产生 visible thinking block。
- decision card block。
- tool status block。
- prompt builder 从 markdown blocks 拼上下文。
- prompt builder 不拼 thinking blocks。
- REST DTO 返回 blocks。
- WS delta payload 包含 block id/type/sequence。

### 10.2 前端单测

必须覆盖：

- block parser 识别 `markdown`、`thinking`、`decision_card`、`tool_status`、`attachment`、`media`、`error`。
- unknown block 不崩溃。
- Markdown block 渲染 GFM 内容。
- model selector 在 composer 内。
- thinking selector 只在支持 thinking 的模型下展示。
- textarea 支持多行状态。
- Enter / Shift+Enter 行为。

### 10.3 浏览器 E2E

必须覆盖：

1. 打开 Agent workspace。
2. 顶部 header 不再出现模型选择和思考选择。
3. composer 工具栏出现模型选择。
4. 选择 Doubao Mini 后，不展示思考控件。
5. 选择 Doubao Pro 后，展示思考控件。
6. 输入多行消息并发送。
7. assistant 返回 Markdown，列表/标题/代码块按 Markdown 渲染。
8. thinking 模型返回 thinking 时，thinking block 可折叠且完成后默认收起。
9. 触发工具调用，tool status block 渲染 running/succeeded。
10. 触发 HITL，decision card block 可点击并刷新后恢复。
11. 上传图片/文本附件，user message 渲染 attachment block。
12. 切换 mini -> pro 后，历史 markdown blocks 作为上下文拼接正常。

### 10.4 严格验收命令

```bash
make sqlc-generate
make server-test
make server-build
make server-lint
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

浏览器验收：

```bash
./scripts/dev-start.sh
```

使用脚本输出的 Vite URL 完成 E2E。不得默认写死 `localhost:5173`，除非脚本实际输出该端口。

## 11. 实施拆分建议

建议分四步：

1. **协议与 builder**
   - 后端 UI message block 类型。
   - user/assistant/tool/HITL builder。
   - REST DTO blocks 输出。

2. **streaming 和 prompt context**
   - WS block delta。
   - Producer responder 写 blocks。
   - prompt builder 从 blocks 提取上下文。

3. **前端 renderer**
   - block parser。
   - renderer registry。
   - Markdown/thinking/tool/decision/attachment/media/error 渲染。

4. **composer redesign**
   - 多行输入框。
   - 模型选择移入 composer。
   - thinking 条件显示。
   - E2E 验收。

## 12. 开放风险

- Markdown 渲染需要注意长代码块和表格在窄聊天框内的横向滚动。
- Provider 可能在 capability 标记不支持 thinking 时仍返回 reasoning delta；协议规定普通用户不可见，但 diagnostics 可保留。
- `message_type` 和 `content.blocks` 会短期共存。`message_type` 只作为粗分类和查询维度，不再作为 UI 主协议。
- 后续如果 blocks 需要编辑或局部更新，需要在 `agent_message` 上增加更明确的 update event；本阶段可以通过新 message 或 final message 替换 streaming placeholder。
