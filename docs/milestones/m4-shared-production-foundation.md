# M4 Shared Production Foundation

**Status**: Completed
**Date**: 2026-06-18
**Completed**: 2026-06-18
**Parent roadmap**: [M3-M6 Studio / Agent roadmap](./m3-m6-studio-agent-roadmap.md)

## Goal

Build the shared production core used by both Studio and Agent mode. M4 is about durable production facts and execution flow, not full Studio UX and not full Agent orchestration.

After M4, ClipAnvil supports this loop:

```text
node inputs -> GenerationIntent -> generation_job -> artifact_version -> current winner -> downstream stale -> failure/retry records
```

## Confirmed Decisions

- M4 may rebuild and clean local test data. Historical data compatibility is not required.
- M4 must be delivered in small phases, each with its own acceptance gate.
- Tests should use mock providers by default. Real Volcengine adapters may be added behind configuration when API details are needed.
- Provider API keys and model choices should be configured through local environment files, not committed defaults.
- Expensive or higher quality models are user-facing/runtime choices; development and automated tests should prefer cheap or mock models.
- Capability validation failures, provider failures, internal processing failures, and retry exhaustion must be persisted as failed `generation_job` records.
- Reference Pack depth in M4 is limited to schema, service behavior, GenerationIntent expansion, and stale rules. Rich Studio UI belongs to M5.
- Sandbox execution is a production boundary, not an Agent-only helper. FFmpeg, Agent shell commands, Composer commands, Python scripts, ImageMagick, yt-dlp, and other unpredictable resource consumers must run through a durable Sandbox Job Service, never inside the application process.
- `generation_job` records production intent and user-visible run state; `sandbox_job` records sandbox execution facts such as sandbox id, command, input/output transfer, stdout/stderr, exit code, duration, and sandbox-level errors.

## Scope

M4 includes:

- Production schema convergence.
- Durable Sandbox Job Service for internal command and media execution.
- GenerationIntent service boundary.
- Mock Provider Bridge and later real provider adapters behind config.
- Job, artifact version, current winner, failure, retry, and stale persistence.
- Model capability validation.
- Internal media operations for first-frame and last-frame extraction.
- Reference Pack data foundation.

M4 does not include:

- Rich Studio property panel and Prompt `@` editor UX.
- Full Agent Producer/Craftsman/Worker orchestration.
- Shot, PSS, HITL, Eino checkpoint, or Agent event runtime.
- Complete Volcengine production-grade rollout.

## Completion Summary

M4 has been completed as a phased backend production foundation. The delivered scope includes schema convergence, GenerationIntent, Provider Bridge, capability validation, durable failure/retry records, input-hash freshness, stale propagation, Reference Pack data foundation, sandbox-backed internal FFmpeg operations, Sandbox Job Service, and read APIs for M5 Studio.

Completed phases:

| Phase | Status | Evidence |
|---|---|---|
| M4.S Sandbox Job Service Foundation | Completed | `sandbox_job` service implemented; FFmpeg execution moved out of the app process; M4.5/M4.6 smoke validates linked sandbox jobs. |
| M4.1 Schema Convergence And Mock Text Run | Completed | `scripts/smoke-m4-1.sh` validates text job/version/current winner. |
| M4.2 GenerationIntent And Provider Bridge Skeleton | Completed | `scripts/smoke-m4-2.sh` validates intent/provider bridge and missing provider-key failure persistence. |
| M4.3 Capability Validation, Failure Records, And Retry | Completed | `scripts/smoke-m4-3.sh` validates capability mismatch, failed jobs, and retry attempts. |
| M4.4 Input Hash And Stale Propagation | Completed | `scripts/smoke-m4-4.sh` validates upstream winner changes, stale reason records, and downstream refresh. |
| M4.5 Internal Media Operations And Reference Pack Foundation | Completed | `scripts/smoke-m4-5.sh` validates Reference Pack expansion, stale rules, internal FFmpeg success, and persisted failures. |
| M4.6 Production Read API | Completed | `scripts/smoke-m4-6.sh` validates capability, version, production state, job, sandbox job list, and sandbox job detail reads. |

