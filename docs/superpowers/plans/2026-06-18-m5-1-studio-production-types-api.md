# M5.1 Studio Production Types And API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the frontend type, API, canvas schema, and minimal UI wiring needed for Studio to understand M4 production fields and create `reference_pack` nodes.

**Architecture:** Keep M5.1 as a frontend foundation slice. Extend shared canvas schema and web API contracts first, then update existing Studio renderers and selectors to accept `reference_pack` without changing the M4 backend or building the M5.2 production panel. Use TypeScript build plus focused Node tests to guard type coverage and resource filtering.

**Tech Stack:** Vite 8, React 19, TypeScript 6, tldraw 5, TanStack Query, Node test runner.

---

## File Structure

- Modify `packages/canvas-schema/src/index.ts`
  - Add `reference_pack` to `MediaType` and tldraw shape prop validation types.
- Modify `apps/web/src/lib/api.ts`
  - Add production-facing frontend types and API helpers for M4 endpoints.
  - Extend `MediaNode` to include production fields already returned by the backend.
  - Split `AssetType` from `MediaType`.
- Modify `apps/web/src/lib/canvas.ts`
  - Keep `reference_pack` nodes convertible to tldraw media shapes and give them a fallback title label.
- Modify `apps/web/src/shapes/MediaShapeUtil.tsx`
  - Allow `reference_pack` in `T.literalEnum`.
  - Render a stable Reference Pack placeholder card.
- Modify `apps/web/src/components/ResourceTree.tsx`
  - Add Reference Pack filter and labels.
- Modify `apps/web/src/components/PropertyPanel.tsx`
  - Add Reference Pack labels so selecting a pack does not crash the inspector.
- Modify `apps/web/src/pages/WorkspaceDetailPage.tsx`
  - Add Reference Pack to the floating toolbar and context menu node creation options.
  - Keep existing node creation flow; it already posts `node_type`.
- Modify `apps/web/src/lib/canvasSelectors.test.mjs`
  - Add a selector test for `reference_pack` filtering.
- Modify `apps/web/src/lib/canvasLayering.test.mjs`
  - Add a source-level guard that `MediaShapeUtil` supports `reference_pack`.
- Modify `apps/web/tsconfig.test.json`
  - No change expected. Existing `canvasSelectors.ts` inclusion type-checks `MediaType` through type-only imports.

## Scope Boundaries

M5.1 does not implement node run UI, version preview, Reference Pack membership management, Prompt `@`, stale reason display, or model selection controls. It only makes those later slices type-safe and callable.

## Task 1: Add Reference Pack Type Coverage Tests

**Files:**
- Modify: `apps/web/src/lib/canvasSelectors.test.mjs`
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`

- [ ] **Step 1: Add a failing ResourceTree selector test for Reference Pack**

Append this test inside the existing `describe("canvas selectors", () => { ... })` block in `apps/web/src/lib/canvasSelectors.test.mjs`:

```js
  it("filters reference pack nodes as a first-class resource type", () => {
    const pack = {
      ...node("pack", "商品身份包"),
      node_type: "reference_pack",
    };
    const sections = getResourceTreeSections([...nodes, pack], groups, {
      query: "",
      type: "reference_pack",
    });

    assert.deepEqual(sections.groups, []);
    assert.deepEqual(
      sections.ungroupedNodes.map((item) => item.id),
      ["pack"],
    );
  });
```

- [ ] **Step 2: Add a failing MediaShapeUtil support test**

Append this test inside the existing `describe("canvas layering", () => { ... })` block in `apps/web/src/lib/canvasLayering.test.mjs`:

```js
  it("declares reference pack as a supported media node type", () => {
    assert.ok(
      mediaShapeUtil.includes(
        'nodeType: T.literalEnum("text", "image", "video", "audio", "reference_pack")',
      ),
      "reference_pack must be accepted by the tldraw media shape validator",
    );
    assert.ok(
      mediaShapeUtil.includes("reference_pack:"),
      "reference_pack must have node display metadata",
    );
  });
```

- [ ] **Step 3: Run the focused tests and verify failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL. TypeScript should reject `"reference_pack"` as not assignable to the current `MediaType`, or the source-level MediaShapeUtil assertion should fail.

- [ ] **Step 4: Commit tests**

```bash
git add apps/web/src/lib/canvasSelectors.test.mjs apps/web/src/lib/canvasLayering.test.mjs
git commit -m "test: cover reference pack frontend type support"
```

## Task 2: Extend Shared Canvas Schema And Shape Rendering

**Files:**
- Modify: `packages/canvas-schema/src/index.ts`
- Modify: `apps/web/src/shapes/MediaShapeUtil.tsx`
- Modify: `apps/web/src/lib/canvas.ts`

- [ ] **Step 1: Add `reference_pack` to shared schema**

In `packages/canvas-schema/src/index.ts`, replace the `MediaType` declaration with:

```ts
export type MediaType = "text" | "image" | "video" | "audio" | "reference_pack";
```

- [ ] **Step 2: Update tldraw media shape validator**

In `apps/web/src/shapes/MediaShapeUtil.tsx`, replace:

```ts
    nodeType: T.literalEnum("text", "image", "video", "audio"),
