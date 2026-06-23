# M6.6 Craftsman / Worker Preview Generation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 实现 M6.6 的第一条真实 Agent 生产链路：Producer 通过 `dispatch_craftsman` 调度 shot，CraftsmanGraph 为每个 shot 生成预览图策略和 prompt，Worker 创建预览图节点并复用 `GenerationIntent` 提交底层生成任务。

**Architecture:** Producer 仍然是用户对话入口，只通过 Eino native tool call 调用 `dispatch_craftsman`。Craftsman 是独立 Eino Graph，使用 shot-scoped `agent_thread`、`agent_message` 和 `eino_checkpoint` 持久化执行过程；Worker 是确定性的独立 `agent_task`，负责创建 Agent-owned image node 并调用现有 `production.Service.SubmitGenerationIntent`。Agent task 表示调度/提交状态，真实生成进度继续由 `generation_job` / `artifact_version` 表示。

**Tech Stack:** Go 1.26, CloudWeGo Eino v0.9.9, Hertz, pgx/sqlc, PostgreSQL migrations, ClipAnvil Agent runtime, ClipAnvil production service, Vite/React/tldraw read-only Agent canvas, Playwright/browser E2E.

---

## 计划边界

本计划只实现 M6.6 spec 中的 preview image 生成链路。

本计划不实现：

- review rubric；
- critique-based prompt rewrite；
- async generation job 失败后的自动视觉重试；
- cross-shot dependency scheduling；
- video generation；
- Composer / final video；
- Studio / Agent import-export。

## 文件结构

### 数据库和 sqlc

- Create: `apps/server/migrations/019_m6_craftsman_worker_preview.sql`
  - 扩展 `agent_task.task_type` check，允许 `craftsman_turn` 和 `worker_generation`。
- Modify: `apps/server/sqlc/queries/agent_task.sql`
  - 增加查询 queued Craftsman / Worker task 的方法。
- Modify: `apps/server/sqlc/queries/agent_thread.sql`
  - 增加按 workspace + role + scope 查询 active Craftsman thread 的方法。
- Modify: `apps/server/sqlc/queries/node.sql`
  - 增加 `CreateAgentGenerationNode`，一次性创建 Agent 生成节点。
- Review/Modify: `apps/server/sqlc/queries/production.sql`
  - 先检查现有查询是否已经支持 PSS 读取 node 最新 generation job/version；如果缺失，增加按 workspace/node 查询最新 generation job/version 的只读查询。
- Generated: `apps/server/internal/store/db/*.sql.go`
  - 由 `make sqlc-generate` 生成，不手写。

### Agent runtime

- Modify: `apps/server/internal/agent/runtime/service.go`
  - Go 侧允许新 task type。
  - 增加 `GetOrCreateScopedThread` 或 `GetOrCreateCraftsmanThread`。
  - 增加 queued Craftsman / Worker task 查询包装方法。
- Modify: `apps/server/internal/agent/runtime/service_test.go`
  - 验证新 task type、Craftsman thread 幂等创建、checkpoint 写入。

### Producer tool

- Create: `apps/server/internal/agent/tools/dispatch_craftsman.go`
  - 实现 `dispatch_craftsman` 工具定义、参数解析、shot 解析、Craftsman task 创建。
- Create: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
  - 覆盖 tool schema、默认 `max_attempts=3`、shot_refs 解析、重复 thread 修复、跳过逻辑。
- Modify: `apps/server/internal/agent/tools/registry_test.go`
  - 确认 registry 可以注册 `dispatch_craftsman`。
- Modify: `apps/server/cmd/server/main.go`
  - 注册 `dispatch_craftsman`，并注入 runtime / queries / broadcaster / Craftsman queue。

### CraftsmanGraph

- Create: `apps/server/internal/agent/craftsman/types.go`
  - 定义 `RunTaskInput`、`GraphInput`、`GraphOutput`、`Strategy`、错误码常量。
- Create: `apps/server/internal/agent/craftsman/context_loader.go`
  - 加载 shot-scoped PSS、Craftsman thread history、最新节点/job/version。
- Create: `apps/server/internal/agent/craftsman/model_responder.go`
  - 使用 Eino model 调用生成结构化 preview strategy JSON。
- Create: `apps/server/internal/agent/craftsman/graph.go`
  - 自定义 Eino Graph：load context -> call model -> validate strategy -> persist strategy -> enqueue Worker。
- Create: `apps/server/internal/agent/craftsman/executor.go`
  - 独立执行 `craftsman_turn` task，负责 task lifecycle、event、broadcast、checkpoint。
- Create: `apps/server/internal/agent/craftsman/*_test.go`
  - 覆盖 context、strategy parse/validation、graph 成功路径、checkpoint、失败重试。

### Worker

- Create: `apps/server/internal/agent/worker/types.go`
  - 定义 preview image Worker 输入/输出结构。
- Create: `apps/server/internal/agent/worker/executor.go`
  - 执行 `worker_generation` task，创建/复用 target node，提交 `GenerationIntent`。