Final verification run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
CLIPANVIL_API_BASE=http://127.0.0.1:8891/api scripts/smoke-m4-1.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8891/api scripts/smoke-m4-2.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8891/api scripts/smoke-m4-3.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8891/api scripts/smoke-m4-4.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8891/api scripts/smoke-m4-5.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8891/api scripts/smoke-m4-6.sh
rg -n 'exec\.Command|os/exec' apps/server/internal
git diff --check
```

Final acceptance notes:

- M4 uses mock providers by default for tests and local smoke.
- Volcengine remains behind configuration and is not required for automated acceptance.
- The app process no longer owns FFmpeg execution; internal media operations are sandbox-backed and persist `sandbox_job` facts.
- M4 intentionally stops at the shared backend foundation. Rich Studio controls belong to M5; full Agent orchestration belongs to M6.

## Completed Phase Plan

### M4.S Sandbox Job Service Foundation

Goal: make sandbox execution a first-class shared production substrate before internal media operations and Agent tools depend on it.

Delivery target:

- A durable `sandbox_job` row exists for every sandbox execution used by production or Agent tooling.
- The Sandbox Job Service owns `EnsureSandbox`, workspace layout, input download, sandbox command execution, output inspection, sandbox-side upload, and failure attribution.
- Production code and Agent runtime do not need to know sandbox id, volume name, temporary paths, or container lifecycle details.

Work items:

- Add `sandbox_job`.
- Add sqlc queries for create/start/succeed/fail/link/list.
- Add `internal/sandbox.JobService`.
- Support command execution and frame extraction through the service.
- Link sandbox job metadata into provider request/response summaries and `generation_job`.

Acceptance:

- Successful sandbox command execution creates `sandbox_job=succeeded`.
- Non-zero exit, timeout, sandbox creation failure, input download failure, output inspection failure, and output upload failure create `sandbox_job=failed`.
- Internal FFmpeg does not use app-local `exec.Command`.
- `generation_job.provider_response.sandbox_job_id` identifies sandbox execution when a provider delegates to sandbox.
- Sandbox id, volume name, and temporary file paths do not participate in input hash.

E2E / self-test standard:

1. Run an internal media operation from a video input.
2. Confirm the operation creates a `generation_job`.
3. Confirm the operation creates a linked `sandbox_job`.
4. Confirm output asset storage is written by sandbox-side upload, then persisted by the application as `media_asset` and `artifact_version`.
5. Force an internal media failure and confirm both `sandbox_job=failed` and `generation_job=failed` are readable.

Completion evidence:

- `sandbox_job` stores durable sandbox execution facts and is linked from sandbox-backed generation jobs.
- `internal/sandbox.JobService` owns sandbox creation, input transfer, command execution, output inspection, sandbox-side upload, and failure attribution.
- Internal FFmpeg execution is sandbox-backed; `rg -n 'exec\.Command|os/exec' apps/server/internal` returns no app-local command execution matches.
- `scripts/smoke-m4-5.sh` and `scripts/smoke-m4-6.sh` validate successful and readable sandbox-backed internal media paths.

### M4.1 Schema Convergence And Mock Text Run

Goal: create the minimum production loop without calling external providers.

Delivery target:

- The database, sqlc types, backend service, and API can express a runnable text node with job and version history.
- Existing Studio canvas behavior survives the schema convergence.
- The phase can be validated entirely with mock provider behavior.

Work items:

- Rebuild/adjust schema for `node_type` and `asset_type`.
- Converge `media_edge` to dependency-only input candidates.
- Add production fields to `media_node`: `operation_type`, `prompt_template`, `prompt_rich`, `prompt_refs`, `model_provider`, `model_id`, `model_params`, `current_version_id`, `metadata`.
- Add `generation_job`.
- Add `artifact_version`.
- Add minimal sqlc queries and service APIs for running a text node through a mock provider.
- Keep existing Studio canvas create/edit/move behavior working.
- Keep `generation_job` focused on production run state; sandbox execution details belong to M4.S `sandbox_job` when a run delegates to sandbox.

Acceptance:

- A Text Node can be run through a mock text provider.
- The run creates `generation_job=succeeded`.
- The run creates a text asset and `artifact_version`.
- `media_node.current_version_id` points to the winner.
- Re-running the same node creates another version and updates the current winner.
- Existing Studio canvas smoke still works.

E2E / self-test standard:

1. Register or log in.
2. Create a Studio Workspace.
3. Create a Text Node with prompt content and mock model settings.
4. Run the node through the backend run endpoint or equivalent service entrypoint.
5. Confirm the API/database state shows one succeeded `generation_job`.
6. Confirm the API/database state shows one text `media_asset`, one `artifact_version`, and `media_node.current_version_id` pointing to that version.
7. Run the same node again and confirm a second version exists and becomes the current winner.
8. Open or fetch the Studio canvas and confirm existing node create/edit/move behavior still works.

Suggested verification:

```bash
make migrate-up
make sqlc-generate
make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

