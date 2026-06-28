# M7.4 Workbench Audio Projection and Final Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show the M7 audio plan, generated audio artifacts, final mix summary, and final review status in Workbench / overview, and make Producer explicitly decide whether to dispatch final video review after Composer completes.

**Architecture:** M7.4 does not introduce a new audio executor. It projects existing facts from `audio_plan`, `media_node`, `artifact_version`, `timeline_plan`, `review_record`, and `artifact_issue` into the existing production overview and Agent workbench APIs, then renders those fields in the React workbench. Producer already receives `composition_completed`; M7.4 tightens that trigger/prompt so it reads current state and either calls `dispatch_reviewer(final_video_review)` or requests user confirmation. Reviewer already supports `final_video_review` with `audio_sync`; M7.4 enriches final-video context with AudioPlan and timeline audio tracks.

**Tech Stack:** Go 1.26, PostgreSQL/sqlc models already generated, Hertz API handlers, existing Agent overview/workbench projection, React 19, Vite 8, TypeScript 6, TailwindCSS 4, existing Reviewer/Producer native tools.

---

## Current Code Facts

- M7.1-M7.3 are complete in `docs/milestones/m7-agent-audio-plan-composer.md`.
- `agent_workbench_projection.go` already returns `final_output`, but `AgentWorkbenchFinalOutput` has no `audio_plan`, `audio_tracks`, `audio_assets`, or final review fields.
- `agent_canvas_detail.go` already has an overview detail field `AudioPlan any`, but overview detail does not populate the active AudioPlan, and final output detail does not summarize audio tracks or reviews.
- `overview.Builder` returns shots, timeline, and final outputs, but has no audio plan summary or audio generation counters.
- `dispatch_reviewer` already supports `final_video_review`, checks duplicate terminal final-video reviews, and validates succeeded artifact versions.
- Reviewer already has `ReviewTaskFinalVideo`, `TargetPhaseFinalVideo`, and required `audio_sync` axis.
- Reviewer final-video context can load a final video without a shot, but does not explicitly summarize active AudioPlan or timeline audio tracks.
- Producer runtime trigger for `composition_completed` says to decide whether to dispatch Reviewer, but it does not explicitly mention AudioPlan, final audio, `audio_sync`, or duplicate review handling.

## API Contract Additions

Backend workbench should add:

```json
{
  "overview": {
    "audio_plan": {
      "id": "...",
      "status": "composing",
      "title": "...",
      "semantic_key": "audio_plan.active",
      "voiceover_script": "...",
      "voiceover_status": "succeeded",
      "bgm_status": "succeeded",
      "timeline_plan_id": "..."
    }
  },
  "final_output": {
    "audio_summary": {
      "has_voiceover": true,
      "has_bgm": true,
      "audio_codec": "aac",
      "track_count": 2,
      "ducking": true
    },
    "audio_tracks": [
      {"role": "voiceover", "asset_id": "...", "volume": 1, "duration_sec": 12},
      {"role": "bgm", "asset_id": "...", "volume": 0.28, "duration_sec": 12, "ducking": true}
    ],
    "final_review": {
      "review_task": "final_video_review",
      "target_phase": "final_video",
      "status": "accepted_with_warnings",
      "verdict": "accepted_with_warnings",
      "score": 0.86
    }
  },
  "counts": {
    "audio_ready": 2,
    "audio_missing": 0,
    "final_reviews": 1
  }
}
```

Frontend workbench should render:
- overview node: AudioPlan status, voiceover/BGM status, short script/BGM direction;
- final output node: audio track count, AAC/mix status, final review verdict;
- detail panel overview: AudioPlan script, voice profile, BGM plan, cue plan;
- detail panel final output: audio tracks and `audio_sync` review issues.

## Task 1: Backend Workbench Audio Projection

**Files:**
- Modify: `apps/server/internal/api/agent_workbench_projection.go`
- Modify tests: `apps/server/internal/api/agent_workbench_projection_test.go`
- Modify TypeScript contract later in Task 3: `apps/web/src/lib/agentWorkbench.ts`

- [x] **Step 1: Write failing API projection tests**

