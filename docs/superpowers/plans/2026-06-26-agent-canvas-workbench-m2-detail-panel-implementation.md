# Agent Canvas Workbench M2 Detail Panel Implementation Plan
> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 Agent Canvas Workbench 的 M2 详情审阅能力：用户点击 Overview、KeyElement、KeyElementState、Scene、Shot、Preview/Video 媒体、RenderPlan、Review、Issue 等画布对象时，右侧或浮层详情面板展示完整、可审阅的结构化信息，让画布从“总览投影”升级为“可追踪制作过程的工作台”。

**Architecture:** 后端新增 Agent Canvas Detail API，按对象类型懒加载详情，避免首屏画布 payload 携带 compiled prompt、provider response 等重信息。前端保留 React Flow 作为投影层，新增结构化 selection、详情查询和 Agent 专用详情面板；不复用 Studio PropertyPanel。数据库仍是事实源，Workbench 画布只负责组织和呈现。

**Tech Stack:** Go 1.26、Hertz、pgx/sqlc、PostgreSQL、React 19、TypeScript 6、Vite 8、`@xyflow/react`、TanStack Query、TailwindCSS 4。

## 当前代码事实

- 当前工作树已经有 M1 Workbench 画布：
  - 后端投影 API：`GET /api/agent/workspaces/:workspaceID/canvas/workbench`
  - 后端投影构建：`apps/server/internal/api/agent_workbench_projection.go`
  - 前端类型：`apps/web/src/lib/agentWorkbench.ts`
  - 前端布局：`apps/web/src/lib/agentWorkbenchViewModel.ts`
  - 前端组件：`apps/web/src/components/agent-workbench/`
  - 页面入口：`apps/web/src/pages/AgentWorkspacePage.tsx`
- 当前页面已经把 Agent Canvas 从散乱节点改为按 Project Overview、Scene Group、Shot Node 组织。
- 当前 `AgentShotNode` 已经支持动态 artifact 展示，不再假设每个 shot 固定只有 preview + video + review。
- 当前 M2 缺口：
  - 画布对象只有浅层摘要，点击后没有完整详情。
  - Preview、Video、RenderPlan、Review、Issue 没有独立选择模型。
  - 首屏 Workbench payload 不适合承载 compiled prompt、compiled request、provider response、review rubric 等重字段。
  - `selectedWorkbenchObjectId` 只是字符串 ID，不能表达对象类型和子对象选择。

## M2 范围

### 必须支持点击查看详情的对象

- `overview`：项目总览，包括 CreativeBrief、ProjectMemory、关键约束、素材概况。
- `key_element`：关键元素，包括类型、来源、状态列表、被哪些 shot 使用。
- `key_element_state`：关键元素状态，包括 `needs_reference` 原因、参考资源、被哪些 shot 依赖。
- `scene`：场景，包括场景摘要、视觉设定、包含 shot、状态。
- `shot`：分镜，包括创意、动作、镜头、音频、引用、依赖、当前生产状态。
- `artifact`：媒体节点，包括图片/视频版本、generation job、worker 状态、URL、错误。
- `render_plan`：模型调用计划，包括目标阶段、操作、模型、reference/subject bindings、prompt parts、compiled prompt、compiled request、能力校验/worker 错误。
- `review`：Reviewer 结果，包括 10 轴 rubric、总分、结论、critique、修复建议、关联 issue。
- `issue`：具体开放问题，包括严重级别、目标对象、描述、建议修复、状态。

### 不纳入 M2

- 不做详情面板内编辑和重新生成按钮。
- 不做 Studio PropertyPanel 复用。
- 不把 Agent 详情 API 暴露为 Studio API。
- 不改 Agent 生产链路，不改变 Producer/Craftsman/Reviewer 调度逻辑。
- 不在首屏 Workbench API 中塞入完整 provider response 或 compiled request。

## 后端数据契约

新增 API：

```http
GET /api/agent/workspaces/:workspaceID/canvas/details?object_type=shot&object_id=<uuid>
```

`object_type` 枚举：