- Create: `apps/server/internal/agent/worker/executor_test.go`
  - 覆盖 node creation、SubmitGenerationIntent 参数、task output、同步失败重试。
- Review/Modify: `apps/server/internal/production/service_test.go`
  - 先检查现有测试是否已经覆盖 `SubmitGenerationIntent` 接受 Agent-created node；如果缺失，增加集成回归测试。

### PSS / canvas / UI 可见性

- Modify: `apps/server/internal/agent/pss/producer.go`
  - 在 PSS 中展示 Craftsman thread id、preview node、generation job/version 状态。
- Modify: `apps/server/internal/agent/pss/producer_test.go`
  - 验证 preview generation 状态会进入 Producer PSS。
- Modify: `apps/server/internal/api/media_node_response.go`
  - 确认 Agent-created generation node 的 prompt/model/params/job/version/shot metadata 会出现在 read-only detail 所需响应中。
- Modify: `apps/server/internal/api/media_node_response_test.go`
  - 覆盖 Agent preview image node 的响应字段。
- Review/Modify: `apps/web/src/**`
  - 先检查 Agent read-only detail 是否已经显示 prompt/model/params/job/version/shot metadata；如果缺失，只补 read-only detail 展示，不新增可编辑入口。

### 启动和恢复

- Modify: `apps/server/cmd/server/main.go`
  - 创建 `craftsmanExecutor` 和 `workerExecutor`。
  - 启动 queued Craftsman / Worker recovery goroutine。
  - 确保 dispatch tool 能把 task 交给 executor 或队列。

---

## Task 1: 数据库和 sqlc 基础

**Files:**

- Create: `apps/server/migrations/019_m6_craftsman_worker_preview.sql`
- Modify: `apps/server/sqlc/queries/agent_task.sql`
- Modify: `apps/server/sqlc/queries/agent_thread.sql`
- Modify: `apps/server/sqlc/queries/node.sql`
- Generated: `apps/server/internal/store/db/*.sql.go`

- [ ] **Step 1: 写 migration**

创建 `apps/server/migrations/019_m6_craftsman_worker_preview.sql`，内容必须完成两件事：

```sql
-- +goose Up
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume',
        'craftsman_turn',
        'worker_generation'
    ));

-- +goose Down
ALTER TABLE agent_task DROP CONSTRAINT agent_task_type_check;
ALTER TABLE agent_task
    ADD CONSTRAINT agent_task_type_check CHECK (task_type IN (
        'producer_turn',
        'tool_call',
        'decision_resume'
    ));
```

- [ ] **Step 2: 增加 agent task 查询**

在 `apps/server/sqlc/queries/agent_task.sql` 增加：

```sql
-- name: ListQueuedCraftsmanTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'craftsman'
  AND task_type = 'craftsman_turn'
  AND status = 'queued'
ORDER BY created_at
LIMIT $1;

-- name: ListQueuedWorkerTasksAcrossWorkspaces :many
SELECT *
FROM agent_task
WHERE role = 'worker'
  AND task_type = 'worker_generation'
  AND status = 'queued'
ORDER BY created_at
LIMIT $1;
```

- [ ] **Step 3: 增加 scoped Craftsman thread 查询**

在 `apps/server/sqlc/queries/agent_thread.sql` 增加：

```sql
-- name: GetActiveAgentThreadByScope :one
SELECT *
FROM agent_thread
WHERE workspace_id = $1
  AND role = $2
  AND scope_type = $3
  AND scope_id = $4
  AND status = 'active'
ORDER BY created_at
LIMIT 1;
```

- [ ] **Step 4: 增加 Agent generation node 创建 query**

在 `apps/server/sqlc/queries/node.sql` 增加 `CreateAgentGenerationNode`。字段必须一次性写入：

```sql
-- name: CreateAgentGenerationNode :one
INSERT INTO media_node (
    workspace_id,
    node_type,
    title,
    prompt,
    prompt_template,
    operation_type,
    status,
    source,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h,
    shot_id,
    model_provider,
    model_id,
    model_params,
    metadata
) VALUES (
    $1, $2, $3, $4, $4, $5, 'queued', 'agent',
    $6, $7, $8, $9,
    $10, $11, $12, $13, $14
)
RETURNING *;
```

- [ ] **Step 5: 生成 sqlc 代码**

Run:

```bash
make sqlc-generate
```

Expected:

- `apps/server/internal/store/db/agent_task.sql.go` 出现 queued Craftsman / Worker 查询。
- `apps/server/internal/store/db/agent_thread.sql.go` 出现 `GetActiveAgentThreadByScope`。
- `apps/server/internal/store/db/node.sql.go` 出现 `CreateAgentGenerationNode`。

- [ ] **Step 6: 验证数据库变更**

Run:

```bash
make server-build
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add apps/server/migrations/019_m6_craftsman_worker_preview.sql apps/server/sqlc/queries/agent_task.sql apps/server/sqlc/queries/agent_thread.sql apps/server/sqlc/queries/node.sql apps/server/internal/store/db
git commit -m "feat: add m6 craftsman worker storage"
```

