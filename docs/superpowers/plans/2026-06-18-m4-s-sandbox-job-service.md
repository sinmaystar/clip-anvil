# M4.S Sandbox Job Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make sandbox execution a first-class production foundation so FFmpeg, Agent shell commands, and future internal media tools never run inside the application process.

**Architecture:** Add a durable `sandbox_job` record for each sandbox execution and route internal media operations through a `sandbox.JobService`. `generation_job` remains the production-level run record; sandbox details live in `sandbox_job` and are referenced in provider request/response summaries. The sandbox service owns workspace sandbox lifecycle, input/output transfer, command execution, output inspection, and failure attribution.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, PostgreSQL migrations, OpenSandbox SDK wrapper, MinIO presigned URLs, existing production Provider Bridge.

---

## File Structure

- Create: `apps/server/migrations/012_m4_s_sandbox_job_service.sql`
  - Adds `sandbox_job` durable execution table.
- Create: `apps/server/sqlc/queries/sandbox_job.sql`
  - sqlc queries for job create/start/succeed/fail/get/list.
- Create: `apps/server/internal/sandbox/job_service.go`
  - High-level sandbox execution service used by production and future Agent tools.
- Create: `apps/server/internal/sandbox/job_service_test.go`
  - Unit tests for lifecycle, command construction, success, and failure persistence.
- Modify: `apps/server/internal/production/internal_ffmpeg_provider.go`
  - Remove app-local FFmpeg execution; call sandbox-backed frame extraction.
- Modify: `apps/server/internal/production/internal_ffmpeg_provider_test.go`
  - Assert provider delegates to sandbox extractor and records sandbox job IDs.
- Modify: `apps/server/internal/production/provider.go`
  - Allow injecting `internal_ffmpeg` provider instead of constructing a local executor.
- Modify: `apps/server/internal/production/service.go`
  - Preserve sandbox job IDs in provider summaries and failure records.
- Modify: `apps/server/cmd/server/main.go`
  - Wire `sandbox.JobService` into production provider registry.
- Modify: `scripts/smoke-m4-5.sh`
  - Verify sandbox job persistence for internal FFmpeg success and failure.
- Modify: milestone and design docs under `docs/milestones/` and `docs/superpowers/specs/`
  - Make sandbox execution boundary explicit across M4/M5/M6.

## Tasks

### Task 1: Persist Sandbox Jobs

**Files:**
- Create: `apps/server/migrations/012_m4_s_sandbox_job_service.sql`
- Create: `apps/server/sqlc/queries/sandbox_job.sql`
- Generated: `apps/server/internal/store/db/sandbox_job.sql.go`
- Generated: `apps/server/internal/store/db/models.go`

- [ ] **Step 1: Add migration**

Create a `sandbox_job` table with workspace, optional target node, operation, status, sandbox ID, command, input/output JSON, exit code, stdout/stderr, duration, and structured error fields.

- [ ] **Step 2: Add sqlc queries**

Add create/start/succeed/fail/get/list queries with explicit params.

- [ ] **Step 3: Generate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: generated DB models and query methods include `SandboxJob`.

### Task 2: Implement Sandbox Job Service With Tests

**Files:**
- Create: `apps/server/internal/sandbox/job_service_test.go`
- Create: `apps/server/internal/sandbox/job_service.go`

- [ ] **Step 1: Write failing tests**

Cover:

- Successful command creates, starts, and succeeds a sandbox job.
- Non-zero exit creates a failed sandbox job with exit code and stderr.
- Frame extraction uses sandbox paths and never local `exec.Command`.

- [ ] **Step 2: Run tests and confirm RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestJobService|TestExtractFrame' -count=1
```

Expected: fail because `JobService` does not exist.

- [ ] **Step 3: Implement minimal service**

Implement `JobService.RunCommand` and `JobService.ExtractFrame`. The service must call `Manager.EnsureSandbox`, `EnsureWorkspaceLayout`, `RunExec`, `InspectArtifact`, and sandbox-side `UploadToMinIO`.

- [ ] **Step 4: Confirm GREEN**

Run the same test command and expect PASS.

### Task 3: Move Internal FFmpeg Provider To Sandbox

**Files:**
- Modify: `apps/server/internal/production/internal_ffmpeg_provider.go`
- Modify: `apps/server/internal/production/internal_ffmpeg_provider_test.go`
- Modify: `apps/server/internal/production/provider.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Update tests first**

Provider tests should use a fake sandbox frame runner and assert `ProviderResponse["sandbox_job_id"]` exists.

- [ ] **Step 2: Confirm RED**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/production -run TestInternalFFmpegProvider -count=1
```

Expected: fail until provider is refactored.

- [ ] **Step 3: Refactor provider**

Remove `os/exec` local FFmpeg implementation. Keep mock extraction for deterministic tests, but real extraction must require a sandbox frame runner.

- [ ] **Step 4: Wire main**

Construct `sandbox.JobService` in `main.go`, then inject sandbox-backed `internal_ffmpeg` provider into the registry.

### Task 4: Update Smoke And Docs

**Files:**
- Modify: `scripts/smoke-m4-5.sh`
- Modify: `docs/milestones/m2-opensandbox-workspace-sandbox.md`
- Modify: `docs/milestones/m3-m6-studio-agent-roadmap.md`
- Modify: `docs/milestones/m4-shared-production-foundation.md`
- Modify: `docs/superpowers/specs/2026-06-17-opensandbox-workspace-sandbox-design.md`
- Modify: `docs/superpowers/specs/2026-06-18-studio-agent-shared-production-design.md`
- Modify: `docs/superpowers/specs/2026-06-18-production-database-technical-design.md`

- [ ] **Step 1: Update smoke**

Check latest internal FFmpeg job responses and assert sandbox job IDs are persisted.

- [ ] **Step 2: Update docs**

Document that M4.1-M4.4 are still valid but now explicitly treat sandbox execution as a durable execution substrate. M4.5, M5, and M6 must require sandbox-backed internal media and Agent shell execution.

### Task 5: Full Verification

- [ ] **Step 1: Run migrations and sqlc**

```bash
make migrate-up
make sqlc-generate
```

- [ ] **Step 2: Run backend checks**

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

- [ ] **Step 3: Run frontend build**

```bash
pnpm --filter @clip-anvil/web... build
```

- [ ] **Step 4: Run M4 smoke scripts**

Run M4.1 through M4.5 smoke scripts against a live local backend and confirm M4.5 creates sandbox job records for FFmpeg operations.

- [ ] **Step 5: Final static checks**

```bash
rg -n 'exec\\.Command|ffmpeg' apps/server/internal
git diff --check
```

Expected: no app-local FFmpeg execution path remains; docs and code have no whitespace errors.
