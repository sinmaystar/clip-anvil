# Dynamic Agent Remotion Route Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Agent mode generate no-Seedance 30s+ commerce ads by preserving the existing dynamic Producer storyboard flow and routing every `shot_video` execution to Remotion `motion_shot_video`.

**Architecture:** Producer remains the planner for CreativeBrief, ProjectMemory, KeyElement, Storyboard, AudioPlan, and dispatch order. Remotion is only the per-shot execution profile for low-cost/no-Seedance `shot_video`; Composer is the only layer that assembles multiple silent motion shots with voiceover, BGM, subtitles, and timing.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, ClipAnvil Agent skills, RenderPlan profiles, Remotion motion shot sandbox, Volcengine Seedream image/audio providers, Composer timeline renderer, Vite Agent UI, Playwright/in-app browser E2E, ffprobe.

---

## Scope And Gates

This plan implements `docs/superpowers/specs/2026-07-02-dynamic-agent-remotion-route-design.md`.

The work is staged. Do not start a later phase until the current phase acceptance gate passes and the phase commit exists. A phase is complete only when the deliverables, tests, and acceptance checks in that phase are all satisfied.

## Current Code Facts

- `apps/server/internal/agent/producer/system_prompt.go` already defines dynamic Scene, Shot, Storyboard, AudioPlan, and `video_route_policy=motion_only`.
- `apps/server/internal/agent/tools/dispatch_craftsman.go` already supports `video_route_policy` and recommends `motion_shot_video` for `motion_only`.
- `apps/server/internal/agent/tools/dispatch_craftsman_test.go` already has baseline tests for motion-only avoiding Seedance, but only with one or two shots.
- `apps/server/internal/motionshot/plan.go` supports short motion shot durations of 3, 4, 5, 6, and 8 seconds. A 30s+ final video must be multiple short shots composed by Composer, not a single 30s Remotion clip.
- `apps/server/cmd/server/e2e_producer_fixture.go` still has an 8 second single-shot `motion_shot_video` fixture. This must stop being the product proof.
- `apps/server/cmd/server/e2e_craftsman_fixture.go` still emits an 8 second fixed motion plan. It must use each shot's dynamic facts.
- `apps/server/cmd/server/e2e_composer_fixture.go` still builds one 8 second segment. It must compose multiple segments for the deterministic route smoke.

## Files

- Modify: `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
  - Responsibility: commerce ad planning skill. It must explicitly preserve dynamic multi-shot storyboard planning under no-Seedance.
- Modify: `apps/server/internal/agent/skills/library/motion-shot-producer/SKILL.md`
  - Responsibility: route policy skill. It must not act like a template or storyboard generator.
- Modify: `apps/server/internal/agent/skills/library/motion-shot-craftsman/SKILL.md`
  - Responsibility: per-shot Remotion RenderPlan skill. It must inherit shot duration, shot purpose, image inputs, and route policy.
- Modify: `apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md`
  - Responsibility: Seedance skill. It must refuse `motion_only` tasks.
- Modify: `apps/server/internal/agent/skills/registry_test.go`
  - Responsibility: skill registration and skill text contract tests.
- Modify: `apps/server/internal/agent/producer/system_prompt.go`
  - Responsibility: Producer global rules. It must make no-Seedance dynamic storyboard behavior unambiguous.
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`
  - Responsibility: Producer prompt contract tests.
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`
  - Responsibility: dispatch task input and recommended route. It must preserve shot facts needed by Craftsman and recommend motion-only for every selected shot.
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
  - Responsibility: dispatch contract tests for multi-shot motion-only routing.
- Modify: `apps/server/internal/motionshot/plan.go`
  - Responsibility: normalize motion-shot params into provider plan. It must keep short per-shot bounds and support explicit visual layer/text layer variety.
- Modify: `apps/server/internal/motionshot/plan_test.go`
  - Responsibility: motion-shot plan normalization tests.
- Modify: `apps/server/cmd/server/e2e_producer_fixture.go`
  - Responsibility: deterministic smoke Producer responder. It must create dynamic multi-shot storyboard and dispatch multiple motion shots.
- Modify: `apps/server/cmd/server/e2e_craftsman_fixture.go`
  - Responsibility: deterministic smoke Craftsman responder. It must emit different RenderPlans by shot facts.
- Modify: `apps/server/cmd/server/e2e_composer_fixture.go`
  - Responsibility: deterministic smoke Composer responder. It must build a multi-segment 30s+ timeline.
- Modify: `apps/server/cmd/server/main_test.go`
  - Responsibility: deterministic fixture unit tests.
- Create: `scripts/smoke-m12-dynamic-remotion-route.sh`
  - Responsibility: repeatable server-side smoke verifier for multi-shot no-Seedance Remotion route and final media evidence.
- Create: `docs/engineering/m12-dynamic-remotion-route-e2e.md`
  - Responsibility: browser E2E runbook and evidence checklist for the final 30s+ Agent test.

---

## Phase 0: Plan And Baseline Gate

**Deliverable:** This implementation plan is saved, self-reviewed, and committed.

**Acceptance Gate:**

- `git diff --check` passes.
- The plan contains phases for skill/prompt contracts, dispatch routing, motion-shot expressiveness, deterministic smoke, and browser E2E.
- The plan does not redefine success as a fixed template or one-shot video.

### Task 0.1: Save The Plan

**Files:**
- Create: `docs/superpowers/plans/2026-07-02-dynamic-agent-remotion-route-implementation.md`

- [ ] **Step 1: Verify the spec exists**

Run:

```bash
test -f docs/superpowers/specs/2026-07-02-dynamic-agent-remotion-route-design.md
```

Expected: exit code 0.

- [ ] **Step 2: Save this plan**

Use `apply_patch` to create `docs/superpowers/plans/2026-07-02-dynamic-agent-remotion-route-implementation.md` with the contents of this plan.

- [ ] **Step 3: Run plan self-review checks**

Run:

```bash
rg -n 'T(O)DO|T(B)D|place(holder)|fixed[[:space:]]30|single-shot[[:space:]]product[[:space:]]proof|do[[:space:]]later|fill[[:space:]]in' docs/superpowers/plans/2026-07-02-dynamic-agent-remotion-route-implementation.md
git diff --check
```

Expected:

- `rg` exits with no matches.
- `git diff --check` prints no whitespace errors.

- [ ] **Step 4: Commit Phase 0**

Run:

```bash
git add docs/superpowers/plans/2026-07-02-dynamic-agent-remotion-route-implementation.md
git commit -m "docs: plan dynamic agent remotion route implementation"
```

Expected: commit succeeds.

---

## Phase 1: Skill And Prompt Contracts

**Deliverable:** Producer and Craftsman instructions make no-Seedance dynamic multi-shot behavior explicit, and tests prove the text contract is present.

**Acceptance Gate:**

- Unit tests fail before the skill/prompt edits and pass after them.
- `commerce-ad-producer` states that no-Seedance must not collapse the storyboard to one shot.
- `motion-shot-producer` states it is only a route policy skill and must be paired with `commerce-ad-producer`.
- `motion-shot-craftsman` states it must inherit the current shot's duration and vary layout/motion/text per shot.
- `seedance-renderplan-craftsman` states it must not create Seedance plans when `video_route_policy=motion_only`.

### Task 1.1: Add Skill Text Contract Tests

**Files:**
- Modify: `apps/server/internal/agent/skills/registry_test.go`

- [ ] **Step 1: Inspect existing skill registry test helpers**

Run:

```bash
sed -n '1,260p' apps/server/internal/agent/skills/registry_test.go
```

Expected: output shows how the tests load skill text from `DefaultRegistry`.

- [ ] **Step 2: Add failing tests**

Append these tests to `apps/server/internal/agent/skills/registry_test.go`, adapting only helper names if the file already has an equivalent helper:

```go
func TestCommerceAdProducerPreservesDynamicStoryboardForNoSeedance(t *testing.T) {
	registry := DefaultRegistry()
	skill, ok := registry.Lookup("commerce-ad-producer")
	if !ok {
		t.Fatal("commerce-ad-producer skill missing")
	}
	body := skill.Prompt
	for _, needle := range []string{
		"no-Seedance does not reduce the storyboard to one shot",
		"20-45 second commerce ads usually need 4-9 shots",
		"each shot must have narrative_purpose, duration_sec, visual_intent, action_text, camera_intent, and narration",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("commerce-ad-producer missing %q\n%s", needle, body)
		}
	}
}