Completion record:

- Schema convergence for `node_type`, `asset_type`, dependency-only edges, production node fields, `generation_job`, and `artifact_version` is implemented.
- Mock Text Node run creates a succeeded job, text asset, artifact version, and current winner.
- Re-running a Text Node creates a new version and updates current winner.
- Existing Studio canvas fetch and node prompt behavior remains compatible.
- Edge API responses keep `edge_type="dependency"` for current frontend compatibility while the database stores dependency-only edges.

Verification:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web... build
CLIPANVIL_API_BASE=http://127.0.0.1:8891/api scripts/smoke-m4-1.sh
git diff --check
```

Smoke result:

```json
{
  "workspaceId": "0694cc04-1692-428b-980b-3eac6ae6253f",
  "nodeId": "1b2a1385-5c85-4d13-98f2-692a0f537693",
  "firstVersion": "65c058d9-f03e-4358-9fa5-21ade91349eb",
  "secondVersion": "9f41fbc3-ad67-44d7-9220-9e1871fb184b",
  "canvasNodes": 1
}
```

### M4.2 GenerationIntent And Provider Bridge Skeleton

Goal: make Studio and future Agent workers submit the same stable generation contract.

Delivery target:

- Node execution no longer talks directly to a provider-specific implementation.
- Studio-initiated runs and future Agent Worker runs share the same `GenerationIntent` structure and provider registry boundary.
- Real provider configuration exists behind environment variables, while automated tests stay on mock providers.

Work items:

- Define backend `GenerationIntent`.
- Build a provider registry with mock providers first.
- Treat Provider Bridge implementations as three families: mock providers, external model providers, and sandbox-backed internal providers.
- Add environment-driven provider config for local development.
- Add adapter boundaries for Volcengine text/image/video details without making real calls required in tests.
- Persist `generation_job.intent`, rendered prompt, provider request summary, provider response summary, and requested-by fields.

Acceptance:

- Node run requests are converted to a GenerationIntent before provider execution.
- Mock providers are selected by capability/config.
- Tests do not need external API keys.
- Missing API keys fail clearly when a real provider is requested.

E2E / self-test standard:

1. Create a Studio Text Node with prompt, operation, model provider, model id, and params.
2. Run it with `CLIPANVIL_PROVIDER_MODE=mock`.
3. Confirm the persisted `generation_job.intent` contains workspace id, target node id, operation, model, prompt, params, and requested-by metadata.
4. Confirm `rendered_prompt`, provider request summary, and provider response summary are persisted.
5. Switch provider mode/config to a real provider without an API key and confirm the run fails clearly before any external call.
6. Confirm all automated tests pass without external network access or real provider credentials.

Suggested verification:

```bash
make server-test
git diff --check
```

Completion record:

- Node runs are converted to the stable nested `GenerationIntent` before provider execution.
- Provider registry selects mock providers by default and supports a Volcengine adapter boundary behind environment config.
- Local `.env` and committed `.env.example` document provider mode, mock defaults, and optional Volcengine credentials.
- Missing Volcengine API keys fail before any external call and persist a failed `generation_job`.
- Successful mock runs persist `intent`, `rendered_prompt`, provider request summary, provider response summary, and requested-by metadata.
- Studio API accepts production node fields needed by intent construction.
- Node job history is queryable for smoke tests and future UI.

Verification:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web... build
CLIPANVIL_API_BASE=http://127.0.0.1:8892/api scripts/smoke-m4-1.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8892/api scripts/smoke-m4-2.sh
```

Smoke result:

```json
{
  "m4_1": {
    "workspaceId": "e1dab3c1-c23c-4bcf-96d4-d45653e523f3",
    "nodeId": "1d0c5ffc-0f30-43cc-8336-93e5c07e8a33",
    "firstVersion": "42435851-2ae8-4fd9-bc02-af197610538c",
    "secondVersion": "0c439cdc-a22a-416d-9f1d-98d56d46437b",
    "canvasNodes": 1
  },
  "m4_2": {
    "workspaceId": "3f741bf6-e470-40ff-88ed-0dd8c56287b6",
    "mockNodeId": "ee11521c-de95-42f2-8bef-289ffdd36cd5",
    "mockJobId": "0198c17f-0692-49c6-ae03-5c9663a16a51",
    "failedNodeId": "d2ac6dc5-06f2-4c47-afb6-036005d33c16",
    "failedJobId": "e2bef5af-f564-4282-9abd-d0a0b3b328f6"
  }
}
```

