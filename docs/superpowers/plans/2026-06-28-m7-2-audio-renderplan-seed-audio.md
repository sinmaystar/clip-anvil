# M7.2 Audio RenderPlan and Seed Audio Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let an approved `AudioPlan` produce two executable audio RenderPlans, `voiceover_audio` and `bgm_audio`, and let Worker/shared production generate stored audio artifacts through Volcengine `seed-audio-1.0`.

**Architecture:** M7.2 extends the existing RenderPlan -> Worker -> shared production path instead of adding a parallel audio executor. `audio_plan` becomes a valid RenderPlan scope, Craftsman can load the approved AudioPlan as task context, Worker maps audio RenderPlans into `GenerationIntent{OutputType:"audio", OperationType:"text_to_audio"}`, and Volcengine production runtime routes audio output to a new seed-audio runtime. Composer mixing is intentionally deferred to M7.3.

**Tech Stack:** Go 1.26, PostgreSQL 16, pgx/sqlc, CloudWeGo Eino native tools, existing Agent Craftsman/Worker executors, ClipAnvil production service, MinIO artifact storage, Volcengine audio generation HTTP (`seed-audio-1.0`, official page last updated 2026-06-24).

---

## Current Code Facts

- M7.1 added `audio_plan`, `upsert_audio_plan`, Producer prompt/context/PSS support, and one-active-plan database enforcement.
- `render_plan` currently rejects `scope_type=audio_plan`, `target_phase=voiceover_audio` / `bgm_audio`, and `model_prompt_profile=seed_audio_1`.
- `upsert_render_plan` schema only documents image/video phases, profiles, and operations.
- Craftsman context loader only supports `scope_type=shot` and `scope_type=key_element_state`.
- `VolcengineProductionRuntime.Start` explicitly rejects `OutputType="audio"` with an audio hold error.
- `model_capability` still has disabled `volcengine-audio-hold`; M7.2 must insert enabled `seed-audio-1.0`.
- `.env.example` already has `CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL=` but the default model is blank.

## Task 1: Extend RenderPlan Schema and Service for Audio

**Files:**
- Create: `apps/server/migrations/033_m7_2_audio_render_plan.sql`
- Modify: `apps/server/internal/agent/renderplan/types.go`
- Modify: `apps/server/internal/agent/renderplan/service.go`
- Modify tests: `apps/server/internal/agent/renderplan/service_test.go`
- Modify: `apps/server/internal/agent/tools/upsert_render_plan.go`
- Modify tests: `apps/server/internal/agent/tools/render_plan_tools_test.go`

- [x] **Step 1: Write failing renderplan service tests**

