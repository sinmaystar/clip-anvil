import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { URL } from "node:url";

const css = readFileSync(new URL("../main.css", import.meta.url), "utf8");
const mediaFlowNode = readFileSync(
  new URL("../components/canvas-flow/MediaFlowNode.tsx", import.meta.url),
  "utf8",
);
const nodeInspector = readFileSync(
  new URL("../components/canvas-flow/NodeInspectorPopover.tsx", import.meta.url),
  "utf8",
);
const canvasSurface = readFileSync(
  new URL("../components/canvas-flow/CanvasFlowSurface.tsx", import.meta.url),
  "utf8",
);
const fileDropZone = readFileSync(
  new URL("../components/FileDropZone.tsx", import.meta.url),
  "utf8",
);
const workspaceDetail = readFileSync(
  new URL("../pages/WorkspaceDetailPage.tsx", import.meta.url),
  "utf8",
);
const propertyPanel = readFileSync(
  new URL("../components/PropertyPanel.tsx", import.meta.url),
  "utf8",
);
const artifactViewer = readFileSync(
  new URL("./artifactViewer.ts", import.meta.url),
  "utf8",
);

describe("canvas layering", () => {
  it("keeps node production popover above animated connection lines", () => {
    assert.ok(
      zIndex(".node-editor-overlay") > zIndex(".connection-overlay"),
      "node production popover must be layered above connection overlay",
    );
  });

  it("keeps node production popover outside individual canvas node cards", () => {
    assert.equal(
      mediaFlowNode.includes("node-production-popover"),
      false,
      "node production popover should not be trapped inside an individual node card",
    );
  });

  it("declares reference pack as a supported media node type", () => {
    assert.ok(
      mediaFlowNode.includes("reference_pack_preview"),
      "reference_pack must have a canvas card branch",
    );
    assert.ok(
      mediaFlowNode.includes("Reference Pack"),
      "reference_pack must have node display metadata",
    );
  });

  it("renders production preview text and stale reason count on media cards", () => {
    assert.ok(
      mediaFlowNode.includes("previewText"),
      "media card must render production preview text",
    );
    assert.ok(
      mediaFlowNode.includes("active_stale_reason_count"),
      "media card must render active stale reason count",
    );
    assert.ok(
      mediaFlowNode.includes("media-node-stale-badge"),
      "media card must expose a stale badge",
    );
  });

  it("keeps media node cards focused on the generated artifact instead of footer metadata", () => {
    assert.equal(
      mediaFlowNode.includes("media-node-footer"),
      false,
      "media nodes should not spend canvas space on type/prompt footer metadata",
    );
    assert.ok(
      css.includes("grid-template-rows: 42px minmax(0, 1fr);"),
      "media node layout should reserve the remaining card area for artifact preview",
    );
  });

  it("renders text media nodes through a Markdown preview component", () => {
    assert.ok(
      mediaFlowNode.includes("MarkdownPreview"),
      "text media nodes should render MarkdownPreview",
    );
  });

  it("uses dedicated media frames that contain generated assets", () => {
    assert.ok(
      mediaFlowNode.includes("media-node-media-frame"),
      "image and video previews should render through a dedicated media frame",
    );
    assert.ok(
      css.includes(".media-node-media-frame"),
      "media frame CSS should be present",
    );
    assert.ok(
      css.includes("object-fit: contain"),
      "image previews should render the full asset without cropping",
    );
    assert.ok(
      css.includes("display: flex;"),
      "media frames should not let images stretch outside the preview area",
    );
    assert.ok(
      css.includes("max-height: 100%;"),
      "image previews should be constrained to the media frame height",
    );
  });

  it("disables native browser drag for media previews inside nodes", () => {
    assert.ok(
      mediaFlowNode.includes("draggable={false}"),
      "node media previews must not start native browser image/video drags",
    );
    assert.ok(
      mediaFlowNode.includes('className="media-node-media-frame"'),
      "node media previews must stay inside the card media frame",
    );
    assert.ok(
      css.includes("-webkit-user-drag: none"),
      "media preview CSS should also disable WebKit native dragging",
    );
  });

  it("uses the shared React Flow surface selectors", () => {
    assert.ok(
      canvasSurface.includes("CanvasFlowSurface"),
      "Studio and Agent should share the same canvas surface component",
    );
    assert.ok(
      css.includes(".canvas-flow-surface .react-flow"),
      "Agent canvas should rely on the shared React Flow surface",
    );
  });

  it("renders video previews as playable media with poster support", () => {
    assert.ok(
      mediaFlowNode.includes("<video"),
      "video nodes should render a playable video element",
    );
    assert.ok(
      mediaFlowNode.includes("poster={previewThumbnailUrl}"),
      "video nodes should use the generated first frame as a poster",
    );
    assert.ok(
      css.includes(".media-node-media-frame video"),
      "video previews should share media frame sizing rules",
    );
  });

  it("opens fullscreen asset review in a separate browser tab", () => {
    assert.ok(
      propertyPanel.includes("openArtifactVersionInNewTab"),
      "version fullscreen review should open the asset in a new tab",
    );
    assert.equal(
      propertyPanel.includes("AssetReviewOverlay"),
      false,
      "fullscreen review should not render an in-page overlay",
    );
    assert.ok(
      css.includes(".media-node-expand-button"),
      "media nodes should expose a compact fullscreen affordance",
    );
    assert.ok(
      workspaceDetail.includes("clip-anvil:node-review-request"),
      "node expand button should dispatch an asset review request",
    );
    assert.ok(
      artifactViewer.includes("renderArtifactViewerHTML"),
      "fullscreen review should open a ClipAnvil viewer document instead of navigating to a downloadable asset URL",
    );
    assert.equal(
      artifactViewer.includes("return source.accessUrl"),
      false,
      "fullscreen review must not navigate the new tab directly to the asset URL",
    );
  });

  it("declares source material labels as first-class node identity", () => {
    assert.ok(
      mediaFlowNode.includes("materialKindLabel"),
      "media shape should use source material labels",
    );
    assert.ok(
      !mediaFlowNode.includes("图片 PROMPT"),
      "source media nodes must not render prompt footer labels",
    );
  });

  it("creates uploaded media as source material nodes", () => {
    assert.ok(
      fileDropZone.includes('operation_type: "upload"'),
      "uploaded files should create upload source nodes",
    );
    assert.ok(
      fileDropZone.includes('status: "succeeded"'),
      "uploaded source nodes should be immediately usable",
    );
  });

  it("routes dropped files through ClipAnvil upload with React Flow coordinates", () => {
    assert.ok(
      fileDropZone.includes("screenToCanvasPoint"),
      "window file drops should be converted through a canvas-neutral point mapper",
    );
    assert.ok(
      fileDropZone.includes("onUploadFiles"),
      "the drop overlay should share the persisted upload path",
    );
    assert.ok(
      workspaceDetail.includes("screenToCanvasPoint={screenToCanvasPoint}"),
      "Studio should pass its React Flow coordinate converter to the drop overlay",
    );
    assert.equal(
      fileDropZone.includes("void uploadFiles(files, point)"),
      false,
      "window drop handling should not create local-only assets",
    );
  });

  it("prevents media previews from being dragged out as URL content", () => {
    assert.ok(
      mediaFlowNode.includes("media-node-media-frame"),
      "canvas media previews should render inside controlled media frames",
    );
    assert.ok(
      mediaFlowNode.includes("draggable={false}"),
      "canvas media preview elements should not expose draggable image or video URLs",
    );
  });

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

  it("keeps shared inspector actions policy-gated", () => {
    assert.ok(
      nodeInspector.includes("policy.canRunNodes"),
      "run action should follow the canvas mode policy",
    );
    assert.ok(
      nodeInspector.includes("policy.canEditNodeContent"),
      "edit action should follow the canvas mode policy",
    );
  });

  it("keeps Studio production popover scrollable within the viewport", () => {
    assert.ok(
      workspaceDetail.includes("maxHeight: nodeEditorPosition.maxHeight"),
      "Studio should pass a computed viewport-safe height to the production popover",
    );
    assert.ok(
      css.includes("max-height: var(--node-editor-max-height"),
      "production popover should use the viewport-safe height variable",
    );
    assert.ok(
      css.includes("overscroll-behavior: contain"),
      "production popover scrolling should not chain into React Flow zoom",
    );
  });
});

function zIndex(selector) {
  const block = css.match(new RegExp(`${escapeRegExp(selector)}\\s*\\{([^}]*)\\}`));
  assert.ok(block, `missing CSS block for ${selector}`);
  const value = block[1].match(/z-index:\s*(\d+)\s*;/);
  assert.ok(value, `missing z-index for ${selector}`);
  return Number(value[1]);
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