```text
overview
key_element
key_element_state
scene
shot
artifact
render_plan
review
issue
```

统一响应外壳：

```go
type agentCanvasDetailResponse struct {
	ObjectType string `json:"object_type"`
	ObjectID   string `json:"object_id"`
	Title      string `json:"title"`
	Status     string `json:"status,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`

	Overview        *agentCanvasOverviewDetailResponse        `json:"overview,omitempty"`
	KeyElement      *agentCanvasKeyElementDetailResponse      `json:"key_element,omitempty"`
	KeyElementState *agentCanvasKeyElementStateDetailResponse `json:"key_element_state,omitempty"`
	Scene           *agentCanvasSceneDetailResponse           `json:"scene,omitempty"`
	Shot            *agentCanvasShotDetailResponse            `json:"shot,omitempty"`
	Artifact        *agentCanvasArtifactDetailResponse        `json:"artifact,omitempty"`
	RenderPlan      *agentCanvasRenderPlanDetailResponse      `json:"render_plan,omitempty"`
	Review          *agentCanvasReviewDetailResponse          `json:"review,omitempty"`
	Issue           *agentCanvasIssueDetailResponse           `json:"issue,omitempty"`
}
```

错误约定：

- 未登录：沿用现有 auth middleware 行为。
- workspace 不存在或非 Agent 模式：404。
- `object_type` 非法：400，返回可读错误。
- `object_id` 非 UUID 且不是 `overview`：400。
- 对象不存在或不属于该 workspace：404。

## 前端交互契约

新增选择对象：

```ts
export type AgentWorkbenchObjectType =
  | "overview"
  | "key_element"
  | "key_element_state"
  | "scene"
  | "shot"
  | "artifact"
  | "render_plan"
  | "review"
  | "issue";