### M4.3 Capability Validation, Failure Records, And Retry

Goal: make invalid or failed runs traceable and retryable.

Delivery target:

- Model compatibility is enforced by backend capability data, not only frontend UI.
- Every invalid or failed run leaves a durable failed job record.
- Retry chains are explicit and bounded.

Work items:

- Add `model_provider` and `model_capability`.
- Seed mock capabilities for text, image, video, and internal operations.
- Validate operation type, output type, input node types, and provider limits before external calls.
- Persist failed jobs for capability mismatch, provider failure, internal processing failure, and retry exhaustion.
- Persist sandbox-backed failures in both `sandbox_job` and user-visible `generation_job`.
- Add retry records with `attempt`, `max_attempts`, and `parent_job_id`.

Acceptance:

- Unsupported model/operation combinations do not call a provider.
- Capability mismatch creates a failed job with structured error code and message.
- Mock provider failure creates a failed job.
- Retry creates a new job linked to the previous attempt.
- Automatic retry has a hard maximum attempt count.

E2E / self-test standard:

1. Seed or create a model capability that supports only text-to-image.
2. Create a Video Node or video operation that intentionally selects the incompatible model.
3. Run the node and confirm no provider execution occurs.
4. Confirm a failed `generation_job` exists with capability-related `error_code`, readable `error_message`, and `attempt=1`.
5. Configure the mock provider to fail a supported run and confirm that provider failure is persisted.
6. Trigger retry and confirm a new `generation_job` is created with incremented attempt and `parent_job_id` pointing to the failed job.
7. Exhaust automatic retries and confirm no attempts are created beyond `max_attempts`.

Suggested verification:

```bash
make server-test
git diff --check
```

Completion record:

- Added `model_provider` and `model_capability` tables with mock, internal ffmpeg, and cheap Volcengine text capability seeds.
- Node runs validate output type, operation, prompt limits, duration limits, and selected model capability before provider execution.
- Capability mismatches persist failed jobs with `error_code=capability_mismatch` and structured provider response summaries.
- Mock provider failures persist failed jobs with provider response summaries.
- Automatic retries create bounded `generation_job` chains using `parent_job_id`, `attempt`, and `max_attempts`.
- Manual retry is exposed through `POST /api/jobs/:id/retry` and does not create attempts beyond the configured maximum.
- Job API responses expose `parent_job_id`, `attempt`, and `max_attempts`.

Verification:

```bash
make migrate-up
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
CLIPANVIL_API_BASE=http://127.0.0.1:8893/api scripts/smoke-m4-1.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8893/api scripts/smoke-m4-2.sh
CLIPANVIL_API_BASE=http://127.0.0.1:8893/api scripts/smoke-m4-3.sh
```

Smoke result:

```json
{
  "m4_1": {
    "workspaceId": "ff19195a-21f1-432e-a131-ec3f7daba993",
    "nodeId": "5be2068a-b93c-4270-8566-7994b1289384",
    "firstVersion": "49581098-4568-4140-ab1e-381b47034e6c",
    "secondVersion": "61d9d485-107d-4b3c-be49-784574028171",
    "canvasNodes": 1
  },
  "m4_2": {
    "workspaceId": "47b63fea-9e9e-4e9f-919c-0a7ba790e7f1",
    "mockNodeId": "226b06df-fddc-4e03-af3d-d9e085b1e095",
    "mockJobId": "54563ad0-7dfa-4138-be3a-965c49561021",
    "failedNodeId": "db293894-c1d2-4e2d-b147-92494b955b06",
    "failedJobId": "fc8773aa-dc3d-4a42-b1fc-73fea468e627"
  },
  "m4_3": {
    "workspaceId": "0f87646f-6712-4994-b845-c90600f80cc3",
    "mismatchNodeId": "acc41aa6-a873-4435-b606-fb83f4a5dfad",
    "mismatchJobId": "a6fdda6b-403d-47fc-9889-06fe85ed6fa2",
    "failedNodeId": "23d55a32-bf70-4cb0-a9f7-a760e8497b06",
    "failedAttempts": 2,
    "latestFailedJobId": "974859a5-5928-4341-b7db-bf8811bcbeeb"
  }
}
```

