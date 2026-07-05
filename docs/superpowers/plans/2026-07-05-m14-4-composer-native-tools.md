# M14.4 Composer Native Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Composer native tools for `agent_remotion_code_v1` so Composer can create renderer attempts, validate sandbox code, render validated attempts, and keep the fixed `remotion_timeline_v1` path as fallback.

**Architecture:** Keep source editing in the sandbox file system and persist durable facts in DB snapshots. Add dedicated renderer attempt tools instead of overloading `render_timeline_template`; extend template enums only so dispatch and timeline plans can choose the dynamic route. Reuse M14.2 workspace/validator helpers and M14.3 `RenderAgentRemotionCode`.

**Tech Stack:** Go 1.26, pgx/sqlc generated DB queries, Composer native tools, OpenSandbox file client, Remotion sandbox job service.

---

## File Structure

- Modify `apps/server/internal/agent/tools/composer_native.go`: add `agent_remotion_code_v1` constant, extend template-key schemas/validators, and keep `render_timeline_template` on fixed template routes.
- Create `apps/server/internal/agent/tools/remotion_renderer_native.go`: implement `create_remotion_renderer_attempt`, `validate_remotion_renderer_attempt`, and `render_agent_remotion_renderer`.
- Modify `apps/server/internal/agent/tools/composer_tools_test.go`: register new tools and add behavior tests with fake DB/sandbox/render dependencies.
- Modify `apps/server/internal/agent/tools/dispatch_composer_native.go`: allow `agent_remotion_code_v1` in dispatch schema and validation.
- Modify `apps/server/internal/agent/tools/dispatch_composer_test.go`: add dispatch test for dynamic template key.
- Modify `apps/server/internal/agent/composer/tool_context_provider.go`: expose dynamic template route and attempt workflow hints in `timeline_plan_schema`.
- Modify `apps/server/cmd/server/main.go`: wire new tools into Composer registry using `queries`, `sandboxManager`, `sandboxClient`, and `sandboxJobService`.

## Task 1: Template Key Surface

**Files:**
- Modify: `apps/server/internal/agent/tools/composer_native.go`
- Modify: `apps/server/internal/agent/tools/dispatch_composer_native.go`
- Modify: `apps/server/internal/agent/composer/tool_context_provider.go`
- Test: `apps/server/internal/agent/tools/composer_tools_test.go`
- Test: `apps/server/internal/agent/tools/dispatch_composer_test.go`

- [ ] **Step 1: Write failing schema/validation tests**

Add tests that require `agent_remotion_code_v1` to appear in `create_timeline_plan`, `render_timeline_template`, and `dispatch_composer` schemas; require `validateCreateTimelinePlan` and `validateDispatchComposerInput` to accept it; and require old templates to remain valid.

```go
func TestComposerTemplateSchemasExposeAgentRemotionCode(t *testing.T) {
	tools := []NativeTool{
		NewCreateTimelinePlanNativeTool(nil),
		NewRenderTimelineTemplateNativeTool(nil),
		NewDispatchComposerNativeTool(nil, nil),
	}
	for _, tool := range tools {
		info, err := tool.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(info.ParamsOneOf)
		if !strings.Contains(string(raw), "agent_remotion_code_v1") {
			t.Fatalf("%s schema missing agent_remotion_code_v1: %s", info.Name, raw)
		}
	}
	if err := validateCreateTimelinePlan(CreateTimelinePlanInput{TemplateKey: "agent_remotion_code_v1", Plan: map[string]any{"route": "dynamic"}}); err != nil {
		t.Fatalf("create timeline should accept dynamic route: %v", err)
	}
	if err := validateDispatchComposerInput(DispatchComposerInput{Instructions: "dynamic", TemplateKey: "agent_remotion_code_v1"}); err != nil {
		t.Fatalf("dispatch should accept dynamic route: %v", err)
	}
}
```

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestComposerTemplateSchemasExposeAgentRemotionCode'
```

Expected: FAIL because schemas and validators only list `simple_concat`, `concat_with_fades`, and `remotion_timeline_v1`.

- [ ] **Step 3: Implement minimal enum support**

Add `composerTemplateAgentRemotionCodeV1 = "agent_remotion_code_v1"`, include it in jsonschema enum tags, and add it to `requireMode` calls for create/render/dispatch. Update the tool context schema:

```go
"template_key": []string{"simple_concat", "concat_with_fades", remotiontimeline.TemplateKeyV1, "agent_remotion_code_v1"},
"agent_remotion_code_v1": "dynamic Agent-authored Remotion renderer; use create_remotion_renderer_attempt, validate_remotion_renderer_attempt, render_agent_remotion_renderer, then submit_composition_artifact",
```

- [ ] **Step 4: Run tests and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestComposerTemplateSchemasExposeAgentRemotionCode|TestDispatchComposer'
```

Expected: PASS.

## Task 2: Create Attempt Tool

