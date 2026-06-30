# M2 Craftsman RenderPlan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build M2 Agent generation so Producer can dispatch Craftsman, Craftsman can create RenderPlan through Eino-native tools, PromptCompiler can compile Seedream/Seedance prompts, Worker can execute the plan, and generated reference images can bind back to KeyElementState.

**Architecture:** Business DB remains the source of truth. Producer is the global Full ReAct orchestrator; Craftsman becomes a bounded ReAct RenderPlanner with a narrow tool whitelist; PromptCompiler, Worker, artifact binding, dependency readiness, and canvas projection are deterministic services. M2 must also remove M1-only wording from Producer system prompt so the live Producer can actually dispatch Craftsman.

**Tech Stack:** Go 1.26, PostgreSQL 16, pgx v5, sqlc, CloudWeGo Eino `compose.Graph` / `compose.ToolsNode`, `components/tool/utils.GoStruct2ParamsOneOf`, Hertz, Volcengine Seedream/Seedance production adapters, React 19 + TypeScript 6 + `@xyflow/react`.

---

## Scope

M2 includes:

- Producer system prompt update from M1-only wording to M2 generation dispatch wording.
- `render_plan` database table and sqlc queries.
- RenderPlan domain service and typed structs.
- `ModelPromptProfile` / `PromptCompiler` v1 for `seedream_5_image` and `seedance_2_video`.
- Craftsman bounded ReAct graph using Eino-native ToolNode.
- Craftsman tools: `read_project_context`, `read_project_memory`, `upsert_render_plan`.
- Producer tools updated/enabled: `dispatch_craftsman`, `select_artifact_version`.
- Worker path that can execute a compiled RenderPlan and write `generation_job` / `artifact_version`.
- Reference image binding path: generated artifact -> `key_element_state.reference_version_id` -> `reference_status=ready|approved`.
- PSS and canvas projection for RenderPlan and reference binding state.
- Unit, integration, and E2E coverage for the Yuexing luggage airport reference-image flow.

M2 excludes:

- Full Reviewer 10-axis implementation and repair loop.
- Separate `reference_bundle` table.
- TimelinePlan and final composition upgrades.
- Complex video `edit` / `extend` / `bridge` execution. The schema may model them, but M2 generated path focuses on `generate`.
- Manual Agent-mode editing of RenderPlan nodes on canvas.

## Current Observations

- `apps/server/internal/agent/producer/system_prompt.go` still contains M1-only constraints:
  - “M1 阶段只记录需求；M2 阶段再调度 Craftsman”
  - “M1 可用工具...”
  - “M1 阶段不要调度 Craftsman、Reviewer 或 Worker”
- Existing Craftsman is a fixed graph that asks the model for JSON `strategy` / `preview_prompt`; it is not yet Eino-native bounded ReAct.
- Existing `dispatch_craftsman` is legacy-style and shot-scoped; it does not support `key_element_state` reference image scope.
- Existing Worker accepts `GenerationInput` with `Prompt`, `InputNodeRefs`, `OperationType`, and `Model`, but has no RenderPlan-native execution path.
- Existing production `InputRef` lacks explicit provider input role / prompt alias / semantic target, which M2 needs for Seedance/Seedream reference semantics.

## File Structure

Create:

- `apps/server/migrations/025_m2_render_plan.sql`: render plan schema and model capability/profile seed rows.
- `apps/server/sqlc/queries/render_plan.sql`: RenderPlan CRUD, revision, lookup, and binding queries.
- `apps/server/internal/agent/renderplan/types.go`: RenderPlan domain structs.
- `apps/server/internal/agent/renderplan/service.go`: create/update/fork/block/compile/submit service.
- `apps/server/internal/agent/renderplan/service_test.go`: RenderPlan service tests.
- `apps/server/internal/agent/renderplan/prompt_compiler.go`: deterministic compiler entrypoint.
- `apps/server/internal/agent/renderplan/prompt_compiler_test.go`: profile compile and validation tests.
- `apps/server/internal/agent/renderplan/profiles.go`: `seedream_5_image` and `seedance_2_video` profile definitions.
- `apps/server/internal/agent/renderplan/profiles_test.go`: profile capability/default tests.
- `apps/server/internal/agent/tools/read_project_memory.go`: shared native read-only memory tool.
- `apps/server/internal/agent/tools/upsert_render_plan.go`: Craftsman-only native write tool.
- `apps/server/internal/agent/tools/render_plan_tools_test.go`: schema and natural result tests.
- `apps/server/internal/agent/craftsman/system_prompt.go`: Craftsman M2 prompt.
- `apps/server/internal/agent/craftsman/native_tool_middleware.go`: Craftsman tool-call persistence for Eino ToolNode.
- `apps/server/internal/agent/craftsman/native_tool_middleware_test.go`: persistence/error tests.
- `apps/server/internal/api/render_plan_canvas_projection.go`: RenderPlan projection helper if not folded into existing projection.
- `scripts/smoke-m2-craftsman-renderplan-e2e.sh`: API + DB E2E smoke for M2.

Modify:

- `apps/server/internal/agent/producer/system_prompt.go`: remove M1-only restrictions and add M2 dispatch rules.
- `apps/server/internal/agent/producer/model_responder_test.go`: update prompt assertions.
- `apps/server/internal/agent/tools/dispatch_craftsman.go`: support native schema, key_element_state scope, render_plan scope, target phases, and natural-language errors.
- `apps/server/internal/agent/tools/dispatch_craftsman_test.go`: cover new scope and validation.
- `apps/server/internal/agent/tools/select_version.go`: support `target_type=key_element_state`.
- `apps/server/internal/agent/tools/creative_tool_common.go`: shared validators if needed.
- `apps/server/internal/agent/craftsman/types.go`: add M2 graph state, same-turn messages, tool calls, and model context.
- `apps/server/internal/agent/craftsman/context_loader.go`: load scope-limited KeyElementState / Shot / RenderPlan context plus ProjectMemory.
- `apps/server/internal/agent/craftsman/model_responder.go`: replace fixed JSON strategy prompt with tool-calling responder using Craftsman system prompt.
- `apps/server/internal/agent/craftsman/graph.go`: replace fixed two-node graph with bounded Eino-native tool loop.
- `apps/server/internal/agent/craftsman/*_test.go`: update graph/model/context tests.
- `apps/server/internal/agent/worker/types.go`: add RenderPlanID, ReferenceBindings, provider input role metadata.
- `apps/server/internal/agent/worker/executor.go`: execute compiled RenderPlan and bind reference image outputs.
- `apps/server/internal/agent/worker/input_refs.go`: resolve RenderPlan reference bindings into production input refs.
- `apps/server/internal/production/intent.go`: extend `InputRef` with role/alias/semantic metadata.
- `apps/server/internal/production/capability.go`: validate reference role and profile limits.
- `apps/server/internal/production/volcengine_image.go`: use Seedream prompt and multi-image references from RenderPlan input refs.
- `apps/server/internal/production/volcengine_video.go`: emit Seedance image/video/audio refs with provider roles and request summary.
- `apps/server/internal/agent/pss/producer.go`: include RenderPlan, compiled prompt state, and reference binding summaries.
- `apps/server/internal/api/domain_canvas_projection.go`: include RenderPlan process nodes and reference binding edges.
- `apps/web/src/lib/api.ts`: expose RenderPlan projection fields if backend payload expands.
- `apps/web/src/components/canvas-flow/DomainFlowNode.tsx`: render RenderPlan / process nodes.
- `apps/web/src/components/canvas-flow/DomainFlowEdge.tsx`: render `references` / `renders_to` edges.
- `apps/server/cmd/server/main.go`: wire RenderPlan service, PromptCompiler, Craftsman native tools, updated Producer tools.
- `apps/server/cmd/server/e2e_producer_fixture.go`: add an M2 fixture responder for deterministic E2E if real model is unavailable.