---

## Task 2: 扩展 Agent runtime task/thread API

**Files:**

- Modify: `apps/server/internal/agent/runtime/service.go`
- Modify: `apps/server/internal/agent/runtime/service_test.go`

- [ ] **Step 1: 写失败测试：允许新 task type**

在 `service_test.go` 增加测试，创建 `craftsman_turn` 和 `worker_generation` task，断言不返回 `ErrInvalidRequest`：

```go
func TestRuntimeAllowsCraftsmanAndWorkerTaskTypes(t *testing.T) {
	service, err := NewService(&fakeBeginner{}, db.New(fakeDBTX{}))
	if err != nil {
		t.Fatal(err)
	}
	workspaceID := uuidWithByte(1)
	threadID := uuidWithByte(2)
	shotID := uuidWithByte(3)

	cases := []struct {
		role string
		taskType string
	}{
		{role: "craftsman", taskType: "craftsman_turn"},
		{role: "worker", taskType: "worker_generation"},
	}
	for _, tc := range cases {
		_, err := service.CreateTask(context.Background(), CreateTaskParams{
			WorkspaceID: workspaceID,
			ThreadID:    threadID,
			Role:        tc.role,
			ScopeType:   "shot",
			ScopeID:     shotID,
			TaskType:    tc.taskType,
			MaxAttempts: 3,
		})
		if err != nil {
			t.Fatalf("%s rejected: %v", tc.taskType, err)
		}
	}
}
```

测试 helper 可以复用现有 runtime test 风格；不要引入真实 PostgreSQL 以外的新测试依赖。

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime -run TestRuntimeAllowsCraftsmanAndWorkerTaskTypes -count=1
```

Expected: fail because `validTaskType` 还不允许新类型。

- [ ] **Step 3: 更新 runtime validation**

在 `validTaskType` 中加入：

```go
case "producer_turn", "tool_call", "decision_resume", "craftsman_turn", "worker_generation":
	return true
```

- [ ] **Step 4: 增加 Craftsman thread 幂等创建 API**

在 `service.go` 增加：

```go
func (s *Service) GetOrCreateCraftsmanThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error)
```

行为：

1. 校验 `workspaceID` 和 `shotID`。
2. 调用 `GetActiveAgentThreadByScope(workspaceID, "craftsman", "shot", shotID)`。
3. 找到则返回。
4. `pgx.ErrNoRows` 时调用 `CreateThread` 创建 `role='craftsman'`、`scope_type='shot'`、`scope_id=shotID`、`runtime_provider='eino'`、`runtime_agent_name='CraftsmanGraph'`。
5. 创建遇到并发冲突时重新查询一次。

- [ ] **Step 5: 增加 queued task 查询 API**

在 `service.go` 增加：

```go
func (s *Service) ListQueuedCraftsmanTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error)
func (s *Service) ListQueuedWorkerTasksAcrossWorkspaces(ctx context.Context, limit int32) ([]db.AgentTask, error)
```

limit 规则复用 Producer：默认 50，最大 200。

- [ ] **Step 6: Run runtime tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add apps/server/internal/agent/runtime/service.go apps/server/internal/agent/runtime/service_test.go
git commit -m "feat: extend agent runtime for craftsman worker"
```

---

## Task 3: 实现 `dispatch_craftsman` Producer tool

**Files:**

- Create: `apps/server/internal/agent/tools/dispatch_craftsman.go`
- Create: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
- Modify: `apps/server/internal/agent/tools/registry_test.go`

- [ ] **Step 1: 定义 tool 依赖接口**

在 `dispatch_craftsman.go` 定义最小接口：

```go
type CraftsmanDispatcherStore interface {
	GetWorkspaceByID(ctx context.Context, id pgtype.UUID) (db.Workspace, error)
	ListActiveShotsByWorkspace(ctx context.Context, workspaceID pgtype.UUID) ([]db.Shot, error)
	GetShotByID(ctx context.Context, id pgtype.UUID) (db.Shot, error)
	GetShotByClientKey(ctx context.Context, params db.GetShotByClientKeyParams) (db.Shot, error)
	SetShotCraftsmanThread(ctx context.Context, params db.SetShotCraftsmanThreadParams) (db.Shot, error)
}

type CraftsmanRuntime interface {
	GetOrCreateCraftsmanThread(ctx context.Context, workspaceID, shotID pgtype.UUID) (db.AgentThread, error)
	CreateTask(ctx context.Context, params agentruntime.CreateTaskParams) (db.AgentTask, error)
	CreateEvent(ctx context.Context, params agentruntime.CreateEventParams) (db.AgentEvent, error)
}

type CraftsmanTaskEnqueuer interface {
	EnqueueCraftsmanTask(ctx context.Context, task db.AgentTask)
}
```

- [ ] **Step 2: 写失败测试：tool schema**

新增测试 `TestDispatchCraftsmanDefinition`：

