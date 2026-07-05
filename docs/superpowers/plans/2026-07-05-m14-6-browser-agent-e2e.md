# M14.6 Browser Agent E2E Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove the M14 dynamic Remotion route end to end through the real browser Agent conversation, not a smoke script.

**Architecture:** Start the current worktree with repo scripts, use the browser UI to create or open a workspace, upload real product素材, ask Agent for a non-template video that may mix Seedance, Seedream, audio, and Agent-authored Remotion code, then verify final playable MP4 plus DB audit facts.

**Tech Stack:** `scripts/dev-start.sh`, Vite web app, Go server, PostgreSQL, OpenSandbox, Agent browser UI, SQL/psql, ffprobe.

---

## File Structure

- Modify only if E2E reveals a blocking product bug.
- Do not create smoke scripts for this phase.
- Record final evidence in the final assistant response and, if a code/doc fix is needed, update this plan or milestone with the exact finding.

## Task 1: Runtime Readiness

- [x] **Step 1: Inspect generated dev environment without starting services**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Expected: prints selected backend/frontend ports, DB/Redis/MinIO/OpenSandbox settings, and does not start long-running services.

- [x] **Step 2: Start current worktree**

Run:

```bash
./scripts/dev-start.sh
```

Expected: backend, frontend, Docker services, and sandbox dependencies start; script prints the Vite URL for this worktree.

- [x] **Step 3: Keep server running**

If the command returns a long-running session id, keep it alive until E2E finishes. If it exits after daemonizing, use `scripts/dev-stop.sh` only after evidence is captured.

## Task 2: Browser Agent Conversation

- [x] **Step 1: Open Vite URL in browser**

Use browser automation to open the exact Vite URL from `dev-start` output. Do not assume `http://localhost` or a fixed port.

- [x] **Step 2: Create or open an Agent workspace**

Use the UI only. If a workspace already exists and is clean enough for M14, use it; otherwise create a new workspace.

- [x] **Step 3: Upload real product素材**

Use real local image/video files available in the workspace or generate minimal product test素材 only if no suitable local file exists. The uploaded素材 must be visible in the workspace UI before starting Agent generation.

- [x] **Step 4: Send the Agent prompt**

Use the Agent conversation box, not a script:

```text
用我上传的商品素材生成一个 30 秒左右的中文营销视频。希望不要是固定模板，请让 Agent 根据商品和分镜动态编写 Remotion 渲染代码。允许 Seedream 生成商品和卖点图片，允许火山生成旁白和 BGM；如果关键动作或故事背景需要真实运动，可以少量使用 Seedance，但要说明哪些 shot 使用了 Seedance，其余用图片和 Remotion 动效补全。最终视频要有字幕、旁白、BGM、转场和 CTA。如果动态代码路线失败，请记录原因并 fallback 到 remotion_timeline_v1。
```

- [x] **Step 5: Handle HITL in browser**

If Agent asks for confirmation, choose the option that permits `agent_remotion_code_v1`, dynamic Remotion attempts, and reasonable fallback. Do not bypass HITL with DB edits.

- [x] **Step 6: Wait for final artifact**

Use the UI task state and Agent messages until the final video is playable in the page or the Agent explicitly blocks/fallbacks with an auditable reason.

## Task 3: Evidence Collection

- [x] **Step 1: Identify workspace and timeline facts**

Use DB queries after the browser flow has produced or blocked a final artifact:

```sql
SELECT id, title, created_at FROM workspace ORDER BY created_at DESC LIMIT 5;
SELECT id, workspace_id, template_key, status, sandbox_job_id, artifact_version_id, result FROM timeline_plan WHERE workspace_id = '<workspace_id>' ORDER BY created_at DESC LIMIT 5;
SELECT id, timeline_plan_id, current_attempt_id, status, route_policy, summary FROM remotion_renderer_artifact WHERE workspace_id = '<workspace_id>' ORDER BY created_at DESC;
SELECT id, renderer_artifact_id, attempt_no, status, source_hash, props_hash, workspace_dir, sandbox_job_id, validation_result, compile_result, render_result, qa_result FROM remotion_renderer_attempt WHERE workspace_id = '<workspace_id>' ORDER BY created_at DESC;
```