```

with:

```ts
    nodeType: T.literalEnum("text", "image", "video", "audio", "reference_pack"),
```

- [ ] **Step 3: Add Reference Pack display metadata**

In `apps/web/src/shapes/MediaShapeUtil.tsx`, update `nodeTypeMeta` to:

```ts
const nodeTypeMeta: Record<
  MediaShape["props"]["nodeType"],
  { icon: string; label: string; emptyTitle: string }
> = {
  text: { icon: "文案", label: "文本", emptyTitle: "未命名文本" },
  image: { icon: "参考", label: "图片", emptyTitle: "未命名图片" },
  video: { icon: "视频", label: "视频", emptyTitle: "未命名视频" },
  audio: { icon: "音频", label: "音频", emptyTitle: "未命名音频" },
  reference_pack: {
    icon: "参考包",
    label: "参考包",
    emptyTitle: "未命名参考包",
  },
};
```

- [ ] **Step 4: Render Reference Pack placeholder content**

In `MediaNodeShape`, replace the final audio-only fallback:

```tsx
            ) : (
              <div className="media-node-placeholder">
                <span className="media-node-waveform" />
                <span>0:00</span>
              </div>
            )}
```

with:

```tsx
            ) : nodeType === "audio" ? (
              <div className="media-node-placeholder">
                <span className="media-node-waveform" />
                <span>0:00</span>
              </div>
            ) : (
              <div className="media-node-placeholder">
                <span>Reference Pack</span>
                <span>等待成员</span>
              </div>
            )}
```

- [ ] **Step 5: Add Reference Pack fallback label in canvas shape conversion**

In `apps/web/src/lib/canvas.ts`, update `nodeTypeLabel`:

```ts
function nodeTypeLabel(nodeType: MediaNode["node_type"]) {
  switch (nodeType) {
    case "image":
      return "图片";
    case "video":
      return "视频";
    case "audio":
      return "音频";
    case "reference_pack":
      return "参考包";
    case "text":
    default:
      return "文本";
  }
}
```

- [ ] **Step 6: Run focused tests and verify they pass**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

- [ ] **Step 7: Commit schema and shape support**

```bash
git add packages/canvas-schema/src/index.ts apps/web/src/shapes/MediaShapeUtil.tsx apps/web/src/lib/canvas.ts
git commit -m "feat: support reference pack canvas shapes"
```

## Task 3: Add Production API Types And Helpers

**Files:**
- Modify: `apps/web/src/lib/api.ts`

- [ ] **Step 1: Extend core frontend types**

In `apps/web/src/lib/api.ts`, replace:

```ts
export type MediaType = "text" | "image" | "video" | "audio";
```

with:

```ts
export type MediaType = "text" | "image" | "video" | "audio" | "reference_pack";

export type AssetType = "text" | "image" | "video" | "audio" | "json";

export type OperationType =
  | "manual"
  | "upload"
  | "collect_references"
  | "text_generation"
  | "text_to_image"
  | "image_to_image"
  | "multi_image_to_image"
  | "text_to_video"
  | "image_to_video"
  | "video_to_video"
  | "multi_reference_to_video"
  | "extract_first_frame"
  | "extract_last_frame";
```

- [ ] **Step 2: Extend `MediaNode` production fields**

In `MediaNode`, keep the existing fields and add these fields before `created_at`:

```ts
  source?: "user" | "agent" | string;
  operation_type?: OperationType | string;
  prompt_template?: string;
  prompt_rich?: unknown;
  prompt_refs?: unknown;
  model_provider?: string | null;
  model_id?: string | null;
  model_params?: unknown;
  current_version_id?: string | null;
  metadata?: unknown;
```

- [ ] **Step 3: Split `MediaAsset.type` from `MediaType`**

Replace:

```ts
  type: Exclude<MediaType, "text">;
```

with:

```ts
  type: Exclude<AssetType, "json">;
```

- [ ] **Step 4: Add production response interfaces**

After `MediaAsset`, add:

```ts
export interface ModelCapability {
  provider_id: string;
  model_id: string;
  display_name: string;
  output_types: string[];
  supported_operations: string[];
  supported_input_node_types: string[];
  limits: Record<string, unknown>;
  pricing: Record<string, unknown>;
  defaults: Record<string, unknown>;
  enabled: boolean;
}