- `Definition().Name == "dispatch_craftsman"`
- `Parameters.properties.mode.enum == ["preview_image"]`
- `Safety.UsesProductionService == true`
- `Safety.WritesCanvas == false`
- `Visibility.UserLabel` 不暴露 Producer/Craftsman 内部名，建议是 `"开始生成预览图"`

- [ ] **Step 3: 写失败测试：默认调度所有 active shots**

新增测试 `TestDispatchCraftsmanDispatchesAllActiveShotsByDefault`：

输入：

```go
ExecuteInput{
	WorkspaceID: agentWorkspaceID,
	ThreadID: producerThreadID,
	TaskID: producerTaskID,
	Arguments: map[string]any{"mode": "preview_image"},
}
```

fake store 返回 3 个 active shots。断言：

- 创建 3 个 Craftsman thread；
- 创建 3 个 `craftsman_turn` task；
- 每个 task `MaxAttempts == 3`；
- 每个 task `Input.mode == "preview_image"`；
- 返回 `dispatched` 长度为 3。

- [ ] **Step 4: 写失败测试：shot_refs 解析和 max_attempts cap**

新增测试 `TestDispatchCraftsmanResolvesShotRefsAndCapsAttempts`：

输入 `shot_refs=["shot-02"]`、`max_attempts=99`。

断言只调度 `shot-02`，并且 task `MaxAttempts == 3`。

- [ ] **Step 5: 实现参数解析和调度**

实现：

```go
func NewDispatchCraftsmanTool(store CraftsmanDispatcherStore, runtime CraftsmanRuntime, enqueuer CraftsmanTaskEnqueuer) DispatchCraftsmanTool
func (t DispatchCraftsmanTool) Definition() Definition
func (t DispatchCraftsmanTool) Execute(ctx context.Context, input ExecuteInput) (ExecuteOutput, error)
```

核心规则：

- 只允许 Agent workspace。
- `mode` 只允许 `preview_image`。
- `max_attempts` 默认 3，范围 1 到 3。
- `shot_refs` 为空时选 active shots 中 `planned`、`draft`、`failed`；`force=true` 时允许 `preview_ready`。
- 根据 UUID 或 client_key 解析 shot。
- 对每个 shot：
  - `GetOrCreateCraftsmanThread`；
  - `SetShotCraftsmanThread` 修复 shot link；
  - `CreateTask(role='craftsman', task_type='craftsman_turn', scope_type='shot')`；
  - 创建 `craftsman_dispatched` event；
  - 调用 `EnqueueCraftsmanTask`。

- [ ] **Step 6: Run tool tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'TestDispatchCraftsman|TestRegistryIncludes' -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add apps/server/internal/agent/tools/dispatch_craftsman.go apps/server/internal/agent/tools/dispatch_craftsman_test.go apps/server/internal/agent/tools/registry_test.go
git commit -m "feat: add dispatch craftsman agent tool"
```

---

## Task 4: 实现 Craftsman scoped PSS/context loader

**Files:**

- Create: `apps/server/internal/agent/craftsman/types.go`
- Create: `apps/server/internal/agent/craftsman/context_loader.go`
- Create: `apps/server/internal/agent/craftsman/context_loader_test.go`

- [ ] **Step 1: 定义 Craftsman 类型**

在 `types.go` 定义：

```go
type RunTaskInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	ShotID      pgtype.UUID
}

type GraphInput struct {
	WorkspaceID pgtype.UUID
	ThreadID    pgtype.UUID
	TaskID      pgtype.UUID
	ShotID      pgtype.UUID
	MaxAttempts int
}

type GraphOutput struct {
	Strategy Strategy
	WorkerTask db.AgentTask
	Metadata map[string]any
}

type Strategy struct {
	Strategy      string         `json:"strategy"`
	PreviewPrompt string         `json:"preview_prompt"`
	NegativePrompt string        `json:"negative_prompt,omitempty"`
	StyleNotes    []string       `json:"style_notes,omitempty"`
	InputNodeRefs []string       `json:"input_node_refs,omitempty"`
	Model         ModelSpec      `json:"model,omitempty"`
	Params        map[string]any `json:"params,omitempty"`
}
```

- [ ] **Step 2: 写失败测试：context 包含 shot 和节点**

新增 `TestContextLoaderBuildsShotScopedContext`：

fake store 返回：

- shot `shot-01`；
- 同 shot 的 source/image nodes；
- 该 node 的 generation job/version；
- Craftsman thread 历史消息。

断言 context text 中包含：

- `shot-01`；
- shot title；
- source node title；
- latest job status；
- 不包含其他 shot 的节点。

- [ ] **Step 3: 实现 context loader**

实现：

```go
type ContextLoader struct {
	Runtime *agentruntime.Service
	Queries *db.Queries
}

func (l ContextLoader) Load(ctx context.Context, input GraphInput) (Context, error)
```

`Context` 至少包含：

- `Shot db.Shot`
- `Messages []db.AgentMessage`
- `Nodes []db.MediaNode`
- `Text string`
- `Structured map[string]any`

只读取 scoped 信息，不读取 Producer 全量历史消息。

- [ ] **Step 4: Run craftsman context tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -run TestContextLoader -count=1
```