## Task 1: Producer Prompt M2 Update

**Files:**

- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`

- [ ] **Step 1: Write failing prompt test**

Add a test in `apps/server/internal/agent/producer/model_responder_test.go`:

```go
func TestProducerSystemPromptEnablesM2DispatchAndRemovesM1OnlyRules(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, forbidden := range []string{
		"M1 阶段只记录需求",
		"M1 可用工具",
		"M1 阶段不要调度 Craftsman",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt still contains M1-only wording %q", forbidden)
		}
	}
	for _, required := range []string{
		"M2 生成调度能力",
		"dispatch_craftsman",
		"select_artifact_version",
		"不要寻找或虚构 compile/submit/schedule 工具",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing M2 wording %q", required)
		}
	}
}
```

- [ ] **Step 2: Run prompt test and confirm failure**

Run:

```bash
go test ./internal/agent/producer -run TestProducerSystemPromptEnablesM2DispatchAndRemovesM1OnlyRules -count=1
```

Expected: FAIL because prompt still contains M1-only wording and lacks the M2 dispatch section.

- [ ] **Step 3: Update Producer system prompt**

In `apps/server/internal/agent/producer/system_prompt.go`, replace:

```text
7. 在后续里程碑调度 Craftsman 创建 RenderPlan，调度 Reviewer 评审结果，向用户请求关键决策。
```

with:

```text
7. 在 M2 调度 Craftsman 创建 RenderPlan，绑定参考图，发起分镜预览图和分镜视频生成，并在关键节点向用户请求决策。
```

Replace:

```text
如果用户只说“先生成一张机场场景图看看”，不要强制规划完整广告。你应该创建或更新“机场出发大厅”的 KeyElementState，并把它标记为需要参考图。M1 阶段只记录需求；M2 阶段再调度 Craftsman 生成 reference image。
```

with:

```text
如果用户只说“先生成一张机场场景图看看”，不要强制规划完整广告。你应该创建或更新“机场出发大厅”的 KeyElementState，并把它标记为需要参考图，然后在 M2 调用 dispatch_craftsman(scope.type=key_element_state, target_phase=reference_image) 派 Craftsman 生成统一参考图。
```

Replace the tool rules section with:

```text
## 工具使用规则

- 可用创作状态工具：read_project_context、upsert_project_brief、update_project_memory、upsert_key_elements、upsert_storyboard。
- M2 可用生成调度工具：dispatch_craftsman、select_artifact_version、request_user_decision。
- 每次工具调用都要填写 brief，说明这次调用的业务目的。
- 写工具只能写自己负责的领域事实，不能借字段夹带模型 prompt。
- 写 ProjectMemory 后，如果还需要创建 storyboard，应基于新 memory 再继续。
- 创建 shot 时应引用已有 KeyElement / KeyElementState；如果缺少关键元素，先创建关键元素。
- 用户 prompt 中提到但没有上传素材的稳定元素，也要创建 KeyElementState，并设置 reference_status=needs_reference。
- 修改某个 shot 时，保留原有关联元素和连续性依赖，除非用户明确要求删除。
- 需要尾帧串联、同商品一致、同场景一致时，写 dependency，不能只写在自然语言描述里。
- 如果用户要求生成全局或场景级参考图，先确保对应 KeyElementState 存在，再派 Craftsman；不要让每个 shot 各自生成同一个机场或柔光房间。
- 生成 shot preview image 前，确保 shot 已引用关键 KeyElementState。
- 生成 shot video 前，优先使用已确认或当前 winner preview image 作为 first frame；如果有 last_frame_chain，遵守依赖顺序。
- PromptCompiler、capability validation、generation job submit、artifact binding 都由工程服务自动完成；不要寻找或虚构 compile_render_plan、submit_render_plan、schedule_ready_render_plans 工具。
```

Add this section before `## 关键禁令`:

```text
---

## M2 生成调度能力

M2 中你可以调度 Craftsman 创建 RenderPlan。RenderPlan 是可执行生成计划，不是 CreativeBrief，也不是 ShotPlan。你仍然不直接写 Seedream / Seedance provider prompt。

你可以使用：
- dispatch_craftsman：派 Craftsman 为 KeyElementState 或 Shot 创建 / 修订 RenderPlan。
- select_artifact_version：选择媒体节点 winner，或把 artifact 绑定为 KeyElementState 参考资源。
- request_user_decision：对关键参考图、高成本生成、核心方向变化或歧义请求用户确认。

dispatch_craftsman 的返回只表示任务已入队或计划已创建，不表示图片/视频已经完成。你需要读取项目上下文确认真实状态。
```

- [ ] **Step 4: Run prompt test and producer package tests**

Run:

```bash
go test ./internal/agent/producer -count=1
```

Expected: PASS.

## Task 2: RenderPlan Schema and sqlc

**Files:**

- Create: `apps/server/migrations/025_m2_render_plan.sql`
- Create: `apps/server/sqlc/queries/render_plan.sql`
- Modify generated: `apps/server/internal/store/db/*.go`

- [ ] **Step 1: Write migration**

Create `apps/server/migrations/025_m2_render_plan.sql`:

```sql
-- +goose Up
CREATE TABLE render_plan (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    scope_type TEXT NOT NULL,
    scope_id UUID NOT NULL,
    target_phase TEXT NOT NULL,
    task_type TEXT NOT NULL DEFAULT 'generate',
    model_prompt_profile TEXT NOT NULL,
    operation TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'draft',
    revision INT NOT NULL DEFAULT 1,
    forked_from_render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    render_plan_key TEXT NOT NULL DEFAULT '',
    reference_bindings JSONB NOT NULL DEFAULT '[]',
    subject_bindings JSONB NOT NULL DEFAULT '[]',
    prompt_parts JSONB NOT NULL DEFAULT '{}',
    params JSONB NOT NULL DEFAULT '{}',
    audit_hints JSONB NOT NULL DEFAULT '{}',
    blocker JSONB NOT NULL DEFAULT '{}',
    compiled_prompt TEXT NOT NULL DEFAULT '',
    compiled_request JSONB NOT NULL DEFAULT '{}',
    prompt_audit JSONB NOT NULL DEFAULT '{}',
    cost_estimate JSONB NOT NULL DEFAULT '{}',
    rationale TEXT NOT NULL DEFAULT '',
    created_by_thread_id UUID REFERENCES agent_thread(id) ON DELETE SET NULL,
    created_by_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    submitted_worker_task_id UUID REFERENCES agent_task(id) ON DELETE SET NULL,
    output_node_id UUID REFERENCES media_node(id) ON DELETE SET NULL,
    output_version_id UUID REFERENCES artifact_version(id) ON DELETE SET NULL,
    archived_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    compiled_at TIMESTAMPTZ,
    submitted_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    CONSTRAINT render_plan_scope_type_check CHECK (scope_type IN ('key_element_state', 'shot')),
    CONSTRAINT render_plan_target_phase_check CHECK (target_phase IN ('reference_image', 'preview_image', 'shot_video')),
    CONSTRAINT render_plan_task_type_check CHECK (task_type IN ('generate', 'edit', 'extend', 'bridge')),
    CONSTRAINT render_plan_profile_check CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video')),
    CONSTRAINT render_plan_status_check CHECK (status IN ('draft', 'blocked', 'compiled', 'waiting_for_approval', 'submitted', 'running', 'succeeded', 'failed', 'archived')),
    CONSTRAINT render_plan_revision_positive CHECK (revision > 0)
);

CREATE INDEX idx_render_plan_workspace_scope
    ON render_plan(workspace_id, scope_type, scope_id, archived_at, updated_at DESC);

CREATE INDEX idx_render_plan_workspace_status
    ON render_plan(workspace_id, status, updated_at DESC);

CREATE UNIQUE INDEX idx_render_plan_active_key
    ON render_plan(workspace_id, render_plan_key)
    WHERE archived_at IS NULL AND render_plan_key <> '';

CREATE UNIQUE INDEX idx_render_plan_scope_phase_revision
    ON render_plan(workspace_id, scope_type, scope_id, target_phase, revision)
    WHERE archived_at IS NULL;

ALTER TABLE agent_task
    ADD COLUMN IF NOT EXISTS render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_agent_task_render_plan
    ON agent_task(render_plan_id)
    WHERE render_plan_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_agent_task_render_plan;
ALTER TABLE agent_task DROP COLUMN IF EXISTS render_plan_id;
DROP INDEX IF EXISTS idx_render_plan_scope_phase_revision;
DROP INDEX IF EXISTS idx_render_plan_active_key;
DROP INDEX IF EXISTS idx_render_plan_workspace_status;
DROP INDEX IF EXISTS idx_render_plan_workspace_scope;
DROP TABLE IF EXISTS render_plan;
```

