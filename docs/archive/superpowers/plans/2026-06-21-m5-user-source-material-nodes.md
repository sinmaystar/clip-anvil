# M5 User Source Material Nodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make user-uploaded images/videos/audio and user-written text scripts first-class source material nodes that can feed downstream generation and reference packs without exposing model-run controls.

**Architecture:** Reuse existing `text/image/video/audio` node types and distinguish source material by `operation_type = "upload"`, `operation_type = "manual"`, or `asset_id`. Add shared frontend helpers for source-material UX, backend guards so source nodes cannot be run, and production input resolution so uploaded/manual source nodes resolve exactly like generated current-version artifacts for downstream provider calls.

**Tech Stack:** Go 1.26 + Hertz + pgx/sqlc, React 19 + TypeScript, React Flow, Vite 8, node:test helpers, existing MinIO/storage/upload APIs.

---

## File Map

- Create: `apps/web/src/lib/sourceMaterial.ts`
  - Shared frontend helper for detecting source material nodes and formatting labels.
- Create: `apps/web/src/lib/sourceMaterial.test.mjs`
  - Node-mode helper tests used by UI and run-control logic.
- Modify: `apps/web/src/lib/productionPanel.ts`
  - Route run-disable logic through source material helpers.
- Modify: `apps/web/src/lib/canvas.ts`
  - Put source material labels and previews into React Flow node data.
- Modify: `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
  - Render source material labels/status without Prompt metadata.
- Modify: `apps/web/src/components/FileDropZone.tsx`
  - Create uploaded media as `operation_type = "upload"` and `status = "succeeded"`.
- Modify: `apps/web/src/components/PropertyPanel.tsx`
  - Add source-material Inspector branch and manual text content editor.
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
  - Add text create menu with `生成文本` and `文本素材`, plus create-node patch for manual text.
- Modify: `apps/web/src/lib/referencePack.ts`
  - Keep source material eligible for packs and format pack member summaries.
- Modify: `apps/web/src/lib/promptRefs.ts`
  - Keep source material nodes eligible for prompt references.
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`, `apps/web/src/lib/productionPanel.test.mjs`, `apps/web/src/lib/referencePack.test.mjs`, `apps/web/src/lib/promptRefs.test.mjs`
  - Add frontend regression coverage.
- Modify: `apps/web/tsconfig.test.json`
  - Include new `sourceMaterial.ts` in dist-test build.
- Modify: `apps/server/sqlc/queries/node.sql`
  - Make create-node status explicit for normal create path.
- Regenerate: `apps/server/internal/store/db/node.sql.go`
  - sqlc output for node create params.
- Modify: `apps/server/internal/api/node_handler.go`
  - Persist requested `status`, `operation_type`, `asset_id` for source nodes.
- Modify: `apps/server/internal/api/node_handler_test.go`
  - Verify upload/manual nodes can be created as succeeded source nodes.
- Modify: `apps/server/internal/api/run_handler.go`
  - Return stable empty production state for source nodes and reject run attempts.
- Modify: `apps/server/internal/api/run_handler_test.go`
  - Verify source nodes cannot run and production state is stable.
- Modify: `apps/server/internal/api/canvas_handler.go`
  - Include direct `asset_id` previews for uploaded source nodes.
- Modify: `apps/server/internal/api/canvas_handler_test.go`
  - Verify uploaded media appears as source material preview in canvas payload.
- Modify: `apps/server/internal/production/service.go`
  - Resolve source material inputs from `media_node.asset_id` or manual text content.
- Modify: `apps/server/internal/production/service_test.go`
  - Verify manual text and uploaded image/video source refs render into prompt/input refs.
- Modify: `apps/server/internal/production/provider_asset_resolver.go`
  - Stage uploaded source image/video URLs where provider requires public access.
- Modify: `apps/server/internal/production/provider_asset_resolver_test.go`
  - Verify uploaded source assets get staged/presigned for provider use.

## Delivery Standards

- Uploading image/video/audio creates a ClipAnvil media node, not a native React Flow media node.
- Source material nodes show `图片素材` / `视频素材` / `音频素材` / `文本素材` and a usable preview.
- Source material Inspector omits Prompt/Operation/Model/Params/Run/Versions.
- Manual text material Inspector uses `内容`, not `Prompt`.
- Source material nodes can be referenced by `@` and connected as dependencies.
- Source material nodes can be added to reference packs.
- Downstream provider requests include text content or staged media URLs from source material inputs.
- Run API rejects source material nodes even if called directly.
- `production-state` on source material nodes returns stable empty production state.

---

### Task 1: Source Material Detection and Frontend Contracts

**Files:**
- Create: `apps/web/src/lib/sourceMaterial.ts`
- Create: `apps/web/src/lib/sourceMaterial.test.mjs`
- Modify: `apps/web/src/lib/productionPanel.ts`
- Modify: `apps/web/src/lib/productionPanel.test.mjs`
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`
- Modify: `apps/web/tsconfig.test.json`

- [ ] **Step 1: Add failing source material helper tests**

Create `apps/web/src/lib/sourceMaterial.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  canRunProductionNode,
  isManualTextMaterialNode,
  isSourceMaterialNode,
  isUploadMaterialNode,
  materialKindLabel,
  materialStatusLabel,
} from "../../dist-test/lib/sourceMaterial.js";