export interface AgentWorkbenchSelection {
  objectType: AgentWorkbenchObjectType;
  objectId: string;
  label?: string;
}
```

选择规则：

- 点击 Project Overview 节点：选择 `overview`。
- 点击 KeyElement chip：选择 `key_element`。
- 点击 KeyElementState chip：选择 `key_element_state`。
- 点击 Scene Group 空白或 header：选择 `scene`。
- 点击 Shot 卡片主体：选择 `shot`。
- 点击 Preview/Video artifact 卡片：选择 `artifact`，`objectId = media_node.id`。
- 点击 RenderPlan summary chip：选择 `render_plan`。
- 点击 Review summary：选择 `review`。
- 点击 Issue chip：选择 `issue`。

详情面板规则：

- 没有选择时，面板隐藏。
- 有选择时，调用 detail API。
- loading 时显示轻量骨架。
- error 时显示可读错误和重试按钮。
- 面板不遮挡底部输入框。
- 面板内部按 section 展示，不把 JSON 直接丢给用户；复杂对象用字段表、短代码块和折叠区展示。

## 文件结构

### 后端新增/修改

```text
apps/server/cmd/server/main.go
apps/server/internal/api/agent_handler.go
apps/server/internal/api/agent_handler_test.go
apps/server/internal/api/agent_canvas_detail.go
apps/server/internal/api/agent_canvas_detail_test.go
apps/server/sqlc/queries/agent_canvas_detail.sql
apps/server/internal/db/generated/
```

### 前端新增/修改

```text
apps/web/src/lib/agentApi.ts
apps/web/src/lib/agentWorkbench.ts
apps/web/src/lib/agentWorkbenchSelection.ts
apps/web/src/lib/agentWorkbenchSelection.test.mjs
apps/web/src/pages/AgentWorkspacePage.tsx
apps/web/src/components/agent-workbench/AgentWorkbenchCanvas.tsx
apps/web/src/components/agent-workbench/AgentProjectOverviewNode.tsx
apps/web/src/components/agent-workbench/AgentSceneGroupNode.tsx
apps/web/src/components/agent-workbench/AgentShotNode.tsx
apps/web/src/components/agent-workbench/AgentWorkbenchSelectionContext.tsx
apps/web/src/components/agent-workbench/AgentCanvasDetailPanel.tsx
apps/web/src/main.css
```

## 任务清单

### 1. 后端路由和 handler 壳

- [ ] 在 `apps/server/cmd/server/main.go` 的 Agent workspace 路由组下新增：

```go
agent.GET("/workspaces/:workspaceID/canvas/details", authMiddleware, agentHandler.GetCanvasDetail)
```

- [ ] 在 `apps/server/internal/api/agent_handler.go` 新增 handler：

```go
func (h *AgentHandler) GetCanvasDetail(ctx context.Context, c *app.RequestContext) {
	workspace, ok := h.agentWorkspaceForRequest(ctx, c)
	if !ok {
		return
	}

	objectType := strings.TrimSpace(c.Query("object_type"))
	objectID := strings.TrimSpace(c.Query("object_id"))
	detail, err := buildAgentCanvasDetail(ctx, h.queries, h.storage, workspace.ID, objectType, objectID)
	if err != nil {
		writeAgentCanvasDetailError(c, err)
		return
	}

	c.JSON(http.StatusOK, detail)
}
```

- [ ] 复用 `agentWorkspaceForRequest`，确保只有当前账号的 Agent workspace 可访问。
- [ ] 在 `apps/server/internal/api/agent_handler_test.go` 增加路由契约测试，断言路由字符串存在，且 handler 名为 `GetCanvasDetail`。

### 2. 后端 detail builder 和错误模型

- [ ] 新增 `apps/server/internal/api/agent_canvas_detail.go`。
- [ ] 定义 `agentCanvasDetailError`，携带 HTTP status 和 message，避免 builder 中直接写 response。
- [ ] 定义 `buildAgentCanvasDetail(ctx, queries, storage, workspaceID, objectType, objectID)`。
- [ ] `overview` 特殊处理：`object_id` 为空时使用 workspace ID；也允许 `object_id = workspaceID`。
- [ ] 其余对象要求 `object_id` 是 UUID。
- [ ] 不在 API 中返回裸数据库 JSON 大块；需要在响应字段中保留结构化信息，但前端展示时按 section 渲染。

核心分发：

```go
switch objectType {
case "overview":
	return buildAgentCanvasOverviewDetail(ctx, q, storage, workspaceID)
case "key_element":
	return buildAgentCanvasKeyElementDetail(ctx, q, storage, workspaceID, id)
case "key_element_state":
	return buildAgentCanvasKeyElementStateDetail(ctx, q, storage, workspaceID, id)
case "scene":
	return buildAgentCanvasSceneDetail(ctx, q, storage, workspaceID, id)
case "shot":
	return buildAgentCanvasShotDetail(ctx, q, storage, workspaceID, id)
case "artifact":
	return buildAgentCanvasArtifactDetail(ctx, q, storage, workspaceID, id)
case "render_plan":
	return buildAgentCanvasRenderPlanDetail(ctx, q, storage, workspaceID, id)
case "review":
	return buildAgentCanvasReviewDetail(ctx, q, storage, workspaceID, id)
case "issue":
	return buildAgentCanvasIssueDetail(ctx, q, storage, workspaceID, id)
default:
	return nil, newAgentCanvasDetailError(http.StatusBadRequest, "unsupported object_type")
}
```

### 3. 补齐 sqlc 查询

- [ ] 新增 `apps/server/sqlc/queries/agent_canvas_detail.sql`，只补 M2 缺少的按 ID 查询，不重复已有查询。
- [ ] 如当前 generated 中已有同名能力，优先复用已有查询，避免重复。

建议查询：

```sql
-- name: GetSceneByID :one
SELECT *
FROM scene
WHERE id = $1 AND workspace_id = $2;

-- name: GetKeyElementByID :one
SELECT *
FROM key_element
WHERE id = $1 AND workspace_id = $2;

-- name: GetArtifactIssueByID :one
SELECT *
FROM artifact_issue
WHERE id = $1 AND workspace_id = $2;
```

- [ ] 如果 `media_node` 现有 `GetMediaNodeByID` 不校验 workspace，则 detail builder 必须二次校验 `node.WorkspaceID == workspaceID`。
- [ ] 运行 sqlc：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make sqlc-generate
```

