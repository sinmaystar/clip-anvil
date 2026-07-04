# M10.4 Reviewer Fallback Strategy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:test-driven-development. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Make Reviewer and Producer treat template video as a first-class fallback route, and stop Seedance provider failures from turning into blind same-route retries.

**Architecture:** Keep fallback decisions in Producer, but enrich the worker failure signal and Producer runtime reminder with route facts and recommended actions. Expose provider/model/rendering metadata to Reviewer context and prompt so template fallback is reviewed against template-specific standards instead of Seedance motion standards.

**Tech Stack:** Go 1.26, existing Agent Worker/Producer/Reviewer packages, existing production job/version metadata, existing `cost_risk` issue dimension.

---

## Files To Change

- Modify: `apps/server/internal/agent/reviewer/context_loader.go`
- Modify: `apps/server/internal/agent/reviewer/context_loader_test.go`
- Modify: `apps/server/internal/agent/reviewer/system_prompt.go`
- Modify: `apps/server/internal/agent/reviewer/system_prompt_test.go`
- Modify: `apps/server/internal/agent/worker/executor.go`
- Modify: `apps/server/internal/agent/worker/executor_test.go`
- Modify: `apps/server/internal/agent/producer/executor.go`
- Modify: `apps/server/internal/agent/producer/executor_test.go`
- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Create: `docs/superpowers/reports/2026-07-01-m10-4-reviewer-fallback-strategy.md`
- Modify: `docs/milestones/m10-hyperframes-template-video-provider.md`

## Task 1: Reviewer Context Understands Template Video

- [x] **Step 1: Write failing context loader test**

Add `TestContextLoaderIncludesTemplateVideoRoutingFacts` in `apps/server/internal/agent/reviewer/context_loader_test.go`.

The test should create a shot video review context where:

- node provider is `internal_template_video/hyperframes-html`
- node metadata includes `rendering_family=template_video`, `template_engine=hyperframes`, `template_key=static_fallback_ken_burns_v1`
- generation job provider is `internal_template_video`
- artifact output/provider response include template metadata

Assert `out.Text` contains:

- `Route Facts`
- `provider=internal_template_video`
- `rendering_family=template_video`
- `template_engine=hyperframes`
- `template_key=static_fallback_ken_burns_v1`
- `review_focus=readability, platform_selling_power, brand_consistency, motion_rhythm, audio_sync, truthfulness`

- [x] **Step 2: Run test and verify RED**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run TestContextLoaderIncludesTemplateVideoRoutingFacts
```

Expected: FAIL because context text does not include route facts.

- [x] **Step 3: Implement route facts summary**

Update `buildReviewContextText` to append a `Route Facts` block for non-empty provider/model/rendering metadata. Read facts from:

- `reviewContext.Node.ModelProvider`
- `reviewContext.Node.ModelID`
- `reviewContext.Node.Metadata`
- `reviewContext.Version.Output`
- `reviewContext.Version.ProviderResponse`
- `reviewContext.GenerationJob.Provider`
- `reviewContext.GenerationJob.ModelID`
- `reviewContext.GenerationJob.OperationType`
- `reviewContext.GenerationJob.ProviderResponse`

Keep the summary short and JSON-safe. Do not dump full provider response.

- [x] **Step 4: Run test and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run TestContextLoaderIncludesTemplateVideoRoutingFacts
```

Expected: PASS.

## Task 2: Reviewer Prompt Covers Template Fallback

- [x] **Step 1: Write failing system prompt test**

Add assertions in `apps/server/internal/agent/reviewer/system_prompt_test.go` that `SystemPrompt()` contains:

- `Template Video`
- `readability`
- `motion_rhythm`
- `truthfulness`
- `cost_risk`
- `requires_user_confirmation`

