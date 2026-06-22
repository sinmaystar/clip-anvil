# M6.1 Agent Runtime Persistence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Build the generic Agent runtime persistence foundation for Producer, Craftsman, Reviewer, Composer, Worker, and future Eino Graph resume state.

**Architecture:** Add a narrow database-backed runtime layer below future Agent APIs and Graph orchestration. The layer owns thread/message/task/event/checkpoint persistence and exposes service methods that future M6 phases can call without touching HTTP, WebSocket, production generation, or UI code.

**Tech Stack:** PostgreSQL 16 migrations with goose, sqlc v1.31.1, pgx v5, Go 1.26, package `internal/agent/runtime`, existing `internal/store/db` generated package.

---

## File Structure

- Create `apps/server/migrations/015_m6_agent_runtime.sql`
  - Defines `agent_thread`, `agent_message`, `agent_task`, `agent_event`, `eino_checkpoint` and indexes.
- Create `apps/server/sqlc/queries/agent_thread.sql`
  - Thread create/get/list/update queries.
- Create `apps/server/sqlc/queries/agent_message.sql`
  - Message append/list queries. Atomic seq allocation is implemented in service transactions.
- Create `apps/server/sqlc/queries/agent_task.sql`
  - Task create/get/list/status transition queries.
- Create `apps/server/sqlc/queries/agent_event.sql`
  - Event create/list/status queries.
- Create `apps/server/sqlc/queries/eino_checkpoint.sql`
  - Checkpoint upsert/get/delete/list queries.
- Generate `apps/server/internal/store/db/agent_*.sql.go`, `eino_checkpoint.sql.go`, and updated `models.go`.
- Create `apps/server/internal/agent/runtime/service.go`
  - Public runtime service API used by later Agent phases.
- Create `apps/server/internal/agent/runtime/service_test.go`
  - Test service behavior with lightweight fake dbtx adapters where possible and pure validation for request contracts.

## Task 1: Migration

**Files:**
- Create: `apps/server/migrations/015_m6_agent_runtime.sql`

- [x] **Step 1: Add migration**

Create the five runtime tables using the schema from `docs/superpowers/specs/2026-06-21-m6-1-agent-runtime-persistence-design.md`. Use TEXT + CHECK constraints for Agent-specific states so M6 can evolve without adding many PostgreSQL enum migrations.

- [x] **Step 2: Verify migration syntax**

Run:

```bash
make migrate-up
```

Expected: goose applies migration `015_m6_agent_runtime.sql` successfully.

## Task 2: sqlc Query Surface

**Files:**
- Create: `apps/server/sqlc/queries/agent_thread.sql`
- Create: `apps/server/sqlc/queries/agent_message.sql`
- Create: `apps/server/sqlc/queries/agent_task.sql`
- Create: `apps/server/sqlc/queries/agent_event.sql`
- Create: `apps/server/sqlc/queries/eino_checkpoint.sql`
- Generate: `apps/server/internal/store/db/*.sql.go`

- [x] **Step 1: Add query files**

Required query names:

```text
CreateAgentThread
GetAgentThreadByID
GetActiveProducerThreadByWorkspace
ListAgentThreadsByWorkspace
UpdateAgentThreadStatus
SetAgentThreadCheckpoint
CreateAgentMessage
NextAgentMessageSeq
ListAgentMessagesByThread
CreateAgentTask
GetAgentTaskByID
ListAgentTasksByWorkspaceStatus
MarkAgentTaskRunning
MarkAgentTaskSucceeded
MarkAgentTaskFailed
MarkAgentTaskCancelled
MarkAgentTaskWaitingForUser
CreateAgentEvent
GetAgentEventByID
ListAgentEventsByWorkspaceStatus
MarkAgentEventHandled
UpsertEinoCheckpoint
GetEinoCheckpoint
DeleteEinoCheckpoint
ListEinoCheckpointsByThread
```

- [x] **Step 2: Generate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: generated Go compiles and no query name collision occurs.

## Task 3: Runtime Service Tests

**Files:**
- Create: `apps/server/internal/agent/runtime/service_test.go`

- [x] **Step 1: Write tests before service implementation**

Cover these behaviors:

```go
func TestNewServiceRejectsNilPool(t *testing.T)
func TestAppendMessageRejectsMissingThread(t *testing.T)
func TestAppendMessageDefaultsTextMessageType(t *testing.T)
func TestCreateTaskRejectsInvalidAttempts(t *testing.T)
func TestCheckpointKeyUsesWorkspaceThreadAndTask(t *testing.T)
```

The tests should fail because `runtime.NewService`, request structs, and helpers do not exist yet.

- [x] **Step 2: Verify red**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime
```

Expected: FAIL with missing package/types/functions.

## Task 4: Runtime Service Implementation

**Files:**
- Create: `apps/server/internal/agent/runtime/service.go`

- [x] **Step 1: Implement request validation and constructor**

Add `Service`, `NewService`, request structs, and validation errors. Keep this layer free of HTTP, WebSocket, Eino Graph execution, and production service calls.

- [x] **Step 2: Implement transaction-backed message append**

`AppendMessage` must begin a pgx transaction, call `NextAgentMessageSeq` through `queries.WithTx(tx)`, create the message with that seq, and commit. Roll back on all errors.

- [x] **Step 3: Implement task/event/checkpoint wrappers**

Expose thin methods around sqlc queries for creating tasks/events, marking statuses, and upserting/getting/deleting checkpoints.

- [x] **Step 4: Verify green**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/runtime
```

Expected: PASS.

## Task 5: Full Verification

**Files:**
- All M6.1 files.

- [x] **Step 1: Run strict M6.1 commands**

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
git diff --check
```

Expected: all commands succeed.

- [x] **Step 2: Frontend commands only if frontend changed**

No frontend files should change in M6.1. If they do, also run:

```bash
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: both commands succeed.

## Self-Review Checklist

- [x] The implementation does not add HTTP APIs, WebSocket handlers, frontend UI, Eino Graph execution, HITL UI cards, Tool registry, Storyboard/PSS, or production generation calls.
- [x] `thread` and `message` persistence is generic Agent runtime infrastructure, not Producer-only storage.
- [x] Producer uniqueness is limited to one active workspace-scoped Producer thread per workspace.
- [x] Message `seq` is allocated inside a transaction.
- [x] Checkpoints are generic Eino persistence records, not tied to one Agent role.
- [x] Verification commands match the M6.1 spec.
