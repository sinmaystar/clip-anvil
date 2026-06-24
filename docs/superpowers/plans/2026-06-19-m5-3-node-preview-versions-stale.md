# M5.3 Node Preview, Versions And Stale Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Studio production results visible where users work: current winner previews on canvas nodes, version/job/stale details in the property panel, and manual stale recovery for dependency chains.

**Architecture:** Extend the existing M5.2 single-node run surface instead of adding a new workflow. The canvas payload should carry a lightweight preview of each node's current winner so the canvas can render text/image/video/audio/reference-pack status without issuing one production-state request per node. The property panel continues to use `fetchNodeProductionState` for selected-node detail, with pure frontend helpers covering preview labels, version rows, latest job rows, and stale reason text.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, React 19, TypeScript 6, React Flow, TanStack Query, Vite 8, Node test runner, browser smoke via the in-app browser.

---

## Current-State Notes

- M5.1/M5.2 are present in the working tree but not committed.
- `MediaNode` already includes production fields such as `operation_type`, `model_provider`, `model_id`, `model_params`, and `current_version_id`.
- `NodeProductionState` already includes `current_version`, `versions`, `latest_job`, `active_stale_reasons`, `capability`, and `sandbox_jobs`.
- `PropertyPanel.tsx` currently shows only a compact current-version summary and latest-job summary.
- `MediaFlowNodeProps` currently has `thumbnailUrl`, but not winner preview fields, asset type, version number, stale reason count, or reference-pack member summary.
- `CanvasHandler.GetCanvas` only joins uploaded media assets for `thumbnail_url`; generated current winner assets are not available in the canvas payload.
- Backend stale propagation is already implemented in M4: successful rerun resolves stale for that node and marks downstream nodes stale when input hashes change.

## Scope Boundaries

M5.3 includes:

- Canvas preview driven by current winner.
- Version list display in the selected-node property panel.
- Latest job detail display in the selected-node property panel.
- Stale badge on canvas nodes.
- Active stale reason display in the selected-node property panel.
- Manual rerun of stale nodes through the M5.2 run button.
- Browser E2E smoke for an A -> B -> C dependency chain.

M5.3 does not include:

- Reference Pack membership management. That is M5.4.
- Prompt `@` references or `prompt_refs` editing. That is M5.5.
- Batch cascade rerun.
- Manual winner switching between versions.
- Full media playback controls or waveform rendering.
- Agent orchestration, Producer/Craftsman/Worker, or PSS.

---

## File Structure

- Modify `apps/server/internal/api/canvas_handler.go`
  - Add lightweight production preview fields to `canvasNodeResponse`.
  - Include current winner preview for nodes with `current_version_id`.
  - Keep uploaded asset thumbnail behavior intact.
- Modify `apps/server/internal/api/canvas_handler_test.go`
  - Add tests for current winner preview fields and stale metadata.
- Modify `apps/web/src/lib/api.ts`
  - Add `ProductionPreview` to `MediaNode`.
- Modify `apps/web/src/components/canvas-flow/flowTypes.ts`
  - Extend `MediaFlowNodeProps` with preview fields.
- Modify `apps/web/src/lib/canvas.ts`
  - Map node preview fields into React Flow node data.
