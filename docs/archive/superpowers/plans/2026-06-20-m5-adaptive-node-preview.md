# M5 Adaptive Node Preview Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Studio canvas nodes adapt to text, image, and video outputs, with Markdown previews and a node popover that prioritizes Prompt / Model / Params / Output over secondary debug details.

**Architecture:** Keep production execution unchanged. Add a small backend preview metadata pass-through for asset dimensions and duration, then move canvas sizing into focused frontend pure helpers. Render canvas nodes as compact result previews, and keep complete editing and debugging inside the existing floating `PropertyPanel` popover.

**Tech Stack:** Go 1.26, Hertz, pgx/sqlc, React 19, TypeScript 6, TanStack Query, React Flow, Vite 8, Node test runner, `react-markdown`, `remark-gfm`.

---

## File Structure

- Modify `apps/server/internal/api/canvas_handler.go`
  - Add optional `width`, `height`, and `duration_ms` to `canvasProductionPreview`.
  - Read dimensions from existing asset metadata without adding a migration.
- Modify `apps/server/internal/api/canvas_handler_test.go`
  - Cover metadata pass-through for image dimensions and video duration.
- Modify `apps/web/src/lib/api.ts`
  - Add optional `width`, `height`, and `duration_ms` to `ProductionPreview`.
- Create `apps/web/src/lib/nodePreviewLayout.ts`
  - Own adaptive node sizing constants and pure layout helpers.
- Create `apps/web/src/lib/nodePreviewLayout.test.mjs`
  - Cover text, image, video, persisted-size, and max-boundary behavior.
- Modify `apps/web/tsconfig.test.json`
  - Include `src/lib/nodePreviewLayout.ts`.
- Modify `apps/web/package.json`
  - Add `src/lib/nodePreviewLayout.test.mjs` to `test:connections`.
  - Add `react-markdown` and `remark-gfm` dependencies.
- Create `apps/web/src/components/MarkdownPreview.tsx`
  - Render safe Markdown previews shared by canvas nodes and property popover output.
- Modify `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
  - Use Markdown preview for text nodes.
  - Use adaptive image/video preview structure and stable status chrome.
- Modify `apps/web/src/components/canvas-flow/flowTypes.ts`
  - Add optional preview dimension props to `MediaFlowNodeProps`.
- Modify `apps/web/src/lib/canvas.ts`
  - Replace fixed `mediaNodeDisplaySize` logic with `adaptiveMediaNodeSize`.
  - Pass preview dimensions into node data.
- Modify `apps/web/src/lib/canvasLayering.test.mjs`
  - Update CSS/source assertions for adaptive nodes and Markdown preview.
- Modify `apps/web/src/components/PropertyPanel.tsx`
  - Put Prompt / Model / Params / Output first.
  - Keep Versions / Latest Job / Stale Reasons / Provider Request / Response secondary.
- Modify `apps/web/src/main.css`
  - Style adaptive nodes, Markdown previews, media previews, and popover hierarchy.

## Task 1: Backend Preview Metadata Pass-Through

**Files:**

- Modify `apps/server/internal/api/canvas_handler.go`
- Modify `apps/server/internal/api/canvas_handler_test.go`

**Deliverable Standard:**

- Canvas node `production_preview` includes `width` and `height` when asset metadata has numeric dimensions.
- Canvas node `production_preview` includes `duration_ms` when asset duration is present.
- No database migration is introduced.

**Acceptance Standard:**

- Existing canvas API behavior remains compatible.
- Missing or malformed metadata simply omits the optional fields.

**E2E Coverage:**

- Browser E2E in Task 5 validates that image nodes can use preview dimensions after a refresh.

- [ ] **Step 1: Add failing Go tests for preview dimensions**

Add tests near `TestCanvasNodeResponsesIncludeCurrentVersionPreview` in `apps/server/internal/api/canvas_handler_test.go`:

```go
func TestCanvasNodeResponsesIncludePreviewDimensions(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x21}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0x22}, Valid: true}
	node := db.MediaNode{
		ID:               pgtype.UUID{Bytes: [16]byte{0x23}, Valid: true},
		CurrentVersionID: versionID,
	}
	versions := map[pgtype.UUID]db.ArtifactVersion{
		versionID: {
			ID:        versionID,
			AssetID:   assetID,
			VersionNo: 1,
			Winner:    true,
			InputHash: "sha256:image",
		},
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:       assetID,
			Type:     db.AssetTypeImage,
			Mime:     "image/png",
			Metadata: []byte(`{"width":1024,"height":576}`),
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, versions, nil, nil)

	if nodes[0].ProductionPreview == nil {
		t.Fatal("production preview should be set")
	}
	if nodes[0].ProductionPreview.Width != 1024 {
		t.Fatalf("width = %d, want 1024", nodes[0].ProductionPreview.Width)
	}
	if nodes[0].ProductionPreview.Height != 576 {
		t.Fatalf("height = %d, want 576", nodes[0].ProductionPreview.Height)
	}
}