### 4. Overview 详情

- [ ] `buildAgentCanvasOverviewDetail` 加载：
  - active creative brief
  - active project memory
  - active key elements
  - active key element states
  - source materials 或可用素材摘要
- [ ] 响应字段包括：
  - workspace ID
  - creative brief title/summary/brief JSON 摘要
  - project memory constraints/soul/reference policy 摘要
  - key element 列表
  - key element state 列表，特别标明 `needs_reference`
- [ ] 详情面板用于回答“当前项目的核心约束和素材基础是什么”。

### 5. KeyElement / KeyElementState 详情

- [ ] `key_element` 详情加载：
  - element 基础信息
  - 所有关联 states
  - 被哪些 shots 引用
  - 是否已有可用参考媒体
- [ ] `key_element_state` 详情加载：
  - state 基础信息
  - 状态为 `needs_reference` 时展示原因、缺失资源类型、推荐下一步
  - 被哪些 shots 依赖
  - 已绑定的参考媒体或生成计划
- [ ] 这部分必须覆盖验收项：“点击 KeyElementState needs_reference 后可以看到为什么缺参考，以及哪些 shots 需要它”。

### 6. Scene 详情

- [ ] `scene` 详情加载：
  - scene 基础字段和 brief
  - scene 下所有 active shots
  - scene 状态
  - scene 相关 key elements/states 摘要
- [ ] 前端展示时优先显示：
  - 场景标题
  - 地点/视觉空间
  - 情绪/风格/节奏
  - shot 列表与状态

### 7. Shot 详情

- [ ] `shot` 详情加载：
  - shot 基础字段
  - `creative_text`
  - `visual_intent`
  - `action_text`
  - `camera_intent`
  - `dialogue`
  - `narration`
  - `audio_plan`
  - `duration_sec`
  - `shot_kind`
  - dependencies
  - key element refs
  - render plans
  - media nodes/artifacts
  - reviews/issues 摘要
- [ ] 这部分必须覆盖验收项：“点击 Shot 01 能看到完整 creative/action/camera/ref/deps”。
- [ ] 如果某些字段为空，前端显示“未填写”，不要显示空 JSON 或空白区块。

### 8. Artifact 详情

- [ ] `artifact` 详情以 `media_node.id` 为 object ID。
- [ ] 加载：
  - media node 基础信息
  - media asset
  - artifact versions
  - generation jobs
  - 当前可展示 URL
  - provider/model/task 状态
  - error message
  - 关联 render plan
  - 关联 reviews/issues
- [ ] URL 仍通过现有 storage/signed URL 逻辑生成，不在前端拼接 MinIO 地址。
- [ ] 这部分必须覆盖验收项：“点击 preview 区域能看到当前 image version、generation job、成功/失败原因”。

### 9. RenderPlan 详情

- [ ] `render_plan` 详情加载：
  - scope object
  - phase
  - operation
  - task type
  - provider/model
  - reference bindings
  - subject bindings
  - prompt parts
  - params
  - audit hints
  - blocker
  - compiled prompt
  - compiled request
  - prompt audit
  - cost estimate
  - rationale
  - submitted worker task ID
  - output node/version
  - capability/compile/worker errors
- [ ] 前端展示：
  - 基础元数据用字段表。
  - reference/subject bindings 用列表。
  - prompt parts 用分块文本。
  - compiled prompt 用可复制代码块。
  - compiled request 默认折叠。
- [ ] 这部分必须覆盖验收项：“点击 RenderPlan summary 能看到 reference bindings 和 invalid media node refs”。

### 10. Review / Issue 详情

- [ ] `review` 详情加载：
  - review record
  - 10 轴 rubric
  - overall score
  - verdict/status
  - critique
  - retry recommendation
  - escalation
  - review task
  - linked issues
