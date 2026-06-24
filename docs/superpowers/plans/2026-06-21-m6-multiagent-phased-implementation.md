# M6 MultiAgent Phased Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Deliver the full M6 Agent mode through small, independently verifiable subprojects: Agent runtime, right-floating chat UI, Eino Graph orchestration, HITL, storyboard/PSS, Craftsman/Worker generation, review retry, and Composer final video.

**Architecture:** M6 uses Eino Graph as the primary orchestration layer. Agent runtime tables persist generic thread/message/task/event/checkpoint state for Producer, Craftsman, Reviewer, and Composer; Graph nodes call a thin ClipAnvil tool registry, and production tools reuse the existing M4/M5 generation job, artifact version, stale, provider, and sandbox services.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc/goose, Eino Graph, PostgreSQL, React 19, Vite 8, TanStack Query, WebSocket, React Flow, existing ClipAnvil production and sandbox services.

---

## Source Specs

- `docs/superpowers/specs/2026-06-21-m6-multiagent-agent-mode-design.md`
- `docs/milestones/m3-m6-studio-agent-roadmap.md`
- `docs/superpowers/specs/2026-06-18-multiagent-agent-mode-design.md`

## Execution Rules

- Implement one subtask at a time.
- Before coding each subtask, write a dedicated detailed plan in `docs/superpowers/plans/YYYY-MM-DD-m6-<subtask>.md`.
- Each subtask must end with runnable behavior and focused verification.
- Keep ordinary user canvas writes blocked in Agent Workspace throughout M6.
- Do not implement Studio / Agent import-export in M6.

## Subtask Dependency Graph

```text
M6.1 Runtime Schema
  -> M6.2 Agent WebSocket + Right Floating Chat
  -> M6.3 ProducerGraph Skeleton
  -> M6.4 HITL Interrupt / Resume
  -> M6.5 Edge Registry + Storyboard + PSS
  -> M6.6 CraftsmanGraph + Worker Preview Generation
  -> M6.7 Video Generation + Review Retry + Shot Dependency Scheduling
  -> M6.8 ComposerGraph + Final Video
  -> M6.9 End-to-End Hardening
```

---

## M6.1 Agent Runtime Schema And Persistence

**Goal:** Add generic Agent runtime persistence shared by Producer, Craftsman, Reviewer, and Composer.

**Files likely touched:**

- Create: `apps/server/migrations/015_m6_agent_runtime.sql`
- Create: `apps/server/sqlc/queries/agent_thread.sql`
- Create: `apps/server/sqlc/queries/agent_message.sql`
- Create: `apps/server/sqlc/queries/agent_task.sql`
- Create: `apps/server/sqlc/queries/agent_event.sql`
- Create: `apps/server/sqlc/queries/eino_checkpoint.sql`
- Modify: `apps/server/internal/store/db/`
- Test: `apps/server/internal/store/db` compile coverage through sqlc and server tests

**Deliverables:**

- `agent_thread`
- `agent_message`
- `agent_task`
- `agent_event`
- `eino_checkpoint`
- sqlc generated types and queries
- helper queries for current Producer thread and message pagination

**Acceptance:**

- A Producer thread can be created or fetched for an Agent Workspace.
- Messages are append-only with stable `seq`.
- Tasks and events can be created and updated independently.
- Checkpoints can be written and read by key.
- No Studio behavior changes.