describe("source material node helpers", () => {
  it("classifies uploaded asset nodes as source material", () => {
    const node = {
      node_type: "image",
      operation_type: "upload",
      asset_id: "asset-1",
      status: "succeeded",
    };

    assert.equal(isSourceMaterialNode(node), true);
    assert.equal(isUploadMaterialNode(node), true);
    assert.equal(isManualTextMaterialNode(node), false);
    assert.equal(canRunProductionNode(node), false);
    assert.equal(materialKindLabel(node), "图片素材");
    assert.equal(materialStatusLabel(node), "可用");
  });

  it("classifies manual text nodes as source material", () => {
    const node = {
      node_type: "text",
      operation_type: "manual",
      asset_id: null,
      status: "succeeded",
    };

    assert.equal(isSourceMaterialNode(node), true);
    assert.equal(isUploadMaterialNode(node), false);
    assert.equal(isManualTextMaterialNode(node), true);
    assert.equal(canRunProductionNode(node), false);
    assert.equal(materialKindLabel(node), "文本素材");
    assert.equal(materialStatusLabel(node), "可用");
  });

  it("keeps normal generation nodes runnable", () => {
    const node = {
      node_type: "image",
      operation_type: "text_to_image",
      asset_id: null,
      status: "draft",
    };

    assert.equal(isSourceMaterialNode(node), false);
    assert.equal(canRunProductionNode(node), true);
    assert.equal(materialKindLabel(node), "图片");
    assert.equal(materialStatusLabel(node), "");
  });
});
```

- [ ] **Step 2: Include helper in test build**

Add `src/lib/sourceMaterial.ts` to `apps/web/tsconfig.test.json` `include`.

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `sourceMaterial.ts` does not exist.

- [ ] **Step 3: Implement the source material helper**

Create `apps/web/src/lib/sourceMaterial.ts`:

```ts
import type { MediaNode } from "./api";

type SourceMaterialNode = Pick<
  MediaNode,
  "asset_id" | "node_type" | "operation_type" | "status"
>;

const generatedLabels: Record<MediaNode["node_type"], string> = {
  text: "文本",
  image: "图片",
  video: "视频",
  audio: "音频",
  reference_pack: "参考包",
};

const sourceLabels: Record<MediaNode["node_type"], string> = {
  text: "文本素材",
  image: "图片素材",
  video: "视频素材",
  audio: "音频素材",
  reference_pack: "参考包",
};

export function isUploadMaterialNode(node: SourceMaterialNode) {
  return node.operation_type === "upload" || Boolean(node.asset_id);
}

export function isManualTextMaterialNode(node: SourceMaterialNode) {
  return node.node_type === "text" && node.operation_type === "manual";
}

export function isSourceMaterialNode(node: SourceMaterialNode) {
  return isUploadMaterialNode(node) || isManualTextMaterialNode(node);
}

export function canRunProductionNode(node: SourceMaterialNode) {
  return !isSourceMaterialNode(node) && node.node_type !== "reference_pack";
}

export function materialKindLabel(node: Pick<MediaNode, "node_type"> & Partial<SourceMaterialNode>) {
  return isSourceMaterialNode({
    asset_id: node.asset_id ?? null,
    node_type: node.node_type,
    operation_type: node.operation_type ?? "",
    status: node.status ?? "draft",
  })
    ? sourceLabels[node.node_type]
    : generatedLabels[node.node_type];
}

export function materialStatusLabel(node: SourceMaterialNode) {
  if (!isSourceMaterialNode(node)) {
    return "";
  }
  return node.status === "failed" ? "不可用" : "可用";
}
```

- [ ] **Step 4: Route run-disable logic through helper**

Modify `apps/web/src/lib/productionPanel.ts`:

```ts
import { isSourceMaterialNode } from "./sourceMaterial.js";
```

Replace the current upload/asset check in `runDisabledReason` with:

```ts
  if (isSourceMaterialNode(node)) {
    return "素材节点不需要运行模型。";
  }
```

- [ ] **Step 5: Add run-control regression test**

In `apps/web/src/lib/productionPanel.test.mjs`, add:

```js
  it("prevents running source material nodes", () => {
    assert.equal(
      runDisabledReason(
        {
          node_type: "text",
          operation_type: "manual",
          asset_id: null,
          status: "succeeded",
          model_params: {},
        },
        null,
        [],
        { invalid: [] },
      ),
      "素材节点不需要运行模型。",
    );
    assert.equal(
      runDisabledReason(
        {
          node_type: "image",
          operation_type: "upload",
          asset_id: "asset-1",
          status: "succeeded",
          model_params: {},
        },
        null,
        [],
        { invalid: [] },
      ),
      "素材节点不需要运行模型。",
    );
  });