- [ ] `issue` 详情加载：
  - issue 基础字段
  - target object
  - linked review
  - severity/category/status
  - description
  - suggested fix
- [ ] 前端必须用 10 轴 rubric 展示 Reviewer 结果，不把 rubric JSON 原样丢出。
- [ ] 这部分必须覆盖验收项：“点击 Review summary 能看到 10-axis rubric + issue”。

### 11. 后端单元测试

- [ ] 新增 `apps/server/internal/api/agent_canvas_detail_test.go`。
- [ ] 使用已有 API 测试风格构造测试数据，至少覆盖：
  - invalid object type 返回 400。
  - object 不属于 workspace 返回 404。
  - `overview` 返回 creative brief、project memory、key elements。
  - `shot` 返回 creative/action/camera/dependencies。
  - `artifact` 返回 generation job 和 artifact version。
  - `render_plan` 返回 bindings、prompt parts、compiled prompt、错误字段。
  - `review` 返回 rubric 和 issues。
  - `key_element_state` 的 `needs_reference` 返回缺失说明和依赖 shots。
- [ ] 优先测试 builder 函数；handler 路由只做契约/轻量测试。

运行：

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestAgentCanvasDetail|TestAgentCanvasWorkbench|TestAgentCanvasDetailRouteContract' -count=1
```

### 12. 前端 API 类型和 fetcher

- [ ] 在 `apps/web/src/lib/agentApi.ts` 增加 detail union 类型。
- [ ] 增加 fetcher：

```ts
export async function fetchAgentCanvasDetail(
  workspaceId: string,
  selection: AgentWorkbenchSelection,
): Promise<AgentCanvasDetail> {
  const params = new URLSearchParams({
    object_type: selection.objectType,
    object_id: selection.objectId,
  });
  return getJson<AgentCanvasDetail>(
    `/api/agent/workspaces/${workspaceId}/canvas/details?${params.toString()}`,
  );
}
```

- [ ] 类型字段和后端 JSON 保持 snake_case，组件内通过 helper 做展示转换，不在 fetcher 里破坏服务端响应结构。

### 13. 前端 selection 模型和上下文

- [ ] 新增 `apps/web/src/lib/agentWorkbenchSelection.ts`。
- [ ] 新增 `apps/web/src/components/agent-workbench/AgentWorkbenchSelectionContext.tsx`。
- [ ] Context 提供：

```ts
interface AgentWorkbenchSelectionContextValue {
  selected: AgentWorkbenchSelection | null;
  select: (selection: AgentWorkbenchSelection) => void;
  isSelected: (objectType: AgentWorkbenchObjectType, objectId: string) => boolean;
}
```

- [ ] 使用 context 是为了避免把回调函数塞进 view model 的节点数据，保持 `agentWorkbenchViewModel` 是纯数据布局。

### 14. 改造 AgentWorkspacePage

- [ ] 将 `selectedWorkbenchObjectId` 替换为 `selectedWorkbenchSelection`。
- [ ] Workbench 加载成功后默认选择 overview：

```ts
useEffect(() => {
  if (!selectedWorkbenchSelection && agentWorkbench?.overview?.workspace_id) {
    setSelectedWorkbenchSelection({
      objectType: "overview",
      objectId: agentWorkbench.overview.workspace_id,
      label: "Project Overview",
    });
  }
}, [agentWorkbench, selectedWorkbenchSelection]);
```

- [ ] 用 TanStack Query 根据 selection 加载 detail：

```ts
const detailQuery = useQuery({
  queryKey: ["agent-canvas-detail", workspaceId, selectedWorkbenchSelection],
  queryFn: () => fetchAgentCanvasDetail(workspaceId, selectedWorkbenchSelection!),
  enabled: Boolean(workspaceId && selectedWorkbenchSelection),
});
```

- [ ] 渲染 `AgentCanvasDetailPanel`，并传入 `selection`、`detailQuery`、`onClose`、`onRetry`。

### 15. 改造画布点击选择

- [ ] `AgentWorkbenchCanvas` props 改为：

```ts
interface AgentWorkbenchCanvasProps {
  workbench: AgentWorkbench;
  selected: AgentWorkbenchSelection | null;
  onSelect: (selection: AgentWorkbenchSelection) => void;
}
```

- [ ] React Flow node click 映射：
  - `overview` node => `overview`
  - `scene` node => `scene`
  - `shot` node => `shot`
- [ ] 子元素点击必须 `event.stopPropagation()`，避免 artifact 点击后又触发 shot 点击。
- [ ] 选中态按 `objectType + objectId` 判断，不再只看 node ID。

### 16. Overview 节点增加 KeyElement/State 可点击入口

- [ ] `AgentProjectOverviewNode` 展示关键元素 chip。
- [ ] 每个 key element chip 点击选择 `key_element`。
- [ ] 每个 key element state chip 点击选择 `key_element_state`。
- [ ] `needs_reference` state 用 warning 样式，但不要用刺眼红色；它是待补资源，不一定是失败。

### 17. Shot 节点增加 RenderPlan/Issue 可点击入口

- [ ] `AgentShotNode` 中 artifact slot 点击选择 `artifact`。
- [ ] 增加 render plan summary 区：
  - 只显示最近或最关键的 2-3 个计划，避免卡片膨胀。
  - 有更多时显示 `+N`，点击 shot 详情看完整列表。
- [ ] render plan chip 点击选择 `render_plan`。
- [ ] review slot 点击选择 `review`。
- [ ] issue chip 点击选择 `issue`。
- [ ] artifact 数量不固定，按实际 `shot.artifacts` 渲染；没有 artifact 时不显示 artifact 区。

### 18. 新增 AgentCanvasDetailPanel

- [ ] 新增 `apps/web/src/components/agent-workbench/AgentCanvasDetailPanel.tsx`。
- [ ] 面板组件只负责 Agent Workbench，不导入 Studio PropertyPanel。
- [ ] 通用展示组件：
  - `DetailHeader`
  - `DetailSection`
  - `FieldGrid`
  - `StatusPill`
  - `JsonFold`
  - `PromptBlock`
  - `RubricGrid`
  - `IssueList`
- [ ] 复杂字段展示规则：
  - prompt 文本使用可滚动代码块。
  - compiled request 默认折叠。
  - provider response 如后端返回，默认折叠。
  - 空字段显示“未填写”。
  - 长列表显示前几项和“查看全部”折叠。

### 19. 样式优化

- [ ] 在 `apps/web/src/main.css` 增加 Agent Detail Panel 样式。
- [ ] 视觉方向：
  - 和 Studio 模式一样干净、克制、信息密度高。
  - 使用白底、细边框、轻阴影、小圆角。
  - 标题、状态、section 层级清晰。
  - 不使用大面积渐变、装饰图案或营销式 hero。
- [ ] 面板布局：
  - desktop 宽度 380-440px。
  - 最大高度跟随画布区域，内部滚动。
  - mobile 下变为 bottom sheet 或全宽浮层。
  - 不遮挡输入框和主聊天区域。

### 20. 前端测试

- [ ] 新增 `apps/web/src/lib/agentWorkbenchSelection.test.mjs`，覆盖：
  - selection key 由 `objectType + objectId` 构成。
  - overview 默认选择构造。
  - child selection 不等同于 parent shot selection。
- [ ] 更新 `apps/web/src/lib/agentCanvas.test.mjs`，增加源码契约：
  - `fetchAgentCanvasDetail` 存在并调用 `/canvas/details`。
  - `AgentCanvasDetailPanel` 存在。
  - `AgentWorkspacePage` 使用 `AgentWorkbenchSelection` 而不是 `selectedWorkbenchObjectId`。
  - 不导入 Studio `PropertyPanel`。
- [ ] 如现有测试 runner 支持，增加轻量渲染测试；若不支持，保持源码契约测试。

运行：

```bash
pnpm --filter @clip-anvil/web exec tsc -p tsconfig.test.json
cd apps/web && node --test src/lib/agentCanvas.test.mjs src/lib/agentWorkbenchSelection.test.mjs
```

### 21. 端到端手工验收

- [ ] 启动当前 worktree：

```bash
./scripts/dev-start.sh
```

- [ ] 打开脚本输出的 Vite URL，不依赖固定 `localhost:5175`。
- [ ] 进入一个已有 Agent workspace。
- [ ] 验收以下点击路径：
  - 点击 Project Overview：能看到 CreativeBrief、ProjectMemory、KeyElements。
  - 点击 `needs_reference` 的 KeyElementState：能看到缺什么参考，以及哪些 shots 依赖它。
  - 点击 Scene：能看到场景信息和 shot 列表。
  - 点击 Shot 01：能看到 creative/action/camera/ref/deps。
  - 点击 preview 图片：能看到 version、generation job、成功或失败信息。
  - 点击 failed video：能看到失败原因。
  - 点击 RenderPlan chip：能看到 reference bindings、subject bindings、prompt parts、compiled prompt。
  - 点击 Review：能看到 10 轴 rubric、总分、critique、issue。
  - 点击 Issue：能看到目标对象和建议修复。

### 22. 全量验证

- [ ] Go 格式化：

```bash
gofmt -w apps/server/internal/api/agent_canvas_detail.go apps/server/internal/api/agent_canvas_detail_test.go apps/server/internal/api/agent_handler.go
```

- [ ] 后端：

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-lint
```