Add tests proving:
- `scope_type=audio_plan`, `target_phase=voiceover_audio`, `model_prompt_profile=seed_audio_1`, `operation=text_to_audio` validates.
- `target_phase=bgm_audio` also validates with `seed_audio_1`.
- `audio_plan` scope rejects image/video phases.
- `voiceover_audio` / `bgm_audio` reject `seedream_5_image` and `seedance_2_video`.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/renderplan -run 'Audio' -count=1)
```

Expected: FAIL because constants and validation do not support audio.

- [x] **Step 2: Add migration for RenderPlan constraints**

Create `apps/server/migrations/033_m7_2_audio_render_plan.sql`:

```sql
-- +goose Up
ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_scope_type_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_scope_type_check
    CHECK (scope_type IN ('key_element_state', 'shot', 'audio_plan'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_target_phase_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_target_phase_check
    CHECK (target_phase IN ('reference_image', 'preview_image', 'shot_video', 'voiceover_audio', 'bgm_audio'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video', 'seed_audio_1'));

-- +goose Down
ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_profile_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_profile_check
    CHECK (model_prompt_profile IN ('seedream_5_image', 'seedance_2_video'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_target_phase_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_target_phase_check
    CHECK (target_phase IN ('reference_image', 'preview_image', 'shot_video'));

ALTER TABLE render_plan DROP CONSTRAINT IF EXISTS render_plan_scope_type_check;
ALTER TABLE render_plan ADD CONSTRAINT render_plan_scope_type_check
    CHECK (scope_type IN ('key_element_state', 'shot'));
```

- [x] **Step 3: Extend renderplan constants and validation**

Add:
- `ScopeAudioPlan = "audio_plan"`
- `PhaseVoiceoverAudio = "voiceover_audio"`
- `PhaseBGMAudio = "bgm_audio"`
- `ProfileSeedAudio1 = "seed_audio_1"`

Validation rules:
- `scope_type=audio_plan` only permits `voiceover_audio` or `bgm_audio`.
- `voiceover_audio` / `bgm_audio` require `model_prompt_profile=seed_audio_1`.
- audio phases require `operation=text_to_audio`.
- `seed_audio_1` cannot be used by image/video phases.
- `prompt_parts.objective` and `rationale` remain required.

- [x] **Step 4: Extend `upsert_render_plan` native tool schema**

Update `UpsertRenderPlanToolInput` JSON schema descriptions and enums:
- scope type includes `audio_plan`.
- target phase includes `voiceover_audio`, `bgm_audio`.
- model profile includes `seed_audio_1`.
- operation includes `text_to_audio`.
- params description includes `speaker`, `format`, `sample_rate`, `speech_rate`, `pitch_rate`, `loudness_rate`, `watermark`.

Update runtime defaults:
- audio target phases default scope to current task `audio_plan`.
- `voiceover_audio` / `bgm_audio` default `task_type=generate`, `model_prompt_profile=seed_audio_1`, `operation=text_to_audio`.

- [x] **Step 5: Verify Task 1**

Run:

```bash
make sqlc-generate
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/renderplan -run 'Audio|Service' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'RenderPlan|Audio' -count=1)
git diff --check
```

Expected: PASS.

## Task 2: Dispatch Craftsman for AudioPlan Scope

**Files:**
- Modify: `apps/server/internal/agent/tools/dispatch_craftsman_native.go`
- Modify tests: `apps/server/internal/agent/tools/dispatch_craftsman_test.go`
- Modify: `apps/server/internal/agent/craftsman/context_loader.go`
- Modify tests: `apps/server/internal/agent/craftsman/*_test.go`
- Modify: `apps/server/internal/agent/craftsman/system_prompt.go`

- [x] **Step 1: Write failing dispatch tests**

Add tests proving Producer can call:
- `dispatch_craftsman(scope.type=audio_plan, scope.key=audio_plan.active, target_phase=voiceover_audio)`
- `dispatch_craftsman(scope.type=audio_plan, scope.key=audio_plan.active, target_phase=bgm_audio)`

The tool must resolve the active approved AudioPlan and create a Craftsman task with audio scope and target phase.

- [x] **Step 2: Add AudioPlan context loading**

Extend Craftsman context store with:
- `GetAudioPlan`
- `GetActiveAudioPlanByWorkspace`
- `ListActiveShotsByWorkspace`

When `scope_type=audio_plan`, load:
- approved AudioPlan fields.
- cue plan.
- active shots ordered by sort order.
- existing audio RenderPlans for the AudioPlan.
- recent Craftsman thread messages.

Block context loading if the active AudioPlan is missing or not `approved`.

- [x] **Step 3: Update Craftsman prompt**

Add audio-specific rules:
- For `target_phase=voiceover_audio`, create exactly one RenderPlan scoped to the approved AudioPlan.
- For `target_phase=bgm_audio`, create exactly one independent RenderPlan scoped to the same AudioPlan.
- Do not combine voiceover and BGM in one RenderPlan.
- `model_prompt_profile=seed_audio_1`, `operation=text_to_audio`, `output_type=audio`.
- BGM first version must use `seed-audio-1.0`; do not use uploaded audio or a library.
- `generation_text` should be concise enough for provider prompt limits and should include voice/BGM style, duration target, cue/script, and language.

- [x] **Step 4: Verify Task 2**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'DispatchCraftsman.*Audio|AudioPlan' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/craftsman -run 'Audio|Context' -count=1)
git diff --check
```

Expected: PASS.

## Task 3: Map Audio RenderPlans to Worker Generation

**Files:**
- Modify: `apps/server/internal/agent/worker/types.go`
- Modify: `apps/server/internal/agent/worker/executor.go`
- Modify tests: `apps/server/internal/agent/worker/executor_test.go`
- Modify: `apps/server/internal/agent/tools/render_plan_submitter.go`
- Modify tests: `apps/server/internal/agent/tools/render_plan_tools_test.go`
- Modify: `apps/server/sqlc/queries/audio_plan.sql`
- Modify generated: `apps/server/internal/store/db/audio_plan.sql.go`

- [x] **Step 1: Write failing worker tests**

Add tests proving:
- `voiceover_audio` creates or resolves an audio `media_node` with `node_type=audio`, `operation_type=text_to_audio`, and `agent_artifact_kind=voiceover_audio`.
- `bgm_audio` creates audio node with `agent_artifact_kind=bgm_audio`.
- submitted `GenerationIntent` has `OutputType="audio"`, `OperationType="text_to_audio"`, Volcengine provider/model from RenderPlan params or defaults, and prompt from compiled RenderPlan.
- successful Worker submission updates the matching `audio_plan.voiceover_node_id` / `bgm_node_id` and render plan output fields.

- [x] **Step 2: Extend worker generation spec**

Update generation spec helpers so:
- `target_phase=voiceover_audio` maps to `OutputType=audio`, `OperationType=text_to_audio`, `ArtifactKind=voiceover_audio`, `SourcePhase=voiceover_audio`.
- `target_phase=bgm_audio` maps to `OutputType=audio`, `OperationType=text_to_audio`, `ArtifactKind=bgm_audio`, `SourcePhase=bgm_audio`.
- audio scopes do not require `shot_id`.
- audio target node title and semantic key use `audio_plan.active.voiceover_audio` / `audio_plan.active.bgm_audio`.

- [x] **Step 3: Add audio_plan linkage queries**

Add sqlc queries:
- `SetAudioPlanVoiceoverRenderPlan`
- `SetAudioPlanBGMRenderPlan`
- `SetAudioPlanVoiceoverNode`
- `SetAudioPlanBGMNode`
- `UpdateAudioPlanTimelinePlan` remains for M7.3 if needed.

Worker or submitter should connect RenderPlan IDs when submitting and node IDs when production is submitted.

- [x] **Step 4: Verify Task 3**

Run:

```bash
make sqlc-generate
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -run 'Audio|Generation' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'RenderPlan.*Audio|Submit' -count=1)
git diff --check
```

Expected: PASS.

## Task 4: Add Volcengine Seed Audio Runtime

**Files:**
- Create: `apps/server/internal/production/volcengine_audio.go`
- Create: `apps/server/internal/production/volcengine_audio_test.go`
- Modify: `apps/server/internal/production/volcengine_runtime.go`
- Modify tests: `apps/server/internal/production/volcengine_runtime_test.go`
- Modify: `apps/server/internal/production/provider.go`
- Modify tests: `apps/server/internal/production/service_test.go`
- Modify: `apps/server/internal/production/mock_provider.go`

- [x] **Step 1: Write failing runtime tests**

Add tests proving:
- Volcengine runtime no longer rejects `OutputType="audio"` when `AudioModel=seed-audio-1.0`.
- missing API key fails before network.
- missing `AudioModel` fails with provider config error.
- request builder includes `model=seed-audio-1.0`, `text_prompt`, `speaker`, `format`, `sample_rate`, `speech_rate`, `pitch_rate`, `loudness_rate`, and `watermark`.
- response with base64 audio returns `ProviderResult.AssetContent`, `AssetMIME`, provider request/response metadata.
- response with temporary URL returns `AssetSourceURL`, so production service uploads it into ClipAnvil object storage.

- [x] **Step 2: Implement audio runtime**

Create `VolcengineAudioRuntime` with an injectable HTTP client/test client. It should:
- use `intent.EffectivePrompt()` as `text_prompt`.
- default model from `cfg.AudioModel`.
- reject non-`text_to_audio` operations.
- map format to MIME:
  - `mp3` -> `audio/mpeg`
  - `wav` -> `audio/wav`
  - `ogg_opus` -> `audio/ogg`
  - `pcm` -> `audio/L16`
- preserve provider response in `ProviderResponse`.
- avoid depending on 2-hour provider URLs by returning content when base64 is present and `AssetSourceURL` only as a fallback for production upload.

- [x] **Step 3: Route audio in runtime and mock provider**

Modify `VolcengineProductionRuntime.Start`:
- `OutputType="audio"` routes to audio runtime.

Modify `MockProvider.Run`:
- `OutputType="audio"` returns a tiny deterministic WAV or MP3 fixture with `AssetMIME`.

Modify default model selection:
- `defaultModelForOutput("audio")` returns `cfg.Volcengine.AudioModel` in real Volcengine mode.

- [x] **Step 4: Verify Task 4**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'Volcengine.*Audio|Audio|ProviderRegistry|MockProvider' -count=1)
git diff --check
```

Expected: PASS.

## Task 5: Capability, Env, and Local Config

**Files:**
- Create: `apps/server/migrations/034_m7_2_seed_audio_capability.sql`
- Modify: `.env.example`
- Modify ignored local file: `.env`
- Modify: `apps/server/internal/config/config_test.go`
- Modify docs if needed: `docs/milestones/m7-agent-audio-plan-composer.md`

- [ ] **Step 1: Add model capability migration**

Create a migration that:
- disables or supersedes `volcengine-audio-hold`.
- inserts enabled `model_capability(provider_id='volcengine', model_id='seed-audio-1.0')`.
- sets:
  - `output_types=["audio"]`
  - `supported_operations=["text_to_audio"]`
  - `supported_input_node_types=["text","audio"]`
  - limits for max prompt chars, 120s output duration, supported formats, supported sample rates, reference audio count/size.
  - defaults `{"format":"mp3","sample_rate":48000,"watermark":false}`.

- [ ] **Step 2: Update env examples and local `.env`**

Update `.env.example`:

```bash
CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_MODEL=seed-audio-1.0
CLIPANVIL_PRODUCTION_VOLCENGINE_AUDIO_DEFAULT_SPEAKER=
```

Write the same keys into ignored local `.env` if missing. Keep secrets blank unless the user has already set real values.

- [ ] **Step 3: Verify config loading**

If adding a default speaker config field, extend `config.go` and config tests. If storing speaker only in RenderPlan params, no new config field is needed; document that `AUDIO_DEFAULT_SPEAKER` is reserved for a follow-up.

- [ ] **Step 4: Verify Task 5**

Run:

```bash
make sqlc-generate
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config ./internal/agent/modelselection -count=1)
git diff --check
git check-ignore -v .env
```

Expected: PASS, and `.env` is ignored.

## Task 6: End-to-End Mock Audio Smoke

**Files:**
- Create or modify smoke script under `scripts/` only if an existing smoke pattern fits.
- Modify: `docs/milestones/m7-agent-audio-plan-composer.md`

- [ ] **Step 1: Add focused smoke or integration test**

Prove the mock path:
- approved AudioPlan exists.
- `voiceover_audio` and `bgm_audio` RenderPlans are created and accepted.
- Worker submits both generation intents.
- two audio media nodes and artifact versions are created.
- audio_plan has render plan and node links.

- [ ] **Step 2: Run full M7.2 verification**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
git check-ignore -v .env
```

If Volcengine credentials are available in local `.env`, run one manual real-provider smoke for `seed-audio-1.0`. If credentials are absent, record that real-provider smoke was not executed.

- [ ] **Step 3: Update milestone and commit**

Update `docs/milestones/m7-agent-audio-plan-composer.md` M7.2 row with verification evidence.

Commit:

```bash
git status --short
git add .env.example \
  apps/server/migrations/033_m7_2_audio_render_plan.sql \
  apps/server/migrations/034_m7_2_seed_audio_capability.sql \
  apps/server/sqlc/queries/audio_plan.sql \
  apps/server/internal/store/db \
  apps/server/internal/agent \
  apps/server/internal/production \
  apps/server/internal/config \
  docs/milestones/m7-agent-audio-plan-composer.md \
  docs/superpowers/plans/2026-06-28-m7-2-audio-renderplan-seed-audio.md
git diff --cached --check
git commit -m "feat: add seed audio render plan generation"
```

Do not stage `.env`; it is local ignored configuration.

## Plan Self-Review

- Spec coverage: This plan covers M7.2 only: audio RenderPlan scope/phase/profile, Craftsman dispatch/context, Worker mapping, Volcengine seed-audio provider, model capability, `.env`, and mock smoke. Composer mixing and final review remain M7.3/M7.4.
- Placeholder scan: No `TBD` or open-ended "add tests" items remain; each task names exact files, expected behavior, and verification commands.
- Type consistency: The plan consistently uses `audio_plan`, `voiceover_audio`, `bgm_audio`, `seed_audio_1`, `seed-audio-1.0`, and `text_to_audio`.
