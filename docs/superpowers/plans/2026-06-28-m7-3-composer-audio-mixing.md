# M7.3 Composer Audio Mixing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task in this session, or use `superpowers:subagent-driven-development` with a fresh worker per task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let Composer read the approved `AudioPlan`, generated voiceover artifact, generated BGM artifact, and current shot video winners, then render a playable final MP4 with an AAC mixed audio track.

**Architecture:** M7.3 keeps `AudioPlan` as the audio truth source and extends the existing Composer + sandbox ffmpeg path. Composer context exposes video winners and audio artifacts; `TimelinePlan` gains explicit audio tracks and mix settings; `render_timeline_template` builds the deterministic ffmpeg filter graph; `submit_composition_artifact` persists the final video and links the completed `timeline_plan` back to `audio_plan`. The `internal_ffmpeg` provider and sandbox `ComposeVideos` API are extended with the same audio mix contract so automated production jobs can exercise the same composition behavior without relying on model-written raw ffmpeg.

**Tech Stack:** Go 1.26, PostgreSQL 16, pgx/sqlc, CloudWeGo Eino native tools, existing Agent Composer graph, OpenSandbox job execution, ffmpeg/ffprobe, MinIO artifact storage.

---

## Current Code Facts

- `apps/server/internal/agent/composer/types.go` has `TimelinePlan{segments, transitions, output}` only; there are no typed audio tracks.
- `get_composition_context` currently returns current shot video winners and a placeholder timeline schema string for audio.
- `render_timeline_template` currently extracts `plan.segments[].workspace_path` and calls `concatFFmpegArgs`, which assumes every video input has audio and concatenates `[v][a]`.
- `sandbox.ComposeVideosInput` currently has only `Sources []SandboxAssetInput`; `JobService.ComposeVideos` downloads only video files and runs a pure concat command.
- `internal_ffmpeg` provider `compose_final_video` currently forwards only video `InputRef`s to sandbox.
- `audio_plan.sql` has voiceover/BGM render plan and node links, but no `timeline_plan_id` update query yet.
- M7.2 already produces `voiceover_audio` and `bgm_audio` audio nodes with stored artifacts; real Volcengine paid smoke remains outside current automated verification.

## TimelinePlan Audio Contract

M7.3 uses this JSON shape in `timeline_plan.plan_json`:

```json
{
  "template_key": "concat_with_fades",
  "segments": [
    {
      "id": "shot-01",
      "asset_id": "...",
      "workspace_path": "/workspace/input/shot-01.mp4",
      "start_sec": 0,
      "duration_sec": 4.2
    }
  ],
  "audio_tracks": [
    {
      "id": "voiceover-main",
      "role": "voiceover",
      "asset_id": "...",
      "workspace_path": "/workspace/input/voiceover.mp3",
      "start_sec": 0,
      "duration_sec": 12,
      "volume": 1,
      "fade_in_sec": 0.05,
      "fade_out_sec": 0.1
    },
    {
      "id": "bgm-main",
      "role": "bgm",
      "asset_id": "...",
      "workspace_path": "/workspace/input/bgm.mp3",
      "start_sec": 0,
      "duration_sec": 12,
      "volume": 0.28,
      "fade_in_sec": 0.5,
      "fade_out_sec": 1.2,
      "ducking": {
        "sidechain_role": "voiceover",
        "threshold": 0.08,
        "ratio": 8,
        "attack_ms": 20,
        "release_ms": 250
      }
    }
  ],
  "output": {
    "workspace_path": "/workspace/output/final.mp4",
    "format": "mp4",
    "audio_codec": "aac"
  }
}
```

Rules:
- `audio_tracks` is optional for legacy silent/video-only composition, but M7.3 final audio path requires at least one `voiceover` or `bgm` track.
- Only `role=voiceover` and `role=bgm` are in M7.3 scope.
- Every audio `workspace_path` must stay under `/workspace/`.
- `volume` defaults to `1.0`; BGM recommended default is `0.25-0.35`.
- `fade_in_sec`, `fade_out_sec`, and `duration_sec` default to `0` when omitted.
- `ducking.sidechain_role=voiceover` is applied only when both BGM and voiceover tracks exist.
- Output must be `video/mp4` with `-c:a aac`; no user-uploaded audio, library BGM, lip-sync, or multi-role dialogue is added in M7.3.

## Task 1: Add Typed TimelinePlan Audio Support