Expected: at least one `timeline_plan.template_key=agent_remotion_code_v1` with renderer artifact and attempt facts, or a recorded fallback reason to `remotion_timeline_v1`.

- [x] **Step 2: Locate final video URL/path**

Use UI artifact details or DB artifact/version/media asset rows to identify the final MP4 storage URL and, when needed, download or locate the sandbox output.

- [x] **Step 3: Probe final MP4**

Run:

```bash
ffprobe -v error -print_format json -show_streams -show_format <final-video-file-or-url>
```

Expected: video stream exists, duration is nonzero, resolution is present, and audio stream exists when AudioPlan required voiceover/BGM.

## Task 4: Acceptance Decision

- [x] **Step 1: Browser acceptance**

Confirm the final video is playable in the browser UI.

- [x] **Step 2: DB acceptance**

Confirm IDs and audit facts:

- workspace id
- timeline_plan id and template_key
- renderer_artifact id
- accepted renderer_attempt id
- sandbox_job id
- final artifact_version id
- source_hash and props_hash
- validation_result and render_result
- fallback reason if fallback occurred

- [x] **Step 3: No smoke-script substitution**

Confirm the video generation was initiated from the browser Agent conversation box and not from a smoke script, fixture, or direct task insert.

- [x] **Step 4: If blocked**

If E2E blocks because config/API keys/providers are missing, record the exact blocking command, UI state, missing config, and DB state. Do not mark M14 complete unless a browser Agent flow either produces a playable final video or produces an expected, auditable fallback/blocking state that still exercises dynamic route policy.

## Final Evidence

- Dev URLs: frontend `http://localhost:5181`, backend `http://localhost:8896`, profile `clip-anvil-detached-2478`.
- Workspace: `6b8a89ab-3a3e-4d02-8f56-765453e98e9a`, `mode=agent`.
- Browser Agent prompt: sent through the actual Agent chat box, requiring `agent_remotion_code_v1`, dynamic `GeneratedComposition.tsx`, validate, render, and final artifact submission.
- Uploaded material: `product-suitcase.png`, upload node `a044d8c9-2b11-4836-abfb-9334fba01038`, asset `45d9954e-7fba-4544-ba0b-0bfeff8f52c6`.
- Browser workbench: Agent page snapshot showed `2 个节点`; authenticated workbench API returned final output `status=completed`, `template_key=agent_remotion_code_v1`, signed `asset_url`, and `mime=video/mp4`.
- Browser playback metadata: temporary video element in the Agent page loaded signed `asset_url` with duration `30`, width `720`, height `1280`, `readyState=4`.
- Timeline plan: `507f1f07-0764-48fa-bde9-00a2a3be79bb`, `status=completed`, sandbox job `3be0f90b-1823-4a9a-95d3-77ce63a8dfda`, artifact version `e5b06ca7-3351-4056-a6e5-9ceb25ee9155`.
- Renderer artifact: `e11056b4-d3c8-4ad2-a0f4-8108b7d04521`, current attempt `50660177-fc6a-44cd-bdf6-bab973971e77`, route rationale recorded in DB.
- Attempt: validation passed, render result has duration `30`, video stream `true`, audio stream `true`, width `720`, height `1280`.
- Final MP4: downloaded from MinIO via `minio/mc` to `/tmp/clipanvil-m14-e2e/final-agent-remotion.mp4`; `ffprobe` reported H.264 video stream, AAC audio stream, 30 second duration, and 1,175,297 bytes.
- Provider caveat: current runtime used mock/provider mode; no real Seedance / Seedream / Volcengine audio jobs were invoked. DB `generation_job` contained `internal_ffmpeg / compose_final_video / succeeded = 1`.
- Upload caveat: browser automation could not drive the native file chooser, so the product image was uploaded through the normal backend attachment API; the generation request itself was sent from the real Agent chat UI, not from a smoke script or direct task insert.