export interface GenerationJob {
  id: string;
  workspace_id: string;
  target_node_id: string;
  parent_job_id?: string;
  operation_type: string;
  provider: string;
  model_id: string;
  intent: Record<string, unknown>;
  rendered_prompt: string;
  provider_request: Record<string, unknown>;
  provider_response: Record<string, unknown>;
  status: "pending" | "queued" | "running" | "succeeded" | "failed" | "cancelled";
  attempt: number;
  max_attempts: number;
  error_code?: string;
  error_message?: string;
  requested_by_type: string;
  requested_by_id?: string;
  created_at: string;
}

export interface ProductionAsset {
  id: string;
  type: AssetType;
  mime: string;
  storage_url?: string;
  access_url?: string;
  text_content?: string;
  size_bytes?: number;
  metadata: Record<string, unknown>;
}

export interface ArtifactVersion {
  id: string;
  workspace_id: string;
  node_id: string;
  job_id?: string;
  asset_id?: string;
  version_no: number;
  winner: boolean;
  output: Record<string, unknown>;
  review_score?: number;
  input_hash: string;
  asset?: ProductionAsset;
  created_at: string;
}

export interface StaleReason {
  id: string;
  node_id: string;
  upstream_node_id: string;
  upstream_version_id?: string;
  reason_code: string;
  reason_message: string;
  details: Record<string, unknown>;
}

export interface SandboxJob {
  id: string;
  workspace_id: string;
  target_node_id?: string;
  generation_job_id?: string;
  job_type: string;
  operation_type: string;
  status: "pending" | "queued" | "running" | "succeeded" | "failed" | "cancelled";
  sandbox_id?: string;
  command: string;
  cwd: string;
  input: Record<string, unknown>;
  output: Record<string, unknown>;
  exit_code?: number;
  stdout?: string;
  stderr?: string;
  duration_ms: number;
  error_code?: string;
  error_message?: string;
  created_at: string;
}

export interface NodeProductionState {
  node: MediaNode;
  current_version?: ArtifactVersion;
  versions: ArtifactVersion[];
  latest_job?: GenerationJob;
  active_stale_reasons: StaleReason[];
  capability?: ModelCapability;
  sandbox_jobs: SandboxJob[];
}

export interface RunNodeResponse {
  node?: MediaNode;
  job: GenerationJob;
  version?: ArtifactVersion;
}

export interface ReferencePackItem {
  id: string;
  pack_node_id: string;
  member_node_id: string;
  position: number;
}
```

- [ ] **Step 5: Allow production config fields in create/update helpers**

Extend the `createMediaNode` input type by adding:

```ts
  operation_type?: OperationType | string;
  model_provider?: string;
  model_id?: string;
  model_params?: Record<string, unknown>;
```

Extend `updateMediaNode` by replacing its input type with:

```ts
  input: Partial<
    Pick<
      MediaNode,
      | "title"
      | "prompt"
      | "status"
      | "group_id"
      | "operation_type"
      | "model_provider"
      | "model_id"
      | "model_params"
    >
  >,
```

- [ ] **Step 6: Add API helpers**

Append these helpers near the other API functions:

```ts
export function fetchModelCapabilities() {
  return apiFetch<ModelCapability[]>("/model-capabilities");
}

export function fetchNodeProductionState(id: string) {
  return apiFetch<NodeProductionState>(`/nodes/${id}/production-state`);
}

