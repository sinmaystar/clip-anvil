# M3 Reviewer Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build M3 Reviewer Gate so Producer can dispatch pre-render and post-render reviews, Reviewer can submit 10-axis structured results through Eino-native tools, issues are persisted and projected to canvas, and Producer keeps repair decisions.

**Architecture:** Business DB remains the source of truth. Producer is the full orchestrator; Craftsman creates and repairs RenderPlan; Reviewer is a bounded ReAct quality gate with read tools plus one write tool. Reviewer never mutates ShotPlan, RenderPlan, ProjectMemory, or generation jobs directly; it submits review records and issues that Producer reads before deciding whether to accept, repair, ask the user, or stop retrying.

**Tech Stack:** Go 1.26, PostgreSQL 16, pgx v5, sqlc, CloudWeGo Eino `compose.Graph` / `compose.ToolsNode`, `components/tool/utils.GoStruct2ParamsOneOf`, Hertz, Volcengine Seedream/Seedance, React 19 + TypeScript 6 + `@xyflow/react`.

---

## Source Documents

- M3 spec: `docs/superpowers/specs/2026-06-25-m3-reviewer-gate-agent-contract-design.md`
- M2 plan: `docs/superpowers/plans/2026-06-25-m2-craftsman-renderplan-implementation.md`
- Producer prompt code: `apps/server/internal/agent/producer/system_prompt.go`
- Craftsman prompt code: `apps/server/internal/agent/craftsman/system_prompt.go`
- Seedance prompt review skill: `/Users/wanwan/Desktop/seedance_seedream/seedance_pe_SKILL.md`

## Current Prompt Review Findings

Producer prompt has stage wording that must be cleaned for M3:

- `apps/server/internal/agent/producer/system_prompt.go` says `在 M2 调度 Craftsman...`
- It says `然后在 M2 调用 dispatch_craftsman...`
- It labels available generation tools as `M2 可用生成调度工具`.
- It has a whole section named `## M2 生成调度能力`.
- `apps/server/internal/agent/producer/model_responder_test.go` has a test named `TestProducerSystemPromptEnablesM2DispatchAndRemovesM1OnlyRules` and requires `M2 生成调度能力`.

These are no longer correct once M3 begins. The prompt should describe current production capability, not milestone labels.

Craftsman prompt has no direct `M1` / `M2` wording, but it lacks M3 repair context:

- It does not explain how to consume Reviewer `artifact_issue` / `retry_recommendation`.
- It does not explicitly say repair work should use `mode=fork_from` when modifying an already submitted RenderPlan.
- It does not mention pre-render review risks or Reviewer feedback as a first-class input.

M3 implementation must update both prompts and their tests before adding Reviewer behavior. Otherwise the agents will carry stale stage boundaries into production decisions.

## Scope

M3 includes:

- Producer prompt cleanup from milestone-specific wording to current capability wording.
- Craftsman prompt update for Reviewer-driven repair.
- Producer native tool `dispatch_reviewer`.
- Reviewer 10-axis rubric and task-specific required-axis validation.
- Review task types:
  - `pre_render_plan_review`
  - `preview_image_review`
  - `shot_video_review`
  - `final_video_review`
- Reviewer tools:
  - `read_project_context`
  - `read_project_memory`
  - `submit_review_result`
- Reviewer Eino-native bounded ReAct graph using `compose.ToolsNode`.
- `review_record` schema extension for M3 targets, verdicts, required axes, retry recommendation, and escalation.
- New `artifact_issue` table and sqlc queries.
- Canvas projection for `review_record`, `artifact_issue`, and `suggests_fix` edges.
- E2E coverage for Yuexing luggage: bad shot video is reviewed, rejected, issue is persisted, Producer dispatches Craftsman repair, and database state is correct.

M3 excludes:

- Reviewer directly dispatching Craftsman.
- Reviewer directly selecting artifact winner.
- Reviewer directly modifying ProjectMemory, Shot, RenderPlan, generation_job, or artifact_version.
- A full manual issue editor in the frontend.
- Commercial-grade final timeline composition review beyond a basic `final_video_review` schema path.

## File Structure

Create:

- `apps/server/migrations/026_m3_reviewer_gate.sql`: review schema extension and `artifact_issue` table.
- `apps/server/sqlc/queries/artifact_issue.sql`: create/list/update issue queries.
- `apps/server/internal/agent/reviewer/system_prompt.go`: Reviewer Chinese system prompt.
- `apps/server/internal/agent/reviewer/native_tool_loop.go`: Reviewer bounded ReAct graph.
- `apps/server/internal/agent/reviewer/native_tool_middleware.go`: reviewer task context injection into native tool calls.
- `apps/server/internal/agent/reviewer/tools.go`: Reviewer native tool registry construction.
- `apps/server/internal/agent/reviewer/system_prompt_test.go`: Reviewer prompt assertions.
- `apps/server/internal/agent/reviewer/native_tool_loop_test.go`: graph structure and tool loop tests.
- `apps/server/internal/agent/tools/dispatch_reviewer.go`: Producer native dispatch tool.
- `apps/server/internal/agent/tools/submit_review_result.go`: Reviewer write tool.
- `apps/server/internal/agent/tools/reviewer_context_tools.go`: Reviewer-scoped read tools that enforce review target limits.
- `apps/server/internal/agent/tools/reviewer_tools_test.go`: native schema, validation, and natural result tests.
- `scripts/smoke-m3-reviewer-gate-e2e.sh`: browser/API plus DB E2E smoke script.

Modify:

- `apps/server/internal/agent/producer/system_prompt.go`: remove stale milestone labels and add Reviewer Gate rules.
- `apps/server/internal/agent/producer/model_responder_test.go`: update prompt tests from M2 wording to M3/current capability wording.
- `apps/server/internal/agent/craftsman/system_prompt.go`: add Reviewer repair context.
- `apps/server/internal/agent/craftsman/model_responder_test.go`: add prompt assertions for repair and forbidden direct user contact.
- `apps/server/internal/agent/reviewer/types.go`: add review task, target, verdict, issue, and retry recommendation types.
- `apps/server/internal/agent/reviewer/rubric.go`: replace 7 old axes with M3 10-axis validation.
- `apps/server/internal/agent/reviewer/model_responder.go`: move to tool-calling responder using Reviewer system prompt.
- `apps/server/internal/agent/reviewer/context_loader.go`: support render_plan target and final video target in addition to shot artifact targets.
- `apps/server/internal/agent/reviewer/graph.go`: replace fixed `load_review_context -> review_artifact` graph with bounded native tool loop.
- `apps/server/internal/agent/reviewer/executor.go`: parse M3 review task input, use graph name `reviewer_gate`, and stop assuming `preview_image`.
- `apps/server/internal/agent/reviewer/*_test.go`: update old accepted/select-version/retry tests to new gate behavior.
- `apps/server/internal/agent/runtime/service.go`: add reviewer thread creation for `render_plan` and `final_video` scopes, and keep reviewer task list recovery compatible.
- `apps/server/cmd/server/main.go`: wire Reviewer native tool registry, remove Reviewer retry dispatcher, register Producer `dispatch_reviewer`.
- `apps/server/internal/agent/pss/producer.go`: include recent review records and open artifact issues.
- `apps/server/internal/api/domain_canvas_projection.go`: project review and issue nodes/edges.
- `apps/web/src/components/canvas-flow/DomainFlowNode.tsx`: render review and issue nodes.
- `apps/web/src/components/canvas-flow/DomainFlowEdge.tsx`: render review/flag/suggests_fix edges.
- `apps/web/src/components/canvas-flow/flowTypes.ts`: add review and issue node/edge payload types.