- [ ] **Step 2: Write sqlc queries**

Create `apps/server/sqlc/queries/render_plan.sql`:

```sql
-- name: CreateRenderPlan :one
INSERT INTO render_plan (
    workspace_id, scope_type, scope_id, target_phase, task_type,
    model_prompt_profile, operation, status, revision, forked_from_render_plan_id,
    render_plan_key, reference_bindings, subject_bindings, prompt_parts, params,
    audit_hints, blocker, rationale, created_by_thread_id, created_by_task_id
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8, $9, $10,
    $11, $12, $13, $14, $15,
    $16, $17, $18, $19, $20
) RETURNING *;

-- name: GetRenderPlanByID :one
SELECT * FROM render_plan
WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL;

-- name: ListRenderPlansByWorkspace :many
SELECT * FROM render_plan
WHERE workspace_id = $1 AND archived_at IS NULL
ORDER BY updated_at DESC, created_at DESC;

-- name: ListRenderPlansByScope :many
SELECT * FROM render_plan
WHERE workspace_id = $1
  AND scope_type = $2
  AND scope_id = $3
  AND archived_at IS NULL
ORDER BY target_phase ASC, revision DESC;

-- name: GetLatestRenderPlanByScopePhase :one
SELECT * FROM render_plan
WHERE workspace_id = $1
  AND scope_type = $2
  AND scope_id = $3
  AND target_phase = $4
  AND archived_at IS NULL
ORDER BY revision DESC
LIMIT 1;

-- name: UpdateRenderPlanDraft :one
UPDATE render_plan
SET task_type = $3,
    model_prompt_profile = $4,
    operation = $5,
    reference_bindings = $6,
    subject_bindings = $7,
    prompt_parts = $8,
    params = $9,
    audit_hints = $10,
    blocker = $11,
    rationale = $12,
    status = $13,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('draft', 'blocked')
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanCompiled :one
UPDATE render_plan
SET status = 'compiled',
    compiled_prompt = $3,
    compiled_request = $4,
    prompt_audit = $5,
    cost_estimate = $6,
    compiled_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status = 'draft'
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanBlocked :one
UPDATE render_plan
SET status = 'blocked',
    blocker = $3,
    audit_hints = $4,
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('draft', 'blocked')
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanSubmitted :one
UPDATE render_plan
SET status = 'submitted',
    submitted_worker_task_id = $3,
    output_node_id = $4,
    submitted_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('compiled', 'waiting_for_approval')
  AND archived_at IS NULL
RETURNING *;

-- name: MarkRenderPlanCompleted :one
UPDATE render_plan
SET status = $3,
    output_version_id = $4,
    completed_at = now(),
    updated_at = now()
WHERE id = $1
  AND workspace_id = $2
  AND status IN ('submitted', 'running')
  AND archived_at IS NULL
RETURNING *;

-- name: NextRenderPlanRevision :one
SELECT COALESCE(MAX(revision), 0)::int + 1 AS next_revision
FROM render_plan
WHERE workspace_id = $1
  AND scope_type = $2
  AND scope_id = $3
  AND target_phase = $4
  AND archived_at IS NULL;
```

- [ ] **Step 3: Generate sqlc and run migration**

Run:

```bash
make sqlc-generate
make migrate-up
```

Expected: sqlc generation succeeds; migration version reaches 25.

- [ ] **Step 4: Run DB-related tests**

Run:

```bash
make server-test
```

Expected: PASS or pre-existing unrelated failure clearly identified before continuing.

## Task 3: RenderPlan Domain Service

**Files:**

- Create: `apps/server/internal/agent/renderplan/types.go`
- Create: `apps/server/internal/agent/renderplan/service.go`
- Create: `apps/server/internal/agent/renderplan/service_test.go`

- [ ] **Step 1: Define service types**

Create `apps/server/internal/agent/renderplan/types.go` with these exported types:

```go
package renderplan

import (
	"encoding/json"

	"github.com/jackc/pgx/v5/pgtype"
)

const (
	ScopeKeyElementState = "key_element_state"
	ScopeShot            = "shot"

	PhaseReferenceImage = "reference_image"
	PhasePreviewImage   = "preview_image"
	PhaseShotVideo      = "shot_video"

	TaskGenerate = "generate"
	TaskEdit     = "edit"
	TaskExtend   = "extend"
	TaskBridge   = "bridge"

	ProfileSeedream5Image = "seedream_5_image"
	ProfileSeedance2Video = "seedance_2_video"

	StatusDraft    = "draft"
	StatusBlocked  = "blocked"
	StatusCompiled = "compiled"
)

type Scope struct {
	Type string
	ID   pgtype.UUID
}

type UpsertInput struct {
	WorkspaceID              pgtype.UUID
	ThreadID                 pgtype.UUID
	TaskID                   pgtype.UUID
	Brief                    string
	Mode                     string
	RenderPlanID             pgtype.UUID
	ForkFromRenderPlanID     pgtype.UUID
	Scope                    Scope
	TargetPhase              string
	TaskType                 string
	ModelPromptProfile       string
	Operation                string
	ReferenceBindings        []ReferenceBinding
	SubjectBindings          []SubjectBinding
	PromptParts              PromptParts
	Params                   Params
	AuditHints               AuditHints
	Blocker                  Blocker
	Rationale                string
	AutoCompileAndSubmit     bool
}

type ReferenceBinding struct {
	ClientKey      string `json:"client_key"`
	SourceType     string `json:"source_type"`
	SourceID       string `json:"source_id"`
	Role           string `json:"role"`
	PromptAlias    string `json:"prompt_alias,omitempty"`
	SemanticTarget string `json:"semantic_target,omitempty"`
	Priority       int    `json:"priority,omitempty"`
	Required       bool   `json:"required,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

type SubjectBinding struct {
	SubjectKey     string   `json:"subject_key"`
	Label          string   `json:"label"`
	ElementStateID string   `json:"element_state_id,omitempty"`
	PromptHandle   string   `json:"prompt_handle,omitempty"`
	StableTraits   []string `json:"stable_traits,omitempty"`
	MustPreserve   bool     `json:"must_preserve,omitempty"`
	AmbiguityNotes string   `json:"ambiguity_notes,omitempty"`
}