**Files:**
- Create: `apps/server/internal/agent/tools/remotion_renderer_native.go`
- Modify: `apps/server/internal/agent/tools/composer_tools_test.go`

- [ ] **Step 1: Write failing tests**

Add tests for tool registration and happy path:

```go
func TestCreateRemotionRendererAttemptCreatesArtifactAttemptAndWorkspace(t *testing.T) {
	store := newFakeRemotionRendererStore()
	manager := &fakeRemotionSandboxManager{sandboxID: "sbx-1"}
	client := newFakeRemotionSandboxClient()
	tool := NewCreateRemotionRendererAttemptNativeTool(store, manager, client)
	ctx := WithNativeRuntimeContext(context.Background(), NativeRuntimeContext{
		WorkspaceID: uuidWithByte(1),
		TaskID:      uuidWithByte(2),
		ScopeID:     uuidWithByte(3),
	})
	out, err := tool.InvokableRun(ctx, `{
		"timeline_plan_id":"04000000-0000-0000-0000-000000000000",
		"attempt_no":1,
		"route_policy":{"route":"agent_remotion_code_v1","rationale":"custom visual system"},
		"summary":"dynamic product ad renderer",
		"files":{"GeneratedComposition.tsx":"import {AbsoluteFill} from 'remotion'; export function AgentGeneratedComposition(){return <AbsoluteFill />;}"},
		"props":{"output":{"width":1080,"height":1920,"fps":30,"duration_sec":6}}
	}`)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "工具调用失败") {
		t.Fatalf("create attempt failed: %s", out)
	}
	if store.createdArtifact.TemplateKey != "agent_remotion_code_v1" {
		t.Fatalf("artifact route not recorded")
	}
	if store.createdAttempt.AttemptNo != 1 || store.createdAttempt.WorkspaceDir == "" {
		t.Fatalf("attempt not created with workspace dir: %#v", store.createdAttempt)
	}
	if _, ok := client.uploads[store.createdAttempt.WorkspaceDir+"/GeneratedComposition.tsx"]; !ok {
		t.Fatalf("GeneratedComposition.tsx not uploaded: %#v", client.uploads)
	}
}
```

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestComposerNativeToolsRegisterExpectedNames|TestCreateRemotionRendererAttemptCreatesArtifactAttemptAndWorkspace'
```

Expected: FAIL because constructors and tool names do not exist.

- [ ] **Step 3: Implement create tool**

Implement:

```go
const toolCreateRemotionRendererAttempt = "create_remotion_renderer_attempt"

type RemotionRendererStore interface {
	CompositionTimelineStore
	sandbox.AgentRemotionAttemptRepository
	CreateRemotionRendererArtifact(context.Context, db.CreateRemotionRendererArtifactParams) (db.RemotionRendererArtifact, error)
	GetRemotionRendererArtifact(context.Context, pgtype.UUID) (db.RemotionRendererArtifact, error)
	CreateRemotionRendererAttempt(context.Context, db.CreateRemotionRendererAttemptParams) (db.RemotionRendererAttempt, error)
	GetRemotionRendererAttempt(context.Context, pgtype.UUID) (db.RemotionRendererAttempt, error)
	SetCurrentRemotionRendererAttempt(context.Context, db.SetCurrentRemotionRendererAttemptParams) (db.RemotionRendererArtifact, error)
	UpdateRemotionRendererArtifactStatus(context.Context, db.UpdateRemotionRendererArtifactStatusParams) (db.RemotionRendererArtifact, error)
	UpdateRemotionRendererAttemptRenderResult(context.Context, db.UpdateRemotionRendererAttemptRenderResultParams) (db.RemotionRendererAttempt, error)
}
```

The tool must get the timeline plan, create or reuse the renderer artifact, write files to `/workspace/agent-remotion/<artifact>/<attempt_no>/`, create the DB attempt with snapshot hashes, and return `renderer_artifact_id`, `renderer_attempt_id`, `attempt_no`, `workspace_dir`, `source_hash`, and `props_hash`.

- [ ] **Step 4: Run tests and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestComposerNativeToolsRegisterExpectedNames|TestCreateRemotionRendererAttemptCreatesArtifactAttemptAndWorkspace'
```

Expected: PASS.

## Task 3: Validate Attempt Tool

**Files:**
- Modify: `apps/server/internal/agent/tools/remotion_renderer_native.go`
- Modify: `apps/server/internal/agent/tools/composer_tools_test.go`

- [ ] **Step 1: Write failing validation tests**