func TestCanvasNodeResponsesIncludePreviewDuration(t *testing.T) {
	assetID := pgtype.UUID{Bytes: [16]byte{0x24}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0x25}, Valid: true}
	node := db.MediaNode{
		ID:               pgtype.UUID{Bytes: [16]byte{0x26}, Valid: true},
		CurrentVersionID: versionID,
	}
	versions := map[pgtype.UUID]db.ArtifactVersion{
		versionID: {
			ID:        versionID,
			AssetID:   assetID,
			VersionNo: 1,
			Winner:    true,
			InputHash: "sha256:video",
		},
	}
	assets := map[pgtype.UUID]db.MediaAsset{
		assetID: {
			ID:         assetID,
			Type:       db.AssetTypeVideo,
			Mime:       "video/mp4",
			DurationMs: pgtype.Int4{Int32: 5000, Valid: true},
			Metadata:   []byte(`{"width":1280,"height":720}`),
		},
	}

	nodes := toCanvasNodeResponses([]db.MediaNode{node}, assets, versions, nil, nil)

	if nodes[0].ProductionPreview == nil {
		t.Fatal("production preview should be set")
	}
	if nodes[0].ProductionPreview.DurationMS != 5000 {
		t.Fatalf("duration = %d, want 5000", nodes[0].ProductionPreview.DurationMS)
	}
	if nodes[0].ProductionPreview.Width != 1280 || nodes[0].ProductionPreview.Height != 720 {
		t.Fatalf("dimensions = %dx%d, want 1280x720", nodes[0].ProductionPreview.Width, nodes[0].ProductionPreview.Height)
	}
}
```

- [ ] **Step 2: Run tests and confirm failure**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestCanvasNodeResponsesIncludePreview(Dimensions|Duration)'
```

Expected: compile failure because `Width`, `Height`, and `DurationMS` do not exist on `canvasProductionPreview`.

- [ ] **Step 3: Implement metadata pass-through**

In `apps/server/internal/api/canvas_handler.go`, extend `canvasProductionPreview`:

```go
type canvasProductionPreview struct {
	VersionID  string `json:"version_id"`
	VersionNo  int32  `json:"version_no"`
	AssetID    string `json:"asset_id,omitempty"`
	AssetType  string `json:"asset_type,omitempty"`
	Mime       string `json:"mime,omitempty"`
	AccessURL  string `json:"access_url,omitempty"`
	Text       string `json:"text,omitempty"`
	Width      int32  `json:"width,omitempty"`
	Height     int32  `json:"height,omitempty"`
	DurationMS int32  `json:"duration_ms,omitempty"`
	InputHash  string `json:"input_hash,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
}
```

Add helpers in the same file:

```go
func applyAssetPreviewMetadata(preview *canvasProductionPreview, asset db.MediaAsset) {
	if asset.DurationMs.Valid {
		preview.DurationMS = asset.DurationMs.Int32
	}
	width, height := dimensionsFromMetadata(asset.Metadata)
	preview.Width = width
	preview.Height = height
}

func dimensionsFromMetadata(raw []byte) (int32, int32) {
	if len(raw) == 0 {
		return 0, 0
	}
	var metadata map[string]any
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return 0, 0
	}
	return int32FromMetadata(metadata["width"]), int32FromMetadata(metadata["height"])
}

