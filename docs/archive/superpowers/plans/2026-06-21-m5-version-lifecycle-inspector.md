# M5 Version Lifecycle Inspector Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every Studio node run immediately create a visible `artifact_version` slot, keep that version synchronized with its `generation_job`, and let users preview current and historical versions in the Inspector before switching current.

**Architecture:** Keep `generation_job` as the execution/audit record, but make `artifact_version` the user-facing lifecycle record. A run creates `generation_job(status=queued)` and `artifact_version(status=queued, job_id=job.id)` in the same submission path. Runner progress updates both job and version. Success updates the existing version with the persisted asset and marks it current; failure leaves a failed version without changing current.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, PostgreSQL migrations, React 19, TypeScript, React Flow, existing Studio production APIs.

---

## Phase 1: Data Model and SQLC

- [ ] Add a migration extending `artifact_version` with `status`, `progress`, `error_code`, `error_message`, `provider_request`, `provider_response`, `started_at`, and `completed_at`.
- [ ] Add a partial unique index for `artifact_version(job_id)` when `job_id IS NOT NULL`.
- [ ] Add SQL queries for fetching/updating versions by `job_id` across queued/running/progress/succeeded/failed states.
- [ ] Regenerate sqlc output.

Deliverable standard:
- Existing versions migrate as `succeeded` with `progress=100`.
- New generated db models expose version status/progress/error/provider fields.

Acceptance standard:
- `make sqlc-generate` succeeds.
- Existing code compiles after generated model changes are wired.

## Phase 2: Backend Version-First Lifecycle

- [ ] Change node run submission to create a queued version immediately after creating the queued job.
- [ ] Return the queued version in `RunNodeResponse`.
- [ ] Update runner/service status helpers so job running/progress/failed updates also update the bound version.
- [ ] Change success persistence to update the pre-created version instead of inserting a second version.
- [ ] Keep current/winner unchanged on failed versions.
- [ ] Reject selecting non-succeeded versions as current.
- [ ] Keep downstream input resolution using only `media_node.current_version_id`.

Deliverable standard:
- Every production-generated version has exactly one `generation_job`.
- A successful run transitions one version from queued/running to succeeded/current.
- A failed run leaves a failed version and preserves the previous current version.

Acceptance standard:
- Backend tests cover queued version creation, success update, failure preservation, and select-current validation.
- `make server-test` and `make server-build` succeed.

## Phase 3: API Contract

- [ ] Extend `artifactVersionResponse` and frontend API types with version lifecycle fields.
- [ ] Include asset preview only when the version has an asset.
- [ ] Keep `latest_job` for diagnostics, but let `versions` carry the primary user-visible run state.
- [ ] Ensure production state reload after run can show queued/running versions after page refresh.

Deliverable standard:
- API responses include `status`, `progress`, error fields, provider request/response, and timestamps for every version.

Acceptance standard:
- Existing production-state consumers continue to work.
- Queued/failed versions serialize without asset assumptions.

## Phase 4: Inspector Version Preview UX

- [ ] Add Inspector-local preview target state for selected version rows.
- [ ] Show a compact current/history preview panel above the version timeline.
- [ ] Let row click preview a version without changing current.
- [ ] Show `Set current` only for succeeded non-current versions.
- [ ] Show running progress and failed error summaries in version rows.
- [ ] Wrap long provider/debug JSON so it does not create extreme horizontal scrolling.

Deliverable standard:
- Users can inspect current, running, failed, and historical succeeded versions from the Inspector.
- Previewing history is visually distinct from setting current.

Acceptance standard:
- Frontend tests cover row display and preview/select semantics.
- `pnpm --filter @clip-anvil/web... build` and `pnpm --filter @clip-anvil/web lint` succeed.

## Phase 5: E2E Verification

- [ ] Mock-provider E2E: run text node, observe queued/running version immediately, then succeeded/current version.
- [ ] Mock-provider E2E: run a second version, preview old version, then set old version current.
- [ ] Failure E2E: failed version remains visible and old current remains selected.
- [ ] Real-provider smoke, when env is configured: image/video long task creates running version immediately, later updates the same version to current.

Deliverable standard:
- Manual and automated verification show job/version one-to-one lifecycle.

Acceptance standard:
- `git diff --check` succeeds.
- Verification output explicitly reports whether real-provider E2E was run or skipped.