```

- [ ] **Step 6: Add layering contract test**

In `apps/web/src/lib/canvasLayering.test.mjs`, add assertions that `MediaFlowNode.tsx` imports `materialKindLabel` and that source material labels are not footer metadata:

```js
  it("declares source material labels as first-class node identity", () => {
    assert.ok(
      mediaFlowNode.includes("materialKindLabel"),
      "media node should use source material labels",
    );
    assert.ok(
      !mediaFlowNode.includes("图片 PROMPT"),
      "source media nodes must not render prompt footer labels",
    );
  });
```

- [ ] **Step 7: Verify**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

**Deliverable:** Shared frontend helper exists and run controls consistently identify source material nodes.

**Acceptance:** Helper tests pass and existing generation nodes remain runnable.

---

### Task 2: Upload and Manual Text Source Node Creation

**Files:**
- Modify: `apps/server/sqlc/queries/node.sql`
- Regenerate: `apps/server/internal/store/db/node.sql.go`
- Modify: `apps/server/internal/api/node_handler.go`
- Modify: `apps/server/internal/api/node_handler_test.go`
- Modify: `apps/web/src/components/FileDropZone.tsx`
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx`
- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`

- [ ] **Step 1: Add backend failing tests for source node create**

In `apps/server/internal/api/node_handler_test.go`, add tests that parse and execute create requests for upload/manual source nodes. Use existing handler-test setup patterns in the file and assert returned fields:

```go
func TestCreateNodeRequestAcceptsUploadSourceMaterial(t *testing.T) {
	req := createNodeRequest{
		WorkspaceID:   "workspace-id",
		NodeType:      "image",
		Title:         "商品主图",
		Status:        "succeeded",
		AssetID:       "asset-id",
		OperationType: "upload",
		CanvasX:       120,
		CanvasY:       160,
	}

	status, ok := req.nodeStatus()
	if !ok || status != db.NodeStatusSucceeded {
		t.Fatalf("status = %q, %v, want succeeded", status, ok)
	}
	if !req.hasProductionConfig() {
		t.Fatal("upload source node should persist production config")
	}
}

func TestCreateNodeRequestAcceptsManualTextSourceMaterial(t *testing.T) {
	req := createNodeRequest{
		WorkspaceID:   "workspace-id",
		NodeType:      "text",
		Title:         "视频脚本",
		Prompt:        "第一幕：机场大厅。",
		Status:        "succeeded",
		OperationType: "manual",
	}

	status, ok := req.nodeStatus()
	if !ok || status != db.NodeStatusSucceeded {
		t.Fatalf("status = %q, %v, want succeeded", status, ok)
	}
	if !req.hasProductionConfig() {
		t.Fatal("manual text source node should persist production config")
	}
}
```

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: existing unit checks may pass, but full behavior still fails in later integration because `CreateMediaNode` normal path ignores explicit `status`.

- [ ] **Step 2: Make normal node create persist status**

Modify `apps/server/sqlc/queries/node.sql` `CreateMediaNode` so it accepts status:

```sql
-- name: CreateMediaNode :one
INSERT INTO media_node (
    workspace_id,
    node_type,
    title,
    prompt,
    status,
    asset_id,
    canvas_x,
    canvas_y,
    canvas_w,
    canvas_h
) VALUES (
    $1, $2, $3, $4, $5, sqlc.narg(asset_id), $6, $7, $8, $9
)
RETURNING *;
```

Update `NodeHandler.Create` normal path:

```go
node, err = h.queries.CreateMediaNode(ctx, db.CreateMediaNodeParams{
	WorkspaceID: workspaceID,
	NodeType:    nodeType,
	Title:       title,
	Prompt:      req.Prompt,
	Status:      status,
	AssetID:     assetID,
	CanvasX:     req.CanvasX,
	CanvasY:     req.CanvasY,
	CanvasW:     w,
	CanvasH:     nodeH,
})
```

- [ ] **Step 3: Regenerate sqlc**

Run:

```bash
make sqlc-generate
```

Expected: `apps/server/internal/store/db/node.sql.go` updates `CreateMediaNodeParams` with `Status db.NodeStatus`.

- [ ] **Step 4: Add frontend upload create contract**

Modify `apps/web/src/components/FileDropZone.tsx` `createNodeForAsset`:

```ts
const node = await createMediaNode({
  workspace_id: workspaceId,
  node_type: asset.type,
  asset_id: asset.id,
  title: fileNameWithoutExtension(file.name),
  status: "succeeded",
  operation_type: "upload",
  canvas_x: point.x + index * 260,
  canvas_y: point.y,
});
```

- [ ] **Step 5: Add manual text creation API path**

Ensure `CreateMediaNodeRequest` in `apps/web/src/lib/api.ts` includes:

```ts
status?: NodeStatus;
operation_type?: OperationType | string;
```

Add a helper in `WorkspaceDetailPage.tsx`:

```ts
const createManualTextSourceAtViewportCenter = useCallback(() => {
  createNodeAtViewportCenter("text", {
    title: "视频脚本",
    prompt: "",
    status: "succeeded",
    operation_type: "manual",
  });
}, [createNodeAtViewportCenter]);
```

If `createNodeAtViewportCenter` does not currently accept an override patch, extend its signature:

```ts
function createNodeAtViewportCenter(
  nodeType: MediaType,
  patch: Partial<CreateMediaNodeRequest> = {},
) {
  // keep existing center calculation
  createNodeMutation.mutate({
    workspace_id: id,
    node_type: nodeType,
    title: patch.title ?? defaultNodeTitle(nodeType),
    prompt: patch.prompt ?? "",
    canvas_x: point.x,
    canvas_y: point.y,
    ...patch,
  });
}
```

- [ ] **Step 6: Replace text toolbar button with a small menu**

Use existing toolbar styling and add a local state:

```ts
const [textCreateMenuOpen, setTextCreateMenuOpen] = useState(false);
```

Render the `文本` toolbar control as a small popover with:

```tsx
<button onClick={() => setTextCreateMenuOpen((open) => !open)} type="button">
  文本