func int32FromMetadata(value any) int32 {
	switch typed := value.(type) {
	case float64:
		if typed > 0 && typed <= float64(math.MaxInt32) {
			return int32(typed)
		}
	case int:
		if typed > 0 {
			return int32(typed)
		}
	}
	return 0
}
```

Add imports:

```go
import (
	"context"
	"encoding/json"
	"math"
	"time"
)
```

Call the helper inside `toCanvasProductionPreview` after `preview.Text = textString(asset.TextContent)`:

```go
applyAssetPreviewMetadata(preview, asset)
```

- [ ] **Step 4: Run backend tests**

Run:

```bash
GOCACHE=/private/tmp/clipanvil-go-build go test ./internal/api -run 'TestCanvasNodeResponsesIncludePreview(Dimensions|Duration)'
```

Expected: PASS.

## Task 2: Frontend Adaptive Size Helper

**Files:**

- Modify `apps/web/src/lib/api.ts`
- Create `apps/web/src/lib/nodePreviewLayout.ts`
- Create `apps/web/src/lib/nodePreviewLayout.test.mjs`
- Modify `apps/web/tsconfig.test.json`
- Modify `apps/web/package.json`
- Modify `apps/web/src/lib/canvas.ts`
- Modify `apps/web/src/components/canvas-flow/flowTypes.ts`

**Deliverable Standard:**

- `adaptiveMediaNodeSize` computes bounded sizes for text, image, video, audio, and reference pack nodes.
- Text height grows with content length up to the max.
- Image/video sizes respect aspect ratio when preview dimensions exist.
- Existing large persisted `canvas_w/canvas_h` remains respected.

**Acceptance Standard:**

- Pure Node tests cover sizing behavior without launching the browser.
- Existing group bounds and custom edge geometry continue using the updated display size.

**E2E Coverage:**

- Task 5 validates the computed sizes visually in the browser.

- [ ] **Step 1: Extend frontend preview types**

In `apps/web/src/lib/api.ts`, extend `ProductionPreview`:

```ts
export interface ProductionPreview {
  version_id: string;
  version_no: number;
  asset_id?: string;
  asset_type?: AssetType;
  mime?: string;
  access_url?: string;
  text?: string;
  width?: number;
  height?: number;
  duration_ms?: number;
  input_hash?: string;
  created_at?: string;
}
```

In `apps/web/src/components/canvas-flow/flowTypes.ts`, extend `MediaFlowNodeProps`:

```ts
  previewWidth?: number;
  previewHeight?: number;
  previewDurationMs?: number;
```

- [ ] **Step 2: Write failing adaptive layout tests**

Create `apps/web/src/lib/nodePreviewLayout.test.mjs`:

```js
import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  adaptiveMediaNodeSize,
  mediaNodePreviewLimits,
} from "../../dist-test/lib/nodePreviewLayout.js";

const baseNode = {
  node_type: "text",
  canvas_w: 0,
  canvas_h: 0,
  prompt: "",
  production_preview: undefined,
  reference_pack_preview: undefined,
};

describe("adaptive media node layout", () => {
  it("grows text nodes for long generated markdown within max bounds", () => {
    const markdown = Array.from({ length: 30 }, (_, index) => `## Scene ${index + 1}\\n- action\\n- camera`).join("\\n\\n");
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      production_preview: { text: markdown },
    });

    assert.equal(size.w, 460);
    assert.ok(size.h > 300, `height ${size.h} should grow`);
    assert.ok(size.h <= mediaNodePreviewLimits.text.maxH);
  });

  it("fits image nodes to preview aspect ratio inside max bounds", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "image",
      production_preview: {
        asset_type: "image",
        width: 1600,
        height: 900,
      },
    });

    assert.ok(size.w <= mediaNodePreviewLimits.image.maxW);
    assert.ok(size.h <= mediaNodePreviewLimits.image.maxH);
    assert.ok(Math.abs(size.w / size.h - 16 / 9) < 0.05);
  });

  it("fits vertical images without cropping by height", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "image",
      production_preview: {
        asset_type: "image",
        width: 900,
        height: 1600,
      },
    });

    assert.ok(size.h <= mediaNodePreviewLimits.image.maxH);
    assert.ok(size.w < size.h);
  });

  it("keeps obviously persisted larger sizes", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "text",
      canvas_w: 700,
      canvas_h: 560,
      production_preview: { text: "short" },
    });

    assert.deepEqual(size, { w: 700, h: 560, sizeMode: "persisted" });
  });

  it("uses a stable video ratio", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "video",
      production_preview: {
        asset_type: "video",
        width: 1280,
        height: 720,
      },
    });

    assert.ok(Math.abs(size.w / size.h - 16 / 9) < 0.05);
  });
});
```

- [ ] **Step 3: Include the new test target**

Update `apps/web/tsconfig.test.json` include list:

```json
"src/lib/nodePreviewLayout.ts",
```

Update `apps/web/package.json` `test:connections` script by adding:

```text
src/lib/nodePreviewLayout.test.mjs
```

- [ ] **Step 4: Run tests and confirm failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL because `nodePreviewLayout.ts` does not exist.

- [ ] **Step 5: Implement adaptive sizing helper**

Create `apps/web/src/lib/nodePreviewLayout.ts`:

```ts
import type { MediaNode, MediaType } from "./api";
import { winnerPreviewText } from "./productionPreview";

