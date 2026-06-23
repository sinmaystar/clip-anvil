import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { URL } from "node:url";

const studioFlowCanvasUrl = new URL(
  "../components/canvas-flow/StudioFlowCanvas.tsx",
  import.meta.url,
);
const canvasSurfaceUrl = new URL(
  "../components/canvas-flow/CanvasFlowSurface.tsx",
  import.meta.url,
);
const workspacePageUrl = new URL(
  "../pages/WorkspaceDetailPage.tsx",
  import.meta.url,
);
const fileDropZoneUrl = new URL(
  "../components/FileDropZone.tsx",
  import.meta.url,
);

describe("studio React Flow canvas", () => {
  it("renders Studio through the shared React Flow surface instead of tldraw", async () => {
    const source = await readFile(studioFlowCanvasUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(source, /CanvasFlowSurface/);
    assert.match(source, /mode="studio"/);
    assert.match(source, /batchUpdateNodePositions/);
    assert.match(source, /updateCamera/);
    assert.match(pageSource, /StudioFlowCanvas/);
    assert.doesNotMatch(pageSource, /<Tldraw|from "tldraw"|from 'tldraw'|tldraw\/tldraw\.css/);
  });

  it("creates Studio nodes from React Flow screen coordinates", async () => {
    const source = await readFile(canvasSurfaceUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(source, /screenToFlowPosition/);
    assert.match(source, /onPaneContextMenu/);
    assert.match(source, /onCreateNodeAtPoint/);
    assert.match(pageSource, /flowPoint/);
    assert.doesNotMatch(pageSource, /screenToPage/);
  });

  it("drops uploaded files using a neutral canvas point converter", async () => {
    const source = await readFile(fileDropZoneUrl, "utf8");

    assert.match(source, /screenToCanvasPoint/);
    assert.doesNotMatch(source, /import type \{ Editor \} from "tldraw"/);
    assert.doesNotMatch(source, /editor\.screenToPage/);
  });

  it("keeps Studio selection synced with Resource Tree state", async () => {
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(pageSource, /selectedNodeId=\{selectedNodeId\}/);
    assert.match(pageSource, /selectedEdgeId=\{selectedEdgeId\}/);
    assert.match(pageSource, /onSelectNode=\{\(nodeId\) =>/);
    assert.match(pageSource, /selectOrConnectNode\(nodeId\)/);
    assert.match(pageSource, /hideActiveNodeEditor\(\)/);
    assert.match(pageSource, /onSelectEdge=\{\(edgeId\) =>/);
    assert.match(pageSource, /selectEdge\(edgeId\)/);
    assert.match(pageSource, /setSelectedEdgeId\(null\)/);
  });
});
