# M6.9 UX Completion Design

**Status**: Draft for review
**Date**: 2026-06-23
**Milestone**: M6 MultiAgent Agent Mode
**Depends on**: M6.6 preview closure, M6.7 review/retry/dependency scheduler, M6.8 video/composer

## Goal

M6.9 completes the user-facing Agent workspace. The backend production path can already create storyboard, preview nodes, review records, retry tasks, shot video tasks, composer tasks, generation jobs, artifact versions, and sandbox jobs. The missing piece is that the user still has to infer too much from raw chat bubbles, canvas nodes, or logs.

This phase turns the Agent page into a readable production cockpit:

```text
overall status
-> storyboard progress
-> per-shot preview / review / video status
-> background task timeline
-> final video confirmation
-> node detail trace
```

M6.9 is not a cosmetic pass. It is the UX projection of durable M6 facts. Every visible status must come from persisted database facts or websocket events, not from LLM prose.

## Current Code State

The current implementation already has useful foundations:

- `AgentWorkspacePage` renders:
  - read-only canvas;
  - right floating `ClipAnvil` chat panel;
  - websocket connection dot;
  - message list;
  - jump-to-latest behavior;
  - attachment upload;
  - model / reasoning selector;
  - busy-state input disabling.
- `AgentMessageRenderer` supports typed message blocks:
  - markdown;
  - thinking;
  - decision card;
  - review card;
  - final video card;
  - tool status;
  - attachment;
  - media;
  - error.
- `AgentNodeDetailDrawer` already exposes read-only production details, including production state, review records and sandbox jobs where available.
- Backend M6 runtime facts exist in:
  - `agent_task`;
  - `agent_event`;
  - `agent_message`;
  - `shot`;
  - `shot_dependency`;
  - `review_record`;
  - `generation_job`;
  - `artifact_version`;
  - `sandbox_job`;
  - `media_node.metadata.agent_artifact_kind`.
- The Agent canvas websocket receives production job updates and node snapshots.

The important gap: these facts are not yet composed into a stable, first-class Agent workspace UI. The chat stream shows events as isolated bubbles, and the canvas is visually rich but not a complete production status surface.

## Product Principles

1. **User-facing labels describe work, not internal roles.**
   Default UI should say "规划分镜", "生成预览", "评审画面", "生成视频", "合成成片". It should not lead with Producer, Craftsman, Worker, Reviewer, Composer.

2. **Diagnostics are available but secondary.**
   Expandable diagnostic sections may show task ids, job ids, event ids, role names, provider payloads, and sandbox traces.

3. **The workspace state must be recoverable after refresh.**
   If the browser refreshes, status bar, storyboard view, timeline, cards, and node detail must rebuild from persisted API data.

4. **WebSocket improves freshness, not correctness.**
   Websocket events update the UI optimistically. The canonical state remains API/DB-backed snapshots.

5. **Cards and panels share one projection model.**
   Storyboard, review, final output, and timeline data should be computed by shared helpers and reused by message blocks, persistent panel sections, and tests.

6. **Do not turn Agent mode into Studio editing.**
   M6.9 keeps Agent canvas read-only. User actions are conversational or HITL actions, not direct canvas mutation.

## Scope

M6.9 includes:

- Agent production status bar.
- Persistent storyboard view inside the floating Agent panel.
- Task timeline with user-facing labels and diagnostic details.
- Review/retry/final output structured rendering polish.
- Final video confirmation UX.
- Node detail trace enhancements for review, retry, dependencies, final composition, and sandbox jobs.
- API projection additions needed to hydrate these views.
- Browser E2E for the terminal M6 production flow.

M6.9 does not include:

- Studio / Agent import-export.
- User editing of Agent canvas nodes.
- New generation providers.
- Replacing Eino Graph orchestration.
- A separate workflow engine.
- Full collaborative multi-user Agent sessions.

## Information Architecture

The Agent page should have three persistent surfaces.