- [ ] 前端：

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
```

- [ ] 通用：

```bash
git diff --check
```

## 验收标准

- 点击 Shot 01 后，详情面板展示完整 creative/action/camera/reference/dependency 信息。
- 点击 Preview/Video artifact 后，详情面板展示当前 media node、artifact versions、generation jobs、成功/失败信息。
- 点击 RenderPlan summary 后，详情面板展示 reference bindings、subject bindings、prompt parts、compiled prompt，并能暴露 invalid media node refs 或 capability/worker errors。
- 点击 Review summary 后，详情面板展示 10 轴 rubric、overall score、critique、issue 和修复建议。
- 点击 KeyElementState `needs_reference` 后，详情面板展示缺少参考的原因，以及依赖它的 shots。
- 画布首屏 Workbench API 不携带 compiled request/provider response 等重字段。
- 详情面板不复用 Studio PropertyPanel。
- 前端画布仍按场景/分镜组织，不因为详情能力回到散乱节点。
- 所有新增后端 API 做 workspace 权限和 Agent mode 校验。
- 后端相关测试、前端 build/lint/source tests、`git diff --check` 通过。

## 风险和处理

- **风险：现有 sqlc 缺少按 ID 查询。**
  - 处理：新增 `agent_canvas_detail.sql`，只补缺失查询，运行 `make sqlc-generate`。

- **风险：详情字段过多，前端变成 JSON 浏览器。**
  - 处理：详情面板按 Agent 制作语义组织 section，JSON 仅用于折叠的审计字段。

- **风险：artifact object ID 语义不清。**
  - 处理：M2 统一用 `media_node.id` 作为 `artifact` object ID；版本和 job 是 artifact 详情的子信息。

- **风险：点击子对象时被父节点 onClick 覆盖。**
  - 处理：所有子对象点击都 `stopPropagation()`；选中态使用 `objectType + objectId`。

- **风险：面板遮挡聊天输入。**
  - 处理：面板放在画布区域内，控制 max-height 和 bottom offset；移动端使用 bottom sheet。

- **风险：当前工作树有历史改动。**
  - 处理：只修改 M2 相关文件，不回滚已有改动；提交动作需用户明确要求。

## 执行说明

- 本计划不包含自动 commit。按照当前仓库执行契约，只有用户明确要求提交时才执行 git commit。
- 如果执行中发现 M2 需要的数据并不存在于当前表结构，优先在 detail API 中返回“未产生/未绑定”的结构化空态，不改生产链路。
- 如果发现当前 Workbench projection 已经能提供某些轻量字段，仍不要把 compiled prompt、compiled request、provider response 移入 Workbench payload。