- Modify `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
  - Render current winner previews and stale badge on canvas cards.
- Create `apps/web/src/lib/productionPreview.ts`
  - Pure helpers for preview text, version rows, latest job rows, stale reason copy, and short hashes.
- Create `apps/web/src/lib/productionPreview.test.mjs`
  - Tests for helper behavior.
- Modify `apps/web/src/components/PropertyPanel.tsx`
  - Replace compact current version/latest job sections with version list, latest job detail, and stale reasons.
- Modify `apps/web/tsconfig.test.json`
  - Include `src/lib/productionPreview.ts`.
- Modify `apps/web/package.json`
  - Add `src/lib/productionPreview.test.mjs` to `test:connections`.
- Optional if needed: modify `apps/server/sqlc/queries/production.sql`
  - Add a bulk current-version query only if simple `current_version_id` lookups are too slow or too awkward in `CanvasHandler`.

---

## Deliverable Standards

- Canvas cards use `current_version` preview, not the node prompt, after successful runs.
- Text nodes show generated text preview.
- Image nodes show current winner image preview when an access URL or thumbnail URL is available.
- Video nodes show asset/video metadata and a stable play-preview placeholder.
- Audio nodes show asset/audio metadata and a stable audio placeholder.
- Reference Pack nodes still show a member summary placeholder in M5.3; real membership management stays in M5.4.
- Canvas cards show a clear stale badge when `node.status === "stale"` or active stale reasons exist.
- Property panel shows all versions with version number, winner status, asset type, created time, and short input hash.
- Property panel shows latest job status, operation, provider/model, rendered prompt, attempt, and error code/message.
- Property panel lists active stale reasons with upstream node id/version and readable reason text.
- Running a stale node through the existing Run button clears that node's stale reasons after success.

## Acceptance Standards

- A -> B -> C all run successfully and show current winner previews.
- Rerunning A marks B and C stale on canvas.
- Selecting B shows stale reasons in the property panel.
- Rerunning B clears B stale while C remains stale.
- Rerunning C clears C stale.
- Historical versions remain listed after reruns.
- No new N+1 production-state fetch is introduced for every canvas node on normal canvas load.
- Existing M5.1/M5.2 flows still work: creating Reference Pack nodes, manually running text/image/video, failed run display, and retry button.

## E2E Smoke Case

1. Start the app with `./scripts/dev-start.sh` and use the printed Vite URL.
2. Register or log in.
3. Create a Studio Workspace named `M5.3 Stale Smoke`.
4. Create Text nodes A, B, and C.
5. Connect A -> B and B -> C.
6. Configure each as `text_generation` / `mock-text`.
7. Run A, then B, then C.
8. Confirm all three canvas cards show generated current winner previews and property-panel version rows.
9. Change A prompt and rerun A.
10. Confirm B and C show stale badges on the canvas.
11. Select B and confirm stale reason text is visible in the property panel.
12. Rerun B and confirm B stale clears while C stays stale.
13. Select and rerun C and confirm C stale clears.
14. Check browser console for new application errors.

---

## Task 1: Add Backend Canvas Production Preview

**Files:**
- Modify: `apps/server/internal/api/canvas_handler.go`
- Modify: `apps/server/internal/api/canvas_handler_test.go`

- [ ] **Step 1: Add failing canvas response tests**

Append tests in `apps/server/internal/api/canvas_handler_test.go` near `TestCanvasNodeResponsesIncludeAssetThumbnailURL`:

```go
func TestCanvasNodeResponsesIncludeCurrentVersionPreview(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x07}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0x08}, Valid: true}
	node := db.MediaNode{
		ID:               pgtype.UUID{Bytes: [16]byte{0x09}, Valid: true},
		CurrentVersionID: versionID,
	}
	versions := map[pgtype.UUID]db.ArtifactVersion{
		versionID: {
			ID:        versionID,
			AssetID:   assetID,
			VersionNo: 2,
			Winner:    true,
			InputHash: "sha256:abcdef1234567890",
		},
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:          assetID,
			Type:        db.AssetTypeText,
			TextContent: pgtype.Text{String: "Generated winner text", Valid: true},
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, versions, nil)

	if len(nodes) != 1 {
		t.Fatalf("nodes len = %d, want 1", len(nodes))
	}
	if nodes[0].ProductionPreview == nil {
		t.Fatal("production preview should be set")
	}
	if nodes[0].ProductionPreview.VersionNo != 2 {
		t.Fatalf("version no = %d, want 2", nodes[0].ProductionPreview.VersionNo)
	}
	if nodes[0].ProductionPreview.Text != "Generated winner text" {
		t.Fatalf("preview text = %q, want generated text", nodes[0].ProductionPreview.Text)
	}
}
```

Add a second test for stale count:

```go
func TestCanvasNodeResponsesIncludeActiveStaleReasonCount(t *testing.T) {
	nodeID := pgtype.UUID{Bytes: [16]byte{0x0a}, Valid: true}
	node := db.MediaNode{ID: nodeID, Status: db.NodeStatusStale}
	reasonsByNode := map[pgtype.UUID]int{nodeID: 2}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, nil, nil, reasonsByNode)

	if nodes[0].ActiveStaleReasonCount != 2 {
		t.Fatalf("stale reason count = %d, want 2", nodes[0].ActiveStaleReasonCount)
	}
}
```

- [ ] **Step 2: Run focused backend tests and verify failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/api -run 'TestCanvasNodeResponsesInclude(CurrentVersionPreview|ActiveStaleReasonCount)'
```