func TestMotionShotProducerIsRoutePolicyOnly(t *testing.T) {
	registry := DefaultRegistry()
	skill, ok := registry.Lookup("motion-shot-producer")
	if !ok {
		t.Fatal("motion-shot-producer skill missing")
	}
	body := skill.Prompt
	for _, needle := range []string{
		"must be paired with commerce-ad-producer",
		"do not create a fixed storyboard",
		"dispatch every ready shot_video with video_route_policy: motion_only",
		"do not dispatch only one synthetic shot unless the dynamic storyboard truly has one shot",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("motion-shot-producer missing %q\n%s", needle, body)
		}
	}
}

func TestMotionShotCraftsmanInheritsDynamicShotFacts(t *testing.T) {
	registry := DefaultRegistry()
	skill, ok := registry.Lookup("motion-shot-craftsman")
	if !ok {
		t.Fatal("motion-shot-craftsman skill missing")
	}
	body := skill.Prompt
	for _, needle := range []string{
		"inherit the current shot duration_sec",
		"vary layout, motion_style, transitions, and text positions by shot purpose",
		"do not bake full voiceover subtitles into the motion shot",
		"block the task when no usable image input is available",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("motion-shot-craftsman missing %q\n%s", needle, body)
		}
	}
}

func TestSeedanceCraftsmanRefusesMotionOnlyRoute(t *testing.T) {
	registry := DefaultRegistry()
	skill, ok := registry.Lookup("seedance-renderplan-craftsman")
	if !ok {
		t.Fatal("seedance-renderplan-craftsman skill missing")
	}
	body := skill.Prompt
	for _, needle := range []string{
		"video_route_policy=motion_only",
		"must not create seedance_2_video",
		"mark the task blocked or ask Producer to change the route",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("seedance-renderplan-craftsman missing %q\n%s", needle, body)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
(cd apps/server && go test ./internal/agent/skills -run 'TestCommerceAdProducerPreservesDynamicStoryboardForNoSeedance|TestMotionShotProducerIsRoutePolicyOnly|TestMotionShotCraftsmanInheritsDynamicShotFacts|TestSeedanceCraftsmanRefusesMotionOnlyRoute' -count=1)
```

Expected: FAIL with missing contract strings.

### Task 1.2: Update Producer And Craftsman Skills

**Files:**
- Modify: `apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/motion-shot-producer/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/motion-shot-craftsman/SKILL.md`
- Modify: `apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md`

- [ ] **Step 1: Patch `commerce-ad-producer`**

In the `## Do` section, add these bullets:

```markdown
- For no-Seedance or low-cost requests, keep normal dynamic storyboard planning. no-Seedance does not reduce the storyboard to one shot.
- For 20-45 second commerce ads, 20-45 second commerce ads usually need 4-9 shots unless the user's requested format is intentionally a very short bumper.
- Make each shot specific enough for downstream execution: each shot must have narrative_purpose, duration_sec, visual_intent, action_text, camera_intent, and narration.
- When motion shots need image inputs, plan preview/reference images first; do not dispatch motion shot video before there is a product image, generated visual, or explicit input strategy.
```

- [ ] **Step 2: Patch `motion-shot-producer`**

In the `## Do` section, add these bullets:

```markdown
- This skill must be paired with commerce-ad-producer. Commerce ad structure still comes from CreativeBrief, ProjectMemory, KeyElement, Storyboard, and AudioPlan.
- Route policy only: do not create a fixed storyboard, do not replace dynamic shot planning, and do not choose a canned 30 second template.
- For no-Seedance requests, dispatch every ready shot_video with video_route_policy: motion_only.
- Preserve real shot_refs from the dynamic storyboard; do not dispatch only one synthetic shot unless the dynamic storyboard truly has one shot.
```

In the `## Do Not` section, add:

```markdown
- Do not turn a multi-shot request into a single internal motion card.
```

- [ ] **Step 3: Patch `motion-shot-craftsman`**

In the `## Do` section, add these bullets:

```markdown
- Always inherit the current shot duration_sec when it is one of the supported short motion-shot durations; otherwise choose the closest supported short duration and explain the adjustment in audit_hints.
- Vary layout, motion_style, transitions, and text positions by shot purpose so a multi-shot ad does not look like the same card repeated.
- Use visual_intent, action_text, camera_intent, narration, recommended_params, and input_node_refs from the task before inventing copy or motion.
- Block the task when no usable image input is available; ask Producer to generate or select a preview/reference image first.
```

In the `## Do Not` section, add:

```markdown
- Do not bake full voiceover subtitles into the motion shot; Composer owns final subtitles, audio mixing, and shot timing.
```

- [ ] **Step 4: Patch `seedance-renderplan-craftsman`**

Find `apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md` and add this rule to the `## Do Not` section:

```markdown
- When task context contains video_route_policy=motion_only, must not create seedance_2_video. Mark the task blocked or ask Producer to change the route.
```

- [ ] **Step 5: Run skill tests**

Run:

```bash
(cd apps/server && go test ./internal/agent/skills -run 'TestCommerceAdProducerPreservesDynamicStoryboardForNoSeedance|TestMotionShotProducerIsRoutePolicyOnly|TestMotionShotCraftsmanInheritsDynamicShotFacts|TestSeedanceCraftsmanRefusesMotionOnlyRoute' -count=1)
```

Expected: PASS.

### Task 1.3: Add Producer Prompt Contract Test

**Files:**
- Modify: `apps/server/internal/agent/producer/model_responder_test.go`
- Modify: `apps/server/internal/agent/producer/system_prompt.go`

- [ ] **Step 1: Inspect existing prompt tests**

Run:

```bash
sed -n '1,140p' apps/server/internal/agent/producer/model_responder_test.go
```

Expected: output shows existing prompt contract style or confirms a new test file is needed.

- [ ] **Step 2: Add failing prompt test**

Add this test to `apps/server/internal/agent/producer/model_responder_test.go`:

```go
func TestProducerPromptNoSeedanceKeepsDynamicStoryboard(t *testing.T) {
	prompt := ProducerSystemPrompt(ProducerContext{})
	for _, needle := range []string{
		"no-Seedance 不等于固定模板",
		"继续使用动态 Storyboard",
		"30 秒左右营销视频通常需要 4-9 个 shot",
		"每个 shot_video 都必须填写 video_route_policy=motion_only",
		"最终 30 秒以上成片由 Composer 拼接多个 motion_shot_video、旁白和 BGM",
	} {
		if !strings.Contains(prompt, needle) {
			t.Fatalf("Producer prompt missing %q", needle)
		}
	}
}
```

- [ ] **Step 3: Run test to verify failure**

Run:

```bash
(cd apps/server && go test ./internal/agent/producer -run TestProducerPromptNoSeedanceKeepsDynamicStoryboard -count=1)
```

Expected: FAIL with a missing prompt string.

- [ ] **Step 4: Update Producer prompt**

In `apps/server/internal/agent/producer/system_prompt.go`, inside `Seedream / Seedance / Remotion Motion Shot 决策摘要`, add this exact rule block after the existing no-Seedance route bullet:

```text
	- no-Seedance 不等于固定模板，也不等于单分镜。继续使用动态 Storyboard：根据商品、目标平台、目标时长和口播结构决定 scene / shot 数量。
	- 30 秒左右营销视频通常需要 4-9 个 shot，每个 shot 维持 3-8 秒的可执行短片段，再由 Composer 合成 30 秒以上最终成片。
	- no-Seedance 或 motion_only 请求中，每个 shot_video 都必须填写 video_route_policy=motion_only；最终 30 秒以上成片由 Composer 拼接多个 motion_shot_video、旁白和 BGM。
```

- [ ] **Step 5: Run Producer prompt test**

Run:

```bash
(cd apps/server && go test ./internal/agent/producer -run TestProducerPromptNoSeedanceKeepsDynamicStoryboard -count=1)
```

Expected: PASS.

### Task 1.4: Phase 1 Full Verification And Commit

**Files:**
- Modified files from Tasks 1.1 through 1.3

- [ ] **Step 1: Run targeted tests**

Run:

```bash
(cd apps/server && go test ./internal/agent/skills ./internal/agent/producer -count=1)
```

Expected: PASS.

- [ ] **Step 2: Run formatting and diff checks**

Run:

```bash
gofmt -w apps/server/internal/agent/skills/registry_test.go apps/server/internal/agent/producer/model_responder_test.go
git diff --check
```

Expected: no output from `git diff --check`.

- [ ] **Step 3: Commit Phase 1**

Run:

```bash
git add apps/server/internal/agent/skills/library/commerce-ad-producer/SKILL.md \
  apps/server/internal/agent/skills/library/motion-shot-producer/SKILL.md \
  apps/server/internal/agent/skills/library/motion-shot-craftsman/SKILL.md \
  apps/server/internal/agent/skills/library/seedance-renderplan-craftsman/SKILL.md \
  apps/server/internal/agent/skills/registry_test.go \
  apps/server/internal/agent/producer/system_prompt.go \
  apps/server/internal/agent/producer/model_responder_test.go
git commit -m "feat: enforce dynamic no-seedance remotion route contracts"
```

Expected: commit succeeds.

---

## Phase 2: Dispatch Routing And Shot Fact Handoff

**Deliverable:** `dispatch_craftsman` can route a dynamic multi-shot storyboard to Remotion, preserve per-shot facts in task input, and never recommend Seedance under `motion_only`.

**Acceptance Gate:**

- Multi-shot motion-only dispatch creates one Craftsman task per selected shot.
- Every created task has `video_route_policy=motion_only`, `recommended_model_prompt_profile=motion_shot_video`, and `recommended_operation=image_to_motion_video`.
- Every created task includes a `shot_facts` object with `duration_sec`, `narrative_purpose`, `visual_intent`, `action_text`, `camera_intent`, and `narration`.
- Planned/draft shots are dispatchable for motion-only only when image inputs are explicit or the default preview image ref can be inferred.

### Task 2.1: Add Multi-Shot Dispatch Test

**Files:**
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`

- [ ] **Step 1: Add failing test for four dynamic shots**

Add this test:

```go
func TestDispatchCraftsmanMotionOnlyPolicyDispatchesEveryDynamicShotWithFacts(t *testing.T) {
	store := &fakeCraftsmanDispatchStore{
		workspace: db.Workspace{ID: uuidWithByte(1), Mode: db.WorkspaceModeAgent},
		shots: []db.Shot{
			{ID: uuidWithByte(11), WorkspaceID: uuidWithByte(1), ClientKey: "shot_01_hook", SemanticKey: "scene_intro.shot_01_hook", Title: "开场钩子", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 6, Valid: true}, NarrativePurpose: "用短途出行痛点吸引注意", VisualIntent: "行李箱居中，背景留白", ActionText: "产品图轻推近", CameraIntent: "缓慢推进", Narration: "短途出行，行李箱别再拖后腿。"},
			{ID: uuidWithByte(12), WorkspaceID: uuidWithByte(1), ClientKey: "shot_02_product", SemanticKey: "scene_intro.shot_02_product", Title: "产品展示", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 8, Valid: true}, NarrativePurpose: "建立悦行行李箱主体", VisualIntent: "展示银灰硬壳和轮子", ActionText: "商品细节分层出现", CameraIntent: "轻微视差", Narration: "悦行行李箱，轻便好推。"},
			{ID: uuidWithByte(13), WorkspaceID: uuidWithByte(1), ClientKey: "shot_03_benefits", SemanticKey: "scene_benefit.shot_03_benefits", Title: "卖点卡", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 8, Valid: true}, NarrativePurpose: "解释万向轮和托运安心", VisualIntent: "卖点文字分组", ActionText: "三点卖点依次入场", CameraIntent: "稳定信息卡", Narration: "顺滑万向轮，转向更稳，安心托运。"},
			{ID: uuidWithByte(14), WorkspaceID: uuidWithByte(1), ClientKey: "shot_04_cta", SemanticKey: "scene_outro.shot_04_cta", Title: "CTA", Status: "preview_ready", DurationSec: pgtype.Float8{Float64: 6, Valid: true}, NarrativePurpose: "收束购买行动", VisualIntent: "按钮和品牌口号清晰", ActionText: "CTA 弹出", CameraIntent: "轻微拉远", Narration: "现在出发。"},
		},
	}
	runtime := &fakeCraftsmanRuntime{}
	tool := NewDispatchCraftsmanTool(store, runtime, &fakeCraftsmanEnqueuer{})

	out, err := tool.Execute(context.Background(), ExecuteInput{
		WorkspaceID: uuidWithByte(1),
		ThreadID:    uuidWithByte(2),
		TaskID:      uuidWithByte(3),
		Arguments: map[string]any{
			"brief":              "生成 30 秒悦行行李箱广告的所有 Remotion 分镜视频，不调用 Seedance。",
			"target_phase":       "shot_video",
			"execution_policy":   "execute_immediately",
			"scope":              map[string]any{"type": "shot"},
			"shot_refs":          []string{"scene_intro.shot_01_hook", "scene_intro.shot_02_product", "scene_benefit.shot_03_benefits", "scene_outro.shot_04_cta"},
			"video_route_policy": "motion_only",
			"force":              true,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Result["status"] != "queued" {
		t.Fatalf("result = %#v", out.Result)
	}
	if len(runtime.createdTasks) != 4 {
		t.Fatalf("created tasks = %d, want 4", len(runtime.createdTasks))
	}
	for _, task := range runtime.createdTasks {
		var input map[string]any
		if err := json.Unmarshal(task.Input, &input); err != nil {
			t.Fatal(err)
		}
		if input["recommended_model_prompt_profile"] != "motion_shot_video" || input["recommended_operation"] != "image_to_motion_video" {
			t.Fatalf("task did not force motion route: %#v", input)
		}
		if input["video_route_policy"] != "motion_only" {
			t.Fatalf("task missing motion_only: %#v", input)
		}
		facts, ok := input["shot_facts"].(map[string]any)
		if !ok {
			t.Fatalf("shot_facts missing: %#v", input)
		}
		for _, key := range []string{"duration_sec", "narrative_purpose", "visual_intent", "action_text", "camera_intent", "narration"} {
			if facts[key] == nil || facts[key] == "" {
				t.Fatalf("shot_facts missing %s: %#v", key, facts)
			}
		}
		params := input["recommended_params"].(map[string]any)
		if params["duration_sec"] != facts["duration_sec"] {
			t.Fatalf("recommended duration does not inherit shot duration: params=%#v facts=%#v", params, facts)
		}
		if strings.Contains(strings.ToLower(mustString(input["recommended_route_reason"])), "seedance") &&
			!strings.Contains(strings.ToLower(mustString(input["recommended_route_reason"])), "no-seedance") {
			t.Fatalf("route reason should only mention Seedance as prohibited policy: %#v", input)
		}
	}
}
```

Add this helper in the same file if it does not already exist:

```go
func mustString(value any) string {
	if value == nil {
		return ""
	}
	return value.(string)
}
```

- [ ] **Step 2: Run test to verify failure**

Run:

```bash
(cd apps/server && go test ./internal/agent/tools -run TestDispatchCraftsmanMotionOnlyPolicyDispatchesEveryDynamicShotWithFacts -count=1)
```

Expected: FAIL because `shot_facts` is missing or `duration_sec` is still hardcoded.

### Task 2.2: Add Shot Facts To Dispatch Input

**Files:**
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman.go`

- [ ] **Step 1: Extend dispatch scope**

Change `craftsmanDispatchScope` to include shot facts:

```go
type craftsmanDispatchScope struct {
	ScopeType        string
	ScopeID          pgtype.UUID
	ScopeKey         string
	ClientKey        string
	Title            string
	DurationSec      float64
	NarrativePurpose string
	VisualIntent     string
	ActionText       string
	CameraIntent     string
	Narration        string
}
```

- [ ] **Step 2: Populate shot facts in `resolveScopes`**

Replace the shot append block with:

```go
out = append(out, craftsmanDispatchScope{
	ScopeType:        "shot",
	ScopeID:          shot.ID,
	ScopeKey:         semanticScopeKey(shot.SemanticKey, "shot", shot.ClientKey),
	ClientKey:        shot.ClientKey,
	Title:            shot.Title,
	DurationSec:      shotDurationSeconds(shot.DurationSec),
	NarrativePurpose: shot.NarrativePurpose,
	VisualIntent:     shot.VisualIntent,
	ActionText:       shot.ActionText,
	CameraIntent:     shot.CameraIntent,
	Narration:        shot.Narration,
})
```

Add helper:

```go
func shotDurationSeconds(value pgtype.Float8) float64 {
	if !value.Valid || value.Float64 <= 0 {
		return 0
	}
	return value.Float64
}
```

- [ ] **Step 3: Add `shot_facts` into task input**

Inside the `if scope.ScopeType == "shot"` block in `Execute`, after `shot_client_key`, add:

```go
taskInput["shot_facts"] = map[string]any{
	"title":             scope.Title,
	"duration_sec":      scope.DurationSec,
	"narrative_purpose": scope.NarrativePurpose,
	"visual_intent":     scope.VisualIntent,
	"action_text":       scope.ActionText,
	"camera_intent":     scope.CameraIntent,
	"narration":         scope.Narration,
}
```

- [ ] **Step 4: Make motion route inherit supported duration**

In `motionRecommendedRoute`, compute duration:

```go
durationSec := motionShotDuration(scope.DurationSec)
```

Change `duration_sec` in params to `durationSec`.

Add helper:

```go
func motionShotDuration(value float64) int {
	rounded := int(value + 0.5)
	switch rounded {
	case 3, 4, 5, 6, 8:
		return rounded
	case 7:
		return 8
	default:
		if rounded <= 0 {
			return 5
		}
		if rounded < 3 {
			return 3
		}
		if rounded > 8 {
			return 8
		}
		return 5
	}
}
```

- [ ] **Step 5: Run dispatch test**

Run:

```bash
(cd apps/server && go test ./internal/agent/tools -run TestDispatchCraftsmanMotionOnlyPolicyDispatchesEveryDynamicShotWithFacts -count=1)
```

Expected: PASS.

### Task 2.3: Phase 2 Full Verification And Commit

**Files:**
- Modified files from Tasks 2.1 and 2.2

- [ ] **Step 1: Run all dispatch tests**

Run:

```bash
gofmt -w apps/server/internal/agent/tools/dispatch_craftsman.go apps/server/internal/agent/tools/dispatch_craftsman_test.go
(cd apps/server && go test ./internal/agent/tools -run 'TestDispatchCraftsman' -count=1)
git diff --check
```

Expected: PASS and no diff check output.

- [ ] **Step 2: Commit Phase 2**

Run:

```bash
git add apps/server/internal/agent/tools/dispatch_craftsman.go apps/server/internal/agent/tools/dispatch_craftsman_test.go
git commit -m "feat: carry dynamic shot facts into motion-only dispatch"
```

Expected: commit succeeds.

---

## Phase 3: Motion Shot Plan Expressiveness

**Deliverable:** The Remotion motion shot plan respects explicit visual/text layer variety and keeps short per-shot durations bounded for composition.

**Acceptance Gate:**

- Motion shot normalization accepts caller-provided `visual_layers` instead of always replacing them with one default product layer.
- Text layers are clipped to the shot duration, preventing final-frame overlap from layer end times exceeding the segment.
- Unsupported long durations still fail in motion-shot provider normalization, because 30s+ is a Composer concern.

### Task 3.1: Add Motion Plan Normalization Tests

**Files:**
- Modify: `apps/server/internal/motionshot/plan_test.go`

- [ ] **Step 1: Add failing test for explicit visual layers and clipped text**

Add this test:

```go
func TestNormalizeUsesExplicitVisualLayersAndClipsTextToDuration(t *testing.T) {
	plan, err := Normalize(RenderInput{
		DurationSec: 6,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
		Params: map[string]any{
			"visual_layers": []any{
				map[string]any{"role": "background", "input_ref": "assets/bg.png", "fit": "cover", "motion": "slow_pan_left", "start_sec": float64(0), "end_sec": float64(6)},
				map[string]any{"role": "product", "input_ref": "primary_image", "fit": "contain", "motion": "float_up", "start_sec": float64(0.4), "end_sec": float64(6)},
			},
			"text_layers": []any{
				map[string]any{"role": "hook", "text": "轻装出发", "start_sec": float64(0.2), "end_sec": float64(9), "animation": "pop_slide_up", "position": "upper_third"},
			},
		},
	})
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(plan.VisualLayers) != 2 {
		t.Fatalf("visual layers = %#v", plan.VisualLayers)
	}
	if plan.VisualLayers[0].Role != "background" || plan.VisualLayers[1].Motion != "float_up" {
		t.Fatalf("visual layers not preserved: %#v", plan.VisualLayers)
	}
	if len(plan.TextLayers) != 1 || plan.TextLayers[0].EndSec != 6 {
		t.Fatalf("text layer not clipped: %#v", plan.TextLayers)
	}
}
```

- [ ] **Step 2: Add long duration boundary test**

Add this test:

```go
func TestNormalizeRejectsLongSingleMotionShotDuration(t *testing.T) {
	_, err := Normalize(RenderInput{
		DurationSec: 30,
		Ratio:       "9:16",
		Resolution:  "1080p",
		FPS:         30,
		Assets:      []Asset{{WorkspacePath: "assets/product.png"}},
	})
	if err == nil || !strings.Contains(err.Error(), "duration_sec 30 is not supported") {
		t.Fatalf("expected duration error, got %v", err)
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
(cd apps/server && go test ./internal/motionshot -run 'TestNormalizeUsesExplicitVisualLayersAndClipsTextToDuration|TestNormalizeRejectsLongSingleMotionShotDuration' -count=1)
```

Expected: first test FAILS because explicit visual layers are ignored or text end is not clipped; second test PASSES.

### Task 3.2: Normalize Visual Layers And Text Layer Bounds

**Files:**
- Modify: `apps/server/internal/motionshot/plan.go`

- [ ] **Step 1: Add `visualLayers` helper**

Add this function:

```go
func visualLayers(params map[string]any, assets []Asset, duration int) []VisualLayer {
	if raw, ok := params["visual_layers"].([]any); ok && len(raw) > 0 {
		out := make([]VisualLayer, 0, len(raw))
		for _, item := range raw {
			values, ok := item.(map[string]any)
			if !ok {
				continue
			}
			role := stringParam(values, "role", "product")
			inputRef := stringParam(values, "input_ref", "")
			if inputRef == "" || inputRef == "primary_image" {
				inputRef = strings.TrimSpace(assets[0].WorkspacePath)
			}
			out = append(out, VisualLayer{
				Role:     role,
				InputRef: inputRef,
				Fit:      stringParam(values, "fit", "contain"),
				Motion:   stringParam(values, "motion", "slow_push_in"),
				StartSec: clampFloat(floatParam(values, "start_sec", 0), 0, float64(duration)),
				EndSec:   clampFloat(floatParam(values, "end_sec", float64(duration)), 0, float64(duration)),
			})
		}
		if len(out) > 0 {
			return out
		}
	}
	return []VisualLayer{{
		Role:     "product",
		InputRef: strings.TrimSpace(assets[0].WorkspacePath),
		Fit:      "contain",
		Motion:   "slow_push_in",
		StartSec: 0,
		EndSec:   float64(duration),
	}}
}
```

Add clamp helper:

```go
func clampFloat(value float64, min float64, max float64) float64 {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
```

- [ ] **Step 2: Use explicit visual layers**

In `Normalize`, replace the hardcoded `VisualLayers` field with:

```go
VisualLayers: visualLayers(input.Params, input.Assets, duration),
```

- [ ] **Step 3: Clip text layer times**

In `textLayers`, change the explicit layer append to:

```go
out = append(out, TextLayer{
	Role:      stringParam(values, "role", "copy"),
	Text:      text,
	StartSec:  clampFloat(floatParam(values, "start_sec", 0), 0, float64(duration)),
	EndSec:    clampFloat(floatParam(values, "end_sec", float64(duration)), 0, float64(duration)),
	Animation: stringParam(values, "animation", "fade_rise"),
	Position:  stringParam(values, "position", "middle_safe"),
})
```

- [ ] **Step 4: Run motion-shot tests**

Run:

```bash
gofmt -w apps/server/internal/motionshot/plan.go apps/server/internal/motionshot/plan_test.go
(cd apps/server && go test ./internal/motionshot -count=1)
```

Expected: PASS.

### Task 3.3: Phase 3 Commit

**Files:**
- Modified files from Tasks 3.1 and 3.2

- [ ] **Step 1: Run diff check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 2: Commit Phase 3**

Run:

```bash
git add apps/server/internal/motionshot/plan.go apps/server/internal/motionshot/plan_test.go
git commit -m "feat: preserve explicit motion shot layers"
```

Expected: commit succeeds.

---

## Phase 4: Deterministic Dynamic Route Smoke

**Deliverable:** The deterministic M12 fixture proves the server route can produce a multi-shot no-Seedance Remotion final video with audio, without pretending the fixture is the product behavior.

**Acceptance Gate:**

- `e2eMotionShotVideoProducerResponder` creates at least five dynamic shots totaling at least 32 seconds.
- It dispatches preview images for multiple shots and then dispatches all shot videos with `video_route_policy=motion_only`.
- `e2eMotionShotVideoCraftsmanResponder` emits different `motion_shot_video` RenderPlans by shot key and duration.
- `e2e_composer_fixture.go` creates a multi-segment timeline totaling at least 32 seconds with voiceover and BGM tracks.
- Unit tests verify no Seedance profile appears in fixture RenderPlan arguments.

### Task 4.1: Replace Single-Shot Fixture Tests

**Files:**
- Modify: `apps/server/cmd/server/main_test.go`

- [ ] **Step 1: Update Producer fixture test expectations**

Change `TestMotionShotVideoFixturePlansAudioAndMotionShotVideo` so it asserts:

```go
if count == 2 && !containsAll(call.Arguments,
	`"client_key":"shot_01_hook"`,
	`"client_key":"shot_02_product"`,
	`"client_key":"shot_03_wheels"`,
	`"client_key":"shot_04_travel"`,
	`"client_key":"shot_05_cta"`,
	`"duration_sec":6`,
	`"duration_sec":8`,
) {
	t.Fatalf("storyboard is not dynamic multi-shot: %s", call.Arguments)
}
if count == 3 && !containsAll(call.Arguments, `"target_duration_sec":34`, `"voiceover_script"`, `"cue_plan"`, `"bgm_plan"`) {
	t.Fatalf("audio plan missing 34s fields: %s", call.Arguments)
}
if count == 5 && !containsAll(call.Arguments, `"target_phase":"preview_image"`, `"shot_refs":["shot_01_hook","shot_02_product","shot_03_wheels","shot_04_travel","shot_05_cta"]`) {
	t.Fatalf("preview image dispatch missing multi-shot route: %s", call.Arguments)
}
if count == 6 && !containsAll(call.Arguments, `"video_route_policy":"motion_only"`, `"target_phase":"shot_video"`, `"shot_01_hook preview image"`, `"shot_05_cta preview image"`) {
	t.Fatalf("shot video dispatch missing multi-shot motion_only policy: %s", call.Arguments)
}
if strings.Contains(call.Arguments, "seedance_2_video") {
	t.Fatalf("fixture must not mention seedance_2_video: %s", call.Arguments)
}
```

- [ ] **Step 2: Add Craftsman fixture diversity test**

Add:

```go
func TestMotionShotVideoCraftsmanFixtureVariesPlansByShotFacts(t *testing.T) {
	fixture := e2eMotionShotVideoCraftsmanResponder{}
	shots := []db.Shot{
		{ID: uuidWithByte(11), ClientKey: "shot_01_hook", Title: "痛点开场", DurationSec: pgtype.Float8{Float64: 6, Valid: true}, NarrativePurpose: "痛点钩子", VisualIntent: "大标题上方", ActionText: "商品轻推近", CameraIntent: "慢推", Narration: "短途出行，别让行李箱拖后腿。"},
		{ID: uuidWithByte(12), ClientKey: "shot_03_wheels", Title: "万向轮卖点", DurationSec: pgtype.Float8{Float64: 8, Valid: true}, NarrativePurpose: "证明顺滑", VisualIntent: "三点卖点卡", ActionText: "卖点逐条入场", CameraIntent: "稳定信息卡", Narration: "顺滑万向轮，转向更稳。"},
	}
	plans := []string{}
	for _, shot := range shots {
		out, err := fixture.Respond(context.Background(), agentcraftsman.Context{
			Input: agentcraftsman.GraphInput{Mode: "shot_video", InputNodeRefs: []string{shot.ClientKey + " preview image"}, VideoRoutePolicy: "motion_only"},
			Shot:  shot,
			SameTurnMessages: []agentcraftsman.CraftsmanSameTurnMessage{{Role: "tool"}},
		})
		if err != nil {
			t.Fatal(err)
		}
		call := out.ModelMessage.ToolCalls[0].Function
		if call.Name != "upsert_render_plan" {
			t.Fatalf("tool = %s", call.Name)
		}
		if !containsAll(call.Arguments, `"model_prompt_profile":"motion_shot_video"`, `"operation":"image_to_motion_video"`, `"video_route_policy=motion_only"`) {
			t.Fatalf("motion plan missing route policy: %s", call.Arguments)
		}
		if strings.Contains(call.Arguments, `"model_prompt_profile":"seedance_2_video"`) {
			t.Fatalf("motion fixture emitted Seedance: %s", call.Arguments)
		}
		plans = append(plans, call.Arguments)
	}
	if plans[0] == plans[1] {
		t.Fatal("different shots produced identical motion plans")
	}
	if !containsAll(plans[0], `"duration_sec":6`) || !containsAll(plans[1], `"duration_sec":8`) {
		t.Fatalf("plans did not inherit durations: %#v", plans)
	}
}
```

- [ ] **Step 3: Run tests to verify failure**

Run:

```bash
(cd apps/server && go test ./cmd/server -run 'TestMotionShotVideoFixturePlansAudioAndMotionShotVideo|TestMotionShotVideoCraftsmanFixtureVariesPlansByShotFacts' -count=1)
```

Expected: FAIL because fixture still uses `shot_01_motion_ad` and an 8 second single segment.

### Task 4.2: Update Producer Fixture To Dynamic Multi-Shot

**Files:**
- Modify: `apps/server/cmd/server/e2e_producer_fixture.go`

- [ ] **Step 1: Change initial brief to 34 seconds**

In the first `upsert_project_brief` fixture response, set:

```json
"duration_sec":34
```

Update text fields so they describe five dynamic shots: hook, product proof, wheels, travel scenario, CTA.

- [ ] **Step 2: Replace storyboard JSON**

In the storyboard response, replace the single `shot_01_motion_ad` with five shots:

```json
[
  {
    "client_key":"shot_01_hook",
    "scene_client_key":"scene_motion_ad_intro",
    "sort_order":1,
    "title":"短途出行痛点钩子",
    "shot_kind":"hook_card",
    "creative_text":"用短途出行拖箱费力的痛点开场，商品图轻推近。",
    "narrative_purpose":"前三秒抓住短途出行用户注意。",
    "duration_sec":6,
    "visual_intent":"深色干净背景，行李箱位于中上，顶部短 hook。",
    "action_text":"产品图慢推近，痛点文字弹出。",
    "camera_intent":"Remotion slow push in，无真实复杂运动。",
    "narration":"短途出行，别让行李箱拖后腿。"
  },
  {
    "client_key":"shot_02_product",
    "scene_client_key":"scene_motion_ad_intro",
    "sort_order":2,
    "title":"悦行行李箱产品展示",
    "shot_kind":"product_hero",
    "creative_text":"建立悦行行李箱主体，突出银灰硬壳、竖向纹理和轻便外观。",
    "narrative_purpose":"让用户记住商品主体和品牌名。",
    "duration_sec":8,
    "visual_intent":"商品大图居中，品牌标题和轻便卖点分层。",
    "action_text":"商品轻微浮起，品牌名淡入。",
    "camera_intent":"轻微视差和中心聚焦。",
    "narration":"悦行行李箱，轻便好推，通勤和短途都省心。"
  },
  {
    "client_key":"shot_03_wheels",
    "scene_client_key":"scene_motion_ad_benefit",
    "sort_order":3,
    "title":"顺滑万向轮卖点",
    "shot_kind":"benefit_card",
    "creative_text":"用信息卡解释顺滑万向轮、转向稳定、推行省力。",
    "narrative_purpose":"证明核心功能卖点。",
    "duration_sec":8,
    "visual_intent":"底部轮子细节和三点卖点文字清晰分组。",
    "action_text":"三条卖点逐条入场。",
    "camera_intent":"稳定信息卡，轻微横向漂移。",
    "narration":"顺滑万向轮，转向更稳，推行更省力。"
  },
  {
    "client_key":"shot_04_travel",
    "scene_client_key":"scene_motion_ad_benefit",
    "sort_order":4,
    "title":"短途旅行场景",
    "shot_kind":"scenario_card",
    "creative_text":"把商品放进短途出行语境，强调安心托运和周末出发。",
    "narrative_purpose":"把功能转成用户生活收益。",
    "duration_sec":6,
    "visual_intent":"旅行氛围背景，商品和目的地标签形成层次。",
    "action_text":"背景慢移，目的地标签滑入。",
    "camera_intent":"柔和拉远，保留字幕安全区。",
    "narration":"短途旅行、商务通勤，轻装出发更从容。"
  },
  {
    "client_key":"shot_05_cta",
    "scene_client_key":"scene_motion_ad_outro",
    "sort_order":5,
    "title":"CTA 现在出发",
    "shot_kind":"cta_card",
    "creative_text":"品牌口号和 CTA 收束，强调现在出发。",
    "narrative_purpose":"促进行动。",
    "duration_sec":6,
    "visual_intent":"商品居中，按钮式 CTA 位于下方安全区。",
    "action_text":"CTA 按钮弹出，背景渐亮。",
    "camera_intent":"轻微拉远后定格。",
    "narration":"悦行行李箱，现在出发。"
  }
]
```

- [ ] **Step 3: Update AudioPlan to 34 seconds**

Set `target_duration_sec` to 34 and use cue ranges:

```json
[
  {"shot_ref":"shot_01_hook","start_sec":0,"end_sec":6,"text":"短途出行，别让行李箱拖后腿。"},
  {"shot_ref":"shot_02_product","start_sec":6,"end_sec":14,"text":"悦行行李箱，轻便好推，通勤和短途都省心。"},
  {"shot_ref":"shot_03_wheels","start_sec":14,"end_sec":22,"text":"顺滑万向轮，转向更稳，推行更省力。"},
  {"shot_ref":"shot_04_travel","start_sec":22,"end_sec":28,"text":"短途旅行、商务通勤，轻装出发更从容。"},
  {"shot_ref":"shot_05_cta","start_sec":28,"end_sec":34,"text":"悦行行李箱，现在出发。"}
]
```

- [ ] **Step 4: Update dispatch calls**

Change preview and shot video dispatch to:

```json
"shot_refs":["shot_01_hook","shot_02_product","shot_03_wheels","shot_04_travel","shot_05_cta"]
```

For shot video dispatch, set:

```json
"input_node_refs":["shot_01_hook preview image","shot_02_product preview image","shot_03_wheels preview image","shot_04_travel preview image","shot_05_cta preview image"]
```

- [ ] **Step 5: Update continuation helpers**

Change continuation checks from `shot_01_motion_ad preview image` to the multi-shot preview refs above. Keep `video_route_policy=motion_only`.

### Task 4.3: Update Craftsman Fixture To Vary Motion Plans

**Files:**
- Modify: `apps/server/cmd/server/e2e_craftsman_fixture.go`

- [ ] **Step 1: Add shot duration helper**

Add:

```go
func e2eShotDuration(shot db.Shot) int {
	if shot.DurationSec.Valid && shot.DurationSec.Float64 > 0 {
		return motionFixtureDuration(shot.DurationSec.Float64)
	}
	return 6
}

func motionFixtureDuration(value float64) int {
	rounded := int(value + 0.5)
	switch rounded {
	case 3, 4, 5, 6, 8:
		return rounded
	case 7:
		return 8
	default:
		if rounded > 8 {
			return 8
		}
		return 6
	}
}
```

- [ ] **Step 2: Add plan variant helper**

Add:

```go
func e2eMotionShotVariant(clientKey string) (motionStyle string, productMotion string, textPosition string, transitionIn string, transitionOut string) {
	switch clientKey {
	case "shot_01_hook":
		return "bold_hook_card", "slow_push_in", "upper_third", "soft_zoom", "swipe_up"
	case "shot_02_product":
		return "premium_product_ad", "float_up", "middle_safe", "fade", "fade"
	case "shot_03_wheels":
		return "benefit_grid", "slow_pan_left", "bottom_safe", "slide_left", "slide_right"
	case "shot_04_travel":
		return "scenario_postcard", "parallax_drift", "middle_safe", "soft_zoom", "fade"
	case "shot_05_cta":
		return "cta_packshot", "settle_center", "bottom_safe", "fade", "hold"
	default:
		return "premium_product_ad", "slow_push_in", "bottom_safe", "fade", "fade"
	}
}
```

- [ ] **Step 3: Rewrite `e2eMotionShotVideoRenderPlanArgs`**

Use `c.Shot.ClientKey`, `c.Shot.DurationSec`, `c.Shot.NarrativePurpose`, `c.Shot.VisualIntent`, `c.Shot.ActionText`, `c.Shot.CameraIntent`, and `c.Shot.Narration` to fill:

```json
"duration_sec": <duration inherited from shot>,
"motion_style": "<variant>",
"visual_layers": [
  {"id":"product","input_ref":"primary_image","role":"product","start_sec":0,"end_sec":<duration>,"animation":"<productMotion>","position":"center","scale_from":0.96,"scale_to":1.06},
  {"id":"accent","role":"background","start_sec":0,"end_sec":<duration>,"animation":"ambient_gradient","position":"full_bleed"}
],
"text_layers": [
  {"role":"hook","text":"<shot title>","start_sec":0.2,"end_sec":2.2,"animation":"pop_slide_up","position":"<textPosition>"},
  {"role":"benefit","text":"<short text from narrative purpose or visual intent>","start_sec":2.2,"end_sec":<duration - 0.3>,"animation":"fade_rise","position":"bottom_safe"}
],
"transitions":{"in":"<transitionIn>","out":"<transitionOut>"}
```

Keep the `generation_text` explicit that the output is silent and Composer owns final audio/subtitles.

- [ ] **Step 4: Update Seedream preview image plan**

Use the shot title and visual intent in the `generation_text` so each preview image can differ:

```go
visualIntent := firstNonEmptyString(strings.TrimSpace(c.Shot.VisualIntent), "清爽现代的短途旅行广告背景，留出字幕空间。")
```

If there is no `firstNonEmptyString` helper in this package, add one local helper:

```go
func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
```

### Task 4.4: Update Composer Fixture Timeline

**Files:**
- Modify: `apps/server/cmd/server/e2e_composer_fixture.go`

- [ ] **Step 1: Build multiple segments from staged clips**

Replace the one-clip requirement with a loop over `stage.Files`, selecting all files whose composition role is `clip`.

Use this segment order and durations when asset IDs or workspace paths contain the shot key:

```go
shotDurations := map[string]int{
	"shot_01_hook":    6,
	"shot_02_product": 8,
	"shot_03_wheels":  8,
	"shot_04_travel":  6,
	"shot_05_cta":     6,
}
```

The generated `segments` array must have starts `0`, `6`, `14`, `22`, and `28`, totaling 34 seconds.

- [ ] **Step 2: Set audio tracks to 34 seconds**

Set voiceover and BGM `duration_sec` to 34. Keep BGM ducking sidechain role as `voiceover`.

- [ ] **Step 3: Rename output**

Use:

```json
"workspace_path":"/workspace/output/yuexing-dynamic-remotion-final.mp4"
```

### Task 4.5: Add Smoke Script

**Files:**
- Create: `scripts/smoke-m12-dynamic-remotion-route.sh`

- [ ] **Step 1: Create script**

Create executable script:

```bash
#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

export CLIPANVIL_E2E_PRODUCER_FIXTURE=motion_shot_video
export CLIPANVIL_E2E_CRAFTSMAN_FIXTURE=motion_shot_video
export CLIPANVIL_E2E_COMPOSER_FIXTURE=motion_shot_video
export CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1

echo "[m12] dev env"
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh

echo "[m12] targeted tests"
(cd apps/server && go test ./cmd/server -run 'TestMotionShotVideoFixturePlansAudioAndMotionShotVideo|TestMotionShotVideoCraftsmanFixtureVariesPlansByShotFacts|TestMotionShotVideoFixtureDispatchesAudioOnContinuationMessages|TestMotionShotVideoFixturePrioritizesFinalCompositionOverMotionShotContinuation' -count=1)
(cd apps/server && go test ./internal/agent/tools -run 'TestDispatchCraftsmanMotionOnlyPolicy' -count=1)
(cd apps/server && go test ./internal/motionshot -count=1)

cat <<'EOF'
[m12] Browser smoke steps:
1. Start the app with ./scripts/dev-start.sh using the env above.
2. Open the printed Vite URL.
3. Create an Agent workspace.
4. Upload /Users/wanwan/Desktop/box.png.
5. Ask: 用这张图生成一个 30 秒以上的悦行行李箱口播广告。不要调用 Seedance；图片可以用 Seedream；旁白和 BGM 用火山；视频用 Remotion 图片动效；需要多分镜、转场、字幕和最终成片。
6. Continue until final_video is completed.
7. Verify DB has at least 5 shots, motion_shot_video plans, no Seedance generation jobs, and final MP4 duration >= 30.
EOF
```

- [ ] **Step 2: Mark executable**

Run:

```bash
chmod +x scripts/smoke-m12-dynamic-remotion-route.sh
```

### Task 4.6: Phase 4 Verification And Commit

**Files:**
- Modified files from Tasks 4.1 through 4.5

- [ ] **Step 1: Run server fixture tests**

Run:

```bash
gofmt -w apps/server/cmd/server/e2e_producer_fixture.go apps/server/cmd/server/e2e_craftsman_fixture.go apps/server/cmd/server/e2e_composer_fixture.go apps/server/cmd/server/main_test.go
(cd apps/server && go test ./cmd/server -run 'TestMotionShotVideo' -count=1)
(cd apps/server && go test ./internal/agent/tools -run 'TestDispatchCraftsmanMotionOnlyPolicy' -count=1)
(cd apps/server && go test ./internal/motionshot -count=1)
bash -n scripts/smoke-m12-dynamic-remotion-route.sh
git diff --check
```

Expected: PASS and no diff check output.

- [ ] **Step 2: Commit Phase 4**

Run:

```bash
git add apps/server/cmd/server/e2e_producer_fixture.go \
  apps/server/cmd/server/e2e_craftsman_fixture.go \
  apps/server/cmd/server/e2e_composer_fixture.go \
  apps/server/cmd/server/main_test.go \
  scripts/smoke-m12-dynamic-remotion-route.sh
git commit -m "feat: add dynamic remotion route smoke fixture"
```

Expected: commit succeeds.

---

## Phase 5: Browser E2E Runbook And Final Agent Test

**Deliverable:** A real browser Agent E2E generates a 30s+悦行行李箱 marketing video from `/Users/wanwan/Desktop/box.png`, with no Seedance video jobs and with Remotion multi-shot final composition.

**Acceptance Gate:**

- The in-app browser opens the Agent route for a fresh workspace.
- The user product image `/Users/wanwan/Desktop/box.png` is uploaded.
- The request asks for 30s+ no-Seedance marketing video.
- The resulting DB state has at least 5 shots, at least 5 `motion_shot_video` RenderPlans, no Seedance video generation jobs, voiceover and BGM artifacts, and a completed final video artifact.
- `ffprobe` proves final MP4 duration is at least 30 seconds and has video and audio streams.
- Screenshots or artifact paths are captured for final reporting.

### Task 5.1: Write E2E Runbook

**Files:**
- Create: `docs/engineering/m12-dynamic-remotion-route-e2e.md`

- [ ] **Step 1: Add runbook**

Create:

```markdown
# M12 Dynamic Remotion Route E2E

## Purpose

Verify Agent mode keeps dynamic storyboard planning while routing no-Seedance shot videos to Remotion motion_shot_video, then composes a 30s+ final ad with voiceover and BGM.

## Required Runtime

- Production provider mode must be real for Seedream and Volcengine audio.
- Seedance video must not be used.
- Remotion motion shot provider must be available through the sandbox.
- Use `/Users/wanwan/Desktop/box.png` as the product input.

## Browser Prompt

用这张图生成一个 30 秒以上的悦行行李箱口播广告。不要调用 Seedance；图片可以用 Seedream；旁白和 BGM 用火山；视频用 Remotion 图片动效；需要 Agent 根据商品自动决定分镜数量和结构，多分镜、转场、字幕和最终成片都要完成。

## DB Evidence

Run the SQL audit from the active dev database:

```sql
select client_key, title, duration_sec, status from shot where workspace_id = $1 and archived_at is null order by sort_order;
select semantic_key, model_prompt_profile, operation, target_phase from render_plan where workspace_id = $1 order by created_at;
select semantic_key, provider, model_id, operation, status from generation_job where workspace_id = $1 order by created_at;
select semantic_key, artifact_kind, mime_type, status from artifact_version where workspace_id = $1 order by created_at;
```

Expected:

- At least 5 shots.
- `render_plan` has at least 5 `motion_shot_video` rows for `shot_video`.
- No `generation_job` has `model_id` containing `seedance`.
- Final video artifact exists.

## Media Evidence

Download the final signed URL and run:

```bash
ffprobe -v error -show_entries format=duration -show_streams -of json /path/to/final.mp4
```

Expected:

- `format.duration` is at least 30.
- A video stream exists.
- An audio stream exists.
```

- [ ] **Step 2: Run docs check**

Run:

```bash
git diff --check docs/engineering/m12-dynamic-remotion-route-e2e.md
```

Expected: no output.

### Task 5.2: Start Runtime For Browser E2E

**Files:**
- Runtime only; no source file modifications.

- [ ] **Step 1: Stop old runtime**

Run:

```bash
./scripts/dev-stop.sh
```

Expected: current worktree services stop. If nothing is running, the script exits successfully or reports no matching processes.

- [ ] **Step 2: Inspect generated dev env**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Expected: output includes a backend port and a Vite URL. Record the Vite URL.

- [ ] **Step 3: Start app**

Run:

```bash
CLIPANVIL_E2E_REQUIRE_REAL_MEDIA=1 ./scripts/dev-start.sh
```

Expected: server and web dev processes stay running. If real provider config is missing, stop and report the exact validation error.

### Task 5.3: Browser Agent E2E

**Files:**
- Runtime only; generated media artifacts must remain untracked.

- [ ] **Step 1: Open browser**

Use the in-app browser control skill to open the Vite URL from Task 5.2.

Expected: ClipAnvil web app loads.

- [ ] **Step 2: Create or open an Agent workspace**

Navigate to a fresh Agent workspace route. If the UI provides a create button, use it. If an API endpoint is the established local flow, create the workspace through the API and navigate to `/workspaces/<workspace_id>/agent`.

Expected: Agent page is visible.

- [ ] **Step 3: Upload product image**

Upload:

```text
/Users/wanwan/Desktop/box.png
```

Expected: upload appears in the workspace input or media area.

- [ ] **Step 4: Send prompt**

Send:

```text
用这张图生成一个 30 秒以上的悦行行李箱口播广告。不要调用 Seedance；图片可以用 Seedream；旁白和 BGM 用火山；视频用 Remotion 图片动效；需要 Agent 根据商品自动决定分镜数量和结构，多分镜、转场、字幕和最终成片都要完成。
```

Expected: Producer begins creating durable facts and dispatching tasks.

- [ ] **Step 5: Continue until final video completes**

If the Agent pauses for confirmation, approve only decisions that preserve:

- no Seedance
- Seedream allowed for still images
- Volcengine audio allowed for voiceover and BGM
- Remotion motion shot for all shot video
- multi-shot 30s+ final composition

Expected: final video appears in the Agent workspace.

### Task 5.4: DB And Media Audit

**Files:**
- Runtime only; evidence can be captured in terminal output and final response.

- [ ] **Step 1: Locate workspace ID**

Use the Agent URL or API response to record the workspace UUID.

- [ ] **Step 2: Run DB audit**

Use the repo's configured Postgres connection from the dev env. Run:

```sql
select count(*) as active_shots
from shot
where workspace_id = '<workspace_id>' and archived_at is null;

select count(*) as motion_shot_plans
from render_plan
where workspace_id = '<workspace_id>'
  and model_prompt_profile = 'motion_shot_video'
  and target_phase = 'shot_video';

select count(*) as seedance_jobs
from generation_job
where workspace_id = '<workspace_id>'
  and lower(model_id) like '%seedance%';

select semantic_key, artifact_kind, mime_type, status, storage_url
from artifact_version
where workspace_id = '<workspace_id>'
  and artifact_kind in ('final_video', 'shot_video', 'voiceover_audio', 'bgm_audio')
order by created_at;
```

Expected:

- `active_shots >= 5`
- `motion_shot_plans >= 5`
- `seedance_jobs = 0`
- at least one completed final video artifact

- [ ] **Step 3: Download and probe final video**

Use the app's signed download URL or MinIO path for the completed final video. Save it outside the repo, for example:

```bash
mkdir -p /tmp/clipanvil-m12-e2e
curl -L '<signed_final_video_url>' -o /tmp/clipanvil-m12-e2e/yuexing-final.mp4
ffprobe -v error -show_entries format=duration -show_streams -of json /tmp/clipanvil-m12-e2e/yuexing-final.mp4
```

Expected:

- duration is at least 30 seconds.
- one video stream exists.
- one audio stream exists.

- [ ] **Step 4: Capture browser evidence**

Take a screenshot of the completed Agent workspace and final video area.

Expected: screenshot shows final video completion and is stored outside committed files.

### Task 5.5: Phase 5 Commit

**Files:**
- Create: `docs/engineering/m12-dynamic-remotion-route-e2e.md`

- [ ] **Step 1: Commit runbook**

Run:

```bash
git add docs/engineering/m12-dynamic-remotion-route-e2e.md
git commit -m "docs: add dynamic remotion route e2e runbook"
```

Expected: commit succeeds.

---

## Final Verification

Run after all phases pass:

```bash
(cd apps/server && go test ./internal/agent/skills ./internal/agent/producer ./internal/agent/tools ./internal/motionshot ./cmd/server -count=1)
make server-build
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected:

- All tests pass.
- Server build passes.
- Web build passes.
- No whitespace errors.

Then report:

- Commit list for phases.
- Browser E2E workspace URL.
- DB audit counts.
- Final video path or signed URL.
- `ffprobe` duration and stream summary.
- Explicit statement that no Seedance generation job was created in the audited workspace.