**Files:**
- Modify: `apps/server/internal/agent/composer/types.go`
- Modify tests: `apps/server/internal/agent/composer/types_test.go`
- Modify tests: `apps/server/internal/agent/composer/timeline_plan_contract_test.go`

- [x] **Step 1: Write failing timeline JSON tests**

Add tests proving:
- `TimelinePlan` marshals/unmarshals `audio_tracks`.
- `AudioTrack.role`, `volume`, `fade_in_sec`, `fade_out_sec`, and `ducking` survive JSON round trip.
- `OutputSettings` accepts `audio_codec`.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run 'TimelinePlan|AudioTrack' -count=1)
```

Expected: FAIL because the typed structs do not include audio fields.

- [x] **Step 2: Implement typed structs**

Add:
- `TimelinePlan.AudioTracks []AudioTrack`
- `AudioTrack`
- `AudioDucking`
- `OutputSettings.AudioCodec`

Keep JSON fields optional where possible so existing timeline plans remain readable.

- [x] **Step 3: Verify Task 1**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run 'TimelinePlan|AudioTrack' -count=1)
git diff --check
```

Expected: PASS.

## Task 2: Expose AudioPlan and Audio Artifacts to Composer Context

**Files:**
- Modify: `apps/server/internal/agent/composer/context_loader.go`
- Modify: `apps/server/internal/agent/composer/tool_context_provider.go`
- Modify: `apps/server/internal/agent/composer/executor.go`
- Modify tests: `apps/server/internal/agent/composer/*_test.go`

- [x] **Step 1: Write failing context tests**

Add tests proving `get_composition_context` returns:
- the active approved `AudioPlan` summary, including `id`, `status`, `semantic_key`, `target_duration_sec`, `voiceover_script`, `voice_profile`, `bgm_plan`, and `cue_plan`;
- a `voiceover` composition asset when `audio_plan.voiceover_node_id` points to a succeeded audio artifact;
- a `bgm` composition asset when `audio_plan.bgm_node_id` points to a succeeded audio artifact;
- no audio asset when the node has no succeeded current version;
- a timeline schema that explicitly documents `audio_tracks`.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run 'CompositionContext|AudioPlan|AudioAsset' -count=1)
```

Expected: FAIL because Composer store/context does not load AudioPlan or audio nodes.

- [x] **Step 2: Extend Composer store interfaces**

Add the minimum store methods needed by Composer:
- `GetActiveAudioPlanByWorkspace(ctx, workspaceID)`
- `GetMediaNodeByID(ctx, nodeID)` if not already available in the relevant interface
- existing `GetArtifactVersionByID` and `GetMediaAssetByID` are reused for audio artifacts.

Keep context loading tolerant of a missing active AudioPlan, but mark the audio path blocked later if final composition requires generated audio and assets are missing.

- [x] **Step 3: Add audio assets to composition context**

Return `available_composition_assets` entries with:
- `role`: `clip`, `voiceover`, or `bgm`
- `node_id`, `node_ref`, `artifact_version_id`, `asset_id`, `source_url`, `mime_type`, `file_name`
- `audio_plan_id` and `audio_plan_ref` for audio assets

Use deterministic file names:
- `voiceover.mp3` / `voiceover.wav` based on MIME
- `bgm.mp3` / `bgm.wav` based on MIME

- [x] **Step 4: Update Composer system prompt**

Tell Composer:
- use approved AudioPlan as read-only input;
- stage shot videos, voiceover, and BGM before rendering;
- include `audio_tracks` in the timeline plan when audio artifacts exist;
- prefer generated voiceover as the primary narration track;
- keep BGM lower than narration and duck BGM under voiceover;
- block with a clear missing-input reason when the approved AudioPlan requires audio but generated artifacts are absent;
- final output must include AAC audio when audio tracks are present.

- [x] **Step 5: Verify Task 2**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/composer -run 'CompositionContext|AudioPlan|AudioAsset|Prompt' -count=1)
git diff --check
```

Expected: PASS.

## Task 3: Render Audio Tracks Through Composer Native Template

**Files:**
- Modify: `apps/server/internal/agent/tools/composer_native.go`
- Modify tests: `apps/server/internal/agent/tools/composer_tools_test.go`

- [x] **Step 1: Write failing renderer tests**