export function runNode(id: string, input: { max_attempts?: number } = {}) {
  return apiFetch<RunNodeResponse>(`/nodes/${id}/run`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function retryJob(id: string) {
  return apiFetch<RunNodeResponse>(`/jobs/${id}/retry`, {
    method: "POST",
  });
}

export function fetchReferencePackItems(id: string) {
  return apiFetch<ReferencePackItem[]>(`/reference-packs/${id}/items`);
}

export function replaceReferencePackItems(id: string, member_node_ids: string[]) {
  return apiFetch<ReferencePackItem[]>(`/reference-packs/${id}/items`, {
    method: "PUT",
    body: JSON.stringify({ member_node_ids }),
  });
}
```

- [ ] **Step 7: Run TypeScript build**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

- [ ] **Step 8: Commit API helpers**

```bash
git add apps/web/src/lib/api.ts
git commit -m "feat: add studio production api contracts"
```

## Task 4: Wire Reference Pack Into Studio UI Labels And Creation

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/components/ResourceTree.tsx`
- Modify: `apps/web/src/components/PropertyPanel.tsx`

- [ ] **Step 1: Add Reference Pack creation option**

In `apps/web/src/pages/WorkspaceDetailPage.tsx`, append this entry to `nodeCreateOptions`:

```ts
  {
    type: "reference_pack",
    title: "参考包",
    description: "商品 / 角色 / 风格参考集合",
    icon: "参考包",
    defaultTitle: "未命名参考包",
  },
```

- [ ] **Step 2: Add Reference Pack inline editor metadata**

In `nodeEditorTypeMeta`, add:

```ts
  reference_pack: { label: "参考包", emptyTitle: "未命名参考包" },
```

- [ ] **Step 3: Add Reference Pack toolbar button**

In the `.studio-floating-toolbar` button list, after the Audio button, insert:

```tsx
          <button
            onClick={() => createNodeAtViewportCenter("reference_pack")}
            type="button"
          >
            参考包
          </button>
```

- [ ] **Step 4: Add Reference Pack ResourceTree filter and label**

In `apps/web/src/components/ResourceTree.tsx`, add this filter item:

```ts
  { value: "reference_pack", label: "参考包" },
```

Update `nodeTypeLabel`:

```ts
const nodeTypeLabel: Record<MediaType, string> = {
  text: "文案",
  image: "参考",
  video: "视频",
  audio: "音频",
  reference_pack: "参考包",
};
```

- [ ] **Step 5: Add Reference Pack PropertyPanel label**

In `apps/web/src/components/PropertyPanel.tsx`, update `nodeTypeLabel`:

```ts
const nodeTypeLabel: Record<MediaNode["node_type"], string> = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
  reference_pack: "参考包",
};
```

- [ ] **Step 6: Run focused tests and lint**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
```

Expected: both PASS.

- [ ] **Step 7: Commit UI wiring**

```bash
git add apps/web/src/pages/WorkspaceDetailPage.tsx apps/web/src/components/ResourceTree.tsx apps/web/src/components/PropertyPanel.tsx
git commit -m "feat: expose reference pack node creation"
```

## Task 5: Verify M5.1 Acceptance

**Files:**
- No source edits expected.

- [ ] **Step 1: Run full M5.1 verification commands**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: all commands PASS.

- [ ] **Step 2: Run local app smoke with worktree-safe ports**

Print the worktree runtime environment:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Expected: export lines for this worktree profile, including `CLIPANVIL_SERVER_PORT` and `CLIPANVIL_WEB_PORT`.

Start the app:

```bash
./scripts/dev-start.sh
```

Expected: script prints the Vite URL for this worktree and health check succeeds.

- [ ] **Step 3: Manual browser smoke**

Using the Vite URL printed by `./scripts/dev-start.sh`:

1. Register or log in.
2. Create a Studio Workspace.
3. Enter the Studio route.
4. Create Text, Image, Video, Audio, and Reference Pack nodes from the toolbar or context menu.
5. Refresh the page.
6. Confirm all five nodes still render.
7. Select a normal node and confirm the app can request `/api/nodes/:id/production-state`.
8. Confirm `/api/model-capabilities` returns model capabilities.

Expected:

- No blank canvas.
- No TypeScript or runtime crash in the browser console.
- Reference Pack card renders with the Reference Pack placeholder.
- Existing text/image/video/audio node creation still works.

- [ ] **Step 4: Stop app processes**

```bash
./scripts/dev-stop.sh
```

Expected: this worktree's frontend/backend processes stop; shared Docker middleware remains running.

- [ ] **Step 5: Commit final verification note only if source changed after Task 4**

If smoke testing required small fixes, commit them:

```bash
git add apps/web packages/canvas-schema
git commit -m "fix: complete m5 1 studio production type wiring"
```

If no fixes were needed, do not create an empty commit.

## Self-Review Checklist

- M5.1 spec coverage:
  - `reference_pack` frontend type support: Task 2.
  - production fields on `MediaNode`: Task 3.
  - production API helpers: Task 3.
  - production state response type: Task 3.
  - Reference Pack creation API call path: Task 4 through existing `createMediaNode`.
  - model capabilities API helper: Task 3.
  - no change to existing node creation behavior: Task 5 smoke.
- Out of scope:
  - Run panel UI, version UI, stale UI, Reference Pack membership UI, and Prompt `@` remain for later M5 phases.
- Type consistency:
  - `MediaType` includes `reference_pack` in `packages/canvas-schema` and `apps/web/src/lib/api.ts`.
  - `nodeTypeLabel` / `nodeTypeMeta` / `nodeEditorTypeMeta` all include `reference_pack`.
  - `fetchReferencePackItems` and `replaceReferencePackItems` use `member_node_ids`, matching the current backend request body.