export interface AdaptiveNodeSize {
  w: number;
  h: number;
  sizeMode: "auto" | "persisted";
}

interface NodePreviewLimit {
  minW: number;
  minH: number;
  defaultW: number;
  defaultH: number;
  maxW: number;
  maxH: number;
}

export const mediaNodePreviewLimits: Record<MediaType, NodePreviewLimit> = {
  text: { minW: 360, minH: 220, defaultW: 460, defaultH: 300, maxW: 620, maxH: 520 },
  image: { minW: 320, minH: 240, defaultW: 480, defaultH: 360, maxW: 680, maxH: 520 },
  video: { minW: 420, minH: 260, defaultW: 560, defaultH: 315, maxW: 720, maxH: 460 },
  audio: { minW: 320, minH: 120, defaultW: 360, defaultH: 140, maxW: 460, maxH: 180 },
  reference_pack: { minW: 320, minH: 180, defaultW: 360, defaultH: 220, maxW: 520, maxH: 360 },
};

export function adaptiveMediaNodeSize(
  node: Pick<
    MediaNode,
    | "node_type"
    | "canvas_w"
    | "canvas_h"
    | "prompt"
    | "production_preview"
    | "reference_pack_preview"
  >,
): AdaptiveNodeSize {
  const limits = mediaNodePreviewLimits[node.node_type];
  if (hasPersistedDisplaySize(node.canvas_w, node.canvas_h, limits)) {
    return {
      w: Math.round(node.canvas_w),
      h: Math.round(node.canvas_h),
      sizeMode: "persisted",
    };
  }

  if (node.node_type === "text") {
    return textNodeSize(node, limits);
  }
  if (node.node_type === "image") {
    return mediaRatioSize(node.production_preview?.width, node.production_preview?.height, limits, 4 / 3);
  }
  if (node.node_type === "video") {
    return mediaRatioSize(node.production_preview?.width, node.production_preview?.height, limits, 16 / 9);
  }
  return {
    w: limits.defaultW,
    h: limits.defaultH,
    sizeMode: "auto",
  };
}

function hasPersistedDisplaySize(width: number, height: number, limits: NodePreviewLimit) {
  return width > limits.defaultW + 24 || height > limits.defaultH + 24;
}

function textNodeSize(
  node: Pick<MediaNode, "node_type" | "prompt" | "production_preview" | "reference_pack_preview">,
  limits: NodePreviewLimit,
): AdaptiveNodeSize {
  const text = winnerPreviewText(node);
  const lines = Math.max(1, text.split("\\n").length);
  const chars = text.length;
  const estimatedWrappedLines = Math.ceil(chars / 54);
  const bodyLines = Math.max(lines, estimatedWrappedLines);
  const h = clamp(114 + bodyLines * 20, limits.minH, limits.maxH);
  return { w: limits.defaultW, h, sizeMode: "auto" };
}

function mediaRatioSize(
  width: number | undefined,
  height: number | undefined,
  limits: NodePreviewLimit,
  fallbackRatio: number,
): AdaptiveNodeSize {
  const ratio = width && height && width > 0 && height > 0 ? width / height : fallbackRatio;
  let w = limits.maxW;
  let h = Math.round(w / ratio);
  if (h > limits.maxH) {
    h = limits.maxH;
    w = Math.round(h * ratio);
  }
  return {
    w: clamp(w, limits.minW, limits.maxW),
    h: clamp(h, limits.minH, limits.maxH),
    sizeMode: "auto",
  };
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, Math.round(value)));
}
```

- [ ] **Step 6: Wire the helper into canvas nodes**

In `apps/web/src/lib/canvas.ts`, import and use the helper:

```ts
import { adaptiveMediaNodeSize } from "./nodePreviewLayout";
```

Replace the body of `mediaNodeDisplaySize`:

```ts
export function mediaNodeDisplaySize(
  node: Pick<
    MediaNode,
    | "node_type"
    | "canvas_w"
    | "canvas_h"
    | "prompt"
    | "production_preview"
    | "reference_pack_preview"
  >,
) {
  const size = adaptiveMediaNodeSize(node);
  return { w: size.w, h: size.h };
}
```

Update `nodeToFlowNodeData` to pass dimensions:

```ts
    previewWidth: node.production_preview?.width,
    previewHeight: node.production_preview?.height,
    previewDurationMs: node.production_preview?.duration_ms,