type PromptParts struct {
	Objective      string   `json:"objective"`
	Subject        string   `json:"subject,omitempty"`
	Setting        string   `json:"setting,omitempty"`
	Action         string   `json:"action,omitempty"`
	Camera         string   `json:"camera,omitempty"`
	Composition    string   `json:"composition,omitempty"`
	Style          string   `json:"style,omitempty"`
	Lighting       string   `json:"lighting,omitempty"`
	Sequence       []string `json:"sequence,omitempty"`
	Dialogue       string   `json:"dialogue,omitempty"`
	Narration      string   `json:"narration,omitempty"`
	Audio          string   `json:"audio,omitempty"`
	TextRendering  string   `json:"text_rendering,omitempty"`
	QualityPack    []string `json:"quality_pack,omitempty"`
	ConstraintPack []string `json:"constraint_pack,omitempty"`
	NegativeHints  []string `json:"negative_hints,omitempty"`
}

type Params struct {
	Ratio                     string  `json:"ratio,omitempty"`
	DurationSec               float64 `json:"duration_sec,omitempty"`
	Resolution                string  `json:"resolution,omitempty"`
	Watermark                 bool    `json:"watermark,omitempty"`
	GenerateAudio             bool    `json:"generate_audio,omitempty"`
	ReturnLastFrame           bool    `json:"return_last_frame,omitempty"`
	CameraFixed               bool    `json:"camera_fixed,omitempty"`
	SequentialImageGeneration string  `json:"sequential_image_generation,omitempty"`
	MaxImages                 int     `json:"max_images,omitempty"`
	Seed                      int64   `json:"seed,omitempty"`
}

type AuditHints struct {
	AutoFilled          []string `json:"auto_filled,omitempty"`
	NeedsUserDecision   []string `json:"needs_user_decision,omitempty"`
	CapabilityRisks     []string `json:"capability_risks,omitempty"`
	ConsistencyRisks    []string `json:"consistency_risks,omitempty"`
	PromptCompilerNotes []string `json:"prompt_compiler_notes,omitempty"`
}