## Task 1: Prompt Cleanup and M3 Prompt Guardrails

**Files:**

- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`
- Modify: `apps/server/internal/agent/craftsman/system_prompt.go`
- Modify: `apps/server/internal/agent/craftsman/model_responder_test.go`

- [ ] **Step 1: Add Producer prompt regression test**

Replace `TestProducerSystemPromptEnablesM2DispatchAndRemovesM1OnlyRules` in `apps/server/internal/agent/producer/model_responder_test.go` with:

```go
func TestProducerSystemPromptEnablesCurrentGenerationAndReviewerGate(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, forbidden := range []string{
		"M1 阶段只记录需求",
		"M1 可用工具",
		"M1 阶段不要调度 Craftsman",
		"在 M2 调度",
		"然后在 M2 调用",
		"M2 可用生成调度工具",
		"## M2 生成调度能力",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt still contains milestone-only wording %q", forbidden)
		}
	}
	for _, required := range []string{
		"当前生成调度能力",
		"dispatch_craftsman",
		"dispatch_reviewer",
		"Reviewer 是质量 gate",
		"Reviewer 不直接重跑生成",
		"不要寻找或虚构 compile_render_plan",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt missing current capability wording %q", required)
		}
	}
}
```

- [ ] **Step 2: Add Craftsman repair prompt test**

Add this test to `apps/server/internal/agent/craftsman/model_responder_test.go`:

```go
func TestCraftsmanSystemPromptIncludesReviewerRepairRules(t *testing.T) {
	prompt := SystemPrompt()
	for _, required := range []string{
		"Reviewer",
		"artifact_issue",
		"retry_recommendation",
		"mode=fork_from",
		"不要直接问用户",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("craftsman prompt missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"M1 阶段",
		"M2 阶段",
		"TODO",
		"TBD",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("craftsman prompt contains stale placeholder wording %q", forbidden)
		}
	}
}
```

- [ ] **Step 3: Run prompt tests and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/producer ./apps/server/internal/agent/craftsman -run 'TestProducerSystemPromptEnablesCurrentGenerationAndReviewerGate|TestCraftsmanSystemPromptIncludesReviewerRepairRules' -count=1
```

Expected: FAIL because Producer still contains M2 wording and Craftsman lacks Reviewer repair rules.

- [ ] **Step 4: Update Producer system prompt**

In `apps/server/internal/agent/producer/system_prompt.go`, make these textual replacements:

```text
在 M2 调度 Craftsman 创建 RenderPlan，绑定参考图，发起分镜预览图和分镜视频生成，并在关键节点向用户请求决策。
```

becomes:

```text
调度 Craftsman 创建或修复 RenderPlan，绑定参考图，发起分镜预览图和分镜视频生成；调度 Reviewer 评审生成计划和生成结果；在关键节点向用户请求决策。
```

```text
然后在 M2 调用 dispatch_craftsman(scope.type=key_element_state, target_phase=reference_image) 派 Craftsman 生成统一参考图。
```

becomes:

```text
然后调用 dispatch_craftsman(scope.type=key_element_state, target_phase=reference_image) 派 Craftsman 生成统一参考图。
```

```text
- M2 可用生成调度工具：dispatch_craftsman、select_artifact_version、request_user_decision。
```

becomes:

```text
- 当前生成调度工具：dispatch_craftsman、dispatch_reviewer、select_artifact_version、request_user_decision。
```

Rename section:

```text
## M2 生成调度能力
```

to:

```text
## 当前生成调度能力
```

Append this paragraph in that section:

```text
Reviewer 是质量 gate。你可以使用 dispatch_reviewer 评审 RenderPlan、preview image、shot video 和 final video。Reviewer 只提交 review_record、artifact_issue 和 retry_recommendation，不直接修改 RenderPlan，不直接选择版本，也不直接重跑生成。你需要读取 Reviewer 结果后决定是否接受、请求用户确认、派 Craftsman repair，或停止自动重试。
```

- [ ] **Step 5: Update Craftsman system prompt**

In `apps/server/internal/agent/craftsman/system_prompt.go`, add this section before `输出要求`:

```text
Reviewer 驱动的修复：
- 如果 Producer 派发的是 repair / revise 任务，你需要读取 Reviewer 的 artifact_issue、rubric、critique 和 retry_recommendation。
- 修复已提交或已执行的 RenderPlan 时，优先使用 upsert_render_plan(mode=fork_from)，不要直接覆盖旧计划。
- issue 指向 RenderPlan 时，修复 prompt_parts、reference_bindings、subject_bindings、params 或 audit_hints。
- issue 指向 artifact_version 时，判断应该 regenerate、edit、extend、bridge 还是 mark_blocked，并在 rationale 里解释选择。
- 如果 Reviewer 指出同一问题多次失败，不要继续简单增强 prompt；应在 audit_hints.needs_user_decision 或 blocker 中说明需要 Producer 决策。
```