</button>
{textCreateMenuOpen ? (
  <div className="studio-context-menu studio-toolbar-menu">
    <button onClick={() => createNodeAtViewportCenter("text")} type="button">
      生成文本
    </button>
    <button onClick={createManualTextSourceAtViewportCenter} type="button">
      文本素材
    </button>
  </div>
) : null}
```

- [ ] **Step 7: Add frontend contract test**

In `apps/web/src/lib/canvasLayering.test.mjs`, assert:

```js
  it("creates uploaded media and manual text as source material nodes", () => {
    assert.ok(
      fileDropZone.includes('operation_type: "upload"'),
      "uploaded files should create upload source nodes",
    );
    assert.ok(
      fileDropZone.includes('status: "succeeded"'),
      "uploaded source nodes should be immediately usable",
    );
    assert.ok(
      workspaceDetail.includes("文本素材"),
      "toolbar should expose manual text source creation",
    );
    assert.ok(
      workspaceDetail.includes('operation_type: "manual"'),
      "manual text source nodes should use manual operation",
    );
  });
```

Add required file reads at the top of the test:

```js
const fileDropZone = readFileSync(
  new URL("../components/FileDropZone.tsx", import.meta.url),
  "utf8",
);
const workspaceDetail = readFileSync(
  new URL("../pages/WorkspaceDetailPage.tsx", import.meta.url),
  "utf8",
);
```

- [ ] **Step 8: Verify**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

**Deliverable:** Uploaded media and manual text can be created as succeeded source material nodes.

**Acceptance:** API returns nodes with correct `operation_type`, `status`, and `asset_id`/manual content fields.

---

### Task 3: Source Material Canvas and Inspector UX

**Files:**
- Modify: `apps/web/src/lib/canvas.ts`
- Modify: `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
- Modify: `apps/web/src/components/PropertyPanel.tsx`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`

- [ ] **Step 1: Add failing UI contract tests**

In `apps/web/src/lib/canvasLayering.test.mjs`, add:

```js
  it("renders source material inspector without generation controls", () => {
    assert.ok(
      propertyPanel.includes("SourceMaterialPanel"),
      "property panel should branch source material nodes",
    );
    assert.ok(
      propertyPanel.includes("内容"),
      "manual text source editor should label text as content",
    );
    assert.ok(
      propertyPanel.includes("isSourceMaterialNode"),
      "property panel should use source material helper",
    );
  });