type Blocker struct {
	BlockerType string   `json:"blocker_type,omitempty"`
	Message     string   `json:"message,omitempty"`
	NeededBy    string   `json:"needed_by,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
}

type CompileResult struct {
	CompiledPrompt  string
	CompiledRequest json.RawMessage
	PromptAudit     json.RawMessage
	CostEstimate    json.RawMessage
}
```

- [ ] **Step 2: Write service tests first**

Create `apps/server/internal/agent/renderplan/service_test.go` with tests named:

```go
func TestServiceCreatesReferenceImageRenderPlanAndCompiles(t *testing.T) {}
func TestServiceRejectsShotVideoWithSeedreamProfile(t *testing.T) {}
func TestServiceForksExecutedPlanInsteadOfUpdating(t *testing.T) {}
func TestServiceMarksBlockedWhenReferenceMissing(t *testing.T) {}
```

Each test should use a fake store implementing the exact methods required by `Service` and assert:

- created plan has `target_phase=reference_image`, `model_prompt_profile=seedream_5_image`, status `compiled`.
- invalid phase/profile returns a domain validation error before DB write.
- update on a `submitted` plan returns an error containing `只能 fork_from`.
- blocked plan stores `blocker.blocker_type=missing_reference`.

- [ ] **Step 3: Implement service**

Create `apps/server/internal/agent/renderplan/service.go` with:

```go
package renderplan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/sinmaystar/clip-anvil/internal/store/db"
)

var (
	ErrInvalidInput = errors.New("invalid render plan input")
	ErrInvalidState = errors.New("invalid render plan state")
)

type Store interface {
	CreateRenderPlan(ctx context.Context, arg db.CreateRenderPlanParams) (db.RenderPlan, error)
	GetRenderPlanByID(ctx context.Context, arg db.GetRenderPlanByIDParams) (db.RenderPlan, error)
	UpdateRenderPlanDraft(ctx context.Context, arg db.UpdateRenderPlanDraftParams) (db.RenderPlan, error)
	MarkRenderPlanCompiled(ctx context.Context, arg db.MarkRenderPlanCompiledParams) (db.RenderPlan, error)
	MarkRenderPlanBlocked(ctx context.Context, arg db.MarkRenderPlanBlockedParams) (db.RenderPlan, error)
	NextRenderPlanRevision(ctx context.Context, arg db.NextRenderPlanRevisionParams) (int32, error)
}

type Compiler interface {
	Compile(ctx context.Context, input UpsertInput) (CompileResult, error)
}

type Service struct {
	store    Store
	compiler Compiler
}

func NewService(store Store, compiler Compiler) *Service {
	return &Service{store: store, compiler: compiler}
}

func (s *Service) Upsert(ctx context.Context, input UpsertInput) (db.RenderPlan, error) {
	if s == nil || s.store == nil {
		return db.RenderPlan{}, fmt.Errorf("%w: render plan service is not configured", ErrInvalidInput)
	}
	if err := validateInput(input); err != nil {
		return db.RenderPlan{}, err
	}
	if input.Mode == "mark_blocked" {
		return s.markBlocked(ctx, input)
	}
	if input.Mode == "update_draft" {
		plan, err := s.store.GetRenderPlanByID(ctx, db.GetRenderPlanByIDParams{ID: input.RenderPlanID, WorkspaceID: input.WorkspaceID})
		if err != nil {
			return db.RenderPlan{}, err
		}
		if plan.Status != StatusDraft && plan.Status != StatusBlocked {
			return db.RenderPlan{}, fmt.Errorf("%w: 已执行 RenderPlan 不能 update_draft，只能 fork_from", ErrInvalidState)
		}
		updated, err := s.store.UpdateRenderPlanDraft(ctx, updateDraftParams(input, plan.ID))
		if err != nil {
			return db.RenderPlan{}, err
		}
		return s.compileIfReady(ctx, input, updated)
	}
	revision, err := s.store.NextRenderPlanRevision(ctx, db.NextRenderPlanRevisionParams{
		WorkspaceID: input.WorkspaceID,
		ScopeType:   input.Scope.Type,
		ScopeID:     input.Scope.ID,
		TargetPhase: input.TargetPhase,
	})
	if err != nil {
		return db.RenderPlan{}, err
	}
	created, err := s.store.CreateRenderPlan(ctx, createParams(input, revision))
	if err != nil {
		return db.RenderPlan{}, err
	}
	return s.compileIfReady(ctx, input, created)
}

func validateInput(input UpsertInput) error {
	if strings.TrimSpace(input.Brief) == "" {
		return fmt.Errorf("%w: brief 必填", ErrInvalidInput)
	}
	if strings.TrimSpace(input.Mode) == "" {
		return fmt.Errorf("%w: mode 必填", ErrInvalidInput)
	}
	if !input.WorkspaceID.Valid || !input.Scope.ID.Valid {
		return fmt.Errorf("%w: workspace_id 和 scope.id 必须有效", ErrInvalidInput)
	}
	if input.Scope.Type == ScopeKeyElementState && input.TargetPhase != PhaseReferenceImage {
		return fmt.Errorf("%w: key_element_state 只能用于 reference_image", ErrInvalidInput)
	}
	if input.TargetPhase == PhaseShotVideo && input.ModelPromptProfile != ProfileSeedance2Video {
		return fmt.Errorf("%w: shot_video 必须使用 seedance_2_video", ErrInvalidInput)
	}
	if input.TargetPhase != PhaseShotVideo && input.ModelPromptProfile == ProfileSeedance2Video {
		return fmt.Errorf("%w: 图片阶段不能使用 seedance_2_video", ErrInvalidInput)
	}
	if strings.TrimSpace(input.PromptParts.Objective) == "" && input.Mode != "mark_blocked" {
		return fmt.Errorf("%w: prompt_parts.objective 必填", ErrInvalidInput)
	}
	return nil
}
```

Fill `createParams`, `updateDraftParams`, `markBlocked`, `compileIfReady`, and JSON helpers in the same file with direct mappings to sqlc params.

- [ ] **Step 4: Run renderplan tests**

Run:

```bash
go test ./internal/agent/renderplan -count=1
```

Expected: PASS.

## Task 4: PromptCompiler v1

**Files:**

- Create: `apps/server/internal/agent/renderplan/profiles.go`
- Create: `apps/server/internal/agent/renderplan/prompt_compiler.go`
- Create: `apps/server/internal/agent/renderplan/prompt_compiler_test.go`
- Create: `apps/server/internal/agent/renderplan/profiles_test.go`

- [ ] **Step 1: Write compiler tests**

In `prompt_compiler_test.go`, write:

```go
func TestCompileSeedreamReferenceImagePrompt(t *testing.T) {
	compiler := NewPromptCompiler()
	out, err := compiler.Compile(context.Background(), UpsertInput{
		TargetPhase: PhaseReferenceImage,
		ModelPromptProfile: ProfileSeedream5Image,
		Operation: "text_to_image",
		PromptParts: PromptParts{
			Objective: "生成现代机场出发大厅参考图。",
			Setting: "清晨自然光，玻璃幕墙，干净开阔。",
			Composition: "9:16 中景，留出人物拉行李箱路径。",
			Style: "真实商业广告质感。",
			ConstraintPack: []string{"不要出现竞品 Logo"},
		},
		Params: Params{Ratio: "9:16", Resolution: "2K", MaxImages: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"现代机场出发大厅", "真实商业广告质感", "不要出现竞品 Logo"} {
		if !strings.Contains(out.CompiledPrompt, want) {
			t.Fatalf("compiled prompt missing %q: %s", want, out.CompiledPrompt)
		}
	}
}

func TestCompileSeedanceShotVideoRequiresActionOrSequence(t *testing.T) {
	compiler := NewPromptCompiler()
	_, err := compiler.Compile(context.Background(), UpsertInput{
		TargetPhase: PhaseShotVideo,
		ModelPromptProfile: ProfileSeedance2Video,
		Operation: "image_to_video_first_frame",
		PromptParts: PromptParts{Objective: "生成分镜视频。"},
		Params: Params{DurationSec: 6, Ratio: "9:16"},
	})
	if err == nil || !strings.Contains(err.Error(), "action 或 sequence") {
		t.Fatalf("error = %v, want action/sequence validation", err)
	}
}
```

- [ ] **Step 2: Implement profiles**

In `profiles.go`, define:

```go
package renderplan

type ModelPromptProfile struct {
	ID                 string
	DefaultProvider    string
	DefaultModelID     string
	OutputType         string
	AllowedOperations  map[string]bool
	DefaultParams      Params
	MaxPromptChars     int
}

func ProfileByID(id string) (ModelPromptProfile, bool) {
	switch id {
	case ProfileSeedream5Image:
		return ModelPromptProfile{
			ID: ProfileSeedream5Image,
			DefaultProvider: "volcengine",
			DefaultModelID: "doubao-seedream-5-0-260128",
			OutputType: "image",
			AllowedOperations: map[string]bool{
				"text_to_image": true,
				"image_to_image": true,
				"multi_image_to_image": true,
			},
			DefaultParams: Params{Ratio: "9:16", Resolution: "2K", MaxImages: 1},
			MaxPromptChars: 2400,
		}, true
	case ProfileSeedance2Video:
		return ModelPromptProfile{
			ID: ProfileSeedance2Video,
			DefaultProvider: "volcengine",
			DefaultModelID: "doubao-seedance-2-0-pro-260428",
			OutputType: "video",
			AllowedOperations: map[string]bool{
				"text_to_video": true,
				"image_to_video_first_frame": true,
				"image_to_video_first_last_frame": true,
				"multi_modal_reference_video": true,
				"video_edit": true,
				"video_extend": true,
				"video_bridge": true,
			},
			DefaultParams: Params{Ratio: "9:16", DurationSec: 6, Resolution: "1080p"},
			MaxPromptChars: 5000,
		}, true
	default:
		return ModelPromptProfile{}, false
	}
}
```

- [ ] **Step 3: Implement compiler**

In `prompt_compiler.go`, implement:

```go
package renderplan

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type PromptCompiler struct{}

func NewPromptCompiler() PromptCompiler {
	return PromptCompiler{}
}

func (PromptCompiler) Compile(_ context.Context, input UpsertInput) (CompileResult, error) {
	profile, ok := ProfileByID(input.ModelPromptProfile)
	if !ok {
		return CompileResult{}, fmt.Errorf("unknown model_prompt_profile %q", input.ModelPromptProfile)
	}
	if !profile.AllowedOperations[input.Operation] {
		return CompileResult{}, fmt.Errorf("operation %q is not allowed for %s", input.Operation, profile.ID)
	}
	if profile.ID == ProfileSeedance2Video && strings.TrimSpace(input.PromptParts.Action) == "" && len(input.PromptParts.Sequence) == 0 {
		return CompileResult{}, fmt.Errorf("seedance_2_video requires action 或 sequence")
	}
	prompt := compilePromptParts(input.PromptParts)
	if len([]rune(prompt)) > profile.MaxPromptChars {
		return CompileResult{}, fmt.Errorf("compiled prompt exceeds profile budget")
	}
	request := map[string]any{
		"profile": input.ModelPromptProfile,
		"operation": input.Operation,
		"params": input.Params,
		"reference_bindings": input.ReferenceBindings,
	}
	audit := map[string]any{
		"profile": profile.ID,
		"operation": input.Operation,
		"prompt_chars": len([]rune(prompt)),
	}
	requestJSON, _ := json.Marshal(request)
	auditJSON, _ := json.Marshal(audit)
	costJSON, _ := json.Marshal(map[string]any{"estimate": "not_charged_until_provider_submit"})
	return CompileResult{
		CompiledPrompt: prompt,
		CompiledRequest: requestJSON,
		PromptAudit: auditJSON,
		CostEstimate: costJSON,
	}, nil
}

func compilePromptParts(parts PromptParts) string {
	lines := []string{}
	add := func(label, value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			lines = append(lines, label+"："+value)
		}
	}
	add("目标", parts.Objective)
	add("主体", parts.Subject)
	add("场景", parts.Setting)
	add("动作", parts.Action)
	add("镜头", parts.Camera)
	add("构图", parts.Composition)
	add("风格", parts.Style)
	add("光影", parts.Lighting)
	if len(parts.Sequence) > 0 {
		lines = append(lines, "事件顺序："+strings.Join(parts.Sequence, "；"))
	}
	add("台词", parts.Dialogue)
	add("旁白", parts.Narration)
	add("音频", parts.Audio)
	add("文字", parts.TextRendering)
	if len(parts.QualityPack) > 0 {
		lines = append(lines, "质量要求："+strings.Join(parts.QualityPack, "；"))
	}
	if len(parts.ConstraintPack) > 0 {
		lines = append(lines, "约束："+strings.Join(parts.ConstraintPack, "；"))
	}
	if len(parts.NegativeHints) > 0 {
		lines = append(lines, "避免："+strings.Join(parts.NegativeHints, "；"))
	}
	return strings.Join(lines, "\n")
}
```

- [ ] **Step 4: Run compiler tests**

Run:

```bash
go test ./internal/agent/renderplan -run 'TestCompile|TestProfile' -count=1
```

Expected: PASS.

## Task 5: Craftsman Native Tools

**Files:**

- Create: `apps/server/internal/agent/tools/read_project_memory.go`
- Create: `apps/server/internal/agent/tools/upsert_render_plan.go`
- Create: `apps/server/internal/agent/tools/render_plan_tools_test.go`

- [ ] **Step 1: Write tool schema tests**

Create `apps/server/internal/agent/tools/render_plan_tools_test.go`:

```go
func TestRenderPlanToolInfosUseTypedSchemasAndChineseDescriptions(t *testing.T) {
	renderTool := NewUpsertRenderPlanNativeTool(nil)
	info, err := renderTool.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "upsert_render_plan" {
		t.Fatalf("name = %q", info.Name)
	}
	if !strings.Contains(info.Desc, "RenderPlan") || !strings.Contains(info.Desc, "Seedream") {
		t.Fatalf("description not specific enough: %s", info.Desc)
	}
	raw, _ := json.Marshal(info.ParamsOneOf)
	for _, want := range []string{"model_prompt_profile", "reference_bindings", "prompt_parts", "jsonschema_description"} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("schema missing %s: %s", want, string(raw))
		}
	}
}

func TestUpsertRenderPlanToolReturnsNaturalValidationError(t *testing.T) {
	tool := NewUpsertRenderPlanNativeTool(nil)
	got, err := tool.InvokableRun(context.Background(), `{"mode":"create"}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "工具调用失败") || !strings.Contains(got, "brief") {
		t.Fatalf("result = %s", got)
	}
}
```

- [ ] **Step 2: Implement `read_project_memory`**

Create `apps/server/internal/agent/tools/read_project_memory.go` with a native tool that:

- uses `toolInfoFor[ReadProjectMemoryInput]`.
- reads current active ProjectMemory from the M1 creative service or a small interface.
- returns natural-language summary: version, core intent, soul, non-negotiables count, visual anchors count, prompt hints if requested.
- returns `NaturalToolError` for missing runtime context or missing service.

Input struct:

```go
type ReadProjectMemoryToolInput struct {
	Brief                  string `json:"brief" jsonschema:"required" jsonschema_description:"读取 ProjectMemory 的目的，例如为分镜视频计划注入商品外观和机场商务氛围约束。不要超过 160 个中文字符。"`
	IncludePromptHints     bool   `json:"include_prompt_hints" jsonschema_description:"是否包含 prompt_injection_hints。Craftsman 写 RenderPlan 时通常需要 true。"`
	IncludeSourceRefs      bool   `json:"include_source_refs" jsonschema_description:"是否包含 memory 来源引用。只有需要解释约束来源时填写 true。"`
	IncludePreviousVersion bool   `json:"include_previous_version" jsonschema_description:"是否包含上一版本摘要。默认 false；只有处理 memory 变化导致的重做时使用。"`
}
```

- [ ] **Step 3: Implement `upsert_render_plan`**

Create `apps/server/internal/agent/tools/upsert_render_plan.go` with:

- `UpsertRenderPlanNativeTool`.
- `UpsertRenderPlanToolInput` matching the M2 spec.
- conversion functions from tool input to `renderplan.UpsertInput`.
- validation for `brief`, `mode`, `scope`, `target_phase`, `model_prompt_profile`, `operation`, `rationale`.
- natural success, blocked, and error responses.

The tool should call:

```go
out, err := t.service.Upsert(ctx, renderplan.UpsertInput{
	WorkspaceID: runtime.WorkspaceID,
	ThreadID: runtime.ThreadID,
	TaskID: runtime.TaskID,
	Brief: input.Brief,
	Mode: input.Mode,
	Scope: renderplan.Scope{Type: input.Scope.Type, ID: scopeID},
	TargetPhase: input.TargetPhase,
	TaskType: input.TaskType,
	ModelPromptProfile: input.ModelPromptProfile,
	Operation: input.Operation,
	ReferenceBindings: toRenderPlanReferenceBindings(input.ReferenceBindings),
	SubjectBindings: toRenderPlanSubjectBindings(input.SubjectBindings),
	PromptParts: toRenderPlanPromptParts(input.PromptParts),
	Params: toRenderPlanParams(input.Params),
	AuditHints: toRenderPlanAuditHints(input.AuditHints),
	Blocker: toRenderPlanBlocker(input.Blocker),
	Rationale: input.Rationale,
	AutoCompileAndSubmit: true,
})
```

- [ ] **Step 4: Run tool tests**

Run:

```bash
go test ./internal/agent/tools -run 'TestRenderPlan|TestUpsertRenderPlan|TestReadProjectMemory' -count=1
```

Expected: PASS.

## Task 6: Craftsman Bounded ReAct Graph

**Files:**

- Create: `apps/server/internal/agent/craftsman/system_prompt.go`
- Create: `apps/server/internal/agent/craftsman/native_tool_middleware.go`
- Modify: `apps/server/internal/agent/craftsman/types.go`
- Modify: `apps/server/internal/agent/craftsman/context_loader.go`
- Modify: `apps/server/internal/agent/craftsman/model_responder.go`
- Modify: `apps/server/internal/agent/craftsman/graph.go`
- Modify tests in `apps/server/internal/agent/craftsman/`

- [ ] **Step 1: Add Craftsman prompt tests**

In `apps/server/internal/agent/craftsman/model_responder_test.go`, add:

```go
func TestCraftsmanSystemPromptContainsM2Boundaries(t *testing.T) {
	prompt := CraftsmanSystemPrompt(Context{})
	for _, want := range []string{
		"Craftsman / RenderPlanner",
		"ProjectMemory 是项目创作宪法",
		"upsert_render_plan",
		"不修改 CreativeBrief",
		"不要手写 @图片1",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}
```

- [ ] **Step 2: Add `system_prompt.go`**

Create `apps/server/internal/agent/craftsman/system_prompt.go` using the Craftsman M2 System Prompt from `docs/superpowers/specs/2026-06-25-m2-craftsman-renderplan-agent-contract-design.md`.

Expose:

```go
func CraftsmanSystemPrompt(c Context) string {
	prompt := strings.TrimSpace(craftsmanSystemPromptTemplate)
	if contextText := strings.TrimSpace(c.Text); contextText != "" {
		prompt += "\n\n当前 Craftsman Context:\n" + contextText
	}
	return prompt
}
```

- [ ] **Step 3: Extend Craftsman types**

In `apps/server/internal/agent/craftsman/types.go`, add:

```go
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type SameTurnMessage struct {
	Role      string
	Content   string
	ToolCalls []ToolCall
	ToolCallID string
}

type TurnOutput struct {
	Content   string
	ToolCalls []ToolCall
	Metadata  map[string]any
}

type GraphState struct {
	Context          Context
	SameTurnMessages []SameTurnMessage
	ToolCallCount   int
	MaxToolCalls     int
}
```

- [ ] **Step 4: Replace fixed JSON responder**

In `model_responder.go`, keep `ParseStrategy` only for old tests if still referenced, but add a tool-calling responder path:

```go
type ModelResponder interface {
	Respond(ctx context.Context, context Context, sameTurn []SameTurnMessage, toolInfos []*schema.ToolInfo) (TurnOutput, error)
}
```

Implement `VolcengineModelResponder.Respond` similar to Producer responder:

- build messages from `CraftsmanSystemPrompt`.
- bind `toolInfos` with `WithTools`.
- stream response.
- collect native tool calls from model output.
- return `TurnOutput`.

- [ ] **Step 5: Replace graph with bounded tool loop**

In `graph.go`, build:

```text
START -> load_scope_context -> prepare_turn_state -> call_model
call_model(tool_calls) -> prepare_tool_message -> execute_tools -> append_tool_results -> call_model
call_model(final) -> finalize_task -> END
```

Use Eino `compose.ToolsNode` for `execute_tools`. Set max tool calls to 8. Inject `tools.NativeRuntimeContext` with workspace/thread/task/tool call id before invoking each tool.

- [ ] **Step 6: Run Craftsman tests**

Run:

```bash
go test ./internal/agent/craftsman -count=1
```

Expected: PASS.

## Task 7: Producer Dispatch and Artifact Selection Tools

**Files:**

- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
- Modify: `apps/server/internal/agent/tools/select_version.go`
- Add or modify: `apps/server/internal/agent/tools/select_version_test.go`

- [ ] **Step 1: Write dispatch tests**

Add tests:

```go
func TestDispatchCraftsmanSupportsKeyElementStateReferenceImage(t *testing.T) {}
func TestDispatchCraftsmanRejectsShotReferenceImage(t *testing.T) {}
func TestDispatchCraftsmanCreatesRenderPlanScopedTaskInput(t *testing.T) {}
```

Assert:

- key_element_state + reference_image creates `agent_task(role='craftsman', scope_type='key_element_state')`.
- shot + reference_image returns natural validation failure.
- task input contains `target_phase`, `mode`, `producer_thread_id`, `producer_task_id`.

- [ ] **Step 2: Update dispatch schema**

Replace legacy `shot_refs`-only schema with typed input:

```go
type DispatchCraftsmanInput struct {
	Brief       string         `json:"brief" jsonschema:"required" jsonschema_description:"派发 Craftsman 的业务目的，例如为机场场景状态创建 reference image RenderPlan。不要超过 160 个中文字符。"`
	Mode        string         `json:"mode" jsonschema:"required,enum=create,enum=revise,enum=repair" jsonschema_description:"create 创建第一版计划；revise 因用户修改或计划问题修订；repair 针对已生成结果的问题创建修复计划。"`
	Scope       DispatchScope  `json:"scope" jsonschema:"required" jsonschema_description:"派发范围。key_element_state 用于关键元素参考图；shot 用于分镜图或分镜视频；render_plan 用于修订已有计划。"`
	TargetPhase string         `json:"target_phase" jsonschema:"required,enum=reference_image,enum=preview_image,enum=shot_video" jsonschema_description:"目标生产阶段。reference_image 生成关键元素参考图；preview_image 生成分镜预览图；shot_video 生成分镜视频。"`
	Priority    string         `json:"priority" jsonschema:"enum=low,enum=normal,enum=high" jsonschema_description:"任务优先级。默认 normal。"`
	Inputs      DispatchInputs `json:"inputs" jsonschema_description:"Producer 已知的输入线索。只传对象 ID 或 client_key，不要传 provider prompt。"`
	Repair      RepairContext  `json:"repair" jsonschema_description:"mode=repair 时填写，说明问题来源、修复目标和用户要求。"`
	Reason      string         `json:"reason" jsonschema:"required" jsonschema_description:"为什么现在需要 Craftsman 处理。必须说明业务原因或依赖关系。"`
}
```

Keep `generate_shot_video` as a compatibility wrapper only if existing code still uses it; Producer M2 prompt should prefer `dispatch_craftsman`.

- [ ] **Step 3: Update thread/runtime support**

Add runtime methods if missing:

```go
GetOrCreateCraftsmanThread(ctx context.Context, workspaceID, scopeID pgtype.UUID, scopeType string) (db.AgentThread, error)
```

If current runtime only supports shot scope, implement a general helper that creates `agent_thread(role='craftsman', scope_type=<scope>, scope_id=<id>)`.

- [ ] **Step 4: Extend artifact selection**

Update `select_version.go` so `target_type=key_element_state`:

- validates artifact version succeeded.
- updates `key_element_state.reference_version_id`.
- sets `reference_status=ready` or `approved` based on `mark_approved`.
- writes `agent_event(event_type='key_element_state_reference_selected')`.

- [ ] **Step 5: Run tool tests**

Run:

```bash
go test ./internal/agent/tools -run 'TestDispatchCraftsman|TestSelect' -count=1
```

Expected: PASS.

## Task 8: RenderPlan Worker Execution and Reference Binding

**Files:**

- Modify: `apps/server/internal/agent/worker/types.go`
- Modify: `apps/server/internal/agent/worker/executor.go`
- Modify: `apps/server/internal/agent/worker/input_refs.go`
- Modify: `apps/server/internal/agent/worker/executor_test.go`
- Modify: `apps/server/internal/production/intent.go`

- [ ] **Step 1: Extend production InputRef**

In `apps/server/internal/production/intent.go`, extend `InputRef`:

```go
type InputRef struct {
	NodeID         pgtype.UUID `json:"node_id"`
	NodeType       string      `json:"node_type"`
	AssetID        pgtype.UUID `json:"asset_id"`
	VersionID      pgtype.UUID `json:"version_id"`
	StorageURL     string      `json:"storage_url"`
	Label          string      `json:"label"`
	Role           string      `json:"role,omitempty"`
	PromptAlias    string      `json:"prompt_alias,omitempty"`
	SemanticTarget string      `json:"semantic_target,omitempty"`
	Priority       int         `json:"priority,omitempty"`
	Required       bool        `json:"required,omitempty"`
}
```

Update JSON shape tests in `apps/server/internal/production/service_test.go` to assert these fields serialize when present.

- [ ] **Step 2: Extend worker input**

In `apps/server/internal/agent/worker/types.go`, add:

```go
RenderPlanID       string                     `json:"render_plan_id,omitempty"`
CompiledPrompt     string                     `json:"compiled_prompt,omitempty"`
ReferenceBindings  []RenderPlanReferenceInput `json:"reference_bindings,omitempty"`

type RenderPlanReferenceInput struct {
	SourceType      string `json:"source_type"`
	SourceID        string `json:"source_id"`
	Role            string `json:"role"`
	PromptAlias     string `json:"prompt_alias,omitempty"`
	SemanticTarget  string `json:"semantic_target,omitempty"`
	Priority        int    `json:"priority,omitempty"`
	Required        bool   `json:"required,omitempty"`
}
```

- [ ] **Step 3: Resolve RenderPlan references**

In `worker/input_refs.go`, add:

```go
func ResolveRenderPlanReferences(ctx context.Context, store inputRefStore, workspaceID pgtype.UUID, refs []RenderPlanReferenceInput) ([]production.InputRef, error)
```

Behavior:

- `source_type=artifact_version`: load artifact version and owning node.
- `source_type=media_node`: load node and current version or asset.
- `source_type=key_element_state`: load state, then use `reference_version_id` or `reference_node_id`.
- preserve `role`, `prompt_alias`, `semantic_target`, `priority`, `required`.
- return explicit error if `required=true` and source has no usable artifact.

- [ ] **Step 4: Submit generation intent from RenderPlan**

In `worker/executor.go`, if `input.RenderPlanID` is present:

- use `input.CompiledPrompt` as `GenerationIntent.Prompt`.
- use RenderPlan reference refs before old `InputNodeRefs`.
- set `RequestedBy.Type='agent_render_plan_worker'`.
- after provider success, call renderplan service or store to mark completed.
- if RenderPlan scope is `key_element_state` and target phase is `reference_image`, update state `reference_status='ready'` and `reference_version_id=<version>`.

- [ ] **Step 5: Run worker tests**

Run:

```bash
go test ./internal/agent/worker -count=1
```

Expected: PASS.

## Task 9: Volcengine Provider Role Support

**Files:**

- Modify: `apps/server/internal/production/volcengine_image.go`
- Modify: `apps/server/internal/production/volcengine_image_test.go`
- Modify: `apps/server/internal/production/volcengine_video.go`
- Modify: `apps/server/internal/production/volcengine_video_test.go`
- Modify: `apps/server/internal/production/capability.go`

- [ ] **Step 1: Add image provider tests**

In `volcengine_image_test.go`, add a test that an image intent with two input refs preserves:

- `role=product_reference`
- `role=scene_reference`
- prompt aliases in provider request summary.

- [ ] **Step 2: Add video provider tests**

In `volcengine_video_test.go`, add tests:

```go
func TestVideoTaskRequestIncludesFirstFrameAndReferenceRoles(t *testing.T) {}
func TestVideoTaskRequestIncludesReturnLastFrameParam(t *testing.T) {}
func TestVideoTaskRequestRejectsMissingRequiredReferenceURL(t *testing.T) {}
```

- [ ] **Step 3: Update provider request summaries**

In `volcengine_image.go` and `volcengine_video.go`, ensure `ProviderRequest` includes:

```go
"input_refs": []map[string]any{
	{
		"node_id": "...",
		"version_id": "...",
		"role": "first_frame",
		"prompt_alias": "图片1",
		"semantic_target": "机场场景",
		"required": true,
	},
}
```

Do not block M2 on perfect official SDK mapping for every Seedance role. The minimal acceptance is that role metadata reaches provider request summary and actual reachable URLs are placed into the content request in deterministic order by priority.

- [ ] **Step 4: Run production tests**

Run:

```bash
go test ./internal/production -run 'TestVolcengine|TestVideoTask|TestImage' -count=1
```

Expected: PASS.

## Task 10: PSS, Canvas Projection, Wiring, and E2E

**Files:**

- Modify: `apps/server/internal/agent/pss/producer.go`
- Modify: `apps/server/internal/agent/pss/producer_test.go`
- Modify: `apps/server/internal/api/domain_canvas_projection.go`
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/components/canvas-flow/DomainFlowNode.tsx`
- Modify: `apps/web/src/components/canvas-flow/DomainFlowEdge.tsx`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/cmd/server/e2e_producer_fixture.go`
- Create: `scripts/smoke-m2-craftsman-renderplan-e2e.sh`

- [ ] **Step 1: Add PSS tests**

In `producer_test.go`, add:

```go
func TestProducerPSSListsRenderPlansAndReferenceBindings(t *testing.T) {}
func TestProducerPSSListsKeyElementStateReferenceReady(t *testing.T) {}
```

Assert text contains `RenderPlan`, `reference_image`, `seedream_5_image`, `reference_status=ready`.

- [ ] **Step 2: Add canvas projection tests**

Add backend projection assertion that domain projection contains:

- `key_element_state`
- `render_plan`
- generated image media node
- `references` edge from RenderPlan to KeyElementState
- `renders_to` edge from RenderPlan to generated image

- [ ] **Step 3: Wire services in main**

In `apps/server/cmd/server/main.go`:

- instantiate `renderplan.NewPromptCompiler()`.
- instantiate `renderplan.NewService(queries, compiler)`.
- add `NewReadProjectMemoryNativeTool(...)`.
- add `NewUpsertRenderPlanNativeTool(renderPlanService)` to Craftsman registry.
- update Producer tool registration for M2 `dispatch_craftsman` and `select_artifact_version`.
- ensure Craftsman graph receives native registry and ToolNode.

- [ ] **Step 4: Add deterministic M2 fixture**

In `e2e_producer_fixture.go`, add `CLIPANVIL_E2E_PRODUCER_FIXTURE=m2_reference_image` that makes Producer call:

1. `read_project_context`
2. `upsert_project_brief`
3. `update_project_memory`
4. `upsert_key_elements` for luggage and airport state
5. `dispatch_craftsman` for airport state reference image

Add a Craftsman fixture responder or static tool-call responder that calls `upsert_render_plan` with the Yuexing airport reference image plan from the M2 spec.

- [ ] **Step 5: Add smoke script**

Create `scripts/smoke-m2-craftsman-renderplan-e2e.sh` with commands:

```bash
#!/usr/bin/env bash
set -euo pipefail

MODE="${1:-verify}"
STATE_FILE="${CLIPANVIL_M2_E2E_STATE_FILE:-/tmp/clipanvil-m2-craftsman-renderplan-e2e.json}"

case "$MODE" in
  setup)
    curl -fsS "$CLIPANVIL_WEB_BASE_URL/api/auth/register" \
      -H 'content-type: application/json' \
      -d '{"email":"m2-e2e@example.com","password":"Password123!","name":"M2 E2E"}' > "$STATE_FILE"
    ;;
  verify)
    test -f "$STATE_FILE"
    psql "$CLIPANVIL_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
