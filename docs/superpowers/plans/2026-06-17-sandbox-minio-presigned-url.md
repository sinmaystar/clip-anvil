# Sandbox MinIO Presigned URL Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let sandboxes and browsers move large media directly to and from MinIO through short-lived presigned URLs, without streaming large files through the Go backend.

**Architecture:** Add `internal/storage.Service` as the single MinIO boundary with a backend client and a sandbox-presign client. Add sandbox transfer helpers that execute `curl` inside OpenSandbox. Convert `submit_artifact` to upload sandbox output through presigned PUT, and add browser direct-upload APIs for presign and completion.

**Tech Stack:** Go 1.26, Hertz, minio-go/v7, OpenSandbox Go SDK, sqlc, PostgreSQL, MinIO.

---

### Task 1: Storage Configuration And Service

**Files:**
- Modify: `apps/server/internal/config/config.go`
- Modify: `apps/server/internal/config/config_test.go`
- Modify: `apps/server/config.yaml`
- Modify: `deploy/config-container.yaml`
- Create: `apps/server/internal/storage/minio.go`
- Create: `apps/server/internal/storage/minio_test.go`

- [ ] **Step 1: Write failing config and storage tests**

Add tests that assert `minio.sandbox_endpoint` loads from YAML/env and that storage URL helpers generate `workspace-{id}/{key}` consistently.

- [ ] **Step 2: Run tests to verify failure**

Run: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config ./internal/storage -count=1`

Expected: build failure until `SandboxEndpoint` and `internal/storage` exist.

- [ ] **Step 3: Implement storage service**

Add `MinIOConfig.SandboxEndpoint`, construct backend and sandbox clients, and expose `EnsureBucket`, `Upload`, `PresignedGetURL`, `PresignedPutURL`, `PresignedSandboxGetURL`, `PresignedSandboxPutURL`, `StatObject`, and `StorageURL`.

- [ ] **Step 4: Run tests**

Run: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/config ./internal/storage -count=1`

Expected: PASS.

### Task 2: Sandbox Transfer Helpers

**Files:**
- Modify: `apps/server/internal/sandbox/client.go`
- Create: `apps/server/internal/sandbox/transfer.go`
- Create: `apps/server/internal/sandbox/transfer_test.go`

- [ ] **Step 1: Write failing transfer tests**

Test that presigned URL downloads use `curl -sS -f -L -o <dest> <url>`, uploads use `curl -sS -f -L -X PUT -T <src> <url>`, and paths outside `/workspace` are rejected.

- [ ] **Step 2: Run tests to verify failure**

Run: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -run 'TestDownloadFromMinIO|TestUploadToMinIO' -count=1`

Expected: build failure until transfer helpers exist.

- [ ] **Step 3: Implement transfer helpers**

Add `DownloadFromMinIO` and `UploadToMinIO` functions that wrap `RunExec` and quote curl arguments safely.

- [ ] **Step 4: Run tests**

Run: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox -count=1`

Expected: PASS.

### Task 3: Direct Artifact Upload And Browser Upload APIs

**Files:**
- Modify: `apps/server/internal/sandbox/artifact.go`
- Modify: `apps/server/internal/sandbox/artifact_test.go`
- Modify: `apps/server/internal/api/upload_handler.go`
- Create: `apps/server/internal/api/storage_handler.go`
- Modify: `apps/server/cmd/server/main.go`

- [ ] **Step 1: Write failing artifact and API tests**

Test `submit_artifact` no longer calls sandbox file download, uses presigned PUT plus sandbox curl upload, records `media_asset`, creates or updates the canvas node, and broadcasts events. Test direct-upload completion creates an asset from an existing MinIO object.

- [ ] **Step 2: Run tests to verify failure**

Run: `GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/sandbox ./internal/api -count=1`

Expected: failure until artifact service and storage handler are updated.

- [ ] **Step 3: Implement direct artifact upload**

Replace backend `Download` usage in artifact submission with sandbox-side metadata inspection, presigned sandbox PUT upload, `media_asset` DB creation, node upsert, and WebSocket broadcast.

- [ ] **Step 4: Implement browser direct upload APIs**

Add authenticated workspace-scoped endpoints:
- `POST /api/workspaces/:id/storage/upload`
- `POST /api/workspaces/:id/storage/presigned-upload`
- `POST /api/workspaces/:id/storage/complete-upload`
- `POST /api/workspaces/:id/sandbox/download-from-minio`
- `POST /api/workspaces/:id/sandbox/upload-to-minio`

- [ ] **Step 5: Run tests and smoke**

Run: `GOCACHE=/private/tmp/clipanvil-go-build make server-build`

Run: `GOCACHE=/private/tmp/clipanvil-go-build make server-test`

Then run a real local smoke against port 8890: upload a text file to MinIO, sandbox curl-download it, sandbox curl-upload a derived file, complete a media asset, and submit a sandbox ffmpeg artifact.

Expected: build/test pass and smoke shows objects move through MinIO without backend file streaming.