```

Add `propertyPanel` file read if not already present:

```js
const propertyPanel = readFileSync(
  new URL("../components/PropertyPanel.tsx", import.meta.url),
  "utf8",
);
```

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `SourceMaterialPanel` branch does not exist.

- [ ] **Step 2: Feed source material labels into canvas props**

In `apps/web/src/lib/canvas.ts`, import:

```ts
import { materialKindLabel, materialStatusLabel } from "./sourceMaterial.js";
```

When building media node data, set:

```ts
nodeTypeLabel: materialKindLabel(node),
sourceMaterialStatusLabel: materialStatusLabel(node),
```

If `MediaFlowNodeProps` lacks these fields, add optional fields in `apps/web/src/components/canvas-flow/flowTypes.ts`:

```ts
nodeTypeLabel: T.optional(T.string),
sourceMaterialStatusLabel: T.optional(T.string),
```

- [ ] **Step 3: Render source material identity in media node**

In `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`, import helper:

```ts
import { materialKindLabel, materialStatusLabel } from "../lib/sourceMaterial";
```

Build labels:

```ts
const typeLabel = node.props.nodeTypeLabel ?? materialKindLabel({
  asset_id: node.props.assetId,
  node_type: nodeType,
  operation_type: node.props.operationType,
  status,
});
const sourceStatus = node.props.sourceMaterialStatusLabel || materialStatusLabel({
  asset_id: node.props.assetId,
  node_type: nodeType,
  operation_type: node.props.operationType,
  status,
});
```

Render header text as `typeLabel` and use `sourceStatus || statusText[status]` for the status pill.

- [ ] **Step 4: Add source material Inspector branch**

In `apps/web/src/components/PropertyPanel.tsx`, import:

```ts
import {
  isManualTextMaterialNode,
  isSourceMaterialNode,
  materialKindLabel,
} from "../lib/sourceMaterial";
```

At the top of `NodePropertyPanel`, branch before generation controls:

```tsx
if (isSourceMaterialNode(node)) {
  return (
    <SourceMaterialPanel
      isUpdatingNode={isUpdatingNode}
      node={node}
      onUpdateNode={onUpdateNode}
    />
  );
}
```

Add component:

```tsx
function SourceMaterialPanel({
  isUpdatingNode,
  node,
  onUpdateNode,
}: {
  isUpdatingNode: boolean;
  node: MediaNode;
  onUpdateNode: (nodeId: string, patch: NodePatch) => void;
}) {
  const [contentValue, setContentValue] = useState(node.prompt);

  useEffect(() => {
    setContentValue(node.prompt);
  }, [node.id, node.prompt]);

  return (
    <aside className="property-panel node-production-panel">
      <PanelHeader eyebrow={materialKindLabel(node)} title={node.title} />
      <span className="property-status-pill" data-status={node.status}>
        可用
      </span>
      {isManualTextMaterialNode(node) ? (
        <label className="property-field property-content-field">
          <span>内容</span>
          <textarea
            disabled={isUpdatingNode}
            onBlur={() => {
              if (contentValue !== node.prompt) {
                onUpdateNode(node.id, {
                  prompt: contentValue,
                  prompt_rich: {
                    version: 1,
                    source: "textarea-at",
                    text: contentValue,
                  },
                });
              }
            }}
            onChange={(event) => setContentValue(event.currentTarget.value)}
            placeholder="粘贴视频脚本、商品卖点或参考文案"
            rows={12}
            value={contentValue}
          />
        </label>
      ) : (
        <div className="property-section property-source-preview">
          <p className="studio-section-label">素材预览</p>
          <VersionPreviewBody
            version={{
              id: node.asset_id ?? node.id,
              workspace_id: node.workspace_id,
              node_id: node.id,
              version_no: 1,
              winner: true,
              output: {},
              input_hash: "",
              status: "succeeded",
              progress: 100,
              provider_request: {},
              provider_response: {},
              created_at: node.created_at,
              asset: node.asset_id
                ? {
                    id: node.asset_id,
                    type: node.node_type,
                    mime: "",
                    access_url: node.asset_url,
                    thumbnail_url: node.thumbnail_url,
                    metadata: {},
                  }
                : undefined,
            }}
          />
        </div>
      )}
      <p className="property-empty">
        这是用户素材节点，可作为依赖输入或加入参考包，不需要运行模型。
      </p>
    </aside>
  );
}
```

Adjust the exact asset object fields to match `ArtifactVersion["asset"]` if TypeScript requires additional optional fields.

- [ ] **Step 5: Style content field and source preview**

In `apps/web/src/main.css`, add:

```css
.property-content-field textarea {
  min-height: 220px;
}

.property-source-preview {
  min-height: 180px;
}
```

- [ ] **Step 6: Verify**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
```

Expected: PASS.

**Deliverable:** Source material nodes have a non-generation canvas/Inspector presentation.

**Acceptance:** Source material Inspector has no Prompt/Model/Run/Versions controls and manual text uses `内容`.

---

### Task 4: Backend Run Guard, Production State, and Canvas Asset Preview

**Files:**
- Modify: `apps/server/internal/api/run_handler.go`
- Modify: `apps/server/internal/api/run_handler_test.go`
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/server/internal/api/canvas_handler_test.go`
- Modify: `apps/server/internal/production/service.go`
- Modify: `apps/server/internal/production/service_test.go`

- [ ] **Step 1: Add backend failing tests for direct run protection**

In `apps/server/internal/api/run_handler_test.go`, add a small pure helper test after creating helper functions:

```go
func TestSourceMaterialNodeCannotRun(t *testing.T) {
	if !isSourceMaterialNode(db.MediaNode{
		NodeType:      db.NodeTypeImage,
		OperationType: "upload",
		AssetID:       pgtype.UUID{Valid: true},
	}) {
		t.Fatal("upload asset node should be source material")
	}
	if !isSourceMaterialNode(db.MediaNode{
		NodeType:      db.NodeTypeText,
		OperationType: "manual",
	}) {
		t.Fatal("manual text node should be source material")
	}
}
```

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: FAIL because `isSourceMaterialNode` does not exist.

- [ ] **Step 2: Implement backend source-material guard helper**

Add to `apps/server/internal/api/run_handler.go`:

```go
func isSourceMaterialNode(node db.MediaNode) bool {
	return node.OperationType == "upload" ||
		node.OperationType == "manual" && node.NodeType == db.NodeTypeText ||
		node.AssetID.Valid
}
```

In `RunNode`, after loading `node` and requiring Studio workspace:

```go
if isSourceMaterialNode(node) {
	writeError(c, consts.StatusBadRequest, "素材节点不需要运行模型。")
	return
}
```

- [ ] **Step 3: Make production state stable for source material**

In `GetNodeProductionState`, after node load and workspace auth:

```go
if isSourceMaterialNode(node) {
	c.JSON(consts.StatusOK, productionStateResponse{
		Node:               toMediaNodeResponse(node),
		Versions:           []artifactVersionResponse{},
		ActiveStaleReasons: []staleReasonResponse{},
		SandboxJobs:        []sandboxJobResponse{},
	})
	return
}
```

Add a handler-level test using the file's existing fake query/service pattern to assert:

```go
if len(resp.Versions) != 0 || resp.LatestJob != nil || resp.CurrentVersion != nil {
	t.Fatalf("source material production state should be empty: %#v", resp)
}
```

- [ ] **Step 4: Add canvas preview test for direct asset nodes**

In `apps/server/internal/api/canvas_handler_test.go`, add a test that constructs a node with `AssetID` but no `CurrentVersionID` and an image asset with `StorageUrl`.

Expected response:

```go
if got.ProductionPreview == nil || got.ProductionPreview.AssetID != uuidToString(assetID) {
	t.Fatalf("production preview = %#v, want direct asset preview", got.ProductionPreview)
}
if got.ProductionPreview.AssetType != "image" {
	t.Fatalf("asset type = %q, want image", got.ProductionPreview.AssetType)
}
```

- [ ] **Step 5: Add canvas direct asset preview implementation**

In `apps/server/internal/api/canvas_handler.go`, when constructing preview:

```go
if node.AssetID.Valid && !node.CurrentVersionID.Valid {
	if asset, ok := assets[node.AssetID]; ok {
		preview := productionPreviewFromAsset(node, asset)
		response.ProductionPreview = &preview
	}
}
```

Implement a local helper using the existing version preview helper style:

```go
func productionPreviewFromAsset(node db.MediaNode, asset db.MediaAsset) canvasProductionPreview {
	return canvasProductionPreview{
		AssetID:   uuidToString(asset.ID),
		AssetType: string(asset.Type),
		Mime:      textString(asset.Mime),
		AccessURL: signedOrStorageURL(asset),
		// include text/width/height/duration if already present in metadata helpers
	}
}
```

Use existing storage signing helpers in `canvas_handler.go` rather than inventing new URL signing.

- [ ] **Step 6: Verify**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
```