```

Update `MediaFlowNodeProps` validator in `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`:

```ts
    previewWidth: T.optional(T.number),
    previewHeight: T.optional(T.number),
    previewDurationMs: T.optional(T.number),
```

- [ ] **Step 7: Run frontend tests**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: PASS.

## Task 3: Markdown Preview Component

**Files:**

- Modify `apps/web/package.json`
- Create `apps/web/src/components/MarkdownPreview.tsx`
- Modify `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
- Modify `apps/web/src/components/PropertyPanel.tsx`
- Modify `apps/web/src/main.css`

**Deliverable Standard:**

- Text node output renders Markdown in canvas preview.
- Property popover output preview uses the same Markdown component.
- Markdown rendering disables raw HTML and uses constrained styles.

**Acceptance Standard:**

- Long headings, lists, and code blocks do not overflow the node chrome.
- Prompt editing remains textarea-based.

**E2E Coverage:**

- Task 5 validates Markdown preview through the browser.

- [ ] **Step 1: Add Markdown dependencies**

Run:

```bash
pnpm --filter @clip-anvil/web add react-markdown remark-gfm
```

Expected: `apps/web/package.json` and the lockfile update.

- [ ] **Step 2: Create shared MarkdownPreview component**

Create `apps/web/src/components/MarkdownPreview.tsx`:

```tsx
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

interface MarkdownPreviewProps {
  value: string;
  variant: "canvas" | "panel";
}

export function MarkdownPreview({ value, variant }: MarkdownPreviewProps) {
  return (
    <div className="markdown-preview" data-variant={variant}>
      <ReactMarkdown remarkPlugins={[remarkGfm]} skipHtml>
        {value}
      </ReactMarkdown>
    </div>
  );
}
```

- [ ] **Step 3: Use MarkdownPreview in text canvas nodes**

In `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`, import:

```ts
import { MarkdownPreview } from "../components/MarkdownPreview";
```

Replace text node body:

```tsx
            {nodeType === "text" ? (
              <MarkdownPreview
                value={previewText || promptValue || "等待输入 prompt"}
                variant="canvas"
              />
```

- [ ] **Step 4: Use MarkdownPreview in property output preview**

In `apps/web/src/components/PropertyPanel.tsx`, import:

```ts
import { MarkdownPreview } from "./MarkdownPreview";
```

Where the current output preview renders text in a `pre`, branch for text output:

```tsx
{node.node_type === "text" && outputText ? (
  <MarkdownPreview value={outputText} variant="panel" />
) : (
  <pre>{outputText || "暂无输出"}</pre>
)}
```

Use the local variable already representing output text if present. If the file currently reads text from `currentVersion.asset.text_content`, keep that source.

- [ ] **Step 5: Add Markdown CSS**

In `apps/web/src/main.css`, add:

```css
.markdown-preview {
  min-width: 0;
  width: 100%;
  color: inherit;
  overflow-wrap: anywhere;
}

.markdown-preview :where(h1, h2, h3, p, ul, ol, pre, blockquote) {
  margin: 0 0 8px;
}

.markdown-preview :where(h1, h2, h3) {
  color: var(--fg-primary);
  font-size: 13px;
  font-weight: 760;
  letter-spacing: 0;
  line-height: 1.35;
}

.markdown-preview[data-variant="panel"] :where(h1, h2, h3) {
  font-size: 15px;
}

.markdown-preview :where(ul, ol) {
  padding-left: 18px;
}

.markdown-preview :where(pre) {
  max-width: 100%;
  overflow: auto;
  border-radius: var(--radius-xs);
  background: color-mix(in srgb, black 10%, transparent);
  padding: 8px;
}

.markdown-preview :where(code) {
  font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
  font-size: 0.92em;
}

.markdown-preview[data-variant="canvas"] {
  max-height: 100%;
  overflow: auto;
  font-size: 12px;
  line-height: 1.55;
}

.markdown-preview[data-variant="panel"] {
  max-height: 420px;
  overflow: auto;
  font-size: 13px;
  line-height: 1.65;
}
```

