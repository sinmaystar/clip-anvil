import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { URL } from "node:url";

const css = readFileSync(new URL("../main.css", import.meta.url), "utf8");
const mediaShapeUtil = readFileSync(
  new URL("../shapes/MediaShapeUtil.tsx", import.meta.url),
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

  it("renders node production popover outside individual tldraw shapes", () => {
    assert.equal(
      mediaShapeUtil.includes("node-production-popover"),
      false,
      "node production popover should not be trapped inside a tldraw shape container",
    );
  });

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

  it("renders production preview text and stale reason count on media shapes", () => {
    assert.ok(
      mediaShapeUtil.includes("previewText"),
      "media shape must render production preview text",
    );
    assert.ok(
      mediaShapeUtil.includes("activeStaleReasonCount"),
      "media shape must render active stale reason count",
    );
    assert.ok(
      mediaShapeUtil.includes("media-node-stale-badge"),
      "media shape must expose a stale badge",
    );
  });

  it("keeps media node cards focused on the generated artifact instead of footer metadata", () => {
    assert.equal(
      mediaShapeUtil.includes("media-node-footer"),
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
      mediaShapeUtil.includes("MarkdownPreview"),
      "text media nodes should render MarkdownPreview",
    );
  });

  it("uses dedicated media frames that contain generated assets", () => {
    assert.ok(
      mediaShapeUtil.includes("media-node-media-frame"),
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
      mediaShapeUtil.includes("draggable={false}"),
      "node media previews must not start native browser image/video drags",
    );
    assert.ok(
      mediaShapeUtil.includes("onDragStart={preventNativeMediaDrag}"),
      "node media previews must cancel dragstart events",
    );
    assert.ok(
      css.includes("-webkit-user-drag: none"),
      "media preview CSS should also disable WebKit native dragging",
    );
  });

  it("hides studio-only node affordances in the agent readonly canvas", () => {
    assert.ok(
      css.includes(".agent-readonly-tldraw .media-node-connect-button"),
      "Agent readonly canvas should not show Studio dependency handles",
    );
    assert.ok(
      css.includes(".agent-readonly-tldraw .media-node-expand-button"),
      "Agent readonly canvas should not show Studio expand controls",
    );
  });

  it("renders video previews as playable media with poster support", () => {
    assert.ok(
      mediaShapeUtil.includes("<video"),
      "video nodes should render a playable video element",
    );
    assert.ok(
      mediaShapeUtil.includes("poster={previewThumbnailUrl || thumbnailUrl}"),
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
      mediaShapeUtil.includes("clip-anvil:node-review-request"),
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
      mediaShapeUtil.includes("materialKindLabel"),
      "media shape should use source material labels",
    );
    assert.ok(
      !mediaShapeUtil.includes("图片 PROMPT"),
      "source media nodes must not render prompt footer labels",
    );
  });

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

  it("routes dropped files through ClipAnvil upload instead of tldraw assets", () => {
    assert.ok(
      workspaceDetail.includes('registerExternalContentHandler("files"'),
      "tldraw file drops should be overridden so they create persisted media nodes",
    );
    assert.ok(
      fileDropZone.includes("onUploadFiles"),
      "the drop overlay should share the persisted upload path",
    );
    assert.equal(
      fileDropZone.includes("void uploadFiles(files, point)"),
      false,
      "window drop handling should not race tldraw's default local asset insertion",
    );
  });

  it("prevents media previews from being dragged out as tldraw URL content", () => {
    assert.ok(
      mediaShapeUtil.includes("preventNativeMediaDrag"),
      "canvas media previews should block native dragstart events",
    );
    assert.ok(
      mediaShapeUtil.includes("draggable={false}"),
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