### 1. Read-Only Canvas

The canvas stays the visual production map. It answers:

- What artifacts exist?
- Where are the source, preview, video, and final output nodes?
- Which node is selected?

M6.9 should not overload canvas nodes with full workflow detail. Node cards should stay compact and visual. Detailed status belongs to the panel and drawer.

### 2. Floating ClipAnvil Panel

The floating panel becomes a stacked workspace panel:

```text
Header
Status bar
Storyboard section
Message / timeline tabs or stacked stream
Composer input
```

The first viewport of the panel should show project status before the user scrolls through long chat history. Long conversations should not hide the current production state.

### 3. Node Detail Drawer

The detail drawer answers:

- Why does this node exist?
- Which shot does it belong to?
- Which prompt/model/input refs created it?
- Which jobs/versions/reviews/sandbox jobs are attached?
- What downstream work depends on it?

The drawer remains read-only in Agent mode.

## Agent Production Status Bar

### Purpose

Give the user a one-line understanding of the current production phase and whether action is required.

### Data

The status bar should be derived from a new frontend projection helper over:

- active `agent_task` rows;
- pending `agent_event` rows;
- `shot.status`;
- `review_record.status`;
- media node artifact kinds;
- generation job statuses;
- final output node status;
- websocket connection status.

### Display

The compact status bar should include:

- current phase label:
  - `规划中`;
  - `生成预览`;
  - `评审中`;
  - `生成视频`;
  - `合成成片`;
  - `等待确认`;
  - `已完成`;
  - `需要处理`;
  - `出错`;
- active task count;
- failed task count;
- waiting decision count;
- connection dot;
- compact progress summary, for example `预览 2/3 · 视频 1/3 · 成片 0/1`.

### Rules

- If any pending decision exists, phase is `等待确认`.
- If any failed task or failed terminal production job needs user-visible action, phase is `需要处理` or `出错`.
- If a final video exists and is waiting confirmation, phase is `等待确认`.
- If final video is accepted, phase is `已完成`.
- Otherwise the latest active or most advanced phase wins in this order:
  1. final composition;
  2. shot video;
  3. review;
  4. preview generation;
  5. storyboard planning.

## Storyboard View

### Purpose

Users need a structured list of shots without inspecting canvas nodes one by one.

### Display

Each shot row should show:

- shot key;
- title;
- duration;
- status;
- preview status;
- review status;
- video status;
- blocked reason if any;
- linked node count;
- latest winner indicator when available.

Rows should be dense and scannable. Use compact chips for phases instead of large cards.

### Actions

M6.9 should support only conversational actions:

- "要求修改该分镜";
- "查看相关节点";
- "查看评审";

These actions may prefill the composer or focus the relevant node/detail. They must not directly mutate canvas data.

## Review And Retry Cards

### Current State

`review_card` exists and renders accepted/rejected status, score, critique, rubric, retry count and node/version refs.

### M6.9 Requirements

Review cards should:

- be visually distinct from tool status bubbles;
- show rubric axes in a compact score grid;
- show retry count and max attempts clearly;
- link to node detail;
- show whether retry has already been queued or exhausted;
- hide internal reviewer role by default.

Allowed actions:

- view node;
- retry if backend policy allows;
- accept anyway only if backend policy and target phase allow it.

If an action is not currently implemented by backend policy, the UI should not render a fake button.

## Final Video Card

### Current State

`final_video_card` has a basic typed frontend block and backend message block type.

### M6.9 Requirements

The final video card should become the final delivery card:

- playable video;
- poster / thumbnail when available;
- source shot list;
- version id / version number;
- output metadata when available:
  - duration;
  - format;
  - size;
  - resolution;
- status:
  - queued;
  - running;
  - ready;
  - failed;
  - waiting for confirmation;
  - accepted;
  - revision requested;
- confirmation actions:
  - confirm final video;
  - request revision.