Expected: PASS.

**Deliverable:** Backend treats source nodes as stable resources and rejects accidental runs.

**Acceptance:** Source node `production-state` never emits `failed to list versions`; canvas payload includes direct asset preview.

---

### Task 5: Prompt Rendering and Provider Input Resolution for Source Materials

**Files:**
- Modify: `apps/server/internal/production/service.go`
- Modify: `apps/server/internal/production/service_test.go`
- Modify: `apps/server/internal/production/provider_asset_resolver.go`
- Modify: `apps/server/internal/production/provider_asset_resolver_test.go`
- Modify: `apps/web/src/lib/promptRefs.test.mjs`
- Modify: `apps/web/src/lib/referencePack.test.mjs`

- [ ] **Step 1: Add failing production tests for direct source inputs**

In `apps/server/internal/production/service_test.go`, add:

```go
type fakeInputQueries struct {
	asset db.MediaAsset
}

func (q fakeInputQueries) ListUpstreamDependencyNodes(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return nil, nil
}

func (q fakeInputQueries) ListReferencePackItemNodes(context.Context, pgtype.UUID) ([]db.MediaNode, error) {
	return nil, nil
}

func (q fakeInputQueries) GetArtifactVersionByID(context.Context, pgtype.UUID) (db.ArtifactVersion, error) {
	return db.ArtifactVersion{}, errors.New("unexpected version lookup")
}

func (q fakeInputQueries) GetMediaAssetByID(context.Context, pgtype.UUID) (db.MediaAsset, error) {
	return q.asset, nil
}

func TestLoadNodeInputFactUsesManualTextSourceNode(t *testing.T) {
	node := db.MediaNode{
		ID:            pgtype.UUID{Bytes: [16]byte{0x31}, Valid: true},
		NodeType:      db.NodeTypeText,
		Title:         "视频脚本",
		Prompt:        "第一幕：机场大厅。\n第二幕：商品特写。",
		OperationType: "manual",
		Status:        db.NodeStatusSucceeded,
	}

	fact, ref, err := loadNodeInputFact(context.Background(), fakeInputQueries{}, node, InputKindExplicit)
	if err != nil {
		t.Fatalf("resolve source text ref: %v", err)
	}
	if ref.TextContent != node.Prompt {
		t.Fatalf("text content = %q, want prompt content", ref.TextContent)
	}
	if fact.InputHash == "" {
		t.Fatal("manual text source should contribute input hash")
	}
}

func TestLoadNodeInputFactUsesUploadedAssetNode(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x32}, Valid: true}
	asset := db.MediaAsset{
		ID:         assetID,
		Type:       db.AssetTypeImage,
		Mime:       "image/png",
		StorageUrl: pgtype.Text{String: "workspace-a/assets/product.png", Valid: true},
	}
	node := db.MediaNode{
		ID:            pgtype.UUID{Bytes: [16]byte{0x33}, Valid: true},
		NodeType:      db.NodeTypeImage,
		Title:         "商品主图",
		OperationType: "upload",
		AssetID:       assetID,
		Status:        db.NodeStatusSucceeded,
	}

	fact, ref, err := loadNodeInputFact(context.Background(), fakeInputQueries{asset: asset}, node, InputKindExplicit)
	if err != nil {
		t.Fatalf("resolve upload source ref: %v", err)
	}
	if ref.AssetID != uuidToString(asset.ID) || ref.StorageURL != asset.StorageUrl.String {
		t.Fatalf("ref = %#v, want uploaded asset", ref)
	}
	if fact.InputHash == "" {
		t.Fatal("uploaded source should contribute input hash")
	}
}
```