SELECT COUNT(*) AS render_plan_count FROM render_plan WHERE target_phase = 'reference_image';
SELECT COUNT(*) AS ready_reference_count FROM key_element_state WHERE reference_status IN ('ready','approved');
SELECT COUNT(*) AS worker_jobs FROM generation_job WHERE requested_by_type IN ('agent_worker','agent_render_plan_worker');
SQL
    ;;
  *)
    echo "usage: $0 setup|verify" >&2
    exit 2
    ;;
esac
```

Adjust endpoint payloads to match existing auth/workspace helper patterns when implementing.

- [ ] **Step 6: Run full verification**

Run:

```bash
make sqlc-generate
make migrate-up
make server-test
make server-build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all PASS.

- [ ] **Step 7: Run E2E**

Start dev server with fixture:

```bash
CLIPANVIL_E2E_PRODUCER_FIXTURE=m2_reference_image ./scripts/dev-start.sh
```

Use the printed Vite URL. Run the smoke setup, operate the browser to send the Yuexing luggage airport reference prompt, then verify:

```bash
scripts/smoke-m2-craftsman-renderplan-e2e.sh setup
scripts/smoke-m2-craftsman-renderplan-e2e.sh verify
```

Acceptance:

- Browser shows Agent conversation completed.
- Canvas shows KeyElementState, RenderPlan, and generated image / reference binding projection.
- DB has one `render_plan(target_phase='reference_image')`.
- DB has airport `key_element_state.reference_status IN ('ready','approved')`.
- DB has linked `generation_job` and `artifact_version`.

## Final Verification Checklist

Run before claiming M2 implementation complete:

```bash
make sqlc-generate
make migrate-up
make server-test
make server-build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Run M2 E2E:

```bash
CLIPANVIL_E2E_PRODUCER_FIXTURE=m2_reference_image ./scripts/dev-start.sh
scripts/smoke-m2-craftsman-renderplan-e2e.sh setup
scripts/smoke-m2-craftsman-renderplan-e2e.sh verify
```

## Self-Review Notes

- Spec coverage: this plan maps Producer M2 dispatch, Craftsman bounded ReAct, `upsert_render_plan`, PromptCompiler, reference image binding, Worker execution, provider role metadata, PSS, canvas projection, and E2E acceptance to explicit tasks.
- Producer prompt M1 leftovers are covered in Task 1.
- `compile_render_plan`, `submit_render_plan`, `schedule_ready_render_plans`, and `approve_render_plan_execution` remain non-Agent engineering steps.
- `reference_bundle` remains out of scope; reference semantics are represented through `render_plan.reference_bindings`.
- Reviewer 10-axis rubric and repair loop remain M3 scope.