Expected: FAIL because `canvasNodeResponse` does not expose production preview or stale reason count yet.

- [ ] **Step 3: Implement response structs**

In `apps/server/internal/api/canvas_handler.go`, replace `canvasNodeResponse` with:

```go
type canvasNodeResponse struct {
	db.MediaNode
	ThumbnailURL           *string                  `json:"thumbnail_url,omitempty"`
	ProductionPreview      *canvasProductionPreview `json:"production_preview,omitempty"`
	ActiveStaleReasonCount int                      `json:"active_stale_reason_count"`
}

type canvasProductionPreview struct {
	VersionID  string `json:"version_id"`
	VersionNo int32  `json:"version_no"`
	AssetID   string `json:"asset_id,omitempty"`
	AssetType string `json:"asset_type,omitempty"`
	Mime      string `json:"mime,omitempty"`
	AccessURL string `json:"access_url,omitempty"`
	Text      string `json:"text,omitempty"`
	InputHash string `json:"input_hash,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}
```

- [ ] **Step 4: Load preview data in `GetCanvas`**

In `GetCanvas`, after loading assets, load current versions and active stale reason counts. Prefer a small helper that loops over nodes by `CurrentVersionID` for M5.3 if no bulk sqlc query exists yet:

```go
versionsByID, err := h.currentVersionsByID(ctx, nodes)
if err != nil {
	writeError(c, consts.StatusInternalServerError, "failed to load current versions")
	return
}
staleCounts, err := h.activeStaleReasonCountsByNode(ctx, nodes)
if err != nil {
	writeError(c, consts.StatusInternalServerError, "failed to load stale reasons")
	return
}
```

Then call:

```go
Nodes: toCanvasNodeResponses(nodes, assetsByID, versionsByID, staleCounts),
```

- [ ] **Step 5: Add helpers for preview data**

Add helpers in `canvas_handler.go`:

```go
func (h *CanvasHandler) currentVersionsByID(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID]db.ArtifactVersion, error) {
	out := map[pgtype.UUID]db.ArtifactVersion{}
	for _, node := range nodes {
		if !node.CurrentVersionID.Valid {
			continue
		}
		version, err := h.queries.GetArtifactVersionByID(ctx, node.CurrentVersionID)
		if err != nil {
			return nil, err
		}
		out[node.CurrentVersionID] = version
	}
	return out, nil
}

func (h *CanvasHandler) activeStaleReasonCountsByNode(ctx context.Context, nodes []db.MediaNode) (map[pgtype.UUID]int, error) {
	out := map[pgtype.UUID]int{}
	for _, node := range nodes {
		reasons, err := h.queries.ListActiveStaleReasonsByNode(ctx, node.ID)
		if err != nil {
			return nil, err
		}
		out[node.ID] = len(reasons)
	}
	return out, nil
}
```

This is acceptable for M5.3 because Studio canvases are still small; if browser smoke shows visible latency, replace it with a bulk sqlc query before finishing M5.3.

- [ ] **Step 6: Update `toCanvasNodeResponses`**

Change the signature:

```go
func toCanvasNodeResponses(
	nodes []db.MediaNode,
	assets map[pgtype.UUID]db.MediaAsset,
	versions map[pgtype.UUID]db.ArtifactVersion,
	staleCounts map[pgtype.UUID]int,
) []canvasNodeResponse
```

For each node:

```go
response.ActiveStaleReasonCount = staleCounts[node.ID]
if node.CurrentVersionID.Valid {
	if version, ok := versions[node.CurrentVersionID]; ok {
		response.ProductionPreview = toCanvasProductionPreview(version, assets)
	}
}
```

Keep existing uploaded asset thumbnail logic unchanged.

- [ ] **Step 7: Add preview conversion helper**

Add:

```go
func toCanvasProductionPreview(version db.ArtifactVersion, assets map[pgtype.UUID]db.MediaAsset) *canvasProductionPreview {
	preview := &canvasProductionPreview{
		VersionID:  uuidToString(version.ID),
		VersionNo: version.VersionNo,
		InputHash: version.InputHash,
		CreatedAt: timeString(version.CreatedAt),
	}
	if version.AssetID.Valid {
		preview.AssetID = uuidToString(version.AssetID)
		if asset, ok := assets[version.AssetID]; ok {
			preview.AssetType = string(asset.Type)
			preview.Mime = asset.Mime
			preview.AccessURL = textString(asset.StorageUrl)
			preview.Text = textString(asset.TextContent)
		}
	}
	return preview
}
```

If image/video generated assets require presigned URLs rather than direct `StorageUrl`, keep `AccessURL` empty in this task and render a typed placeholder; do not add storage dependencies to `CanvasHandler` in M5.3 unless browser smoke proves image preview impossible.

- [ ] **Step 8: Run backend API tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./apps/server/internal/api
```