Add one passing and one failing test. The passing test should use a sandbox workspace containing `GeneratedComposition.tsx` and `props.json`, then assert attempt status becomes `validated`. The failing test should include `import fs from "fs"` and assert the tool returns `passed:false` with status `validation_failed` without returning a Go error.

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestValidateRemotionRendererAttempt'
```

Expected: FAIL because the validate tool is not implemented.

- [ ] **Step 3: Implement validate tool**

Implement `validate_remotion_renderer_attempt` to load attempt by ID, ensure sandbox, read the attempt workspace through `sandbox.ReadAgentRemotionAttemptWorkspace`, run `sandbox.ValidateAgentRemotionSnapshot`, and persist through `sandbox.PersistAgentRemotionValidation`. Return structured JSON:

```json
{
  "renderer_attempt_id": "...",
  "renderer_artifact_id": "...",
  "status": "validated",
  "passed": true,
  "workspace_dir": "/workspace/agent-remotion/.../1",
  "source_hash": "sha256:...",
  "props_hash": "sha256:...",
  "issues": []
}
```

- [ ] **Step 4: Run tests and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestValidateRemotionRendererAttempt'
```

Expected: PASS.

## Task 4: Render Attempt Tool

**Files:**
- Modify: `apps/server/internal/agent/tools/remotion_renderer_native.go`
- Modify: `apps/server/internal/agent/tools/composer_tools_test.go`

- [ ] **Step 1: Write failing render tests**

Add tests that `render_agent_remotion_renderer` rejects an unvalidated attempt, renders a validated attempt through `RenderAgentRemotionCode`, writes `render_result` and `sandbox_job_id`, updates current artifact attempt, and returns data that can be passed to `submit_composition_artifact`.

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestRenderAgentRemotionRenderer'
```

Expected: FAIL because render tool is not implemented.

- [ ] **Step 3: Implement render tool**

Add a `RemotionRendererSandbox` interface with:

```go
RenderAgentRemotionCode(context.Context, sandbox.RenderAgentRemotionCodeInput) (sandbox.SandboxJobResult, error)
```

The tool must reject attempts whose status is not `validated`, default output path to `/workspace/output/final-agent-<timeline_short>.mp4`, call `RenderAgentRemotionCode`, store render metadata with `UpdateRemotionRendererAttemptRenderResult`, call `SetCurrentRemotionRendererAttempt` with status `rendered`, and return `output_path`, `sandbox_job_id`, `renderer_artifact_id`, `renderer_attempt_id`, `timeline_plan_id`, `status`, and `result_for_timeline_plan`.

- [ ] **Step 4: Run tests and confirm GREEN**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools -run 'TestRenderAgentRemotionRenderer'
```

Expected: PASS.

## Task 5: Server Wiring And Regression Checks

**Files:**
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/agent/tools/composer_tools_test.go`

- [ ] **Step 1: Write/extend registration test**

Extend `TestComposerNativeToolsRegisterExpectedNames` so the expected set includes:

```go
"create_remotion_renderer_attempt",
"validate_remotion_renderer_attempt",
"render_agent_remotion_renderer",
```

- [ ] **Step 2: Wire production registry**

In the Composer registry in `apps/server/cmd/server/main.go`, add:

```go
agenttools.NewCreateRemotionRendererAttemptNativeTool(queries, sandboxManager, sandboxClient),
agenttools.NewValidateRemotionRendererAttemptNativeTool(queries, sandboxManager, sandboxClient),
agenttools.NewRenderAgentRemotionRendererNativeTool(queries, sandboxJobService),
```

- [ ] **Step 3: Run focused regression tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools ./internal/agent/composer ./internal/sandbox ./internal/remotiontimeline
```

Expected: PASS, including old `simple_concat`, `concat_with_fades`, and `remotion_timeline_v1` tests.

## Task 6: Acceptance Verification

**Files:**
- All files touched in M14.4.

- [ ] **Step 1: Format Go**

Run:

```bash
gofmt -w apps/server/internal/agent/tools/composer_native.go apps/server/internal/agent/tools/dispatch_composer_native.go apps/server/internal/agent/tools/remotion_renderer_native.go apps/server/internal/agent/tools/composer_tools_test.go apps/server/internal/agent/tools/dispatch_composer_test.go apps/server/internal/agent/composer/tool_context_provider.go apps/server/cmd/server/main.go
```

- [ ] **Step 2: Run M14.4 acceptance tests**

Run:

```bash
cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test -count=1 ./internal/agent/tools ./internal/agent/composer ./internal/sandbox ./internal/remotiontimeline
```

Expected: PASS.

- [ ] **Step 3: Run server build from repo root**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS. This uses repo-root Makefile rather than `cd apps/server && make server-build`.

- [ ] **Step 4: Run diff whitespace check**

Run:

```bash
git diff --check
```

Expected: no output.

- [ ] **Step 5: Confirm milestone acceptance**

Manually confirm:

- Composer tool schema contains `agent_remotion_code_v1`.
- Composer registry contains the three new renderer tools.
- `render_agent_remotion_renderer` rejects attempts that have not passed validation.
- A later attempt number can be created after editing sandbox files, then validate/rendered independently.
- Existing fixed-template Composer tests still pass.