If `resolveNodeInputRefFromNode` does not exist yet, first extract the existing node-to-input-ref logic from `resolveNodeInputRef` into that helper so it can be unit tested.

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
```

Expected: FAIL because current resolver requires `CurrentVersionID` for generated nodes.

- [ ] **Step 2: Implement source input resolution**

In `apps/server/internal/production/service.go`, update `loadNodeInputFact` before the `CurrentVersionID` check:

```go
	if node.OperationType == "manual" && node.NodeType == db.NodeTypeText {
		ref.TextContent = node.Prompt
		fact.InputHash = sourceMaterialInputHash("manual_text", uuidToString(node.ID), node.Prompt)
		return fact, ref, nil
	}

	if node.AssetID.Valid {
		asset, err := q.GetMediaAssetByID(ctx, node.AssetID)
		if err != nil {
			return InputHashDependency{}, InputRef{}, err
		}
		ref.AssetID = uuidToString(asset.ID)
		ref.AssetType = string(asset.Type)
		ref.Mime = asset.Mime
		ref.StorageURL = textString(asset.StorageUrl)
		ref.TextContent = textString(asset.TextContent)
		fact.InputHash = sourceMaterialInputHash("asset", uuidToString(asset.ID), ref.StorageURL, ref.TextContent)
		return fact, ref, nil
	}
```

Add helper near `loadNodeInputFact`:

```go
func sourceMaterialInputHash(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "sha256:source-material:" + hex.EncodeToString(sum[:])
}
```

Add imports if they are not already present in `service.go`:

```go
import (
	"crypto/sha256"
	"encoding/hex"
)
```

Keep the existing generated-node `CurrentVersionID` path unchanged after the new source-material branches.

- [ ] **Step 3: Keep prompt renderer behavior aligned**

Add or update tests in `service_test.go`:

```go
func TestRenderPromptRefsExpandsManualTextAndAliasesUploadedImage(t *testing.T) {
	intent := GenerationIntent{
		PromptTemplate: "根据 @视频脚本 和 @商品主图 生成广告图。",
		InputRefs: []InputRef{
			{
				NodeID:      pgtype.UUID{Bytes: [16]byte{0x31}, Valid: true},
				NodeType:    "text",
				TextContent: "第一幕：机场大厅。",
			},
			{
				NodeID:     pgtype.UUID{Bytes: [16]byte{0x33}, Valid: true},
				NodeType:   "image",
				StorageURL: "workspace-a/assets/product.png",
			},
		},
	}
	rendered, err := renderPromptRefs(intent, []byte(`{"version":1,"refs":[{"node_id":"31000000-0000-0000-0000-000000000000","label":"视频脚本","node_type":"text"},{"node_id":"33000000-0000-0000-0000-000000000000","label":"商品主图","node_type":"image"}]}`), intent.InputRefs)
	if err != nil {
		t.Fatalf("render prompt refs: %v", err)
	}
	if strings.Contains(rendered.RenderedPrompt, "@视频脚本") || strings.Contains(rendered.RenderedPrompt, "@商品主图") {
		t.Fatalf("rendered prompt still contains raw mentions: %q", rendered.RenderedPrompt)
	}
	if !strings.Contains(rendered.RenderedPrompt, "第一幕：机场大厅。") {
		t.Fatalf("rendered prompt = %q, want manual text content", rendered.RenderedPrompt)
	}
	if !strings.Contains(rendered.RenderedPrompt, "图1") {
		t.Fatalf("rendered prompt = %q, want image alias", rendered.RenderedPrompt)
	}
}
```

- [ ] **Step 4: Stage uploaded source media for provider access**

Update `apps/server/internal/production/provider_asset_resolver.go` so it handles image and video source refs:

```go
if (ref.NodeType != "image" && ref.NodeType != "video") || strings.TrimSpace(ref.StorageURL) == "" {
	continue
}
allowed := allowedImageMIMEs()
if ref.NodeType == "video" {
	allowed = allowedVideoMIMEs()
}
content, mime, err := downloadProviderAsset(ctx, r.httpClient, sourceURL, defaultProviderInputMaxBytes, allowed)
```

Add:

```go
func allowedVideoMIMEs() map[string]bool {
	return map[string]bool{
		"video/mp4":       true,
		"video/quicktime": true,
		"video/webm":      true,
	}
}
```

Keep the max bytes conservative unless existing video generation needs a larger bound; if tests need video staging only with small fixtures, use the existing default.

- [ ] **Step 5: Add provider resolver tests**

In `provider_asset_resolver_test.go`, add:

```go
func TestTOSProviderAssetResolverStagesVideoInputRefs(t *testing.T) {
	store := &fakeProviderStagingStore{}
	source := &fakeProviderSourceStore{url: "https://source.example/input.mp4"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write([]byte("small mp4 fixture"))
	}))
	defer server.Close()
	source.url = server.URL + "/input.mp4"

	resolver := NewTOSProviderAssetResolver(store, server.Client(), TOSProviderAssetResolverConfig{}, source)
	intent := GenerationIntent{InputRefs: []InputRef{{
		NodeType:   "video",
		StorageURL: "workspace-a/assets/input.mp4",
	}}}

	resolved, err := resolver.ResolveInputRefs(context.Background(), ProductionJob{}, intent)
	if err != nil {
		t.Fatalf("resolve video input: %v", err)
	}
	if resolved.InputRefs[0].StorageURL != store.url {
		t.Fatalf("storage url = %q, want staged url %q", resolved.InputRefs[0].StorageURL, store.url)
	}
}
```

- [ ] **Step 6: Add frontend reference-pack/prompt-ref tests**

In `apps/web/src/lib/referencePack.test.mjs`, add:

```js
it("allows source material nodes as reference pack members", () => {
  const pack = { id: "pack", node_type: "reference_pack" };
  const nodes = [
    pack,
    { id: "image-source", node_type: "image", operation_type: "upload", asset_id: "asset-1" },
    { id: "text-source", node_type: "text", operation_type: "manual", asset_id: null },
  ];

  assert.deepEqual(
    candidateReferencePackMembers(pack, nodes, []).map((node) => node.id),
    ["image-source", "text-source"],
  );
});
```

In `apps/web/src/lib/promptRefs.test.mjs`, add:

```js
it("keeps source material nodes available as prompt references", () => {
  const target = { id: "target" };
  const source = {
    id: "script",
    title: "视频脚本",
    node_type: "text",
    operation_type: "manual",
  };
  const refs = candidatePromptRefNodes(target, [target, source], [
    { from_node_id: "script", to_node_id: "target" },
  ]);

  assert.equal(refs[0].id, "script");
});
```

- [ ] **Step 7: Verify**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

**Deliverable:** Downstream model runs can consume source material nodes.

**Acceptance:** `rendered_prompt` expands manual text and aliases uploaded media; provider request input refs include staged URLs.

---

### Task 6: Browser E2E and Final Verification

**Files:**
- No required source edits.

- [ ] **Step 1: Start or reuse local dev server**

Run:

```bash
./scripts/dev-start.sh
```

If the profile is already running, use:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
```

