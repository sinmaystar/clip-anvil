# M6.4 Agent Chat Shell And Attachments Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first polished user-facing ClipAnvil Agent chat shell and Agent-owned attachment ingestion path.

**Architecture:** Keep internal Producer runtime names, but hide them from the UI. Add an Agent-specific attachment endpoint that creates assets and `source='agent'` source material nodes without reopening ordinary Studio write APIs in Agent mode. The frontend uploads attachments before posting the message, persists attachment metadata in the user message, and refreshes the read-only canvas.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, MinIO storage service, React 19, TypeScript 6, TanStack Query, Vite, plain CSS.

---

### Task 1: Backend Agent Attachment API

**Files:**
- Modify: `apps/server/internal/api/agent_handler.go`
- Modify: `apps/server/internal/api/agent_response.go`
- Modify: `apps/server/cmd/server/main.go`
- Test: `apps/server/internal/api/agent_handler_test.go`

- [ ] Write failing tests for attachment validation and message content persistence:
  - `TestAgentMessageContentIncludesAttachments`
  - `TestAgentAttachmentRequestRejectsUnsupportedMIME`

- [ ] Implement request/response types:
  - `postAgentAttachmentResponse`
  - `agentAttachmentResponse`
  - `agentMessageAttachmentRequest`

- [ ] Implement `AgentHandler.PostAttachment`:
  - require Agent workspace ownership.
  - accept multipart `file`.
  - create image/video assets through storage.
  - create text assets through `CreateTextMediaAsset`.
  - create source material node through `CreateAgentMediaNode`.

- [ ] Register `POST /api/agent/workspaces/:workspaceID/attachments`.

- [ ] Extend `postAgentMessageRequest` with `Attachments`.

- [ ] Persist valid attachment metadata in `agent_message.content.attachments`.

- [ ] Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api
```

Expected: PASS.

### Task 2: Frontend Attachment Helpers

**Files:**
- Create: `apps/web/src/lib/agentAttachments.ts`
- Create: `apps/web/src/lib/agentAttachments.test.mjs`
- Modify: `apps/web/src/lib/agentApi.ts`
- Modify: `apps/web/tsconfig.test.json`
- Modify: `apps/web/package.json`

- [ ] Write failing tests for accepted file type classification and attachment label formatting.

- [ ] Add `AgentAttachment` and `PostAgentAttachmentResponse` API types.

- [ ] Add `uploadAgentAttachment(workspaceId, file)`.

- [ ] Add helpers:
  - `agentAttachmentKindForFile`
  - `formatAgentAttachmentLabel`
  - `attachmentAccept`

- [ ] Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

### Task 3: Modern ClipAnvil Chat Shell

**Files:**
- Create: `apps/web/src/lib/agentLayout.ts`
- Create: `apps/web/src/lib/agentLayout.test.mjs`
- Modify: `apps/web/src/pages/AgentWorkspacePage.tsx`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/tsconfig.test.json`
- Modify: `apps/web/package.json`

- [ ] Write failing tests for width clamping.

- [ ] Replace visible `Producer` copy with `ClipAnvil`.

- [ ] Add status dot class mapping:
  - connected -> green.
  - connecting/reconnecting -> amber.
  - offline -> red.

- [ ] Add resize handle and persisted width.

- [ ] Redesign the composer with icon-only attachment and send buttons.

- [ ] Render attachment chips before send and in user messages.

- [ ] Refresh canvas query after attachment upload.

- [ ] Run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

### Task 4: End-To-End Verification

**Files:**
- No code files expected.

- [ ] Run full automated verification:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
make server-lint
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web test:connections
git diff --check
```

- [ ] Start runtime:

```bash
./scripts/dev-start.sh
```

- [ ] Browser e2e:
  - Register a fresh user.
  - Create an Agent workspace.
  - Confirm no visible `Producer` label.
  - Confirm `ClipAnvil` title and connection dot.
  - Upload a `.txt` file through the composer.
  - Confirm a chip appears.
  - Send a message.
  - Confirm a source node appears in read-only canvas.
  - Confirm streaming assistant reply.
  - Refresh and confirm message + attachment + node persist.
  - Query DB for `agent_message.content.attachments`.

- [ ] Stop runtime and confirm ports are no longer listening.