Add tests proving:
- `agentWorkbenchOverviewResponse` includes active AudioPlan fields.
- `agentWorkbenchFinalOutputFromTimelinePlan` extracts `audio_tracks` from `timeline_plan.plan_json`.
- final output `audio_summary` reports `has_voiceover`, `has_bgm`, `track_count`, `ducking`, and `audio_codec`.
- final output includes latest final-video review summary when review target is final video.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'AgentWorkbench.*Audio|FinalOutput.*Review' -count=1)
```

Expected: FAIL because these fields are not defined.

- [x] **Step 2: Add response structs and helpers**

Add:
- `agentWorkbenchAudioPlanResponse`
- `agentWorkbenchAudioTrackResponse`
- `agentWorkbenchAudioSummaryResponse`
- `agentWorkbenchFinalReview` reuse via existing `agentWorkbenchReviewSummaryResponse`

Add helpers:
- `agentWorkbenchAudioPlanSummary(audioPlan db.AudioPlan, nodesByID map[pgtype.UUID]db.MediaNode) agentWorkbenchAudioPlanResponse`
- `agentWorkbenchAudioTracks(planJSON []byte) []agentWorkbenchAudioTrackResponse`
- `agentWorkbenchAudioSummary(planJSON []byte, resultJSON []byte) agentWorkbenchAudioSummaryResponse`
- `latestFinalVideoReview(reviews []db.ReviewRecord, outputNodeID pgtype.UUID, versionID pgtype.UUID) *agentWorkbenchReviewSummaryResponse`

- [x] **Step 3: Load and project active AudioPlan**

In `buildAgentWorkbenchProjection`:
- call `queries.GetActiveAudioPlanByWorkspace(ctx, workspaceID)`;
- ignore `pgx.ErrNoRows`;
- add `response.Overview.AudioPlan`;
- pass `audioPlan` into final output projection so `audio_plan.timeline_plan_id` can be used as an authoritative link when present.

- [x] **Step 4: Project final review and counts**

Extend `agentWorkbenchCountsResponse`:
- `AudioReady int json:"audio_ready"`
- `AudioMissing int json:"audio_missing"`
- `FinalReviews int json:"final_reviews"`

Count voiceover/BGM node statuses from active AudioPlan.
Count terminal or running `final_video_review` records.

- [x] **Step 5: Verify Task 1**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'AgentWorkbench.*Audio|FinalOutput.*Review' -count=1)
git diff --check
```

Expected: PASS.

Actual:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'AgentWorkbench.*Audio|FinalOutput.*Review|FinalOutputFromTimelinePlan' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -count=1)
```

Result: PASS.

## Task 2: Production Overview and Detail API Audio Fields

**Files:**
- Modify: `apps/server/internal/agent/overview/types.go`
- Modify: `apps/server/internal/agent/overview/builder.go`
- Modify tests: `apps/server/internal/agent/overview/builder_test.go`
- Modify: `apps/server/internal/api/agent_canvas_detail.go`
- Modify tests: `apps/server/internal/api/agent_canvas_detail_test.go`

- [x] **Step 1: Write failing overview tests**

Add tests proving production overview includes:
- `audio_plan.status`;
- `audio_plan.voiceover_status`;
- `audio_plan.bgm_status`;
- `audio_plan.timeline_plan_id`;
- counts for `audio_ready` and `final_reviews`.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/overview -run 'AudioPlan|FinalReview' -count=1)
```

Expected: FAIL because overview store and response types do not expose AudioPlan.

- [x] **Step 2: Extend overview store and response**

Add to `overview.Store`:
- `GetActiveAudioPlanByWorkspace(ctx, workspaceID pgtype.UUID) (db.AudioPlan, error)`

Add:
- `AudioPlan *AudioPlanSummary json:"audio_plan,omitempty"`
- `Counts.AudioReady`
- `Counts.AudioMissing`
- `Counts.FinalReviews`

- [x] **Step 3: Populate overview audio summary**

In `Builder.Build`:
- load active AudioPlan;
- derive voiceover/BGM status from linked node IDs and existing node list;
- ignore `pgx.ErrNoRows`;
- count final-video review records from existing `reviews`.