Expected: PASS.

---

## Task 2: Add Frontend Preview Helpers And Node Props

**Files:**
- Create: `apps/web/src/lib/productionPreview.ts`
- Create: `apps/web/src/lib/productionPreview.test.mjs`
- Modify: `apps/web/src/lib/api.ts`
- Modify: `apps/web/src/components/canvas-flow/flowTypes.ts`
- Modify: `apps/web/src/lib/canvas.ts`
- Modify: `apps/web/tsconfig.test.json`
- Modify: `apps/web/package.json`

- [ ] **Step 1: Add failing helper tests**

Create `apps/web/src/lib/productionPreview.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  formatInputHash,
  jobDetailRows,
  staleReasonText,
  versionRows,
  winnerPreviewText,
} from "../../dist-test/lib/productionPreview.js";

describe("production preview helpers", () => {
  it("prefers current winner text over prompt text", () => {
    assert.equal(
      winnerPreviewText({
        node_type: "text",
        prompt: "old prompt",
        production_preview: { text: "new generated output", asset_type: "text" },
      }),
      "new generated output",
    );
  });

  it("shortens input hashes for version rows", () => {
    assert.equal(formatInputHash("sha256:abcdef1234567890"), "abcdef12");
  });

  it("builds version rows with winner and asset labels", () => {
    const rows = versionRows([
      {
        id: "v1",
        version_no: 1,
        winner: false,
        input_hash: "sha256:111111111111",
        created_at: "2026-06-19T00:00:00Z",
        output: {},
        asset: { type: "text", mime: "text/plain", metadata: {} },
      },
      {
        id: "v2",
        version_no: 2,
        winner: true,
        input_hash: "sha256:222222222222",
        created_at: "2026-06-19T00:01:00Z",
        output: {},
        asset: { type: "image", mime: "image/png", metadata: {} },
      },
    ]);

    assert.deepEqual(
      rows.map((row) => [row.versionLabel, row.assetLabel, row.isWinner]),
      [
        ["v2", "image", true],
        ["v1", "text", false],
      ],
    );
  });

  it("exposes latest job details including rendered prompt and errors", () => {
    const rows = jobDetailRows({
      status: "failed",
      operation_type: "text_generation",
      provider: "mock",
      model_id: "mock-text",
      rendered_prompt: "hello",
      attempt: 1,
      max_attempts: 1,
      error_code: "provider_failed",
      error_message: "mock provider failure",
    });

    assert.equal(rows.find((row) => row.label === "Prompt")?.value, "hello");
    assert.equal(rows.find((row) => row.label === "Error")?.value, "provider_failed: mock provider failure");
  });

  it("renders stale reasons with upstream context", () => {
    assert.equal(
      staleReasonText({
        reason_code: "upstream_current_version_changed",
        reason_message: "Upstream dependency current version changed.",
        upstream_node_id: "node-a",
        upstream_version_id: "version-a",
        details: { old_input_hash: "sha256:old", new_input_hash: "sha256:new" },
      }),
      "Upstream dependency current version changed. (node-a -> version-a)",
    );
  });
});
```

- [ ] **Step 2: Register tests and expected failure**