Expected: PASS。

- [ ] **Step 5: 提交**

```bash
git add apps/server/internal/agent/craftsman/types.go apps/server/internal/agent/craftsman/context_loader.go apps/server/internal/agent/craftsman/context_loader_test.go
git commit -m "feat: add craftsman scoped context"
```

---

## Task 5: 实现 Craftsman model responder 和 strategy 校验

**Files:**

- Create: `apps/server/internal/agent/craftsman/model_responder.go`
- Create: `apps/server/internal/agent/craftsman/model_responder_test.go`

- [ ] **Step 1: 写失败测试：解析合法 strategy JSON**

新增 `TestParseCraftsmanStrategy`：

输入：

```json
{
  "strategy": "用明亮货架背景突出商品卖点。",
  "preview_prompt": "A clean commercial product shot, bright lighting...",
  "negative_prompt": "blur, watermark",
  "style_notes": ["commercial", "clean"],
  "input_node_refs": ["node-1"],
  "model": {"provider": "", "model_id": ""},
  "params": {"size": "1024x1024"}
}
```

断言 `PreviewPrompt` 非空、`Params.size == "1024x1024"`。

- [ ] **Step 2: 写失败测试：拒绝空 prompt**

新增 `TestParseCraftsmanStrategyRejectsEmptyPrompt`，断言返回错误码 `craftsman_strategy_invalid`。

- [ ] **Step 3: 实现 parser/validator**

实现：

```go
func ParseStrategy(raw string) (Strategy, error)
func ValidateStrategy(strategy Strategy) error
```

规则：

- `strategy` 非空；
- `preview_prompt` 非空；
- `input_node_refs` 可空，但不能含空字符串；
- `params` nil 时归一化为空 map；
- model 可空，留给 Worker/provider defaults。

- [ ] **Step 4: 实现 Eino model responder**

实现：

```go
type ModelResponder interface {
	DraftPreviewStrategy(ctx context.Context, context Context) (Strategy, map[string]any, error)
}
```

默认实现使用现有 Volcengine Ark/Eino model 创建方式，配置从 `VolcengineModelResponderConfig` 复用或抽出共享配置。Prompt 要求模型只输出 JSON，不输出 Markdown fence。

- [ ] **Step 5: Run tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -run 'TestParseCraftsmanStrategy|TestVolcengineCraftsmanResponder' -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/agent/craftsman/model_responder.go apps/server/internal/agent/craftsman/model_responder_test.go
git commit -m "feat: add craftsman strategy responder"
```

---

## Task 6: 实现 Worker preview generation executor

**Files:**

- Create: `apps/server/internal/agent/worker/types.go`
- Create: `apps/server/internal/agent/worker/executor.go`
- Create: `apps/server/internal/agent/worker/executor_test.go`

- [ ] **Step 1: 定义 Worker 输入输出**

在 `types.go` 定义：

```go
type GenerationInput struct {
	Mode              string         `json:"mode"`
	ShotID            string         `json:"shot_id"`
	CraftsmanThreadID string         `json:"craftsman_thread_id"`
	CraftsmanTaskID   string         `json:"craftsman_task_id"`
	Strategy          string         `json:"strategy"`
	Prompt            string         `json:"prompt"`
	NegativePrompt    string         `json:"negative_prompt,omitempty"`
	InputNodeRefs     []string       `json:"input_node_refs,omitempty"`
	TargetNodeID      string         `json:"target_node_id,omitempty"`
	Model             ModelSpec      `json:"model,omitempty"`
	Params            map[string]any `json:"params,omitempty"`
	MaxAttempts       int            `json:"max_attempts"`
}