- [x] **Step 2: Run test and verify RED**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run TestSystemPrompt
```

Expected: FAIL if template-specific instructions are missing.

- [x] **Step 3: Add template review rules**

Update `apps/server/internal/agent/reviewer/system_prompt.go`:

- Explain Seedance vs template route distinction.
- For template video, emphasize readability, platform selling power, brand consistency, motion rhythm, audio sync, truthfulness.
- Tell Reviewer to use `accepted_with_warnings` or `rejected` with `cost_risk` / `faithfulness` issues when a template fallback contradicts a brief that explicitly required real dynamic motion.
- Tell Reviewer to set `retry_recommendation.requires_user_confirmation=true` when accepting static/semi-dynamic fallback changes user intent or quality bar.

- [x] **Step 4: Run test and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run TestSystemPrompt
```

Expected: PASS.

## Task 3: Worker Failure Signal Recommends Template Fallback Or HITL

- [x] **Step 1: Write failing worker test**

Add `TestWorkerSignalsSeedanceFailureFallbackGuidance` in `apps/server/internal/agent/worker/executor_test.go`.

Use a `shot_video` worker input with:

- `Model.Provider = "volcengine"`
- `Model.ModelID = "doubao-seedance-2-0-pro-260428"`
- `OperationType = "image_to_video_first_frame"`
- `MaxAttempts = 3`

Make `fakeProductionSubmitter` fail 3 times with a failed job whose provider/model match Seedance. Assert signal payload contains:

- `model_provider=volcengine`
- `model_id=doubao-seedance-2-0-pro-260428`
- `operation_type=image_to_video_first_frame`
- `fallback_strategy=template_fallback_or_hitl`
- `recommended_next_action=route_to_template_fallback_or_request_user_confirmation`
- `should_stop_same_route_retry=true`
- `cost_risk=true`

- [x] **Step 2: Run test and verify RED**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -run TestWorkerSignalsSeedanceFailureFallbackGuidance
```

Expected: FAIL because payload lacks fallback guidance.

- [x] **Step 3: Implement fallback signal fields**

In `wakeProducerOnFailure`, include model/provider/operation fields and call a helper such as:

```go
func fallbackGuidanceForWorkerFailure(input GenerationInput, code string, result production.RunResult) map[string]any
```

Return fallback guidance only when:

- target phase is `shot_video`
- provider/model indicate Seedance or Volcengine video
- worker reached its max attempts or generation job status is failed

Do not automatically create a template RenderPlan here. Producer must decide fallback vs HITL.

- [x] **Step 4: Run test and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/worker -run TestWorkerSignalsSeedanceFailureFallbackGuidance
```

Expected: PASS.

## Task 4: Producer Runtime Reminder Uses Fallback Guidance

- [x] **Step 1: Write failing Producer reminder test**

Add or extend a test in `apps/server/internal/agent/producer/executor_test.go` for `producerRuntimeTriggerText`.

Construct a `producerTaskTriggerPayload` with:

- `Trigger = "worker_generation_completed"`
- `RenderPlanStatus = "failed"`
- `TargetPhase = "shot_video"`
- `ModelProvider = "volcengine"`
- `ModelID = "doubao-seedance-2-0-pro-260428"`
- `FallbackStrategy = "template_fallback_or_hitl"`
- `RecommendedNextAction = "route_to_template_fallback_or_request_user_confirmation"`
- `ShouldStopSameRouteRetry = true`

Assert output contains:

- `不要继续同一路线自动重试`
- `template fallback`
- `请求用户确认`

- [x] **Step 2: Run test and verify RED**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestProducerRuntimeTriggerTextIncludesFallbackGuidance
```

Expected: FAIL because trigger text omits fallback guidance.

- [x] **Step 3: Implement payload fields and reminder text**

Extend `producerTaskTriggerPayload` with:

- `ModelProvider`
- `ModelID`
- `OperationType`
- `FallbackStrategy`
- `RecommendedNextAction`
- `ShouldStopSameRouteRetry`
- `CostRisk`

Update `producerRuntimeTriggerText` for failed `worker_generation_completed` to include a direct instruction:

- do not continue automatic same-route retries
- consider `template_video/image_to_template_video`
- request user confirmation when brief required real motion

- [x] **Step 4: Run test and verify GREEN**

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run TestProducerRuntimeTriggerTextIncludesFallbackGuidance
```

Expected: PASS.

## Verification

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer ./internal/agent/worker ./internal/agent/producer
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

If Node engine warning appears under Node 24, record it in the report but treat exit code 0 as pass.