Add `src/lib/productionPreview.ts` to `apps/web/tsconfig.test.json`, and add `src/lib/productionPreview.test.mjs` to `apps/web/package.json` `test:connections`.

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `productionPreview.ts` does not exist.

- [ ] **Step 3: Add frontend production preview types**

In `apps/web/src/lib/api.ts`, add:

```ts
export interface ProductionPreview {
  version_id: string;
  version_no: number;
  asset_id?: string;
  asset_type?: AssetType;
  mime?: string;
  access_url?: string;
  text?: string;
  input_hash?: string;
  created_at?: string;
}
```

Then add these optional fields to `MediaNode`:

```ts
  production_preview?: ProductionPreview;
  active_stale_reason_count?: number;
```

- [ ] **Step 4: Extend node data**

In `apps/web/src/components/canvas-flow/flowTypes.ts`, add to `MediaFlowNodeProps`:

```ts
  previewText?: string;
  previewAssetType?: string;
  previewAssetUrl?: string;
  previewVersionNo?: number;
  activeStaleReasonCount?: number;
```

- [ ] **Step 5: Implement `productionPreview.ts`**

Create `apps/web/src/lib/productionPreview.ts` with helpers:

```ts
import type { ArtifactVersion, GenerationJob, MediaNode, StaleReason } from "./api";
import { formatJobAttempt } from "./productionPanel";

export function winnerPreviewText(
  node: Pick<MediaNode, "node_type" | "prompt" | "production_preview">,
) {
  const preview = node.production_preview;
  if (node.node_type === "text" && preview?.text) {
    return preview.text;
  }
  if (preview?.asset_type) {
    return `${preview.asset_type} v${preview.version_no}`;
  }
  return node.prompt || "等待输入 prompt";
}

export function formatInputHash(hash?: string) {
  if (!hash) {
    return "-";
  }
  return hash.replace(/^sha256:/, "").slice(0, 8);
}

export function versionRows(versions: ArtifactVersion[]) {
  return [...versions]
    .sort((a, b) => b.version_no - a.version_no)
    .map((version) => ({
      id: version.id,
      versionLabel: `v${version.version_no}`,
      isWinner: version.winner,
      assetLabel: version.asset?.type ?? "output",
      inputHash: formatInputHash(version.input_hash),
      createdAt: version.created_at,
    }));
}

export function jobDetailRows(job: Partial<GenerationJob>) {
  const error =
    job.error_code && job.error_message
      ? `${job.error_code}: ${job.error_message}`
      : job.error_message || job.error_code || "";
  return [
    { label: "Status", value: job.status ?? "-" },
    { label: "Operation", value: job.operation_type ?? "-" },
    { label: "Model", value: `${job.provider ?? "-"}/${job.model_id ?? "-"}` },
    { label: "Attempt", value: job.attempt && job.max_attempts ? formatJobAttempt(job as GenerationJob) : "-" },
    { label: "Prompt", value: job.rendered_prompt || "-" },
    { label: "Error", value: error || "-" },
  ];
}

export function staleReasonText(reason: Pick<StaleReason, "reason_code" | "reason_message" | "upstream_node_id" | "upstream_version_id" | "details">) {
  const upstream = reason.upstream_version_id
    ? `${reason.upstream_node_id} -> ${reason.upstream_version_id}`
    : reason.upstream_node_id;
  return `${reason.reason_message || reason.reason_code} (${upstream})`;
}
```

Adjust type casts if TypeScript requires a ncustom edgeer node for `formatJobAttempt`; keep the public behavior covered by tests.

- [ ] **Step 6: Map node preview into canvas node data**

In `apps/web/src/lib/canvas.ts`, add to `nodeToFlowNodeData`:

```ts
    previewText: winnerPreviewText(node),
    previewAssetType: node.production_preview?.asset_type,
    previewAssetUrl: node.production_preview?.access_url,
    previewVersionNo: node.production_preview?.version_no,
    activeStaleReasonCount: node.active_stale_reason_count ?? 0,
```

Import `winnerPreviewText` from `./productionPreview`.

- [ ] **Step 7: Run frontend helper tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

---

## Task 3: Render Canvas Current Winner Preview And Stale Badge

**Files:**
- Modify: `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/src/lib/canvasLayering.test.mjs`

