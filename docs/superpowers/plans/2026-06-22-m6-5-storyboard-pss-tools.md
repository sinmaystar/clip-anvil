# M6.5 Storyboard / PSS / Production State Edges Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add durable Agent storyboard facts, production-state/PSS tooling, and ProducerGraph PSS injection.

**Architecture:** Add `shot` and `shot_dependency` as Agent production facts, with optional `media_node.shot_id` projection links. Implement a ncustom edge storyboard service for transactional writes, a deterministic Producer PSS builder for state projection, and two Agent tools wired into the existing Edge Registry and Producer tool loop.

**Tech Stack:** Go 1.26, pgx/sqlc/goose, Eino ProducerGraph, existing Agent runtime/tool registry/UI message protocol, React 19 + Vite 8 + React Flow read-only Agent canvas.

---

## File Map

Backend schema and queries:

- Create `apps/server/migrations/018_m6_storyboard_pss.sql`: `shot`, `shot_dependency`, `media_node.shot_id`.
- Create `apps/server/sqlc/queries/shot.sql`: shot CRUD and active listing.
- Create `apps/server/sqlc/queries/shot_dependency.sql`: dependency create/list/delete.
- Modify `apps/server/sqlc/queries/node.sql`: add `UpdateMediaNodeShot` and `ListMediaNodesByShot`.
- Regenerate `apps/server/internal/store/db/*.go`.

Backend services/tools:

- Create `apps/server/internal/agent/storyboard/service.go`: transactional storyboard write service and validation.
- Create `apps/server/internal/agent/storyboard/service_test.go`: service validation and transaction tests.
- Create `apps/server/internal/agent/pss/producer.go`: deterministic Producer PSS builder.
- Create `apps/server/internal/agent/pss/producer_test.go`: empty/populated PSS tests.
- Create `apps/server/internal/agent/tools/production_state.go`: `get_production_state`.
- Create `apps/server/internal/agent/tools/storyboard.go`: `update_storyboard`.
- Create tests in `apps/server/internal/agent/tools/*_test.go`.
- Modify `apps/server/internal/agent/producer/types.go`: add PSS text/structured state to `ProducerContext`.
- Modify `apps/server/internal/agent/producer/context_loader.go`: build PSS before model calls.
- Modify `apps/server/internal/agent/producer/model_responder.go`: include PSS in model prompt.
- Modify `apps/server/internal/agent/producer/*_test.go`: PSS injection and tool fixture tests.
- Modify `apps/server/cmd/server/main.go`: wire PSS builder, storyboard service, and new tools.

Frontend:

- Modify `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`: display `shot_id`/shot metadata when present.
- Modify `apps/web/src/lib/agentReadonlyCanvas.test.mjs`: source-level assertion for shot fields.
- Modify `apps/web/src/lib/agentMessageBlocks.test.mjs` or `agentTasks.test.mjs`: assert tool labels can render new tools if needed.

Verification:

- Run migration/sqlc/server tests.
- Run frontend tests/build/lint.
- Start with `./scripts/dev-start.sh`.
- Browser E2E: create Agent workspace, send deterministic storyboard request, verify tool status, DB shots, PSS, refresh persistence, and Agent-mode write guard.

## Task 1: Storyboard Schema And Queries

**Files:**

- Create: `apps/server/migrations/018_m6_storyboard_pss.sql`
- Create: `apps/server/sqlc/queries/shot.sql`
- Create: `apps/server/sqlc/queries/shot_dependency.sql`
- Modify: `apps/server/sqlc/queries/node.sql`

- [ ] **Step 1: Add migration**

Create `018_m6_storyboard_pss.sql` with:

```sql
-- +goose Up
CREATE TABLE shot (...);
CREATE TABLE shot_dependency (...);
ALTER TABLE media_node ADD COLUMN shot_id UUID REFERENCES shot(id) ON DELETE SET NULL;
CREATE INDEX idx_media_node_shot ON media_node(shot_id);

-- +goose Down
DROP INDEX IF EXISTS idx_media_node_shot;
ALTER TABLE media_node DROP COLUMN IF EXISTS shot_id;
DROP TABLE IF EXISTS shot_dependency;
DROP TABLE IF EXISTS shot;
```

- [ ] **Step 2: Add sqlc queries**

Add shot and dependency queries with names from the spec: `CreateShot`, `GetShotByID`, `GetShotByClientKey`, `ListActiveShotsByWorkspace`, `UpdateShot`, `ArchiveShot`, `SetShotCraftsmanThread`, `CreateShotDependency`, `ListShotDependenciesByWorkspace`, `DeleteShotDependenciesByWorkspace`, `DeleteShotDependenciesForShot`.

- [ ] **Step 3: Add node shot queries**

Add:

```sql
-- name: UpdateMediaNodeShot :one
UPDATE media_node SET shot_id = $2, updated_at = now()
WHERE id = $1 AND workspace_id = $3
RETURNING *;

-- name: ListMediaNodesByShot :many
SELECT * FROM media_node
WHERE workspace_id = $1 AND shot_id = $2
ORDER BY created_at;
```

- [ ] **Step 4: Generate and verify**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: generated models include `Shot`, `ShotDependency`, and `MediaNode.ShotID`.

## Task 2: Storyboard Service

**Files:**

- Create: `apps/server/internal/agent/storyboard/service.go`
- Create: `apps/server/internal/agent/storyboard/service_test.go`

- [ ] **Step 1: Write service tests**

Tests must cover:

- replace creates ordered active shots;
- replace archives omitted shots;
- dependency references resolve by `client_key`;
- unresolved dependency fails before dependency write;
- non-Agent workspace writes return `ErrAgentWorkspaceRequired`.