**Verification:**

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
git diff --check
```

---

## M6.2 Agent API, WebSocket, And Right Floating Chat UI

**Goal:** Build the Agent conversation shell with persisted messages and realtime sync, with the Producer chat floating on the right above the read-only canvas.

**Files likely touched:**

- Create: `apps/server/internal/api/agent_handler.go`
- Create: `apps/server/internal/api/agent_ws_handler.go`
- Create: `apps/server/internal/api/agent_hub.go`
- Modify: `apps/server/cmd/server/main.go`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Create: `apps/web/src/lib/agentApi.ts`
- Create: `apps/web/src/lib/agentWs.ts`
- Modify: `apps/web/src/main.css`
- Test: `apps/server/internal/api/agent_handler_test.go`
- Test: `apps/web/src/lib/agentWs.test.mjs`

**Deliverables:**

- `GET /api/agent/workspaces/:workspaceID/thread`
- `GET /api/agent/workspaces/:workspaceID/messages`
- `POST /api/agent/workspaces/:workspaceID/messages`
- `GET /ws/agent?workspaceId=<uuid>&token=<jwt>`
- Right floating Producer chat panel over the read-only Agent canvas
- Persisted `message_created` event path that later ProducerGraph can consume

**Acceptance:**

- User sends a message in Agent Workspace.
- Message persists and appears after refresh.
- Another browser tab receives the message through `/ws/agent`.
- Right floating chat can collapse without hiding the read-only canvas.
- Ordinary node/edge/group/camera write APIs still return `403` in Agent Workspace.
- No assistant message, mock Producer reply, or `producer_turn` task is created before M6.3.

**Verification:**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

---

## M6.3 ProducerGraph Skeleton

**Goal:** Introduce Eino Graph as the primary orchestration layer and run a minimal Producer turn through explicit graph nodes.

**Files likely touched:**

- Create: `apps/server/internal/agent/graph/producer.go`
- Create: `apps/server/internal/agent/runtime/service.go`
- Create: `apps/server/internal/agent/runtime/messages.go`
- Create: `apps/server/internal/agent/runtime/events.go`
- Create: `apps/server/internal/agent/pss/producer.go`
- Modify: `apps/server/internal/api/agent_handler.go`
- Test: `apps/server/internal/agent/graph/producer_test.go`
- Test: `apps/server/internal/agent/runtime/service_test.go`

**Deliverables:**

- ProducerGraph nodes:
  - `load_thread_and_messages`
  - `build_producer_pss`
  - `call_producer_model`
  - `persist_assistant_message`
  - `emit_agent_events`
- Mock model implementation for deterministic local tests
- `producer_turn` task lifecycle: queued -> running -> succeeded/failed

**Acceptance:**

- Posting a user message creates a `producer_turn` task.
- ProducerGraph reads persisted messages and PSS.
- Mock model response is persisted as assistant message.
- Task and event status updates are visible through API/WebSocket.
- Graph errors persist an error message and failed task state.

**Verification:**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

---

## M6.4 HITL Interrupt And Resume

**Goal:** Implement `request_user_decision` as a Graph-controlled HITL tool with persisted card messages, events, checkpoint, and resume.

**Files likely touched:**

- Create: `apps/server/internal/agent/tools/decision.go`
- Create: `apps/server/internal/agent/hitl/checkpoint.go`
- Modify: `apps/server/internal/agent/graph/producer.go`
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Create: `apps/web/src/components/AgentDecisionCard.tsx`
- Test: `apps/server/internal/agent/tools/decision_test.go`
- Test: `apps/server/internal/api/agent_handler_test.go`

**Deliverables:**

- `request_user_decision` tool definition
- `POST /api/agent/decisions/:eventID/resolve`
- `ui_card` agent message type
- `decision_requested` and `decision_resolved` events
- Checkpoint write/read path for interrupted ProducerGraph

**Acceptance:**

- ProducerGraph can create a blocking decision card.
- User sees the card in the right floating chat panel.
- User clicks an option and the card becomes resolved.
- Decision result persists across refresh.
- ProducerGraph resumes from the saved checkpoint.

**Verification:**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

---

## M6.5 Edge Registry, Storyboard, And PSS

**Goal:** Add the first production-semantic tools and storyboard facts: `get_production_state`, `update_storyboard`, `shot`, `shot_dependency`, and full Producer PSS.

**Files likely touched:**

- Create: `apps/server/migrations/018_m6_storyboard_pss.sql`
- Create: `apps/server/sqlc/queries/shot.sql`
- Create: `apps/server/sqlc/queries/shot_dependency.sql`
- Create: `apps/server/internal/agent/tools/registry.go`
- Create: `apps/server/internal/agent/tools/production_state.go`
- Create: `apps/server/internal/agent/tools/storyboard.go`
- Modify: `apps/server/internal/agent/pss/producer.go`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Test: `apps/server/internal/agent/tools/storyboard_test.go`
- Test: `apps/server/internal/agent/pss/producer_test.go`

**Deliverables:**

- `shot`
- `shot_dependency`
- optional `media_node.shot_id`
- Edge registry with JSON-schema-style tool metadata
- `get_production_state`
- `update_storyboard`
- Producer PSS with workspace, source materials, shots, dependencies, nodes, versions, jobs, stale reasons, decisions, and running tasks

**Acceptance:**

- ProducerGraph can call `update_storyboard`.
- Shots persist and survive refresh.
- PSS describes current shots and dependencies.
- Edge call/result messages appear in the chat.
- Agent canvas can show shot/node association summary when available.

**Verification:**

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

---

## M6.6 CraftsmanGraph, Worker Task, And Preview Generation

**Goal:** Dispatch shot-level Craftsman work and generate preview images through the existing production service.

**Files likely touched:**

- Create: `apps/server/internal/agent/graph/craftsman.go`
- Create: `apps/server/internal/agent/worker/generation.go`
- Create: `apps/server/internal/agent/tools/generation.go`
- Modify: `apps/server/internal/agent/tools/registry.go`
- Modify: `apps/server/sqlc/queries/node.sql`
- Modify: `apps/server/internal/production/service.go` if `SubmitGenerationIntent` needs Agent input-ref parity with `SubmitNodeRun`
- Test: `apps/server/internal/agent/graph/craftsman_test.go`
- Test: `apps/server/internal/agent/worker/generation_test.go`
- Test: `apps/server/internal/agent/tools/generation_test.go`

**Deliverables:**

- Craftsman thread per shot
- CraftsmanGraph preview strategy path
- Worker task executor for generation
- `generate_asset`
- `generate_shot_preview`
- Agent-created media nodes with `source='agent'`
- Generation jobs with `requested_by_type='agent_task'`

**Acceptance:**

- Producer dispatches Craftsman for a shot.
- Craftsman creates or updates a preview image node.
- Worker submits generation through M4/M5 production service.
- Queued/running/succeeded/failed version states appear in existing production state and canvas preview.
- Failure records persist in `generation_job` and `artifact_version`.

**Verification:**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

---

## M6.7 Video Generation, Review Retry, And Shot Dependency Scheduling

**Goal:** Generate shot videos, evaluate outputs with rubric-based review, retry with Craftsman revisions, and respect blocking shot dependencies.

**Files likely touched:**

- Create: `apps/server/migrations/017_m6_review_records.sql`
- Create: `apps/server/sqlc/queries/review_record.sql`
- Create: `apps/server/internal/agent/graph/review.go`
- Create: `apps/server/internal/agent/scheduler/shot_scheduler.go`
- Modify: `apps/server/internal/agent/graph/craftsman.go`
- Modify: `apps/server/internal/agent/tools/generation.go`
- Test: `apps/server/internal/agent/graph/review_test.go`
- Test: `apps/server/internal/agent/scheduler/shot_scheduler_test.go`

**Deliverables:**

- `review_record`
- `generate_shot_video`
- rubric axes:
  - `proportion`
  - `physics`
  - `style`
  - `visual_quality`
  - `product_visibility`
  - `selling_power`
  - `platform_fit`
- reject/accept decision
- automatic retry up to 3 attempts
- blocking dependency scheduler for `last_frame_continuity`, `same_subject_consistency`, and `visual_reference`

**Acceptance:**

- Video generation uses existing generation job/version chain.
- Review records store scores, critique, and suggested revision.
- Craftsman revises prompt and retries when review rejects.
- Retry stops after max attempts and records failure.
- Shot with blocking dependency waits for upstream required artifact.
- New winner marks downstream affected work stale.

**Verification:**

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

---

## M6.8 ComposerGraph And Final Video

**Goal:** Compose confirmed shot videos into a final video through Sandbox Job Service and version it like other production outputs.

**Files likely touched:**

- Create: `apps/server/internal/agent/graph/composer.go`
- Create: `apps/server/internal/agent/tools/composer.go`
- Modify: `apps/server/internal/sandbox/job_service.go`
- Modify: `apps/server/internal/agent/tools/registry.go`
- Test: `apps/server/internal/agent/graph/composer_test.go`
- Test: `apps/server/internal/agent/tools/composer_test.go`
- Test: `apps/server/internal/sandbox/job_service_test.go`

**Deliverables:**

- ComposerGraph
- `compose_final`
- final video media node
- Sandbox FFmpeg composition job
- generation job / artifact version / sandbox job linkage
- HITL final confirmation card

**Acceptance:**

- Composer refuses to run until required shot video winners exist.
- FFmpeg runs only through Sandbox Job Service.
- Final output becomes a succeeded artifact version.
- Canvas and production state show final video preview.
- User can confirm final output through HITL card.

**Verification:**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

---

## M6.9 End-To-End Hardening

**Goal:** Validate the full M6 loop from user message to storyboard, preview, video, review retry, final composition, and HITL confirmation.

**Files likely touched:**

- Create: `scripts/smoke-m6-agent-runtime.sh`
- Create: `scripts/smoke-m6-storyboard.sh`
- Create: `scripts/smoke-m6-preview.sh`
- Create: `scripts/smoke-m6-video-review.sh`
- Create: `scripts/smoke-m6-composer.sh`
- Modify: `docs/milestones/m3-m6-studio-agent-roadmap.md`
- Modify: `docs/superpowers/specs/2026-06-21-m6-multiagent-agent-mode-design.md` if implementation diverges from spec

**Deliverables:**

- Smoke scripts for each M6 stage
- Browser smoke checklist for the right-floating chat UI
- Roadmap completion notes
- Final verification record

**Acceptance:**

- Agent Workspace can complete a single-video production flow.
- State recovers after refresh during message wait, HITL wait, running generation, failed review, and final confirmation.
- Existing Studio M5 flows still pass.
- Ordinary user canvas writes remain blocked in Agent Workspace.

**Verification:**

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

---

## Commit Strategy

Use one branch for the M6 milestone, but commit each subtask independently:

```text
feat: add m6 agent runtime schema
feat: add agent chat websocket
feat: add producer graph skeleton
feat: add agent hitl decision flow
feat: add agent storyboard tools
feat: add craftsman preview generation
feat: add agent video review retry
feat: add composer final video graph
test: add m6 e2e smoke scripts
docs: mark m6 multiagent milestone complete
```

## Plan Self-Review

- Spec coverage: all M6 design sections are mapped to subtasks.
- Excluded scope: Studio / Agent import-export remains outside M6.
- Iteration safety: each subtask has a runnable acceptance target.
- Reuse guarantee: generation subtasks explicitly route through M4/M5 production job/version/stale/provider/sandbox services.
- UI direction: Agent chat is right-floating over the read-only canvas.