type GenerationOutput struct {
	Status            string `json:"status"`
	NodeID            string `json:"node_id"`
	GenerationJobID   string `json:"generation_job_id"`
	ArtifactVersionID string `json:"artifact_version_id"`
	OperationType     string `json:"operation_type"`
}
```

- [ ] **Step 2: 写失败测试：自动创建 preview image node**

新增 `TestWorkerCreatesPreviewNodeAndSubmitsGenerationIntent`：

fake queries 记录 `CreateAgentGenerationNode` 参数；fake production service 记录 `SubmitGenerationIntent` 参数并返回 job/version。

断言：

- node `source='agent'`；
- node `node_type='image'`；
- node `operation_type='text_to_image'`；
- node `shot_id` 等于 Worker input；
- metadata 有 `agent_artifact_kind='preview_image'`、`worker_task_id`；
- `GenerationIntent.OutputType == "image"`；
- `GenerationIntent.OperationType == "text_to_image"`；
- `RequestedBy.Type == "agent_worker"`。

- [ ] **Step 3: 写失败测试：复用 target_node_id**

新增 `TestWorkerUsesExistingTargetNodeWhenProvided`：

输入包含 `target_node_id`，断言不调用 `CreateAgentGenerationNode`，直接提交 `SubmitGenerationIntent`。

- [ ] **Step 4: 写失败测试：同步失败固定重试**

新增 `TestWorkerRetriesSynchronousSubmitFailure`：

fake production service 前两次返回错误，第三次成功。断言调用次数为 3，task 最终 succeeded。

- [ ] **Step 5: 实现 executor**

实现：

```go
type Executor struct {
	Runtime Runtime
	Queries *db.Queries
	Production ProductionSubmitter
	Broadcaster Broadcaster
	Logger *slog.Logger
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error
```

执行顺序：

1. `MarkTaskRunning`
2. parse task input
3. create `worker_generation_started` event
4. create or resolve target node
5. submit `production.GenerationIntent`
6. mark task succeeded with `GenerationOutput`
7. broadcast node/task/event

失败时：

- 同步错误按 `MaxAttempts` 内部重试；
- 每次失败写结构化 log；
- 最终失败 `MarkTaskFailed(code, message)`，code 使用稳定值，例如 `worker_generation_submit_failed`。

- [ ] **Step 6: Run worker tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -count=1
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add apps/server/internal/agent/worker/types.go apps/server/internal/agent/worker/executor.go apps/server/internal/agent/worker/executor_test.go
git commit -m "feat: add agent worker preview generation"
```

---

## Task 7: 实现 CraftsmanGraph 和 Craftsman executor

**Files:**

- Create: `apps/server/internal/agent/craftsman/graph.go`
- Create: `apps/server/internal/agent/craftsman/executor.go`
- Create: `apps/server/internal/agent/craftsman/graph_test.go`
- Create: `apps/server/internal/agent/craftsman/executor_test.go`

- [ ] **Step 1: 写失败测试：Graph 创建 Worker task**

新增 `TestCraftsmanGraphPersistsStrategyAndCreatesWorkerTask`：

fake context loader 返回 shot context；fake responder 返回合法 strategy；fake runtime 记录 message/checkpoint/task。

断言：

- 写入 assistant message 到 Craftsman thread；
- `UpsertCheckpoint` key 格式为 `craftsman:<workspace_id>:<shot_id>:<task_id>`；
- 创建 `agent_task(role='worker', task_type='worker_generation', scope_type='shot')`；
- worker task input 包含 prompt、shot id、craftsman thread id、craftsman task id；
- Craftsman output 包含 worker task id。

- [ ] **Step 2: 写失败测试：strategy invalid 后重试**

新增 `TestCraftsmanGraphRetriesInvalidStrategy`：

fake responder 第一次返回空 prompt，第二次返回合法 prompt。断言 responder 被调用 2 次，task succeeded。

- [ ] **Step 3: 实现 Graph**

实现：

```go
type Graph struct {
	Loader ContextLoader
	Responder ModelResponder
	Runtime Runtime
	WorkerEnqueuer WorkerTaskEnqueuer
	Logger *slog.Logger
}

func (g *Graph) Run(ctx context.Context, input GraphInput) (GraphOutput, error)
```

Graph 内部逻辑：

```text
load scoped context
for attempt <= max_attempts:
  call model
  validate strategy
  if valid break
persist assistant strategy message
upsert checkpoint
create worker task
enqueue worker task
return output
```

- [ ] **Step 4: 实现 Craftsman executor**

实现：

```go
type Executor struct {
	Runtime Runtime
	Graph Runner
	Broadcaster Broadcaster
	Logger *slog.Logger
}

func (e *Executor) RunTask(ctx context.Context, input RunTaskInput) error
```

行为复用 Producer executor 的生命周期风格：

- `MarkTaskRunning`
- event `craftsman_started`
- `Graph.Run`
- event `craftsman_strategy_created`
- `MarkTaskSucceeded`
- 失败时 message_type=`error` 写入 Craftsman thread，event `craftsman_failed`，task failed。

- [ ] **Step 5: Run craftsman tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -count=1
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add apps/server/internal/agent/craftsman
git commit -m "feat: add craftsman preview graph"
```

---

## Task 8: 接入 main.go、队列恢复和 tool registry

**Files:**

- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
- Review/Modify: `apps/server/internal/api/agent_broadcaster.go`

- [ ] **Step 1: 调整初始化顺序**

`dispatch_craftsman` 需要 production/craftsman/worker 依赖，而现有 `agentToolRegistry` 初始化早于 `productionService`。调整 `main.go` 顺序：

1. 创建 `agentRuntime`、`agentModelSelection`、`agentBroadcaster`、`hitlService`、`producerPSSBuilder`、`storyboardService`。
2. 创建 provider registry、sandbox job service、production service、production runner。
3. 创建 worker executor。
4. 创建 craftsman graph/executor。
5. 创建 `agentToolRegistry`，注册 `dispatch_craftsman`。
6. 创建 Producer graph/executor。

- [ ] **Step 2: 实现内存 enqueuer**

在 `main.go` 或一个小文件中实现轻量 enqueuer：

```go
type craftsmanTaskEnqueuer struct {
	executor *agentcraftsman.Executor
}

func (e craftsmanTaskEnqueuer) EnqueueCraftsmanTask(ctx context.Context, task db.AgentTask) {
	go func() {
		_ = e.executor.RunTask(context.Background(), agentcraftsman.RunTaskInput{
			WorkspaceID: task.WorkspaceID,
			ThreadID:    task.ThreadID,
			TaskID:      task.ID,
			ShotID:      task.ScopeID,
		})
	}()
}
```

Worker enqueuer 同理。实现时必须避免 nil executor panic；nil 时只保留 queued task，等待 recovery。

- [ ] **Step 3: 注册 dispatch tool**

在 `agenttools.NewRegistry(...)` 中增加：

```go
agenttools.NewDispatchCraftsmanTool(queries, agentRuntime, craftsmanEnqueuer)
```

工具描述中用户可见 label 不使用 Producer / Craftsman 字样。

- [ ] **Step 4: 增加 queued recovery**

启动时恢复：

```go
tasks, err := agentRuntime.ListQueuedCraftsmanTasksAcrossWorkspaces(context.Background(), 50)
...
workerTasks, err := agentRuntime.ListQueuedWorkerTasksAcrossWorkspaces(context.Background(), 50)
...
```

每个恢复失败只 `slog.Warn`，不阻断服务启动。

- [ ] **Step 5: Run server build**

Run:

```bash
make server-build
```

Expected: PASS。

- [ ] **Step 6: 提交**

```bash
git add apps/server/cmd/server/main.go apps/server/internal/agent/tools/dispatch_craftsman_test.go apps/server/internal/api/agent_broadcaster.go
git commit -m "feat: wire craftsman worker runtime"
```

如果 `agent_broadcaster.go` 未修改，不要加入提交。

---

## Task 9: PSS、node response 和只读画布可见性

**Files:**

- Modify: `apps/server/internal/agent/pss/producer.go`
- Modify: `apps/server/internal/agent/pss/producer_test.go`
- Modify: `apps/server/internal/api/media_node_response.go`
- Modify: `apps/server/internal/api/media_node_response_test.go`
- Review/Modify: `apps/web/src/**`

- [ ] **Step 1: 写失败测试：PSS 显示 preview state**

在 `producer_test.go` 新增测试，准备：

- shot `shot-01`
- preview image node with `metadata.agent_artifact_kind='preview_image'`
- generation job status `queued` or `succeeded`
- artifact version id

断言 PSS text 包含：

- `shot-01`
- `preview image`
- job status
- node id

structured state 包含 `shots[].preview_nodes[]`。

- [ ] **Step 2: 更新 PSS builder**

在 `producer.go` 中扩展现有 DB projection，读取 shot-linked nodes 和 generation jobs。保持 deterministic 输出顺序：

```text
shot sort_order
-> preview nodes by created_at
-> latest jobs by created_at desc
```

- [ ] **Step 3: 写失败测试：node response 暴露 Agent generation metadata**

在 `media_node_response_test.go` 增加 Agent preview node case，断言响应包含：

- `shot_id`
- `operation_type`
- `model_provider`
- `model_id`
- `model_params`
- `metadata.agent_artifact_kind`
- `current_version_id`

- [ ] **Step 4: 更新 API response**

先运行该测试；如果字段已经存在，只补测试；如果缺失，在 `media_node_response.go` 显式加入字段。

- [ ] **Step 5: 前端只读 detail 检查**

运行前端类型检查/构建前，先用 `rg` 查现有 detail 字段：

```bash
rg "shot_id|operation_type|model_provider|current_version_id|metadata" apps/web/src
```

如果 Agent detail 已经渲染这些字段，不改前端。如果缺失，只补只读展示，不新增编辑能力。

- [ ] **Step 6: Run focused tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/pss ./internal/api -run 'TestProducerPSS|TestMediaNode' -count=1
pnpm --filter @clip-anvil/web... build
```

Expected: PASS。

- [ ] **Step 7: 提交**

```bash
git add apps/server/internal/agent/pss/producer.go apps/server/internal/agent/pss/producer_test.go apps/server/internal/api/media_node_response.go apps/server/internal/api/media_node_response_test.go apps/web/src
git commit -m "feat: expose agent preview generation state"
```

如果 `apps/web/src` 未修改，不要加入提交。

---

## Task 10: 全量验证、真实 E2E 和日志/数据检查

**Files:**

- Modify: no code expected unless tests reveal bugs.
- Optional Create: `docs/superpowers/specs/2026-06-22-m6-6-e2e-results.md`
  - 仅当需要记录 E2E 证据时创建。

- [ ] **Step 1: 运行后端验证**

Run:

```bash
make sqlc-generate
make server-build
make server-test
make server-lint
```

Expected: all PASS。

- [ ] **Step 2: 运行前端验证**

Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: all PASS。

- [ ] **Step 3: 启动本地运行时**

Run:

```bash
./scripts/dev-start.sh
```

记录脚本输出的 Vite URL，不要假设 `http://localhost:5173`。

- [ ] **Step 4: 浏览器 E2E：创建 storyboard**

使用脚本输出的 Vite URL：

1. 打开 Agent workspace。
2. 在 ClipAnvil 对话框输入：

```text
我想创作一个 15 秒口播种草短视频，请拆成 3 个分镜，每个分镜包含时长、画面、口播和卖点。
```

3. 等待模型完成。
4. 数据库确认 3 个 shots：

```sql
SELECT id, client_key, title, status, craftsman_thread_id
FROM shot
WHERE workspace_id = '<workspace-id>'
ORDER BY sort_order;
```

Expected:

- 3 rows；
- `client_key` 稳定；
- status 为 planned/draft。

- [ ] **Step 5: 浏览器 E2E：触发 preview generation**

继续输入：

```text
为所有分镜生成预览图。
```

Expected:

- Producer 通过 native tool call 调用 `dispatch_craftsman`；
- chat 里显示面向用户的任务状态，不出现 “Producer”；
- 每个 shot 创建/复用 Craftsman thread；
- 每个 shot 有 worker_generation task。

- [ ] **Step 6: 数据库检查 Agent task/thread/checkpoint**

Run:

```sql
SELECT id, role, scope_type, scope_id, current_checkpoint_key
FROM agent_thread
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT id, role, task_type, status, attempt, max_attempts, scope_type, scope_id, output, error_code, error_message
FROM agent_task
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT key, thread_id, task_id, metadata
FROM eino_checkpoint
WHERE workspace_id = '<workspace-id>'
ORDER BY updated_at DESC;
```

Expected:

- Producer thread 1 个；
- Craftsman thread 每个 shot 1 个；
- `craftsman_turn` succeeded；
- `worker_generation` succeeded 或在 provider 慢时至少 queued/running 后最终 succeeded；
- checkpoint key 以 `craftsman:` 开头。

- [ ] **Step 7: 数据库检查 node/job/version**

Run:

```sql
SELECT id, node_type, source, operation_type, status, shot_id, current_version_id, metadata
FROM media_node
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT id, target_node_id, operation_type, provider, model_id, status, attempt, max_attempts, requested_by_type, requested_by_id, error_code, error_message
FROM generation_job
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;

SELECT id, node_id, job_id, status, is_winner, asset_id, error_message
FROM artifact_version
WHERE workspace_id = '<workspace-id>'
ORDER BY created_at;
```

Expected:

- 每个 shot 至少 1 个 Agent-owned image node；
- `operation_type='text_to_image'`；
- `requested_by_type='agent_worker'`；
- `max_attempts=3`；
- artifact version 被创建。

- [ ] **Step 8: 浏览器 E2E：只读画布和详情**

在 Agent canvas 中确认：

- 每个 preview image node 可见；
- 节点使用 Studio canvas renderer；
- 节点不可拖拽编辑；
- 打开 detail panel 能看到 prompt、model、params、shot、job/version 信息；
- 关闭/展开 Agent 对话框不导致画布卡顿或 overlay canvas 无限增高回归。

- [ ] **Step 9: 检查日志**

查看 `dev-start.sh` 输出的后端日志路径，确认存在以下结构化日志字段：

```text
workspace_id
shot_id
shot_client_key
producer_task_id
craftsman_thread_id
craftsman_task_id
worker_task_id
node_id
generation_job_id
artifact_version_id
attempt
max_attempts
provider
model_id
error_code
```

Expected:

- 成功路径可串起 Producer -> Craftsman -> Worker -> generation job。
- 失败路径有稳定 error_code 和真实 provider/runtime 错误信息。

- [ ] **Step 10: 停止运行时**

Run:

```bash
./scripts/dev-stop.sh
```

Expected: 当前 worktree profile 的前后端进程停止，不停止 PostgreSQL / Redis / MinIO / NGINX。

- [ ] **Step 11: 最终 diff 检查**

Run:

```bash
git diff --check
git status --short --branch
```

Expected:

- `git diff --check` PASS；
- 只有本任务相关文件变更；
- `.codex/environments/` 如仍为未跟踪文件，不纳入提交。

- [ ] **Step 12: 最终提交**

```bash
git add apps/server apps/web docs/superpowers/specs/2026-06-22-m6-6-e2e-results.md
git commit -m "feat: add m6 craftsman worker preview generation"
```

如果没有创建 E2E 结果文档，提交时不要加入该文件。

---

## 自检清单

- [ ] `dispatch_craftsman` 是 Producer 工具，Producer 不直接生成图片。
- [ ] CraftsmanGraph 是独立 Graph，有独立 thread、message、checkpoint。
- [ ] Worker 是独立 task，不是 CraftsmanGraph 内部函数调用。
- [ ] Worker 默认创建 image node，并复用 Studio canvas renderer 的底层 `media_node`。
- [ ] Worker 只提交 `GenerationIntent`，不绕过 `production.Service`。
- [ ] Agent task 状态和 `generation_job` 状态边界清楚。
- [ ] 固定 retry 默认 3 次，且 task/job 记录可见。
- [ ] 前端不出现 Producer/Craftsman 内部命名。
- [ ] Agent canvas 只读，不新增编辑能力。
- [ ] E2E 覆盖真实对话、工具调用、Craftsman/Worker task、DB、日志和画布。