- [ ] **Step 2: Run RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/storyboard -count=1
```

Expected: FAIL because package/service is missing.

- [ ] **Step 3: Implement service**

Implement a store interface backed by sqlc and a `Service.UpdateStoryboard(ctx, input)` method that:

- validates workspace is `agent`;
- supports `replace`, `upsert`, `patch`, `archive`;
- upserts shots by UUID or `client_key`;
- archives omitted active shots on `replace`;
- deletes workspace dependencies before replacing dependencies;
- creates new dependencies after resolving all refs;
- optionally links existing node IDs to shot IDs;
- returns counts and resulting active shots/dependencies.

- [ ] **Step 4: Run GREEN**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/storyboard -count=1
```

Expected: PASS.

## Task 3: Producer PSS Builder

**Files:**

- Create: `apps/server/internal/agent/pss/producer.go`
- Create: `apps/server/internal/agent/pss/producer_test.go`

- [ ] **Step 1: Write PSS tests**

Tests must cover:

- empty storyboard renders `当前还没有 storyboard`;
- populated storyboard lists `[shot-01]` and dependency `shot-01 -> shot-02`;
- output excludes signed URLs and provider secrets;
- structured state contains `workspace`, `shots`, `shot_dependencies`, `nodes`, `pending_decisions`, and `running_tasks`.

- [ ] **Step 2: Run RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/pss -count=1
```

Expected: FAIL because package is missing.

- [ ] **Step 3: Implement builder**

Implement `Builder.BuildProducerPSS(ctx, workspaceID)` using a ncustom edge store interface and deterministic sorting.

- [ ] **Step 4: Run GREEN**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/pss -count=1
```

Expected: PASS.

## Task 4: Agent Edges

**Files:**

- Create: `apps/server/internal/agent/tools/production_state.go`
- Create: `apps/server/internal/agent/tools/storyboard.go`
- Test: `apps/server/internal/agent/tools/context_test.go` or new test files.

- [ ] **Step 1: Write tool tests**

Tests must assert:

- `get_production_state` definition is read-only and returns PSS text;
- `update_storyboard` definition has correct safety and calls storyboard service;
- invalid `update_storyboard` args return an error;
- `update_storyboard` does not mark `WritesCanvas`.

- [ ] **Step 2: Run RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'Test(GetProductionState|UpdateStoryboard)' -count=1
```

Expected: FAIL because tools are missing.

- [ ] **Step 3: Implement tools**

Implement executors matching the existing `tools.Executor` interface.

- [ ] **Step 4: Run GREEN**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/tools -run 'Test(GetProductionState|UpdateStoryboard)' -count=1
```

Expected: PASS.

## Task 5: ProducerGraph PSS Integration

**Files:**

- Modify: `apps/server/internal/agent/producer/types.go`
- Modify: `apps/server/internal/agent/producer/context_loader.go`
- Modify: `apps/server/internal/agent/producer/model_responder.go`
- Modify tests under `apps/server/internal/agent/producer/`

- [ ] **Step 1: Add failing producer tests**

Tests must assert:

- context loader adds PSS text from a builder;
- model responder includes PSS text in model messages;
- fixture tool call to `update_storyboard` can pass through the existing loop.

- [ ] **Step 2: Run RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'Test.*PSS|Test.*Storyboard' -count=1
```

Expected: FAIL because PSS fields are missing.

- [ ] **Step 3: Implement integration**

Add PSS fields to `ProducerContext`, load them in `RuntimeContextLoader`, and inject them into the responder's system/context prompt.

- [ ] **Step 4: Run GREEN**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/agent/producer -run 'Test.*PSS|Test.*Storyboard' -count=1
```

Expected: PASS.

## Task 6: Wire Runtime

**Files:**

- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Register services and tools**

Wire:

- `storyboard.NewService(pgPool, queries)`
- `pss.NewBuilder(queries)`
- `tools.NewGetProductionStateEdge(pssBuilder)`
- `tools.NewUpdateStoryboardEdge(storyboardService)`

- [ ] **Step 2: Compile**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

## Task 7: Frontend Read-Only Shot Display

**Files:**

- Modify: `apps/web/src/components/agent/AgentNodeDetailDrawer.tsx`
- Modify: `apps/web/src/lib/agentReadonlyCanvas.test.mjs`

- [ ] **Step 1: Write source-level test**

Assert `AgentNodeDetailDrawer.tsx` includes `shot_id` and a read-only label for shot association.

- [ ] **Step 2: Run RED**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL if shot display is missing.

- [ ] **Step 3: Implement minimal read-only shot display**

Show shot ID/client key/title when available from node/detail payload. Do not add edit controls.

- [ ] **Step 4: Run GREEN**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

## Task 8: Full Verification And E2E

**Files:**

- No code changes unless verification exposes defects.

- [ ] **Step 1: Run backend verification**

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

- [ ] **Step 2: Run frontend verification**

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

- [ ] **Step 3: Browser E2E**

Start:

```bash
./scripts/dev-start.sh
```

Use the printed Vite URL. Verify:

- Agent workspace loads;
- send prompt that triggers deterministic `update_storyboard` fixture or real model tool call;
- chat shows tool status;
- database has active shots;
- `get_production_state` includes those shots;
- refresh preserves state;
- ordinary Agent canvas mutation API still returns `403`.

## Plan Self-Review

- Spec coverage: schema, queries, service, PSS, tools, Producer integration, frontend read-only shot display, and E2E are covered.
- Scope guard: no preview/video/review/composer generation is included.
- Migration numbering: uses `018_m6_storyboard_pss.sql`, not stale `016`.
- Type consistency: tools use existing `agenttools.Executor`; Producer integration extends current `ProducerContext`.