- [x] **Step 4: Write failing detail API tests**

Add tests proving:
- overview detail includes active AudioPlan JSON;
- final output detail includes `audio_summary`, `audio_tracks`, and final review summaries;
- `audio_sync` issues appear in the final output detail issue list.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'AgentCanvas.*Audio|FinalOutput.*Audio' -count=1)
```

Expected: FAIL until detail response fields are populated.

- [x] **Step 5: Populate detail API fields**

In `agent_canvas_detail.go`:
- populate overview `AudioPlan`;
- extend `agentCanvasFinalOutputDetailResponse` with `AudioSummary`, `AudioTracks`, `FinalReviews`;
- load reviews by output node or artifact version;
- keep existing `Plan` and `Result` JSON.

- [x] **Step 6: Verify Task 2**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/overview -run 'AudioPlan|FinalReview' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'AgentCanvas.*Audio|FinalOutput.*Audio' -count=1)
git diff --check
```

Expected: PASS.

Actual:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/overview -run 'AudioPlan|FinalReview' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'AgentCanvas.*Audio|FinalOutput.*Audio' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/overview -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -count=1)
```

Result: PASS.

## Task 3: Frontend Workbench Audio UI

**Files:**
- Modify: `apps/web/src/lib/agentWorkbench.ts`
- Modify: `apps/web/src/lib/agentApi.ts`
- Modify: `apps/web/src/components/agent-workbench/AgentProjectOverviewNode.tsx`
- Modify: `apps/web/src/components/agent-workbench/AgentFinalOutputNode.tsx`
- Modify: `apps/web/src/components/agent-workbench/AgentCanvasDetailPanel.tsx`
- Modify CSS where existing workbench styles live: `apps/web/src/main.css`

- [x] **Step 1: Add TypeScript API types**

Extend types:
- `AgentWorkbenchOverview.audio_plan?: AgentWorkbenchAudioPlan`
- `AgentWorkbenchFinalOutput.audio_summary?: AgentWorkbenchAudioSummary`
- `AgentWorkbenchFinalOutput.audio_tracks?: AgentWorkbenchAudioTrack[]`
- `AgentWorkbenchFinalOutput.final_review?: AgentWorkbenchReviewSummary`
- `AgentWorkbenchCounts.audio_ready`, `audio_missing`, `final_reviews`
- matching `AgentCanvasDetail` final output fields in `agentApi.ts`.

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: FAIL until components consume the new optional fields correctly or generated type mismatches are fixed.

- [x] **Step 2: Render AudioPlan on overview node**

In `AgentProjectOverviewNode`:
- show a compact audio row with AudioPlan status;
- show voiceover/BGM statuses;
- show a clipped voiceover script or BGM direction if present;
- do not add instructional text or marketing copy.

- [x] **Step 3: Render final output audio and review summary**

In `AgentFinalOutputNode`:
- show `audio_summary.audio_codec`, track count, ducking status;
- show `final_review.verdict` and score when present;
- keep node height stable and avoid text overflow.

- [x] **Step 4: Render audio details in detail panel**

In `AgentCanvasDetailPanel`:
- overview detail shows AudioPlan script, voice profile, BGM plan, cue plan;
- final output detail shows audio summary, audio tracks, final review and issues.

- [x] **Step 5: Verify frontend**

Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: PASS.

Actual:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Result: PASS. Both commands reported the existing Node engine warning because this shell is on Node v24.14.0 while the repo expects Node >=26 <27.

## Task 4: Producer Post-Composer Review Decision

**Files:**
- Modify: `apps/server/internal/agent/producer/executor.go`
- Modify tests: `apps/server/internal/agent/producer/executor_test.go`
- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify: `apps/server/internal/agent/producer/system_prompt.go`
- Modify tests if present: `apps/server/internal/agent/producer/*_test.go`

- [x] **Step 1: Write failing trigger tests**

Add tests proving `producerRuntimeTriggerText(composition_completed)` says:
- read project context;
- confirm final video node/artifact;
- confirm AudioPlan and final audio track state;
- decide whether to call `dispatch_reviewer(final_video_review)`;
- avoid duplicate final review when one already exists;
- request user confirmation if review should be skipped.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'CompositionSignals|FinalReview|Audio' -count=1)
```

Expected: FAIL because current trigger text lacks explicit audio/final review instructions.

- [x] **Step 2: Update Producer reminder and prompt**

Update `composition_completed` reminder:
- mention AudioPlan, voiceover/BGM, final audio track, `audio_sync`;
- tell Producer to dispatch final review when final artifact is reviewable and no terminal final review exists;
- tell Producer to request user confirmation or explain skip reason when not dispatching review.

Update Producer system prompt:
- after Composer completes, Producer owns whether final review runs;
- Reviewer final video review must cover audio_sync and platform selling power;
- do not silently mark final video complete without either final review or explicit user-facing rationale.

- [x] **Step 3: Verify Task 4**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'CompositionSignals|FinalReview|Audio|SystemPrompt' -count=1)
git diff --check
```

Expected: PASS.

Actual:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'CompositionSignals|FinalReview|Audio|SystemPromptStable' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -count=1)
```

Result: PASS.

## Task 5: Reviewer Final Video Audio Context

**Files:**
- Modify: `apps/server/internal/agent/reviewer/context_loader.go`
- Modify tests: `apps/server/internal/agent/reviewer/context_loader_test.go`
- Modify: `apps/server/internal/agent/reviewer/system_prompt.go`
- Modify tests: `apps/server/internal/agent/reviewer/system_prompt_test.go`

- [x] **Step 1: Write failing reviewer context tests**

Add tests proving final video review context includes:
- active AudioPlan status/script/BGM direction from Producer PSS or direct loader fields;
- final timeline `audio_tracks` summary when the final node/timeline metadata exposes it;
- required `audio_sync` axis is described in the prompt for final video review.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'FinalVideo.*Audio|SystemPrompt' -count=1)
```

Expected: FAIL until context/prompt mentions audio review facts explicitly.

- [x] **Step 2: Enrich final review context**

Keep direct DB dependencies minimal:
- rely on Producer PSS text for active AudioPlan when available;
- add generation job/provider metadata and node metadata/result JSON snippets only when compact;
- explicitly add a `Final Audio Review Focus` section for final video phase.

- [x] **Step 3: Update Reviewer prompt**

Ensure prompt says final video review must judge:
- voiceover/BGM presence and relative volume;
- BGM ducking under narration;
- audio/visual timing and continuity;
- whether audio supports the marketing objective without overpowering product clarity.

- [x] **Step 4: Verify Task 5**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'FinalVideo.*Audio|SystemPrompt' -count=1)
git diff --check
```

Expected: PASS.

Actual:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -run 'FinalVideo.*Audio|FinalVideoReviewContext|SystemPrompt' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/reviewer -count=1)
```

Result: PASS.

## Task 6: Full M7.4 Verification and Milestone Update

**Files:**
- Modify: `docs/milestones/m7-agent-audio-plan-composer.md`
- Modify this plan as steps complete.

- [x] **Step 1: Run full verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

- [x] **Step 2: Update milestone acceptance**

Update M7.4 in `docs/milestones/m7-agent-audio-plan-composer.md` with:
- implementation status;
- exact verification commands and results;
- any skipped live/paid checks.

- [x] **Step 3: Commit M7.4**

Suggested commit message:

```bash
git commit -m "feat: show agent audio status and final review"
```

Actual verification:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Result: PASS. The two `pnpm` commands reported the existing Node engine warning because this shell is on Node v24.14.0 while the repo expects Node >=26 <27.

## Acceptance Criteria

- Workbench overview node and detail panel show active AudioPlan, voiceover script, BGM direction, cue plan, voiceover/BGM generation statuses.
- Workbench final output node and detail panel show final video, audio track summary, AAC/mix status, and final review verdict.
- Production overview exposes audio status and final review counters for polling/status UI.
- Producer composition-completed reminder explicitly drives final review or explicit user-facing skip/confirmation.
- Reviewer final video context/prompt explicitly covers `audio_sync`, BGM ducking, voiceover timing, and final marketing quality.
- Full server and web verification passes, and M7.4 acceptance is recorded in the milestone file.