### M4.4 Input Hash And Stale Propagation

Goal: make downstream freshness deterministic.

Delivery target:

- Version freshness is derived from deterministic input facts rather than UI state.
- Upstream winner changes propagate stale state to downstream dependency nodes.
- Users and future Agents can query why a node is stale.

Work items:

- Compute `artifact_version.input_hash` from prompt, operation, model, params, explicit refs, implicit dependency inputs, and relevant provider bridge version.
- For sandbox-backed internal operations, include stable semantic inputs in the hash: source asset/version, operation, params, and provider/internal tool version. Do not include sandbox id, volume name, presigned URL, temporary path, or retry-specific execution details.
- When an upstream winner changes, mark downstream nodes stale.
- Preserve old versions and assets.
- Provide a way to query stale reasons for a node.
- Include Reference Pack membership changes in downstream stale detection once M4.5 lands.

Acceptance:

- A -> B dependency can be run for both nodes.
- Re-running A changes A's winner and marks B stale.
- B retains its previous version.
- Stale reason identifies the upstream change.
- Re-running B clears stale when its new input hash matches current inputs.

E2E / self-test standard:

1. Create Text Node A and Text Node B in a Studio Workspace.
2. Create dependency A -> B.
3. Run A and B successfully.
4. Record B's current version and input hash.
5. Change A's prompt or params and re-run A.
6. Confirm A receives a new current winner.
7. Confirm B is marked `stale` and its old version remains available.
8. Query B's stale reason and confirm it points to A's changed winner or input.
9. Re-run B and confirm its stale state clears only after the new input hash reflects current upstream facts.

Suggested verification:

```bash
make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

Completion evidence:

- `artifact_version.input_hash` is computed from node prompt/operation/model/params, prompt refs, current dependency winners, empty M4.5 reference-pack facts, and provider bridge version.
- `node_stale_reason` stores active stale reasons; `GET /api/nodes/:id/stale-reasons` exposes them.
- Re-running an upstream node marks downstream dependency nodes `stale` while preserving their current version.
- Re-running the downstream node writes a refreshed version and clears active stale reasons.

### M4.5 Internal Media Operations And Reference Pack Foundation

Goal: add the remaining shared-production primitives needed by M5 and M6.

Delivery target:

- Reference Pack exists as a production node foundation without implementing the full M5 Studio interaction layer.
- Internal media operations use the same job/version/winner path as provider-backed generation.
- Pack membership and extracted frame outputs participate in downstream stale rules.

Work items:

- Add `reference_pack` node type.
- Add `reference_pack_item`.
- Prevent Reference Pack nesting in the first version.
- Expand Reference Pack direct members in GenerationIntent.
- Mark downstream nodes stale when pack membership or member winner changes.
- Add `extract_first_frame` and `extract_last_frame` internal operations through the same job/version chain.
- Use `internal_ffmpeg` provider records for internal media operations, backed by `sandbox_job` execution.

Acceptance:

- A Reference Pack can contain direct member nodes.
- Pack membership is separate from dependency edges.
- Referencing a pack expands only direct member current winners.
- Changing pack membership marks dependent nodes stale.
- First-frame and last-frame extraction create jobs, assets, versions, and winners.
- First-frame and last-frame extraction run FFmpeg in sandbox and expose `sandbox_job_id` in the production job response.

E2E / self-test standard:

1. Create Image Nodes A and B with current winners.
2. Create Reference Pack Node P and add A and B as direct members.
3. Confirm no dependency edge is created merely because of pack membership.
4. Create Node C depending on or explicitly referencing P.
5. Run C and confirm GenerationIntent expands only P's direct member current winners.
6. Add or remove a pack member and confirm C is marked stale.
7. Upload or create a Video Node V with a current video winner.
8. Run `extract_first_frame` and `extract_last_frame` image nodes from V.
9. Confirm both internal operations create `generation_job`, output image asset, `artifact_version`, and current winner.
10. Confirm ffmpeg/internal failures are persisted as failed sandbox jobs and failed generation jobs with readable error messages.

Suggested verification:

```bash
make server-test
pnpm --filter @clip-anvil/web... build
git diff --check
```

Completion evidence:

- `reference_pack` nodes and `reference_pack_item` direct membership are persisted separately from dependency edges.
- GenerationIntent expands Reference Pack direct member current winners; nested packs are rejected.
- Reference Pack membership changes and member winner changes mark downstream dependency nodes `stale`.
- `internal_ffmpeg` operations `extract_first_frame` and `extract_last_frame` use the shared job/version/winner path and persist readable failures.
- `internal_ffmpeg` operations run through Sandbox Job Service; the application process does not execute FFmpeg locally.
- `scripts/smoke-m4-5.sh` validates the Reference Pack and internal media operation E2E standard.

### M4.6 Production Read API

Goal: expose the production read surface M5 Studio needs to hydrate property panels, version history, job inspection, and sandbox trace details.

Delivery target:

- Studio and future Agent UI can read model capabilities, node versions, node production state, generation jobs, and sandbox jobs through authenticated API endpoints.
- Text outputs expose `text_content`; binary outputs expose presigned `access_url` values.
- Sandbox-backed internal media runs can be traced from `generation_job` to linked `sandbox_job` details.

Work items:

- Add `GET /api/model-capabilities`.
- Add `GET /api/nodes/:id/versions`.
- Add `GET /api/nodes/:id/production-state`.
- Add `GET /api/jobs/:id`.
- Add `GET /api/jobs/:id/sandbox-jobs`.
- Add `GET /api/sandbox-jobs/:id`.
- Add `ListSandboxJobsByGenerationJob`.
- Add M4.6 smoke coverage for mock text and internal FFmpeg read paths.

Acceptance:

- Enabled model capabilities are readable with parsed JSON arrays and config maps.
- Node version history returns version ids, winner status, input hashes, output summaries, and asset read data.
- Text assets include `text_content`; binary assets include a short-lived `access_url`.
- Node production state returns node, versions, current version, latest job, active stale reasons, capability, and sandbox job summaries.
- Generation job detail is readable through `/api/jobs/:id`.
- Linked sandbox jobs are readable through `/api/jobs/:id/sandbox-jobs`.
- Sandbox job detail is readable through `/api/sandbox-jobs/:id` and enforces workspace ownership.

E2E / self-test standard:

1. List model capabilities and confirm mock and internal FFmpeg capabilities are present.
2. Run a mock text node.
3. Read versions and confirm the current winner includes text asset content.
4. Read production state and confirm current version, latest job, and capability are present.
5. Read job detail and confirm it matches the run.
6. Confirm mock text job has no linked sandbox jobs.
7. Run an internal FFmpeg frame extraction from a video winner.
8. Read linked sandbox jobs and sandbox job detail.
9. Read frame node production state and confirm binary asset access URL and sandbox summary are present.

Suggested verification:

```bash
make sqlc-generate
make server-test
make server-build
pnpm --filter @clip-anvil/web... build
scripts/smoke-m4-6.sh
git diff --check
```

Completion evidence:

- `scripts/smoke-m4-6.sh` validates the Production Read API over a real server.
- The read API reuses existing production tables and keeps sandbox execution details behind `sandbox_job`.
- M4.6 does not implement the M5 property panel UI; it provides the backend surface for that phase.

## Environment Configuration

M4 implementation should support local-only environment variables such as:

```text
CLIPANVIL_PROVIDER_MODE=mock
CLIPANVIL_VOLCENGINE_API_KEY=
CLIPANVIL_VOLCENGINE_TEXT_MODEL_CHEAP=
CLIPANVIL_VOLCENGINE_IMAGE_MODEL_CHEAP=
CLIPANVIL_VOLCENGINE_VIDEO_MODEL_CHEAP=
CLIPANVIL_VOLCENGINE_IMAGE_MODEL_QUALITY=
CLIPANVIL_VOLCENGINE_VIDEO_MODEL_QUALITY=
```

Concrete names may be adjusted to match existing config conventions. API keys must not be committed.

## Completion Rule

Each M4 phase should be treated as complete only when:

- Its acceptance criteria are met.
- Its E2E / self-test standard has been executed or explicitly superseded by an equivalent automated test.
- Relevant backend/frontend checks pass.
- Existing M3 Studio and Agent route behavior remains intact.
- New failure paths are covered by tests.
- The phase completion record is added to this document or the parent roadmap.