Add tests proving:
- `render_timeline_template` accepts a plan with video `segments` and `audio_tracks`.
- generated ffmpeg args include voiceover and BGM inputs.
- generated filter graph includes audio trim/delay/volume/fade.
- when BGM has `ducking.sidechain_role=voiceover`, filter graph includes `sidechaincompress`.
- output maps a video stream and mixed audio stream and uses `-c:a aac`.
- legacy no-`audio_tracks` plans still render through the old video-only path.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'Composer.*Audio|TimelineTemplate' -count=1)
```

Expected: FAIL because `timelineWorkspacePaths` and `concatFFmpegArgs` ignore audio tracks.

- [x] **Step 2: Add timeline parser helpers**

Add structured parser helpers inside `composer_native.go`:
- `timelineSegments(plan)`
- `timelineAudioTracks(plan)`
- `timelineOutputPath(plan, fallback)`
- `audioFloatValue(value, defaultValue)`

Validate all workspace paths with `path.Clean` and `/workspace/` prefix.

- [x] **Step 3: Build deterministic ffmpeg audio filter graph**

Implement `timelineFFmpegArgs(plan, outputPath)`:
- video inputs first, audio inputs after video inputs;
- for one video input, map `[0:v]` through format/setsar or copy-safe path rather than dropping audio;
- for multiple video inputs, concatenate video only (`a=0`) so final audio is controlled by `audio_tracks`;
- for each audio track: `atrim`, `asetpts`, `adelay`, `volume`, optional `afade`;
- BGM can be looped with `-stream_loop -1` when its role is `bgm` and a target duration exists;
- mix multiple audio tracks with `amix`;
- duck BGM with `sidechaincompress` when voiceover and BGM are both present;
- map `[vout]` and `[aout]`, encode `libx264` + `aac`, and use `-shortest`.

Keep a no-audio fallback that preserves current behavior for existing video-only timeline plans.

- [x] **Step 4: Verify Task 3**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'Composer.*Audio|TimelineTemplate' -count=1)
git diff --check
```

Expected: PASS.

## Task 4: Extend Sandbox ComposeVideos and internal_ffmpeg Provider

**Files:**
- Modify: `apps/server/internal/sandbox/job_service.go`
- Modify tests: `apps/server/internal/sandbox/job_service_test.go`
- Modify: `apps/server/internal/production/internal_ffmpeg_provider.go`
- Modify tests: `apps/server/internal/production/internal_ffmpeg_provider_test.go`

- [x] **Step 1: Write failing provider and sandbox tests**

Add tests proving:
- `InternalFFmpegProvider` converts video `InputRef`s and audio `InputRef`s into `sandbox.ComposeVideosInput`.
- audio refs carry `role=voiceover` or `role=bgm` from `InputRef.Metadata["agent_artifact_kind"]` or params.
- provider request/response metadata reports `audio_track_count`.
- `JobService.ComposeVideos` downloads audio inputs and builds an ffmpeg command with AAC mixed audio.
- pure video `ComposeVideos` behavior remains compatible.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'InternalFFmpeg.*Compose' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'ComposeVideos' -count=1)
```

Expected: FAIL because sandbox input has no audio track support.

- [x] **Step 2: Extend sandbox input contract**

Add:
- `ComposeAudioTrackInput`
- `ComposeAudioDuckingInput`
- `ComposeVideosInput.AudioTracks []ComposeAudioTrackInput`
- optional `TargetDurationSec`, `OutputAudioCodec`, and `MixSettings` only if needed by the command builder.

Keep existing `Sources` unchanged for compatibility.

- [x] **Step 3: Implement audio downloads and ffmpeg command**

In `JobService.ComposeVideos`:
- include `video_count` and `audio_track_count` in sandbox job input JSON;
- download audio assets with safe deterministic names;
- use the same command builder contract as `render_timeline_template`;
- ensure output MIME remains `video/mp4`;
- include `audio_track_count` in sandbox output JSON.

- [x] **Step 4: Extend provider mapping**

In `InternalFFmpegProvider.runCompose`:
- keep video refs ordered as before;
- collect audio refs by `NodeType=="audio"` and non-empty `StorageURL`;
- infer track role from metadata/artifact kind when present, otherwise skip unknown audio refs;
- pass tracks to sandbox;
- report `source_count` and `audio_track_count`.

- [x] **Step 5: Verify Task 4**

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run 'InternalFFmpeg.*Compose' -count=1)
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'ComposeVideos' -count=1)
git diff --check
```

Expected: PASS.

## Task 5: Link Completed TimelinePlan Back to AudioPlan

**Files:**
- Modify: `apps/server/sqlc/queries/audio_plan.sql`
- Regenerate: `apps/server/internal/store/db/audio_plan.sql.go`
- Modify: `apps/server/internal/agent/tools/composer_native.go`
- Modify tests: `apps/server/internal/agent/tools/composer_tools_test.go`