- [ ] **Step 6: Run prompt tests and prompt scan**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/producer ./apps/server/internal/agent/craftsman -run 'TestProducerSystemPromptEnablesCurrentGenerationAndReviewerGate|TestCraftsmanSystemPromptIncludesReviewerRepairRules' -count=1
rg -n "M1 阶段|M2 阶段|在 M2 调度|然后在 M2 调用|M2 可用生成调度工具|## M2 生成调度能力|TODO|TBD" apps/server/internal/agent/producer/system_prompt.go apps/server/internal/agent/craftsman/system_prompt.go apps/server/internal/agent/producer/model_responder_test.go apps/server/internal/agent/craftsman/model_responder_test.go
```

Expected: tests PASS. `rg` prints no matches.

## Task 2: Review Schema and sqlc

**Files:**

- Create: `apps/server/migrations/026_m3_reviewer_gate.sql`
- Create: `apps/server/sqlc/queries/artifact_issue.sql`
- Modify: `apps/server/sqlc/queries/review_record.sql`
- Modify generated: `apps/server/internal/store/db/*.go`

- [ ] **Step 1: Write migration**

Create `apps/server/migrations/026_m3_reviewer_gate.sql`:

```sql
-- +goose Up
ALTER TABLE review_record DROP CONSTRAINT review_record_phase_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_status_check;

ALTER TABLE review_record
    ADD COLUMN review_task TEXT NOT NULL DEFAULT 'preview_image_review',
    ADD COLUMN target_object_type TEXT NOT NULL DEFAULT 'artifact_version',
    ADD COLUMN target_object_id UUID,
    ADD COLUMN render_plan_id UUID REFERENCES render_plan(id) ON DELETE SET NULL,
    ADD COLUMN required_axes JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN escalation JSONB NOT NULL DEFAULT '{}';

UPDATE review_record
SET review_task = CASE target_phase
    WHEN 'shot_video' THEN 'shot_video_review'
    WHEN 'final_video' THEN 'final_video_review'
    ELSE 'preview_image_review'
END,
target_object_type = 'artifact_version',
target_object_id = artifact_version_id;

ALTER TABLE review_record
    ADD CONSTRAINT review_record_phase_check CHECK (target_phase IN ('pre_render_plan', 'preview_image', 'shot_video', 'final_video')),
    ADD CONSTRAINT review_record_task_check CHECK (review_task IN ('pre_render_plan_review', 'preview_image_review', 'shot_video_review', 'final_video_review')),
    ADD CONSTRAINT review_record_target_type_check CHECK (target_object_type IN ('render_plan', 'artifact_version', 'shot', 'final_video')),
    ADD CONSTRAINT review_record_status_check CHECK (status IN ('running', 'accepted', 'accepted_with_warnings', 'rejected', 'blocked', 'failed'));

CREATE TABLE artifact_issue (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL REFERENCES workspace(id) ON DELETE CASCADE,
    review_record_id UUID NOT NULL REFERENCES review_record(id) ON DELETE CASCADE,
    dimension TEXT NOT NULL,
    severity TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    target_object_type TEXT NOT NULL,
    target_object_id UUID NOT NULL,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    evidence TEXT NOT NULL DEFAULT '',
    suggested_fix TEXT NOT NULL DEFAULT 'none',
    fix_hint TEXT NOT NULL DEFAULT '',
    requires_user_confirmation BOOLEAN NOT NULL DEFAULT false,
    superseded_by_issue_id UUID REFERENCES artifact_issue(id) ON DELETE SET NULL,
    resolved_by_review_record_id UUID REFERENCES review_record(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT artifact_issue_dimension_check CHECK (dimension IN (
        'faithfulness',
        'subject_consistency',
        'product_visibility',
        'brand_style_consistency',
        'composition_proportion',
        'motion_physics',
        'visual_quality',
        'continuity',
        'audio_sync',
        'platform_selling_power',
        'model_capability',
        'prompt_validity',
        'reference_role_validity',
        'cost_risk',
        'dependency_not_ready',
        'project_memory_conflict'
    )),
    CONSTRAINT artifact_issue_severity_check CHECK (severity IN ('info', 'warning', 'blocking')),
    CONSTRAINT artifact_issue_status_check CHECK (status IN ('open', 'resolved', 'superseded', 'accepted_risk')),
    CONSTRAINT artifact_issue_target_type_check CHECK (target_object_type IN ('render_plan', 'artifact_version', 'shot', 'final_video', 'project_memory')),
    CONSTRAINT artifact_issue_suggested_fix_check CHECK (suggested_fix IN ('none', 'regenerate', 'edit', 'extend', 'bridge', 'revise_render_plan', 'revise_shot_plan', 'manual'))
);

CREATE INDEX idx_artifact_issue_workspace_status ON artifact_issue(workspace_id, status, created_at DESC);
CREATE INDEX idx_artifact_issue_review_record ON artifact_issue(review_record_id);
CREATE INDEX idx_artifact_issue_target ON artifact_issue(target_object_type, target_object_id, status);
CREATE INDEX idx_artifact_issue_dimension ON artifact_issue(workspace_id, dimension, status);
CREATE INDEX idx_review_record_task_target ON review_record(workspace_id, review_task, target_object_type, target_object_id, created_at DESC);
CREATE INDEX idx_review_record_render_plan ON review_record(render_plan_id, created_at DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_review_record_render_plan;
DROP INDEX IF EXISTS idx_review_record_task_target;
DROP INDEX IF EXISTS idx_artifact_issue_dimension;
DROP INDEX IF EXISTS idx_artifact_issue_target;
DROP INDEX IF EXISTS idx_artifact_issue_review_record;
DROP INDEX IF EXISTS idx_artifact_issue_workspace_status;
DROP TABLE IF EXISTS artifact_issue;

ALTER TABLE review_record DROP CONSTRAINT review_record_phase_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_status_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_target_type_check;
ALTER TABLE review_record DROP CONSTRAINT review_record_task_check;

ALTER TABLE review_record
    DROP COLUMN escalation,
    DROP COLUMN required_axes,
    DROP COLUMN render_plan_id,
    DROP COLUMN target_object_id,
    DROP COLUMN target_object_type,
    DROP COLUMN review_task;

ALTER TABLE review_record
    ADD CONSTRAINT review_record_phase_check CHECK (target_phase IN ('preview_image', 'shot_video', 'final_video')),
    ADD CONSTRAINT review_record_status_check CHECK (status IN ('running', 'accepted', 'rejected', 'failed'));
```

- [ ] **Step 2: Update review_record queries**

Modify `apps/server/sqlc/queries/review_record.sql` so `CreateReviewRecord` inserts the new fields:

```sql
-- name: CreateReviewRecord :one
INSERT INTO review_record (
    workspace_id,
    shot_id,
    node_id,
    artifact_version_id,
    generation_job_id,
    reviewer_thread_id,
    reviewer_task_id,
    parent_review_record_id,
    target_phase,
    review_task,
    target_object_type,
    target_object_id,
    render_plan_id,
    status,
    attempt_no,
    max_attempts,
    model_provider,
    model_id,
    required_axes
) VALUES (
    $1, $2, $3, $4, $5,
    $6, $7, $8,
    $9, $10, $11, $12, $13,
    'running', $14, $15, $16, $17, $18
) RETURNING *;
```

Modify `CompleteReviewRecord`:

```sql
-- name: CompleteReviewRecord :one
UPDATE review_record
SET status = $2,
    overall_score = $3,
    rubric = $4,
    critique = $5,
    retry_recommendation = $6,
    escalation = $7,
    error_code = '',
    error_message = '',
    completed_at = now()
WHERE id = $1
RETURNING *;
```

Add:

```sql
-- name: ListReviewRecordsByTarget :many
SELECT *
FROM review_record
WHERE workspace_id = $1
  AND target_object_type = $2
  AND target_object_id = $3
ORDER BY created_at DESC
LIMIT $4;

-- name: ListReviewRecordsByRenderPlan :many
SELECT *
FROM review_record
WHERE render_plan_id = $1
ORDER BY created_at DESC;
```

- [ ] **Step 3: Add artifact issue queries**

Create `apps/server/sqlc/queries/artifact_issue.sql`:

```sql
-- name: CreateArtifactIssue :one
INSERT INTO artifact_issue (
    workspace_id,
    review_record_id,
    dimension,
    severity,
    status,
    target_object_type,
    target_object_id,
    title,
    description,
    evidence,
    suggested_fix,
    fix_hint,
    requires_user_confirmation
) VALUES (
    $1, $2, $3, $4, 'open',
    $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING *;

-- name: ListArtifactIssuesByWorkspace :many
SELECT *
FROM artifact_issue
WHERE workspace_id = $1
ORDER BY created_at DESC
LIMIT $2;

-- name: ListOpenArtifactIssuesByWorkspace :many
SELECT *
FROM artifact_issue
WHERE workspace_id = $1
  AND status = 'open'
ORDER BY created_at DESC
LIMIT $2;

-- name: ListArtifactIssuesByReviewRecord :many
SELECT *
FROM artifact_issue
WHERE review_record_id = $1
ORDER BY created_at ASC;

-- name: ListArtifactIssuesByTarget :many
SELECT *
FROM artifact_issue
WHERE target_object_type = $1
  AND target_object_id = $2
ORDER BY created_at DESC;

-- name: MarkArtifactIssueResolved :one
UPDATE artifact_issue
SET status = 'resolved',
    resolved_by_review_record_id = $2,
    updated_at = now()
WHERE id = $1
RETURNING *;
```

- [ ] **Step 4: Generate sqlc and run database tests**

Run:

```bash
make sqlc-generate
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/store/db -count=1
```

Expected: sqlc generation succeeds. Store package tests pass or package reports no test files.

## Task 3: Reviewer Types and 10-Axis Rubric

**Files:**

- Modify: `apps/server/internal/agent/reviewer/types.go`
- Modify: `apps/server/internal/agent/reviewer/rubric.go`
- Modify: `apps/server/internal/agent/reviewer/rubric_test.go`

- [ ] **Step 1: Replace old rubric tests**

Update `apps/server/internal/agent/reviewer/rubric_test.go` with tests that assert:

```go
func TestRequiredAxesByReviewTask(t *testing.T) {
	tests := []struct {
		task string
		want []string
	}{
		{ReviewTaskPreRenderPlan, []string{AxisFaithfulness, AxisSubjectConsistency, AxisContinuity}},
		{ReviewTaskPreviewImage, []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality}},
		{ReviewTaskShotVideo, []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality, AxisMotionPhysics, AxisContinuity, AxisAudioSync}},
		{ReviewTaskFinalVideo, []string{AxisFaithfulness, AxisBrandStyleConsistency, AxisVisualQuality, AxisContinuity, AxisAudioSync, AxisPlatformSellingPower}},
	}
	for _, tt := range tests {
		got := RequiredAxesForReviewTask(tt.task)
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("%s axes = %#v, want %#v", tt.task, got, tt.want)
		}
	}
}
```

and:

```go
func TestValidateReviewResultRequiresBlockingIssueOnRejected(t *testing.T) {
	result := ReviewResult{
		ReviewTask: ReviewTaskShotVideo,
		Verdict:    ReviewVerdictRejected,
		OverallScore: 0.4,
		Rubric: passingRequiredAxes(ReviewTaskShotVideo),
		Critique: "商品颜色漂移，不能进入剪辑。",
	}
	_, err := ValidateRubric(result, DefaultReviewPolicyForTask(ReviewTaskShotVideo))
	if err == nil {
		t.Fatal("expected rejected result without blocking issue to fail")
	}
}
```

- [ ] **Step 2: Run rubric tests and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/reviewer -run 'TestRequiredAxesByReviewTask|TestValidateReviewResultRequiresBlockingIssueOnRejected' -count=1
```

Expected: FAIL because old constants and validation do not exist.

- [ ] **Step 3: Add M3 types**

In `apps/server/internal/agent/reviewer/types.go`, add constants and structs:

```go
const (
	ReviewTaskPreRenderPlan = "pre_render_plan_review"
	ReviewTaskPreviewImage  = "preview_image_review"
	ReviewTaskShotVideo     = "shot_video_review"
	ReviewTaskFinalVideo    = "final_video_review"

	ReviewVerdictAccepted             = "accepted"
	ReviewVerdictAcceptedWithWarnings = "accepted_with_warnings"
	ReviewVerdictRejected             = "rejected"
	ReviewVerdictBlocked              = "blocked"

	AxisFaithfulness           = "faithfulness"
	AxisSubjectConsistency     = "subject_consistency"
	AxisProductVisibility      = "product_visibility"
	AxisBrandStyleConsistency  = "brand_style_consistency"
	AxisCompositionProportion  = "composition_proportion"
	AxisMotionPhysics          = "motion_physics"
	AxisVisualQuality          = "visual_quality"
	AxisContinuity             = "continuity"
	AxisAudioSync              = "audio_sync"
	AxisPlatformSellingPower   = "platform_selling_power"
)

type ReviewTarget struct {
	WorkspaceScope       string `json:"workspace_scope"`
	ShotID               string `json:"shot_id,omitempty"`
	RenderPlanID         string `json:"render_plan_id,omitempty"`
	NodeID               string `json:"node_id,omitempty"`
	ArtifactVersionID    string `json:"artifact_version_id,omitempty"`
	GenerationJobID      string `json:"generation_job_id,omitempty"`
	ParentReviewRecordID string `json:"parent_review_record_id,omitempty"`
}

type ReviewIssue struct {
	Dimension                string `json:"dimension"`
	Severity                 string `json:"severity"`
	Title                    string `json:"title"`
	Description              string `json:"description"`
	Evidence                 string `json:"evidence,omitempty"`
	TargetObjectType         string `json:"target_object_type"`
	TargetObjectID           string `json:"target_object_id"`
	SuggestedFix             string `json:"suggested_fix"`
	FixHint                  string `json:"fix_hint"`
	RequiresUserConfirmation bool   `json:"requires_user_confirmation"`
}
```

Update `ReviewResult` so it includes `ReviewTask`, `Target`, `Verdict`, `RequiredAxes`, `Issues`, `EvidenceSummary`, and M3 `RetryRecommendation`.

- [ ] **Step 4: Implement task-specific validation**

In `apps/server/internal/agent/reviewer/rubric.go`, replace old 7-axis `RequiredPreviewAxes` with:

```go
func RequiredAxesForReviewTask(task string) []string {
	switch task {
	case ReviewTaskPreRenderPlan:
		return []string{AxisFaithfulness, AxisSubjectConsistency, AxisContinuity}
	case ReviewTaskPreviewImage:
		return []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality}
	case ReviewTaskShotVideo:
		return []string{AxisFaithfulness, AxisSubjectConsistency, AxisProductVisibility, AxisBrandStyleConsistency, AxisCompositionProportion, AxisVisualQuality, AxisMotionPhysics, AxisContinuity, AxisAudioSync}
	case ReviewTaskFinalVideo:
		return []string{AxisFaithfulness, AxisBrandStyleConsistency, AxisVisualQuality, AxisContinuity, AxisAudioSync, AxisPlatformSellingPower}
	default:
		return nil
	}
}
```

Keep `ValidateRubric` as the public entrypoint, but make it enforce:

- `overall_score` and every axis score are `0..1`.
- current task required axes exist.
- `rejected` has at least one blocking issue.
- `accepted` has no blocking issue.
- `blocked` has a non-empty critique and reason.
- all dimensions are either 10-axis dimensions or pre-render dimensions.

- [ ] **Step 5: Run reviewer rubric package tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/reviewer -run 'TestRequiredAxes|TestValidateRubric' -count=1
```

Expected: PASS.

## Task 4: Reviewer Native Tools

**Files:**

- Create: `apps/server/internal/agent/tools/submit_review_result.go`
- Create: `apps/server/internal/agent/tools/reviewer_context_tools.go`
- Create: `apps/server/internal/agent/tools/reviewer_tools_test.go`
- Modify: `apps/server/internal/agent/tools/native.go`
- Modify: `apps/server/internal/agent/tools/native_result.go`

- [ ] **Step 1: Add tool schema tests**

Create `apps/server/internal/agent/tools/reviewer_tools_test.go` with tests for:

```go
func TestReviewerNativeToolInfosUseChineseDescriptions(t *testing.T) {
	tools := []NativeTool{
		NewSubmitReviewResultNativeTool(nil),
	}
	for _, item := range tools {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == "" || !strings.Contains(info.Desc, "Reviewer") {
			t.Fatalf("bad tool info: %#v", info)
		}
		if info.ParamsOneOf == nil {
			t.Fatalf("%s ParamsOneOf is nil", info.Name)
		}
	}
}
```

and:

```go
func TestSubmitReviewResultReturnsNaturalValidationError(t *testing.T) {
	tool := NewSubmitReviewResultNativeTool(nil)
	out, err := tool.InvokableRun(context.Background(), `{"review_task":"shot_video_review"}`)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"工具调用失败", "submit_review_result", "重试建议"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run tool tests and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/tools -run 'TestReviewerNativeToolInfosUseChineseDescriptions|TestSubmitReviewResultReturnsNaturalValidationError' -count=1
```

Expected: FAIL because the tools do not exist.

- [ ] **Step 3: Implement `submit_review_result` input structs**

Create `apps/server/internal/agent/tools/submit_review_result.go` with structs from the M3 spec:

```go
type SubmitReviewResultInput struct {
	Brief               string                   `json:"brief" jsonschema:"required" jsonschema_description:"提交评审结果的业务目的，例如提交 shot_01 视频评审并指出商品漂移问题。"`
	ReviewTask          string                   `json:"review_task" jsonschema:"required,enum=pre_render_plan_review,enum=preview_image_review,enum=shot_video_review,enum=final_video_review" jsonschema_description:"评审任务类型。必须与当前 reviewer task 一致。"`
	Target              ReviewTargetInput        `json:"target" jsonschema:"required" jsonschema_description:"被评审对象。必须与当前 reviewer task 一致。"`
	Verdict             string                   `json:"verdict" jsonschema:"required,enum=accepted,enum=accepted_with_warnings,enum=rejected,enum=blocked" jsonschema_description:"最终评审结论。accepted 可继续；accepted_with_warnings 可继续但需提示；rejected 阻塞推进；blocked 表示无法可靠评审。"`
	OverallScore        float64                  `json:"overall_score" jsonschema:"required" jsonschema_description:"整体评分，范围 0 到 1。blocked 时可填 0。"`
	Rubric              []ReviewRubricAxisInput  `json:"rubric" jsonschema:"required" jsonschema_description:"10 轴 rubric 的评分结果。必须包含当前 review_task 的 required axes。"`
	Critique            string                   `json:"critique" jsonschema:"required" jsonschema_description:"面向 Producer 和用户可读的评审摘要。必须指出通过理由或阻塞问题。"`
	Issues              []ReviewIssueInput       `json:"issues" jsonschema_description:"结构化问题列表。rejected 或 accepted_with_warnings 通常至少一条。"`
	RetryRecommendation RetryRecommendationInput `json:"retry_recommendation" jsonschema_description:"给 Producer 的下一步建议。Reviewer 只建议，不直接执行。"`
	EvidenceSummary     string                   `json:"evidence_summary" jsonschema_description:"证据摘要，例如参考图、分镜目标、画面帧、音频片段或 prompt 问题。不要写长篇逐帧日志。"`
	Reason              string                   `json:"reason" jsonschema:"required" jsonschema_description:"为什么给出这个 verdict。必须能和 rubric、issues 对上。"`
}
```

Use `toolInfoFor[SubmitReviewResultInput]` so schema generation uses `GoStruct2ParamsOneOf`.

- [ ] **Step 4: Implement natural validation**

`InvokableRun` must:

- decode arguments with `decodeToolArgs`.
- return `NaturalToolError` for malformed JSON, missing required fields, invalid enum, invalid UUID, missing required axes, invalid score, rejected-without-blocking-issue, and accepted-with-blocking-issue.
- never return a non-nil error for business validation failures.
- require `agenttools.NativeRuntimeContext` so the tool can identify workspace/thread/task.

- [ ] **Step 5: Implement persistence adapter**

Define a narrow store interface in `submit_review_result.go`:

```go
type SubmitReviewResultStore interface {
	CreateReviewRecord(ctx context.Context, params db.CreateReviewRecordParams) (db.ReviewRecord, error)
	CompleteReviewRecord(ctx context.Context, params db.CompleteReviewRecordParams) (db.ReviewRecord, error)
	CreateArtifactIssue(ctx context.Context, params db.CreateArtifactIssueParams) (db.ArtifactIssue, error)
}
```

The tool should create a running review record, complete it with submitted rubric/retry/escalation, and create one `artifact_issue` per issue.

- [ ] **Step 6: Implement reviewer read tools**

Create `apps/server/internal/agent/tools/reviewer_context_tools.go` with:

- `ReadReviewContextNativeTool`
- `ReadProjectMemoryForReviewNativeTool`

Both tools must return natural-language summaries. They can reuse existing services, but the implementation must enforce review target scope and must not expose unrelated workspace objects.

- [ ] **Step 7: Run reviewer tool tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/tools -run 'TestReviewer|TestSubmitReviewResult' -count=1
```

Expected: PASS.

## Task 5: Reviewer System Prompt and Tool-Calling Responder

**Files:**

- Create: `apps/server/internal/agent/reviewer/system_prompt.go`
- Create: `apps/server/internal/agent/reviewer/system_prompt_test.go`
- Modify: `apps/server/internal/agent/reviewer/model_responder.go`
- Modify: `apps/server/internal/agent/reviewer/model_responder_test.go`

- [ ] **Step 1: Add Reviewer prompt test**

Create `apps/server/internal/agent/reviewer/system_prompt_test.go`:

```go
func TestReviewerSystemPromptContainsGateRules(t *testing.T) {
	prompt := SystemPrompt()
	for _, required := range []string{
		"Reviewer / Quality Gate",
		"ProjectMemory 是项目创作宪法",
		"10 轴 Rubric",
		"Seedance",
		"submit_review_result",
		"不直接触发重跑",
		"不修改 RenderPlan",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("reviewer prompt missing %q", required)
		}
	}
	for _, forbidden := range []string{"TODO", "TBD", "M1 阶段", "M2 阶段"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("reviewer prompt contains placeholder wording %q", forbidden)
		}
	}
}
```

- [ ] **Step 2: Run prompt test and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/reviewer -run TestReviewerSystemPromptContainsGateRules -count=1
```

Expected: FAIL because `SystemPrompt` does not exist.

- [ ] **Step 3: Implement Reviewer prompt**

Create `apps/server/internal/agent/reviewer/system_prompt.go` using the M3 spec `Reviewer System Prompt 草案`. Keep it as a raw string returned by:

```go
func SystemPrompt() string {
	return strings.TrimSpace(`...`)
}
```

The prompt must include:

- Role boundary.
- ClipAnvil domain concepts.
- 10-axis rubric.
- Seedream image review rules.
- Seedance video review rules from `seedance_pe_SKILL.md`.
- Tool loop rules.
- Verdict rules.
- Concrete good/bad repair hint examples.
- Key prohibitions.

- [ ] **Step 4: Update model responder for tool calls**

Modify `apps/server/internal/agent/reviewer/model_responder.go` so the model responder returns a `ReviewerTurnOutput` containing:

```go
type ReviewerTurnOutput struct {
	Text         string
	ModelMessage *schema.Message
	Metadata     map[string]any
}
```

It should pass messages:

- system: `SystemPrompt()`
- user: current review context text
- tool infos: from Reviewer context
- same-turn assistant/tool messages from loop state

Do not ask the model to output JSON review result directly. The model must call `submit_review_result`.

- [ ] **Step 5: Run responder tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/reviewer -run 'TestReviewerSystemPrompt|TestVolcengineReviewerResponder' -count=1
```

Expected: PASS after updating tests to assert tool-capable messages.

## Task 6: Reviewer Eino-Native Bounded Tool Loop

**Files:**

- Create: `apps/server/internal/agent/reviewer/native_tool_loop.go`
- Create: `apps/server/internal/agent/reviewer/native_tool_middleware.go`
- Create: `apps/server/internal/agent/reviewer/native_tool_loop_test.go`
- Modify: `apps/server/internal/agent/reviewer/graph.go`
- Modify: `apps/server/internal/agent/reviewer/executor.go`
- Modify: `apps/server/internal/agent/reviewer/graph_test.go`
- Modify: `apps/server/internal/agent/einoruntime/graph_info_test.go`: update reviewer graph-name assertions from `reviewer_preview` to `reviewer_gate`.

- [ ] **Step 1: Add graph info test**

Create `apps/server/internal/agent/reviewer/native_tool_loop_test.go`:

```go
func TestReviewerGraphUsesNativeToolLoop(t *testing.T) {
	registry := einoruntime.NewGraphInfoRegistry()
	graph, err := NewGraph(GraphConfig{
		Loader:            fakeReviewerLoader{context: minimalReviewContext()},
		ToolResponder:     fakeReviewerToolResponder{message: toolCallMessage("submit_review_result", `{}`)},
		NativeToolRegistry: fakeReviewerNativeRegistry(),
		Runtime:           &fakeReviewerRuntime{},
		CheckPointStore:   fakeCheckPointStore{},
		CompileCallbacks:  []compose.GraphCompileCallback{registry.GraphCompileCallback()},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = graph
	info, ok := registry.Get("reviewer_gate")
	if !ok {
		t.Fatal("reviewer_gate graph info was not captured")
	}
	for _, want := range []string{"load_context", "prepare_turn_state", "call_model", "prepare_tool_message", "execute_tools", "append_tool_results", "finalize_response"} {
		if !graphInfoHasNode(info.Nodes, want) {
			t.Fatalf("reviewer graph missing node %q", want)
		}
	}
}
```

- [ ] **Step 2: Run graph test and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/reviewer -run TestReviewerGraphUsesNativeToolLoop -count=1
```

Expected: FAIL because the graph is still fixed two-node.

- [ ] **Step 3: Add native graph config**

Extend `GraphConfig` with:

```go
ToolResponder      ToolResponder
NativeToolRegistry *agenttools.NativeRegistry
```

Keep old `Responder ModelResponder` only until all tests are migrated, then remove the fixed review path if no callers remain.

- [ ] **Step 4: Implement loop graph**

`native_tool_loop.go` should mirror Craftsman structure:

```text
START -> load_context -> prepare_turn_state -> call_model
call_model -> prepare_tool_message -> execute_tools -> append_tool_results -> call_model
call_model -> finalize_response -> END
call_model -> fail_turn -> END
```

Rules:

- Max tool iterations: 6.
- Graph name: `reviewer_gate`.
- Tools execute sequentially.
- Only Reviewer native tool registry is exposed.
- `submit_review_result` successful call marks state as submitted.
- If the model finishes without successful `submit_review_result`, graph returns an invalid-review error.

- [ ] **Step 5: Update executor task parsing**

In `apps/server/internal/agent/reviewer/executor.go`, parse the M3 task input:

```go
type TaskInput struct {
	Brief              string       `json:"brief"`
	ReviewTask         string       `json:"review_task"`
	Target             ReviewTarget `json:"target"`
	Policy             ReviewPolicy `json:"policy"`
	AllowAutoAccept    bool         `json:"allow_auto_accept"`
	AllowAutoRepair    bool         `json:"allow_auto_repair"`
	RequireUserOnReject bool        `json:"require_user_on_reject"`
}
```

Accept all four M3 review tasks. Do not require `target_phase == preview_image`.

- [ ] **Step 6: Remove Reviewer-owned retry and version selection**

In `apps/server/internal/agent/reviewer/graph.go`, remove behavior that:

- calls `Selector.SelectArtifactVersion`.
- calls `RetryDispatcher.DispatchRetry`.
- emits `retry_requested` as if Reviewer owns the retry.

Replace it with events:

- `review_submitted`
- `review_accepted`
- `review_warning`
- `review_rejected`
- `review_blocked`

All next-step decisions remain Producer-owned.

- [ ] **Step 7: Run reviewer package tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/reviewer -count=1
```

Expected: PASS.

## Task 7: Producer `dispatch_reviewer`

**Files:**

- Create: `apps/server/internal/agent/tools/dispatch_reviewer.go`
- Modify: `apps/server/internal/agent/tools/registry.go` if registry helper needs no change.
- Modify: `apps/server/internal/agent/tools/review_shot.go` or deprecate it from Producer registry.
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/internal/agent/runtime/service.go`
- Create or modify: `apps/server/internal/agent/tools/dispatch_reviewer_test.go`

- [ ] **Step 1: Add dispatch tool tests**

Create `apps/server/internal/agent/tools/dispatch_reviewer_test.go` with:

```go
func TestDispatchReviewerNativeToolRequiresMatchingTarget(t *testing.T) {
	tool := NewDispatchReviewerNativeTool(fakeDispatchReviewerStore{}, fakeDispatchReviewerRuntime{}, nil)
	out, err := tool.InvokableRun(context.Background(), `{
		"brief":"评审 shot video",
		"review_task":"shot_video_review",
		"target":{"workspace_scope":"shot"},
		"reason":"视频生成完成，需要评审"
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "工具调用失败") || !strings.Contains(out, "artifact_version_id") {
		t.Fatalf("unexpected output: %s", out)
	}
}
```

and:

```go
func TestDispatchReviewerCreatesReviewerTask(t *testing.T) {
	store := fakeDispatchReviewerStore{workspaceMode: db.WorkspaceModeAgent}
	runtime := &fakeDispatchReviewerRuntime{}
	enqueuer := &fakeReviewerTaskEnqueuer{}
	tool := NewDispatchReviewerNativeTool(store, runtime, enqueuer)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{WorkspaceID: uuidWithByte(1), ThreadID: uuidWithByte(2), TaskID: uuidWithByte(3), ToolCallID: "call_1"})
	out, err := tool.InvokableRun(ctx, validShotVideoReviewJSON())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "已派发 Reviewer") {
		t.Fatalf("unexpected output: %s", out)
	}
	if runtime.createdTask.TaskType != "reviewer_turn" {
		t.Fatalf("task type = %s", runtime.createdTask.TaskType)
	}
	if len(enqueuer.tasks) != 1 {
		t.Fatalf("enqueued tasks = %d", len(enqueuer.tasks))
	}
}
```

- [ ] **Step 2: Run dispatch tests and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/tools -run TestDispatchReviewer -count=1
```

Expected: FAIL because `dispatch_reviewer` does not exist.

- [ ] **Step 3: Implement native tool**

Create `apps/server/internal/agent/tools/dispatch_reviewer.go` with the M3 spec input structs:

- `DispatchReviewerInput`
- `ReviewTargetInput`
- `ReviewPolicyInput`
- `AutoDecisionInput`

Validation:

- `brief`, `review_task`, `target`, `reason` required.
- `pre_render_plan_review` requires `target.render_plan_id`.
- `preview_image_review` and `shot_video_review` require `target.shot_id`, `target.node_id`, `target.artifact_version_id`.
- `final_video_review` requires `target.node_id`, `target.artifact_version_id`.
- UUIDs must parse and belong to the current workspace.
- Artifact review target must be succeeded.
- Node type must match review task.

Return strings:

- Success title: `已派发 Reviewer 评审任务`
- Error title: `工具调用失败`
- Success next step: `Producer 应读取 review_record 和 artifact_issue 后决定是否接受、修复或请求用户确认。`

- [ ] **Step 4: Wire Producer tool registry**

In `apps/server/cmd/server/main.go`, register `NewDispatchReviewerNativeTool(...)` in Producer native registry.

Remove old `NewReviewShotTool(...)` from the Producer-facing registry or leave it unregistered. Producer should use `dispatch_reviewer`, not `review_shot`.

- [ ] **Step 5: Run dispatch tool tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/tools -run TestDispatchReviewer -count=1
```

Expected: PASS.

## Task 8: Context, PSS, and Canvas Projection

**Files:**

- Modify: `apps/server/internal/agent/reviewer/context_loader.go`
- Modify: `apps/server/internal/agent/pss/producer.go`
- Modify: `apps/server/internal/api/domain_canvas_projection.go`
- Modify: `apps/web/src/components/canvas-flow/DomainFlowNode.tsx`
- Modify: `apps/web/src/components/canvas-flow/DomainFlowEdge.tsx`
- Modify: `apps/web/src/components/canvas-flow/flowTypes.ts`
- Modify tests in corresponding packages.

- [ ] **Step 1: Add context loader tests**

In `apps/server/internal/agent/reviewer/context_loader_test.go`, add cases:

- `pre_render_plan_review` loads RenderPlan and ProjectMemory context.
- `shot_video_review` loads shot, node, artifact version, generation job, render plan if present, prior reviews, and open issues.
- invalid workspace-owned IDs are rejected.

- [ ] **Step 2: Run context tests and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/agent/reviewer -run TestContextLoader -count=1
```

Expected: FAIL for new M3 cases.

- [ ] **Step 3: Extend context loader**

Update `ContextStore` and `ContextLoader.Load` to support:

- RenderPlan lookup for pre-render.
- `review_task` based target validation.
- `artifact_issue` lookup for prior open issues.
- generation job rendered prompt and provider request metadata.

The returned context text must avoid naked storage URLs and asset IDs unless they are necessary for internal lookup; user-facing critique should use semantic names.

- [ ] **Step 4: Extend Producer PSS**

In `apps/server/internal/agent/pss/producer.go`, include:

- recent review records with verdict and score.
- open artifact issues grouped by target object.
- retry recommendations summarized in Chinese.

Keep PSS concise; do not dump full rubric JSON.

- [ ] **Step 5: Add canvas projection tests**

Add backend tests that assert the projection contains:

- node kind `review_record`
- node kind `artifact_issue`
- edge kind `reviews`
- edge kind `flags`
- edge kind `suggests_fix`

- [ ] **Step 6: Implement canvas projection**

In `apps/server/internal/api/domain_canvas_projection.go`, add review and issue domain nodes. In web components, render:

- Review node: verdict, score, task type.
- Issue node: severity, dimension, title, status.
- Edges: review target, issue target, suggested fix target.

- [ ] **Step 7: Run projection tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/internal/api ./apps/server/internal/agent/pss -run 'Test.*Review|Test.*Issue|TestProducerPSS' -count=1
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

## Task 9: Server Wiring and Scheduler Recovery

**Files:**

- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/server/cmd/server/main_test.go`
- Modify: `apps/server/internal/agent/runtime/service.go`
- Modify: `apps/server/sqlc/queries/agent_task.sql` if reviewer queue filtering needs updates.

- [ ] **Step 1: Add server wiring test**

In `apps/server/cmd/server/main_test.go`, add assertions that:

- Producer native registry includes `dispatch_reviewer`.
- Reviewer native registry includes `read_project_context`, `read_project_memory`, `submit_review_result`.
- Reviewer graph config does not include `RetryDispatcher`.

- [ ] **Step 2: Run server wiring test and confirm failure**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/cmd/server -run TestAgentToolWiring -count=1
```

Expected: FAIL until wiring is complete.

- [ ] **Step 3: Wire Reviewer native graph**

In `apps/server/cmd/server/main.go`:

- Build `agenttools.NewNativeRegistry(...)` for Reviewer tools.
- Pass it into `agentreviewer.NewGraph`.
- Remove `agentReviewerRetryDispatcher`.
- Keep queued reviewer recovery, but recovery now runs `reviewer_gate`.

- [ ] **Step 4: Run server build tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" go test ./apps/server/cmd/server -run TestAgentToolWiring -count=1
make server-build
```

Expected: PASS.

## Task 10: M3 E2E Smoke

**Files:**

- Create: `scripts/smoke-m3-reviewer-gate-e2e.sh`
- Modify: `apps/server/cmd/server/e2e_producer_fixture.go`: extend deterministic Producer fixture to dispatch Reviewer after shot video artifact creation.
- Modify: `apps/server/cmd/server/e2e_craftsman_fixture.go`: extend deterministic Craftsman fixture to create a `mode=fork_from` repair RenderPlan when Producer passes Reviewer issues.
- Create: `apps/server/cmd/server/e2e_reviewer_fixture.go`: deterministic Reviewer fixture that submits rejected-then-accepted results through `submit_review_result`.

- [ ] **Step 1: Write smoke script**

Create `scripts/smoke-m3-reviewer-gate-e2e.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT_DIR"

COMMAND="${1:-}"
STATE_FILE="${CLIPANVIL_E2E_STATE_FILE:-/tmp/clipanvil-m3-reviewer-gate-e2e.json}"
BASE="${CLIPANVIL_API_BASE:-http://127.0.0.1:${CLIPANVIL_SERVER_PORT:-8888}/api}"
WEB_BASE="${CLIPANVIL_WEB_BASE:-${CLIPANVIL_PUBLIC_BASE_URL:-http://127.0.0.1:${CLIPANVIL_WEB_PORT:-5173}}}"
DB_DSN="${CLIPANVIL_POSTGRES_DSN:-postgres://clipanvil:clipanvil_dev@localhost:5432/clipanvil?sslmode=disable}"

usage() {
  cat <<'USAGE'
Usage:
  scripts/smoke-m3-reviewer-gate-e2e.sh setup
  scripts/smoke-m3-reviewer-gate-e2e.sh verify

Environment:
  CLIPANVIL_E2E_PRODUCER_FIXTURE=m3_reviewer_gate, CLIPANVIL_E2E_CRAFTSMAN_FIXTURE=m3_reviewer_gate, and CLIPANVIL_E2E_REVIEWER_FIXTURE=m3_reviewer_gate must be set before starting the server.
  CLIPANVIL_SERVER_PORT / CLIPANVIL_WEB_PORT are read from the running dev profile.
  CLIPANVIL_E2E_STATE_FILE can override the setup state path.
USAGE
}

if [[ "$COMMAND" != "setup" && "$COMMAND" != "verify" ]]; then
  usage >&2
  exit 1
fi

if [[ "$COMMAND" == "setup" ]]; then
  BASE="$BASE" WEB_BASE="$WEB_BASE" STATE_FILE="$STATE_FILE" node <<'NODE'
import fs from "node:fs";

const base = process.env.BASE;
const webBase = process.env.WEB_BASE;
const stateFile = process.env.STATE_FILE;
const suffix = `${Date.now()}-${Math.floor(Math.random() * 10000)}`;
const email = `m3-reviewer-e2e-${suffix}@clipanvil.local`;
const password = "clipanvil-e2e-pass";

async function req(path, init = {}) {
  const headers = {
    ...(init.headers ?? {}),
    "Content-Type": "application/json",
  };
  const response = await fetch(base + path, { ...init, headers });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`${init.method ?? "GET"} ${path} -> ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

const auth = await req("/auth/register", {
  method: "POST",
  body: JSON.stringify({ email, password, name: "M3 Reviewer E2E" }),
});
const workspace = await req("/workspaces", {
  method: "POST",
  headers: { Authorization: `Bearer ${auth.token}` },
  body: JSON.stringify({ name: "M3 Reviewer Gate E2E", mode: "agent" }),
});
const state = {
  email,
  password,
  token: auth.token,
  account: auth.account,
  workspace,
  agent_url: `${webBase}/workspaces/${workspace.id}/agent`,
  message_text: "E2E_M3_REVIEWER_GATE：请为悦行银灰色硬壳行李箱做一条机场抖音广告，生成分镜视频，评审视频质量，不合格就局部修复。",
};
fs.writeFileSync(stateFile, JSON.stringify(state, null, 2));
console.log(JSON.stringify(state, null, 2));
NODE
  echo "state_file=$STATE_FILE"
  exit 0
fi

if [[ ! -f "$STATE_FILE" ]]; then
  echo "state file not found: $STATE_FILE" >&2
  exit 1
fi

BASE="$BASE" STATE_FILE="$STATE_FILE" node <<'NODE'
import fs from "node:fs";

const base = process.env.BASE;
const state = JSON.parse(fs.readFileSync(process.env.STATE_FILE, "utf8"));
const headers = { Authorization: `Bearer ${state.token}` };

async function req(path) {
  const response = await fetch(base + path, { headers });
  const text = await response.text();
  if (!response.ok) {
    throw new Error(`GET ${path} -> ${response.status}: ${text}`);
  }
  return text ? JSON.parse(text) : null;
}

async function waitFor(label, fn, timeoutMs = 60000) {
  const started = Date.now();
  let lastError;
  while (Date.now() - started < timeoutMs) {
    try {
      const value = await fn();
      if (value) return value;
    } catch (error) {
      lastError = error;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`${label} timed out${lastError ? `: ${lastError.message}` : ""}`);
}

const messages = await waitFor("producer final message", async () => {
  const data = await req(`/agent/workspaces/${state.workspace.id}/messages?limit=100`);
  const final = data.messages.find((message) => {
    const raw = JSON.stringify(message.content ?? {});
    return message.role === "assistant" && raw.includes("M3 Reviewer Gate");
  });
  return final ? data.messages : null;
});

const canvas = await waitFor("domain canvas projection with review issues", async () => {
  const data = await req(`/workspaces/${state.workspace.id}/canvas`);
  const nodes = data.domain_projection?.nodes ?? [];
  const kinds = new Set(nodes.map((node) => node.kind));
  return kinds.has("review_record") && kinds.has("artifact_issue") ? data : null;
});

const projection = canvas.domain_projection;
console.log(JSON.stringify({
  message_count: messages.length,
  review_nodes: projection.nodes.filter((node) => node.kind === "review_record").length,
  issue_nodes: projection.nodes.filter((node) => node.kind === "artifact_issue").length,
  domain_edge_count: projection.edges.length,
}, null, 2));
NODE

WORKSPACE_ID="$(jq -r '.workspace.id' "$STATE_FILE")"
if [[ -z "$WORKSPACE_ID" || "$WORKSPACE_ID" == "null" ]]; then
  echo "workspace id missing from state file" >&2
  exit 1
fi

db_rows="$(psql "$DB_DSN" -AtX -F '|' -v workspace_id="$WORKSPACE_ID" <<'SQL'
SELECT 'reviewer_task_succeeded', count(*), 2 FROM agent_task WHERE workspace_id = :'workspace_id'::uuid AND role = 'reviewer' AND task_type = 'reviewer_turn' AND status = 'succeeded'
UNION ALL
SELECT 'shot_video_review_records', count(*), 2 FROM review_record WHERE workspace_id = :'workspace_id'::uuid AND review_task = 'shot_video_review'
UNION ALL
SELECT 'blocking_artifact_issue', count(*), 1 FROM artifact_issue WHERE workspace_id = :'workspace_id'::uuid AND severity = 'blocking'
UNION ALL
SELECT 'repair_render_plan', count(*), 1 FROM render_plan WHERE workspace_id = :'workspace_id'::uuid AND forked_from_render_plan_id IS NOT NULL
UNION ALL
SELECT 'accepted_review', count(*), 1 FROM review_record WHERE workspace_id = :'workspace_id'::uuid AND review_task = 'shot_video_review' AND status = 'accepted';
SQL
)"

echo "$db_rows"
while IFS='|' read -r name count minimum; do
  if [[ -z "$name" ]]; then
    continue
  fi
  if (( count < minimum )); then
    echo "DB check failed: $name count=$count minimum=$minimum" >&2
    exit 1
  fi
done <<< "$db_rows"

echo "M3 Reviewer Gate browser+DB E2E checks passed for workspace $WORKSPACE_ID"
```

- [ ] **Step 2: Add deterministic fixtures**

The fixture should create this sequence without calling real paid models:

1. Producer creates creative state and dispatches Craftsman.
2. Craftsman creates shot video RenderPlan.
3. Worker fixture writes a succeeded video artifact with metadata that marks it as intentionally flawed.
4. Producer dispatches Reviewer.
5. Reviewer fixture calls `submit_review_result` with `verdict=rejected`, issues `subject_consistency` and `motion_physics`.
6. Producer reads review result and dispatches Craftsman repair.
7. Craftsman creates `mode=fork_from` RenderPlan.
8. Worker fixture writes repaired artifact.
9. Reviewer fixture submits accepted result.

- [ ] **Step 3: Run E2E smoke**

Run:

```bash
bash scripts/smoke-m3-reviewer-gate-e2e.sh
```

Expected: script exits 0 and SQL assertions return `t`.

## Task 11: Full Verification

**Files:** all modified files.

- [ ] **Step 1: Run sqlc generation**

Run:

```bash
make sqlc-generate
```

Expected: exit 0.

- [ ] **Step 2: Run server tests**

Run:

```bash
GOCACHE="${GOCACHE:-/private/tmp/clipanvil-go-build}" make server-test
```

Expected: exit 0.

- [ ] **Step 3: Run server lint and build**

Run:

```bash
make server-lint
make server-build
```

Expected: both exit 0.

- [ ] **Step 4: Run web checks**

Run:

```bash
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
```

Expected: both exit 0.

- [ ] **Step 5: Run M3 smoke**

Run:

```bash
bash scripts/smoke-m3-reviewer-gate-e2e.sh
```

Expected: exit 0 and DB assertions show reviewer task, review record, issue, and repair RenderPlan.

- [ ] **Step 6: Run prompt placeholder scan**

Run:

```bash
rg -n "M1 阶段|M2 阶段|在 M2 调度|然后在 M2 调用|M2 可用生成调度工具|## M2 生成调度能力|TODO|TBD" apps/server/internal/agent/producer apps/server/internal/agent/craftsman apps/server/internal/agent/reviewer
```

Expected: no matches.

- [ ] **Step 7: Run diff check**

Run:

```bash
git diff --check
```

Expected: exit 0.

## Self-Review Against Spec

- M3 Reviewer remains a quality gate, not a fourth orchestrator.
- Producer owns dispatch, repair decision, user confirmation, and retry stopping.
- Craftsman owns RenderPlan creation and repair via `fork_from`.
- Reviewer has only scoped reads plus `submit_review_result`.
- 10-axis rubric and task-specific required axes are implemented.
- Seedance skill is represented as Reviewer checks, not copied as a prompt optimizer.
- Rejected review creates persisted issues that can be projected to canvas.
- E2E verifies browser/API path plus database state.