Use the printed Vite URL.

- [ ] **Step 2: Apply migrations and confirm health**

Run:

```bash
make migrate-up
curl -sf http://localhost:<server-port>/api/health
```

Expected: health JSON has `postgres`, `redis`, `minio`, and `sandbox` connected.

- [ ] **Step 3: Browser E2E for uploaded image source**

In browser:

1. Register or log in.
2. Create Studio workspace.
3. Drag a PNG fixture from local disk to the canvas.
4. Wait for upload to finish.
5. Click the created node.

Expected:

- Canvas node label is `图片素材`.
- Node shows image preview.
- Inspector does not show `Prompt`, `Operation`, `Model`, or `运行节点`.
- Inspector shows source material explanation and preview.

- [ ] **Step 4: Browser E2E for manual text source and reference pack**

In browser:

1. Use `文本` menu.
2. Click `文本素材`.
3. Click created text source node.
4. Paste:

```text
第一幕：机场大厅里摆放三只行李箱。
第二幕：镜头推近产品材质和颜色。
```

5. Create a reference pack.
6. Add the text source node and uploaded image source node to the pack.

Expected:

- Inspector textarea label is `内容`.
- No run button is visible.
- Reference pack member list includes both source nodes.

- [ ] **Step 5: Browser/API E2E for downstream prompt refs**

In browser/API:

1. Create image generation node.
2. Connect uploaded image source and manual text source to it.
3. Prompt:

```text
参考 @商品主图，根据 @视频脚本 生成一张广告主视觉。
```

4. Run with mock provider first.
5. Query `/api/nodes/:id/production-state`.

Expected:

- `latest_job.prompt_template` still contains `@商品主图` and `@视频脚本`.
- `latest_job.rendered_prompt` contains the script content.
- `latest_job.rendered_prompt` contains `图1` or equivalent stable image alias.
- `latest_job.provider_request` includes media input refs / image URL.
- Canvas node updates to running/complete according to async state.

- [ ] **Step 6: Optional real-provider smoke**

If Volcengine/TOS env is configured:

1. Re-run downstream image generation using Doubao Seedream.
2. Confirm provider request includes staged public image URL.
3. Confirm generated image returns and persists to MinIO.

Expected:

- No `resource download failed` for source image URL.
- Result image is visible in canvas and fullscreen review.

- [ ] **Step 7: Final verification**

Run:

```bash
make sqlc-generate
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
pnpm --filter @clip-anvil/web lint
git diff --check
```

Expected: all commands pass. Vite build must not emit the previous oversized `WorkspaceDetailPage` chunk warning after the manual chunk split.

**Deliverable:** User source material nodes are verified end to end.

**Acceptance:** User-uploaded media and manual text can feed reference packs and downstream generation without exposing generation controls on the source nodes.
