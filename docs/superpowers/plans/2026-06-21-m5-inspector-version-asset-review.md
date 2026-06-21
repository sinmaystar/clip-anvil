# M5 Inspector Version Asset Review Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Implement the approved Inspector option B: edit-first Inspector, version-bound call records, inline title editing, fullscreen asset review, and smaller canvas node previews.

**Architecture:** Keep the backend version/job lifecycle unchanged and use existing `ArtifactVersion` fields as the UI source of truth. Move low-frequency call data into per-version details, add a reusable fullscreen asset review overlay, and tune canvas preview sizing so nodes remain compact while full content is reviewed in the overlay.

**Tech Stack:** React 19, TypeScript, tldraw 5, Tailwind/CSS, existing Go/Hertz/sqlc APIs, node:test helper tests.

---

### Task 1: Inspector Header and Version Detail Helpers

**Files:**
- Modify: `apps/web/src/lib/productionPreview.ts`
- Modify: `apps/web/src/lib/productionPreview.test.mjs`
- Modify: `apps/web/src/components/PropertyPanel.tsx`
- Modify: `apps/web/src/main.css`

- [x] **Step 1: Add failing tests for version detail rows**

Add tests proving version detail helpers format current/running/failed details and keep version rows selectable only for usable succeeded versions.

Run: `pnpm --filter @clip-anvil/web test:connections`

Expected before implementation: FAIL because `versionDetailRows` and `versionCallRecordBlocks` are not exported.

- [x] **Step 2: Implement version detail helper functions**

Add helpers in `productionPreview.ts`:

- `versionDetailRows(version)`
- `versionCallRecordBlocks(version)`
- `versionHasCallRecord(version)`

They should derive details from `ArtifactVersion.provider_request`, `provider_response`, `error_code`, `error_message`, `started_at`, `completed_at`, `job_id`, and `input_hash`.

- [x] **Step 3: Replace global call log UI with per-version detail**

In `PropertyPanel.tsx`, remove the global `调用记录` section. Add a `Details` action inside `VersionPreviewPanel` that toggles detail content for the previewed version.

- [x] **Step 4: Replace duplicate title field with inline header editing**

In `PropertyPanel.tsx`, remove the standalone `标题` input. Make the header title double-click editable with Enter save, Escape cancel, and blur save.

- [x] **Step 5: Verify**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
```

Expected: tests and lint pass.

### Task 2: Fullscreen Asset Review

**Files:**
- Modify: `apps/web/src/components/PropertyPanel.tsx`
- Modify: `apps/web/src/shapes/MediaShapeUtil.tsx`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`

- [x] **Step 1: Add failing tests for fullscreen CSS contracts**

Update `canvasLayering.test.mjs` to assert the stylesheet contains:

- `.asset-review-overlay`
- `.asset-review-content`
- `.media-node-expand-button`

Run: `pnpm --filter @clip-anvil/web test:connections`

Expected before implementation: FAIL because CSS classes do not exist.

- [x] **Step 2: Add fullscreen review state and overlay**

Add local state in `NodePropertyPanel` for the fullscreen version. Add `AssetReviewOverlay` supporting text/image/video/audio/fallback states.

- [x] **Step 3: Add fullscreen entry points**

Add `Fullscreen` action to `VersionPreviewPanel`. Add a small expand button on `MediaNodeShape` that dispatches a browser event containing the node id; the page opens the selected node Inspector/fullscreen current asset where possible.

- [x] **Step 4: Style overlay**

Add overlay CSS with fixed full viewport, high z-index, contain image/video, scrollable markdown text, and JSON/detail-safe layout.

- [x] **Step 5: Verify**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: tests, build, and lint pass.

### Task 3: Compact Node Preview Sizing

**Files:**
- Modify: `apps/web/src/lib/nodePreviewLayout.ts`
- Modify: `apps/web/src/lib/nodePreviewLayout.test.mjs`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/src/lib/layout.test.mjs`
- Modify: `apps/web/src/lib/connectionGeometry.test.mjs`

- [x] **Step 1: Add failing tests for compact max sizes**

Update `nodePreviewLayout.test.mjs` so long text and large image previews use smaller max bounds than the current oversized preview behavior.

Run: `pnpm --filter @clip-anvil/web test:connections`

Expected before implementation: FAIL because current max sizes are larger.

- [x] **Step 2: Tune sizing constants**

Adjust `nodePreviewLayout.ts` to cap text/image/video preview sizes to the ranges from the spec while preserving aspect ratio and minimum usable sizes.

- [x] **Step 3: Add visual affordances**

Update CSS so text previews fade/truncate in compact nodes and media previews expose the expand button without crowding connection controls.

- [x] **Step 4: Verify layout and connection tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: tests, build, and lint pass.

### Task 4: Browser E2E

**Files:**
- No required source edits.

- [x] **Step 1: Start or reuse dev server**

Use `./scripts/dev-start.sh` if needed. Use the printed Vite URL.

- [x] **Step 2: Run Studio smoke**

In browser:

1. Register or log in.
2. Create Studio workspace.
3. Create text node.
4. Run node.
5. Confirm Inspector shows version timeline and no global call log.
6. Open version Details and verify provider request/response.
7. Open fullscreen review and close it.
8. Confirm browser console has no errors.

- [x] **Step 3: Final verification**

Run:

```bash
make server-test
make server-build
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: all commands pass.
