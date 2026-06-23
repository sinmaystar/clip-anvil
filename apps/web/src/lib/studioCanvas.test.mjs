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
const mediaFlowNodeUrl = new URL(
  "../components/canvas-flow/MediaFlowNode.tsx",
  import.meta.url,
);
const dependencyFlowEdgeUrl = new URL(
  "../components/canvas-flow/DependencyFlowEdge.tsx",
  import.meta.url,
);
const connectionLinePreviewUrl = new URL(
  "../components/canvas-flow/ConnectionLinePreview.tsx",
  import.meta.url,
);
const mainCssUrl = new URL("../main.css", import.meta.url);
const resourceTreeUrl = new URL(
  "../components/ResourceTree.tsx",
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

  it("keeps Studio canvas creation on context menu and resource tree instead of the old top toolbar", async () => {
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.doesNotMatch(pageSource, /studio-floating-toolbar/);
    assert.doesNotMatch(pageSource, /创建节点工具栏/);
    assert.doesNotMatch(pageSource, /createNodeAtViewportCenter/);
    assert.doesNotMatch(pageSource, /startToolbarConnection/);
    assert.match(pageSource, /onContextMenuCapture=\{openCanvasMenu\}/);
    assert.match(pageSource, /studio-context-menu/);
  });

  it("uses only the Studio production popover when selecting a node", async () => {
    const studioSource = await readFile(studioFlowCanvasUrl, "utf8");
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(studioSource, /renderInspector=\{false\}/);
    assert.match(surfaceSource, /renderInspector/);
    assert.match(pageSource, /node-production-popover/);
  });

  it("does not open the Studio property popover for selected dependency edges", async () => {
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.doesNotMatch(
      pageSource,
      /\(selectedNode \|\| selectedGroup \|\| selectedEdgeId\)/,
    );
    assert.match(pageSource, /\(selectedNode \|\| selectedGroup\)/);
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
    assert.match(pageSource, /selectNode\(nodeId\)/);
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

  it("uses drag-to-connect instead of legacy click-to-connect pending state", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const mediaSource = await readFile(mediaFlowNodeUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");
    const resourceTreeSource = await readFile(resourceTreeUrl, "utf8");

    assert.match(surfaceSource, /connectOnClick=\{false\}/);
    assert.match(surfaceSource, /connectionDragThreshold=\{0\}/);
    assert.match(surfaceSource, /connectionMode=\{ConnectionMode\.Strict\}/);
    assert.match(surfaceSource, /onConnectStart=\{/);
    assert.match(surfaceSource, /onConnectEnd=\{/);
    assert.match(surfaceSource, /connectionState\.fromHandle\?\.nodeId/);
    assert.match(surfaceSource, /document\s*\.\s*elementsFromPoint/);
    assert.match(surfaceSource, /mediaNodeShellFromConnectionPoint/);
    assert.match(surfaceSource, /media-node-shell/);
    assert.match(mediaSource, /media-node-connect-handle/);
    assert.match(mediaSource, /media-node-target-handle/);
    assert.match(mediaSource, /isConnectableStart=\{false\}/);
    assert.match(mediaSource, /isConnectableEnd=\{false\}/);
    assert.doesNotMatch(mediaSource, /media-node-connect-button/);
    assert.doesNotMatch(pageSource, /pendingConnectionRef/);
    assert.doesNotMatch(pageSource, /beginDependencyConnection/);
    assert.doesNotMatch(pageSource, /connectionSourceId/);
    assert.doesNotMatch(pageSource, /clip-anvil:connection-start/);
    assert.doesNotMatch(resourceTreeSource, /onStartConnection/);
    assert.doesNotMatch(resourceTreeSource, /studio-resource-connect/);
  });

  it("anchors dragged dependency lines on the source node edge", async () => {
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(
      cssSource,
      /\.media-node-connect-handle\.react-flow__handle\s*\{[\s\S]*right:\s*-14px;/,
    );
    assert.match(
      cssSource,
      /\.media-node-connect-handle\.react-flow__handle\s*\{[\s\S]*transform:\s*translate\(0,\s*-50%\)/,
    );
    assert.doesNotMatch(cssSource, /right:\s*-16px/);
    assert.doesNotMatch(cssSource, /transform:\s*translate\(5px,\s*-50%\)/);
  });

  it("lets the full target node body receive dragged connections", async () => {
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(
      cssSource,
      /\.media-node-target-handle\.react-flow__handle\s*\{[\s\S]*width:\s*100%;/,
    );
    assert.match(
      cssSource,
      /\.media-node-target-handle\.react-flow__handle\s*\{[\s\S]*height:\s*100%;/,
    );
    assert.match(
      cssSource,
      /\.media-node-target-handle\.react-flow__handle\s*\{[\s\S]*transform:\s*none;/,
    );
    assert.doesNotMatch(cssSource, /media-node-target-handle[\s\S]{0,180}width:\s*1px/);
    assert.doesNotMatch(
      cssSource,
      /media-node-target-handle[\s\S]{0,220}pointer-events:\s*none/,
    );
  });

  it("renders animated source-to-target flow for saved and dragged dependency edges", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const edgeSource = await readFile(dependencyFlowEdgeUrl, "utf8");
    const connectionLineSource = await readFile(connectionLinePreviewUrl, "utf8");

    assert.match(surfaceSource, /connectionLineComponent=\{ConnectionLinePreview\}/);
    assert.match(edgeSource, /connection-overlay-flow/);
    assert.match(edgeSource, /strokeDashoffset/);
    assert.match(connectionLineSource, /connection-overlay-preview-flow/);
    assert.match(connectionLineSource, /getBezierPath/);
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