Confirmation actions should use the existing HITL decision path when possible. M6.9 should not create a separate final-confirmation mechanism unless the existing `request_user_decision` card cannot represent the interaction.

## Task Timeline

### Purpose

The user needs a readable history of what ClipAnvil did, especially for long-running jobs.

### Data

Timeline entries derive from:

- `agent_task`;
- `agent_event`;
- `generation_job`;
- `artifact_version`;
- `review_record`;
- `sandbox_job`;
- tool status messages.

### User-Facing Labels

Map internal facts to labels:

| Internal Fact | User Label |
| --- | --- |
| `producer_turn` | 理解需求 |
| `update_storyboard` | 更新分镜 |
| `dispatch_craftsman(mode=preview_image)` | 开始生成预览 |
| `worker_generation` + image node | 提交预览任务 |
| `preview_generation_succeeded` | 预览完成 |
| `reviewer_turn` / `review_*` | 评审画面 |
| `retry_generation` | 重新生成 |
| `generate_shot_video` | 开始生成视频 |
| `shot_video_succeeded` | 分镜视频完成 |
| `composer_turn` / `compose_final` | 合成成片 |
| `composition_succeeded` | 成片完成 |
| `request_user_decision` | 请求确认 |

### Display

The timeline should render as a compact vertical list:

- icon / phase chip;
- short label;
- status chip;
- timestamp;
- optional summary;
- expandable diagnostics.

Default collapsed diagnostics should include:

- task id;
- event id;
- job id;
- node id;
- version id;
- role;
- provider;
- sandbox job id.

## Node Detail Enhancements

The Agent node detail drawer should add or improve these sections:

1. **Shot Context**
   - shot key/title/duration/status;
   - incoming/outgoing dependencies;
   - blocked reasons.

2. **Production Trace**
   - latest job;
   - version list;
   - current winner;
   - input refs;
   - stale reasons.

3. **Review Records**
   - latest review status;
   - score;
   - critique;
   - retry recommendation;
   - parent review if this node was regenerated from critique.

4. **Retry Chain**
   - previous attempt nodes/versions;
   - current attempt number;
   - max attempts;
   - exhaustion reason.

5. **Final Composition Trace**
   - source shot video nodes;
   - composition job;
   - final artifact version;
   - final confirmation event.

6. **Sandbox Trace**
   - sandbox job type;
   - command summary;
   - status;
   - duration;
   - exit code;
   - stderr excerpt;
   - output artifact path.

## Backend/API Projection

M6.9 should avoid making the frontend reconstruct the whole production graph from unrelated endpoints.

Add or extend an Agent workspace projection endpoint:

```text
GET /api/agent/workspaces/:workspaceID/production-overview
```

Response shape:

```json
{
  "workspace_id": "uuid",
  "phase": "review",
  "counts": {
    "active_tasks": 1,
    "failed_tasks": 0,
    "waiting_decisions": 1,
    "shots_total": 3,
    "previews_ready": 2,
    "videos_ready": 0,
    "final_outputs_ready": 0
  },
  "shots": [],
  "timeline": [],
  "final_outputs": [],
  "diagnostics": {}
}
```

The endpoint should be read-only and Agent-workspace only. It can reuse the PSS builder and existing sqlc queries, but the response should be structured for UI, not natural-language prompt context.

If implementation cost is lower, M6.9 may first extend the existing Agent thread/bootstrap response with this overview, but the long-term contract should be named and testable as a production overview.

## WebSocket Updates

M6.9 should support overview refresh without full page reload.

When these websocket messages arrive, the frontend should invalidate or patch the production overview:

- `agent.task.updated`;
- `agent.event.created`;
- `agent.message.created`;
- `agent.message.updated`;
- `production.job.updated`;
- `NodeCreated`;
- `NodeUpdated`.

Do not depend on websocket-only fields. On reconnect, refetch the overview endpoint.

## Message Protocol Additions

Existing message blocks remain valid. M6.9 may add:

```ts
type AgentStoryboardBlock = {
  id: string;
  type: "storyboard";
  shots: StoryboardShotSummary[];
};

type AgentTimelineBlock = {
  id: string;
  type: "task_timeline";
  items: AgentTimelineItem[];
};
```

These blocks are optional. The persistent panel sections should not depend on them. The overview endpoint is the preferred source for current workspace state.

## Error Handling

- If overview loading fails, chat remains usable and shows a compact "状态加载失败" row with retry.
- If websocket disconnects, status bar shows offline/reconnecting but keeps the last known overview.
- If a task is running too long, timeline shows elapsed time and last event.
- If a tool call succeeded but async production is still queued, timeline must say "已提交，等待生成完成" instead of implying the artifact is done.
- If a final video is ready but no HITL event exists, the final card should show "成片已生成，等待确认入口恢复" and diagnostics should expose missing event id.

## Testing Strategy

### Unit Tests

Frontend:

- production overview reducer computes current phase from shots/tasks/events/jobs;
- storyboard summary maps shots and node statuses;
- timeline maps internal event/task names to user labels;
- final video card renders ready/failed/waiting states;
- node detail sections render review/sandbox/final trace;
- websocket events invalidate or patch overview state.

Backend:

- production overview endpoint rejects Studio workspace;
- overview includes active/failed task counts;
- overview includes waiting decisions;
- overview includes shot preview/video/review status;
- overview includes final output summaries;
- overview timeline includes task/event/job/sandbox facts;
- diagnostics include ids but do not leak provider secrets.

### Integration Tests

- Create an Agent workspace with seeded shots, preview nodes, review records, video nodes, final node, tasks, events, jobs and sandbox jobs; assert overview response.
- Trigger production terminal event and assert overview changes after refetch.
- Resolve decision event and assert waiting decision count changes.

### Browser E2E

Run through a deterministic or low-cost M6 terminal scenario:

1. Create Agent workspace.
2. Upload a product image.
3. Ask for a 3-shot short video.
4. Confirm storyboard through HITL.
5. Generate preview images.
6. Review and retry at least one rejected preview.
7. Generate shot videos.
8. Compose final video.
9. Confirm final video through HITL.
10. Verify:
    - status bar phase and counts update without refresh;
    - storyboard section shows all shots;
    - timeline shows preview/review/retry/video/composition steps;
    - final video card is playable or shows deterministic mock output;
    - node detail exposes prompt/model/job/version/review/sandbox trace;
    - raw internal role names are hidden in default labels.

## Strict Verification Commands

Backend:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
make server-lint
```

Frontend:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Runtime:

```bash
./scripts/dev-start.sh
./scripts/smoke-m6-9-ux-completion.sh
```

Final:

```bash
git diff --check
```

Browser E2E must use the Vite URL printed by `./scripts/dev-start.sh`.

## Acceptance Criteria

M6.9 is complete when:

- Agent page has a persistent production status bar.
- Agent page has a persistent storyboard section independent of chat history.
- Long-running Agent work is visible as a task timeline and updates from websocket/refetch.
- Review/retry/final output are structured UI, not raw JSON.
- Final video confirmation is available through the existing HITL mechanism or a clearly justified compatible extension.
- Node detail exposes review, retry, dependency, final composition and sandbox traces.
- Refreshing the browser restores the same overview from persisted facts.
- Default labels do not expose Producer/Craftsman/Worker/Reviewer/Composer role names.
- Diagnostic expanders expose task/job/event ids for debugging.
- Full backend, frontend and browser E2E verification passes.

## Implementation Notes

Recommended implementation order:

1. Backend production overview projection.
2. Frontend pure reducers/types for overview, phase, progress and timeline.
3. Status bar and storyboard persistent panel sections.
4. Timeline UI and websocket invalidation.
5. Final video confirmation polishing.
6. Node detail enhancements.
7. Smoke script and terminal browser E2E.

This keeps UI work grounded in stable facts before adding interaction polish.