- [ ] **Step 1: Add source-level node rendering guards**

Append tests in `apps/web/src/lib/canvasLayering.test.mjs`:

```js
  it("renders production preview text and stale reason count on media nodes", () => {
    assert.ok(
      mediaFlowNode.includes("previewText"),
      "media node must render production preview text",
    );
    assert.ok(
      mediaFlowNode.includes("activeStaleReasonCount"),
      "media node must render active stale reason count",
    );
    assert.ok(
      mediaFlowNode.includes("media-node-stale-badge"),
      "media node must expose a stale badge",
    );
  });
```

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL until the node renders the new props.

- [ ] **Step 2: Read preview props in `MediaNodeNode`**

In `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`, read:

```ts
const {
  title,
  prompt,
  status,
  nodeType,
  thumbnailUrl,
  previewText,
  previewAssetType,
  previewAssetUrl,
  previewVersionNo,
  activeStaleReasonCount,
  w,
  h,
} = node.props;
```

- [ ] **Step 3: Render stale badge**

Inside `.media-node-header`, after the status span, render:

```tsx
{status === "stale" || Number(activeStaleReasonCount) > 0 ? (
  <span className="media-node-stale-badge">
    stale{activeStaleReasonCount ? ` · ${activeStaleReasonCount}` : ""}
  </span>
) : null}
```

- [ ] **Step 4: Render current winner preview**

Update `.media-node-content` rendering:

```tsx
{nodeType === "text" ? (
  <p>{previewText || promptValue || "等待输入 prompt"}</p>
) : nodeType === "image" ? (
  previewAssetUrl || thumbnailUrl ? (
    <img alt={titleValue || typeMeta.emptyTitle} src={previewAssetUrl || thumbnailUrl} />
  ) : (
    <div className="media-node-placeholder">
      {previewVersionNo ? `image v${previewVersionNo}` : "图片占位"}
    </div>
  )
) : nodeType === "video" ? (
  <div className="media-node-placeholder">
    <span>播放预览</span>
    <span>{previewAssetType ? `${previewAssetType} v${previewVersionNo ?? "-"}` : "0:00"}</span>
  </div>
) : nodeType === "audio" ? (
  <div className="media-node-placeholder">
    <span className="media-node-waveform" />
    <span>{previewAssetType ? `${previewAssetType} v${previewVersionNo ?? "-"}` : "0:00"}</span>
  </div>
) : (
  <div className="media-node-placeholder">
    <span>Reference Pack</span>
    <span>{previewText || "等待成员"}</span>
  </div>
)}
```

- [ ] **Step 5: Style stale badge and preview media**

In `apps/web/src/main.css`, add compact styles:

```css
.media-node-stale-badge {
  border: 1px solid rgba(245, 158, 11, 0.45);
  border-radius: 999px;
  color: #92400e;
  font-size: 11px;
  line-height: 1;
  padding: 3px 6px;
  white-space: nowrap;
}
```

If image previews overflow, add:

```css
.media-node-content img {
  max-height: 100%;
  object-fit: cover;
  width: 100%;
}
```

- [ ] **Step 6: Run frontend tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

---

## Task 4: Expand Property Panel Versions, Latest Job, And Stale Reasons

**Files:**
- Modify: `apps/web/src/components/PropertyPanel.tsx`
- Modify: `apps/web/src/main.css`
- Modify: `apps/web/src/lib/productionPanel.test.mjs`

- [ ] **Step 1: Add helper coverage where existing helpers are reused**

Extend `apps/web/src/lib/productionPanel.test.mjs` only if `PropertyPanel` needs new pure helpers that are not covered by `productionPreview.test.mjs`. Do not add DOM tests for React in M5.3 unless the repo already has a React test harness.

- [ ] **Step 2: Import preview helpers**

In `PropertyPanel.tsx`, import:

```ts
import {
  jobDetailRows,
  staleReasonText,
  versionRows,
} from "../lib/productionPreview";
```

- [ ] **Step 3: Replace Current Version section with version list**

Replace the current compact `Current Version` section with:

```tsx
<div className="property-section">
  <p className="studio-section-label">Versions</p>
  {isProductionStateLoading ? (
    <p className="property-empty">正在读取 production state。</p>
  ) : nodeProductionState?.versions.length ? (
    <div className="property-version-list">
      {versionRows(nodeProductionState.versions).map((version) => (
        <div className="property-version-row" key={version.id}>
          <strong>{version.versionLabel}</strong>
          <span>{version.assetLabel}</span>
          <span>{version.inputHash}</span>
          {version.isWinner ? <span>current</span> : null}
        </div>
      ))}
    </div>
  ) : (
    <p className="property-empty">尚无 artifact version。</p>
  )}
</div>
```

- [ ] **Step 4: Expand Latest Job section**

Replace latest job `<dl>` rows with:

```tsx
{latestJob ? (
  <dl className="property-list">
    {jobDetailRows(latestJob).map((row) => (
      <div key={row.label}>
        <dt>{row.label}</dt>
        <dd>{row.value}</dd>
      </div>
    ))}
  </dl>
) : (
  <p className="property-empty">尚未运行。</p>
)}
```

Keep the existing retry button behavior unchanged.

- [ ] **Step 5: Add Stale Reasons section**

After dependencies and before Run, add:

```tsx
<div className="property-section">
  <p className="studio-section-label">Stale Reasons</p>
  {nodeProductionState?.active_stale_reasons.length ? (
    <ul className="property-stale-list">
      {nodeProductionState.active_stale_reasons.map((reason) => (
        <li key={reason.id}>{staleReasonText(reason)}</li>
      ))}
    </ul>
  ) : (
    <p className="property-empty">当前节点没有 active stale reason。</p>
  )}
</div>
```

- [ ] **Step 6: Add property list styles**

In `apps/web/src/main.css`, add:

```css
.property-version-list,
.property-stale-list {
  display: grid;
  gap: 6px;
  margin: 0;
}

.property-version-row {
  align-items: center;
  border: 1px solid var(--studio-border);
  border-radius: 8px;
  display: grid;
  gap: 4px;
  grid-template-columns: auto 1fr auto auto;
  min-width: 0;
  padding: 8px;
}

.property-stale-list {
  padding-left: 18px;
}
```

- [ ] **Step 7: Run frontend tests and build**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

---

## Task 5: Stale Recovery Integration And Browser Smoke

**Files:**
- Modify: `apps/web/src/pages/WorkspaceDetailPage.tsx` only if canvas refresh does not update stale badges after run/retry.
- No new source file expected.

- [ ] **Step 1: Verify cache invalidation after run and retry**

Confirm `runNodeMutation` and `retryJobMutation` invalidate:

```ts
queryClient.invalidateQueries({ queryKey: ["workspace", id, "canvas"] });
queryClient.invalidateQueries({
  queryKey: ["node", selectedNodeId, "production-state"],
});
```

If stale badges do not refresh in browser smoke, add invalidation for the concrete `nodeId` returned by run/retry.

- [ ] **Step 2: Start runtime**

Run:

```bash
CLIPANVIL_PRINT_DEV_ENV=1 ./scripts/dev-start.sh
./scripts/dev-start.sh
```

Use the printed Vite URL. If Docker access is blocked by the sandbox, rerun `./scripts/dev-start.sh` with escalation.

- [ ] **Step 3: Browser smoke M5.3 A -> B -> C**

Run the E2E smoke case listed above. Capture the observed Vite URL, workspace name, and any console errors in the final response.

- [ ] **Step 4: Stop runtime**

Run:

```bash
CLIPANVIL_DEV_NAME=<printed-profile> ./scripts/dev-stop.sh
```

- [ ] **Step 5: Final verification**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build make server-test
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
git diff --check
```

Expected: all commands pass.

---

## Self-Review

- Spec coverage: M5.3 canvas preview, version list, latest job detail, stale badge, stale reasons, and manual stale recovery are all mapped to tasks.
- Scope: Reference Pack membership and Prompt `@` remain explicitly deferred to M5.4/M5.5.
- Test coverage: backend response node tests, frontend pure helper tests, source-level node guard, existing build/lint, and browser E2E smoke are included.
- Risk: `CanvasHandler` helper loops are acceptable for M5.3 but should become a bulk sqlc query if canvases grow or browser smoke shows latency.