- [x] **Step 1: Write failing linkage test**

Add a submit artifact test proving:
- when a completed timeline plan belongs to the same workspace as the active `AudioPlan`, submitting the final artifact sets `audio_plan.timeline_plan_id`;
- missing AudioPlan does not fail final artifact persistence.

Run:

```bash
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'SubmitCompositionArtifact.*AudioPlan|TimelinePlan' -count=1)
```

Expected: FAIL because the query/store method does not exist.

- [x] **Step 2: Add sqlc query**

Add:

```sql
-- name: UpdateAudioPlanTimelinePlan :one
UPDATE audio_plan
SET
    timeline_plan_id = sqlc.arg(timeline_plan_id),
    status = 'composing',
    updated_at = now()
WHERE workspace_id = sqlc.arg(workspace_id)
  AND archived_at IS NULL
  AND status IN ('approved', 'generating', 'voiceover_ready', 'composing')
RETURNING *;
```

Run `make sqlc-generate`.

- [x] **Step 3: Wire submit-composition update**

Extend the composition artifact store interface with `UpdateAudioPlanTimelinePlan`.

After `timeline_plan` is marked completed, attempt to update the active AudioPlan timeline link. Treat `pgx.ErrNoRows` as non-fatal because legacy/video-only compositions may not have an AudioPlan.

- [x] **Step 4: Verify Task 5**

Run:

```bash
make sqlc-generate
(cd apps/server && GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'SubmitCompositionArtifact.*AudioPlan|TimelinePlan' -count=1)
git diff --check
```

Expected: PASS.

## Task 6: Add Minimal End-to-End Audio Composer Smoke

**Files:**
- Create: `scripts/smoke-m7-3-audio-composer.sh`
- Modify docs: `docs/milestones/m7-agent-audio-plan-composer.md`

- [ ] **Step 1: Add smoke script**

The smoke should:
- require a running local stack;
- create or reuse a test workspace;
- create 2-3 short generated test videos or import fixtures through existing helper APIs;
- create or reuse generated/mock `voiceover_audio` and `bgm_audio` artifacts;
- call the Composer/sandbox path to render final MP4;
- verify final artifact exists;
- run ffprobe and assert an AAC audio stream exists.

Do not require paid Volcengine calls. The smoke may use mock audio artifacts generated by existing test/mock production paths.

- [ ] **Step 2: Document manual paid smoke separately**

Update M7 milestone notes to distinguish:
- automated local smoke with mock/generated fixtures;
- optional paid Volcengine smoke using real `seed-audio-1.0` artifacts.

- [ ] **Step 3: Verify script syntax**

Run:

```bash
bash -n scripts/smoke-m7-3-audio-composer.sh
git diff --check
```

Expected: PASS.

## Task 7: Full M7.3 Verification and Milestone Update

**Files:**
- Modify: `docs/milestones/m7-agent-audio-plan-composer.md`

- [ ] **Step 1: Run full verification**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
bash -n scripts/smoke-m7-3-audio-composer.sh
git diff --check
```

If a live local stack is available, also run:

```bash
./scripts/smoke-m7-3-audio-composer.sh
```

- [ ] **Step 2: Update milestone acceptance**

Update M7.3 in `docs/milestones/m7-agent-audio-plan-composer.md` with:
- implementation status;
- verification commands and results;
- whether the live smoke ran or was skipped;
- whether real Volcengine paid smoke ran or was skipped.

- [ ] **Step 3: Commit M7.3 implementation**

Commit after all required checks pass.

Suggested commit message:

```bash
git commit -m "feat: compose agent audio into final video"
```

## Acceptance Criteria

- Composer context exposes current shot video winners, approved AudioPlan, generated voiceover artifact, and generated BGM artifact.
- TimelinePlan supports `audio_tracks`, volume, fades, output AAC, and BGM ducking under voiceover.
- `render_timeline_template` can produce an ffmpeg command that mixes 2-3 shot videos plus generated voiceover and generated BGM into one MP4.
- `internal_ffmpeg` provider and sandbox `ComposeVideos` can carry the same audio track contract.
- Final artifact persistence still creates/reuses the final video node and stores the output artifact.
- `audio_plan.timeline_plan_id` links to the completed timeline plan when an active AudioPlan exists.
- Automated tests pass and the smoke script can verify the final MP4 has an AAC audio stream.
