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
const canvasViewModelUrl = new URL(
  "../components/canvas-flow/canvasViewModel.ts",
  import.meta.url,
);
const groupFlowNodeUrl = new URL(
  "../components/canvas-flow/GroupFlowNode.tsx",
  import.meta.url,
);

describe("studio React Flow canvas", () => {
  it("renders Studio through the shared React Flow surface", async () => {
    const source = await readFile(studioFlowCanvasUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(source, /CanvasFlowSurface/);
    assert.match(source, /mode="studio"/);
    assert.match(source, /batchUpdateNodePositions/);
    assert.match(source, /updateCamera/);
    assert.match(pageSource, /StudioFlowCanvas/);
  });

  it("creates Studio nodes from React Flow screen coordinates", async () => {
    const source = await readFile(canvasSurfaceUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(source, /screenToFlowPosition/);
    assert.match(source, /onPaneContextMenu/);
    assert.match(source, /onCreateNodeAtPoint/);
    assert.match(pageSource, /flowPoint/);
  });

  it("drops uploaded files using a neutral canvas point converter", async () => {
    const source = await readFile(fileDropZoneUrl, "utf8");

    assert.match(source, /screenToCanvasPoint/);
  });

  it("keeps Studio selection synced with Resource Tree state", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(pageSource, /selectedNodeId=\{selectedNodeId\}/);
    assert.match(pageSource, /selectedGroupId=\{selectedGroupId\}/);
    assert.match(pageSource, /selectedEdgeId=\{selectedEdgeId\}/);
    assert.match(pageSource, /onSelectNode=\{\(nodeId\) =>/);
    assert.match(pageSource, /selectOrConnectNode\(nodeId\)/);
    assert.match(pageSource, /hideActiveNodeEditor\(\)/);
    assert.match(pageSource, /onSelectGroup=\{\(groupId\) =>/);
    assert.match(pageSource, /selectGroup\(groupId\)/);
    assert.match(pageSource, /onSelectEdge=\{\(edgeId\) =>/);
    assert.match(pageSource, /selectEdge\(edgeId\)/);
    assert.match(pageSource, /setSelectedEdgeId\(null\)/);
    assert.doesNotMatch(
      surfaceSource,
      /onEdgeClick=\{[\s\S]{0,120}onSelectNode\(null\)/,
    );
    assert.doesNotMatch(
      surfaceSource,
      /node\.type === "group"[\s\S]{0,180}onSelectNode\(null\)/,
    );
  });

  it("wires React Flow edge creation and explicit delete mutations", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const studioSource = await readFile(studioFlowCanvasUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(surfaceSource, /onConnect=\{/);
    assert.match(surfaceSource, /onConnectNodes\?\./);
    assert.match(surfaceSource, /deleteKeyCode=\{null\}/);
    assert.match(studioSource, /onConnectNodes/);
    assert.match(pageSource, /deleteMediaNode/);
    assert.match(pageSource, /deleteNodeById/);
    assert.match(pageSource, /deleteEdgeById/);
  });

  it("moves groups by persisting member node positions", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const studioSource = await readFile(studioFlowCanvasUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");
    const viewModelSource = await readFile(canvasViewModelUrl, "utf8");
    const groupNodeSource = await readFile(groupFlowNodeUrl, "utf8");

    assert.match(surfaceSource, /onNodeDragStart/);
    assert.match(surfaceSource, /onGroupMove\?\./);
    assert.match(surfaceSource, /node\.type === "group"/);
    assert.match(surfaceSource, /node\.type !== "media"/);
    assert.match(studioSource, /onGroupMove/);
    assert.match(viewModelSource, /dragHandle:\s*"\.group-flow-drag-handle"/);
    assert.match(groupNodeSource, /group-flow-drag-handle/);
    assert.match(pageSource, /getGroupMemberMovePositions/);
    assert.match(pageSource, /moveGroupMembers/);
  });

  it("restores group editing and deletion through the shared Studio panel", async () => {
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(pageSource, /replaceMediaGroupNodes/);
    assert.match(pageSource, /updateMediaGroup/);
    assert.match(pageSource, /deleteMediaGroup/);
    assert.match(pageSource, /onAddGroupMember/);
    assert.match(pageSource, /onRemoveGroupMember/);
    assert.match(pageSource, /onDeleteGroup/);
    assert.match(pageSource, /onRenameGroup/);
    assert.match(pageSource, /selectedGroupId=\{selectedGroupId\}/);
    assert.match(pageSource, /selectedEdgeId=\{selectedEdgeId\}/);
  });
});