- [ ] **Step 6: Build frontend**

Run:

```bash
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 4: Adaptive Node And Popover Visual Hierarchy

**Files:**

- Modify `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`
- Modify `apps/web/src/components/PropertyPanel.tsx`
- Modify `apps/web/src/main.css`
- Modify `apps/web/src/lib/canvasLayering.test.mjs`

**Deliverable Standard:**

- Canvas node body is the generated artifact preview.
- Text/image/video previews use stable dimensions and do not show redundant node footer labels.
- Property popover primary area is Prompt / Model / Params / Output.
- Versions / Latest Job / Stale Reasons / Provider Request / Provider Response stay secondary.

**Acceptance Standard:**

- Text does not overlap node chrome.
- Image uses `object-fit: contain` and is fully visible.
- Debug details do not squeeze Prompt textarea.

**E2E Coverage:**

- Task 5 validates layout using screenshots and DOM bounds.

- [ ] **Step 1: Add source/CSS assertions**

Extend `apps/web/src/lib/canvasLayering.test.mjs` with:

```js
  it("uses adaptive media previews and Markdown instead of plain text-only nodes", () => {
    assert.ok(
      mediaFlowNode.includes("MarkdownPreview"),
      "text media nodes should render MarkdownPreview",
    );
    assert.ok(
      css.includes(".media-node-media-frame"),
      "media nodes should expose a dedicated media preview frame",
    );
    assert.ok(
      css.includes("object-fit: contain"),
      "image previews should contain the full asset",
    );
  });
```

- [ ] **Step 2: Run test and confirm failure**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
```

Expected: FAIL until media preview classes are implemented.

- [ ] **Step 3: Update media node markup**

In `apps/web/src/components/canvas-flow/MediaFlowNode.tsx`, wrap image and video previews with dedicated frames:

```tsx
            ) : nodeType === "image" ? (
              <div className="media-node-media-frame" data-kind="image">
                {previewAssetUrl || thumbnailUrl ? (
                  <img
                    alt={titleValue || typeMeta.emptyTitle}
                    src={previewAssetUrl || thumbnailUrl}
                  />
                ) : (
                  <div className="media-node-placeholder">
                    {previewVersionNo ? `image v${previewVersionNo}` : "图片占位"}
                  </div>
                )}
              </div>
            ) : nodeType === "video" ? (
              <div className="media-node-media-frame" data-kind="video">
                <div className="media-node-placeholder">
                  <span>播放预览</span>
                  <span>
                    {previewAssetType
                      ? `${previewAssetType} v${previewVersionNo ?? "-"}`
                      : "0:00"}
                  </span>
                </div>
              </div>
```

Keep the existing reference pack and audio placeholder paths.

- [ ] **Step 4: Update node CSS**

In `apps/web/src/main.css`, adjust node preview styles:

```css
.media-node {
  grid-template-rows: 42px minmax(0, 1fr);
}

.media-node-content {
  min-width: 0;
  min-height: 0;
}

.media-node-content[data-type="text"] {
  align-items: stretch;
  overflow: hidden;
}

.media-node-media-frame {
  display: grid;
  width: 100%;
  height: 100%;
  min-width: 0;
  min-height: 0;
  place-items: center;
  overflow: hidden;
  background: color-mix(in srgb, var(--fg-primary) 5%, transparent);
}

.media-node-media-frame img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.media-node[data-status="running"] .media-node-status {
  color: var(--status-running);
}

.media-node[data-status="failed"] .media-node-status {
  color: var(--status-failed);
}

.media-node[data-status="succeeded"] .media-node-status {
  color: var(--status-succeeded);
}
```

Remove or ncustom edge older `.media-node-content img` rules so they do not conflict with `.media-node-media-frame img`.

- [ ] **Step 5: Reorder property panel sections**

In `apps/web/src/components/PropertyPanel.tsx`, keep this order in `NodePropertyPanel`:

```text
title/status/run
Prompt editor
Operation + Model
Params
Output preview
Secondary details
```

Move `Versions`, `Latest Job`, `Stale Reasons`, `Rendered Prompt`, `Provider Request`, and `Provider Response` under `<details className="property-details">`.

Use detail labels:

```tsx
<summary>Versions</summary>
<summary>Latest job</summary>
<summary>Stale reasons</summary>
<summary>Provider request</summary>
<summary>Provider response</summary>
```

Do not render graph-obvious dependency and prompt reference summaries in this popover unless they are errors requiring user action.

- [ ] **Step 6: Update popover CSS hierarchy**

In `apps/web/src/main.css`, ensure Prompt and Output have larger useful areas:

```css
.property-prompt-textarea {
  min-height: 180px;
}

.property-output-section {
  display: grid;
  gap: var(--space-2);
}

.property-details {
  margin-top: 2px;
}
```

If the existing textarea selector has another class name, apply the min-height to the current prompt textarea selector instead.

- [ ] **Step 7: Run frontend verification**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web... build
```

Expected: PASS.

## Task 5: Browser E2E And Final Verification

**Files:**

- No required source file changes unless E2E finds a bug.

**Deliverable Standard:**

- Local browser verifies the full user experience against a running dev server.
- Evidence includes text Markdown rendering, image contain behavior, running state feedback, and provider request visibility.

**Acceptance Standard:**

- No blank canvas.
- No text overlap in text nodes or buttons.
- Image node shows the full image.
- Property popover keeps primary controls usable while debug details are available.

**E2E Test Cases:**

1. Long Markdown text output.
2. Image aspect-ratio preview.
3. Immediate queued/running feedback.
4. Provider request / rendered prompt visibility.

- [ ] **Step 1: Start local runtime**

Run:

```bash
./scripts/dev-start.sh
```

Expected: script prints a Vite URL and backend health is OK. Use the printed Vite URL, not a guessed port.

- [ ] **Step 2: Open the app in the built-in browser**

Use the browser tooling to open the Vite URL from Step 1.

Expected: Studio app loads without console errors that block rendering.

- [ ] **Step 3: Verify long Markdown text node**

In the UI:

1. Register or log in.
2. Create a Studio workspace.
3. Create a Text node.
4. Configure mock text model.
5. Use a prompt that produces Markdown-like output, for example:

```text
生成一个 Markdown 格式的 TVC 分镜说明，包含标题、3 个小节、列表和一个代码块。
```

6. Run the node.

Expected:

- Node enters queued/running soon after click.
- Node succeeds.
- Canvas node is larger than the old compact card.
- Markdown headings/lists/code are styled.
- Content stays inside the node body.
- Clicking node opens popover with readable Output preview.

- [ ] **Step 4: Verify image aspect-ratio preview**

In the same workspace:

1. Create an Image node.
2. Configure mock image model or real Volcengine image model.
3. Run the node.

Expected:

- Node enters queued/running soon after click.
- Node succeeds.
- Image is fully visible with `object-fit: contain`.
- Image node dimensions stay inside max bounds.
- Refreshing the page keeps a stable preview.

- [ ] **Step 5: Verify provider request visibility**

In the Image node popover:

1. Expand Latest Job or Provider Request.
2. Inspect rendered prompt / provider request prompt.

Expected:

- The executed prompt is visible for debugging.
- Debug blocks do not collapse the Prompt editor into an unusably small area.

- [ ] **Step 6: Run final commands**

Run:

```bash
pnpm --filter @clip-anvil/web test:connections
pnpm --filter @clip-anvil/web lint
pnpm --filter @clip-anvil/web... build
GOCACHE=/private/tmp/clipanvil-go-build make server-test
GOCACHE=/private/tmp/clipanvil-go-build make server-build
git diff --check
```

Expected: all commands pass.

- [ ] **Step 7: Stop local runtime**

Run:

```bash
./scripts/dev-stop.sh
```

Expected: frontend and backend processes for this worktree stop; PostgreSQL / Redis / MinIO containers remain running.

## Plan Self-Review

- Spec coverage:
  - Adaptive size strategy: Task 2.
  - Markdown text preview: Task 3.
  - Image/video preview and max bounds: Task 2 and Task 4.
  - Popover primary/secondary hierarchy: Task 4.
  - Running state and full browser flow: Task 5.
  - No database migration: Task 1 explicitly uses existing metadata.
- Placeholder scan:
  - No unresolved placeholder markers or unspecified test command is used.
- Type consistency:
  - Backend uses `Width`, `Height`, `DurationMS`.
  - Frontend API uses `width`, `height`, `duration_ms`.
  - Node props use `previewWidth`, `previewHeight`, `previewDurationMs`.
