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

  it("renders media nodes as lightweight icon title cards instead of type tabs", async () => {
    const mediaSource = await readFile(mediaFlowNodeUrl, "utf8");
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(mediaSource, /media-node-floating-title/);
    assert.match(mediaSource, /media-node-kind-icon/);
    assert.match(mediaSource, /aria-label=\{nodeTypeLabel/);
    assert.match(mediaSource, /naturalWidth/);
    assert.match(mediaSource, /useUpdateNodeInternals/);
    assert.match(mediaSource, /onMediaDimensionsChange/);
    assert.match(surfaceSource, /handleMediaDimensionsChange/);
    assert.match(surfaceSource, /mediaNodeDisplaySize\(node\.data\.node,\s*dimensions\)/);
    assert.doesNotMatch(mediaSource, /media-node-header/);
    assert.doesNotMatch(mediaSource, /media-node-title-row/);
    assert.doesNotMatch(mediaSource, /media-node-status/);
    assert.doesNotMatch(mediaSource, /media-node-icon">\{materialKindLabel/);
    assert.doesNotMatch(cssSource, /\.media-node\[data-status="succeeded"\]\s*\{[\s\S]*border:\s*2px/);
    assert.doesNotMatch(cssSource, /\.media-node-icon\s*\{[\s\S]*border:/);
    assert.doesNotMatch(cssSource, /\.media-node-header/);
    assert.match(cssSource, /\.media-node-floating-title\s*\{[\s\S]*background:\s*transparent;/);
    assert.match(cssSource, /\.media-node-media-frame\s*\{[\s\S]*background:\s*transparent;/);
    assert.match(cssSource, /\.media-node-media-frame img,[\s\S]*object-fit:\s*cover;/);
  });

  it("renders selected node editing as a compact prompt composer with dependency inputs and bottom controls", async () => {
    const propertyPanelSource = await readFile(
      new URL("../components/PropertyPanel.tsx", import.meta.url),
      "utf8",
    );
    const cssSource = await readFile(mainCssUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(propertyPanelSource, /NodeInputStrip/);
    assert.match(propertyPanelSource, /onDeleteInputEdge/);
    assert.match(propertyPanelSource, /node-composer-panel/);
    assert.match(propertyPanelSource, /node-composer-inputs/);
    assert.match(propertyPanelSource, /node-composer-prompt/);
    assert.match(propertyPanelSource, /node-composer-toolbar/);
    assert.match(propertyPanelSource, /node-composer-run-button/);
    assert.match(propertyPanelSource, /NodeComposerDropdown/);
    assert.match(propertyPanelSource, /node-composer-operation-select/);
    assert.match(propertyPanelSource, /node-composer-model-select/);
    assert.match(propertyPanelSource, /ariaLabel="选择生成任务"/);
    assert.match(propertyPanelSource, /ariaLabel="选择模型"/);
    assert.match(propertyPanelSource, /node-composer-dropdown-button/);
    assert.match(propertyPanelSource, /node-composer-dropdown-menu/);
    assert.match(propertyPanelSource, /node-composer-dropdown-root/);
    assert.match(propertyPanelSource, /document\.addEventListener\("pointerdown"[\s\S]*true\)/);
    assert.match(propertyPanelSource, /document\.removeEventListener\([\s\S]*"pointerdown"[\s\S]*true/);
    assert.match(propertyPanelSource, /dropdownRootRef\.current\.contains/);
    assert.match(propertyPanelSource, /node-composer-duration-select/);
    assert.match(propertyPanelSource, /ariaLabel="选择时长"/);
    assert.match(propertyPanelSource, /labelPrefix="时长"/);
    assert.match(propertyPanelSource, /node-composer-temperature-control/);
    assert.match(propertyPanelSource, /aria-label="设置温度"/);
    assert.match(propertyPanelSource, /node-composer-more-button/);
    assert.match(propertyPanelSource, /node-composer-details-popover/);
    assert.match(propertyPanelSource, /detailsPopoverPosition/);
    assert.match(propertyPanelSource, /setPointerCapture/);
    assert.match(propertyPanelSource, /onPointerDown=\{beginDetailsDrag\}/);
    assert.match(propertyPanelSource, /onPointerMove=\{moveDetailsDrag\}/);
    assert.match(propertyPanelSource, /onPointerUp=\{endDetailsDrag\}/);
    assert.match(propertyPanelSource, /data-has-preview/);
    assert.match(propertyPanelSource, /property-input-strip/);
    assert.match(propertyPanelSource, /Versions 与诊断/);
    assert.doesNotMatch(propertyPanelSource, /property-run-footer/);
    assert.doesNotMatch(propertyPanelSource, /node-composer-toggle/);
    assert.doesNotMatch(propertyPanelSource, /node-composer-control-row/);
    assert.doesNotMatch(propertyPanelSource, /<span>Operation<\/span>/);
    assert.doesNotMatch(propertyPanelSource, /<span>Model<\/span>/);
    assert.doesNotMatch(propertyPanelSource, /node-composer-settings-button/);
    assert.doesNotMatch(propertyPanelSource, /node-composer-settings-popover/);
    assert.doesNotMatch(propertyPanelSource, />参数<\/button>/);
    assert.doesNotMatch(propertyPanelSource, /<select[\s\S]{0,240}选择生成任务/);
    assert.doesNotMatch(propertyPanelSource, /<select[\s\S]{0,240}选择模型/);
    assert.doesNotMatch(propertyPanelSource, /mock_fail/);
    assert.doesNotMatch(propertyPanelSource, /<details className="property-section property-more-details node-composer-more">/);
    assert.match(cssSource, /\.node-production-popover\s*\{[\s\S]*max-width:\s*min\(760px/);
    assert.match(cssSource, /\.node-composer-prompt textarea\s*\{[\s\S]*min-height:\s*96px/);
    assert.match(cssSource, /\.node-composer-prompt textarea\s*\{[\s\S]*font-size:\s*14px/);
    assert.match(cssSource, /\.node-composer-toolbar\s*\{[\s\S]*grid-template-columns:\s*minmax\(0,\s*1fr\) auto;/);
    assert.match(cssSource, /\.node-composer-dropdown-button\s*\{[\s\S]*width:\s*max-content;/);
    assert.match(cssSource, /\.node-composer-dropdown-button:focus-visible\s*\{[\s\S]*box-shadow:\s*none;/);
    assert.match(cssSource, /\.node-composer-dropdown-menu\s*\{[\s\S]*width:\s*max-content;/);
    assert.match(cssSource, /\.node-composer-inputs \.property-input-chip\[data-has-preview="false"\]\s*\{[\s\S]*border:\s*1px dashed/);
    assert.match(cssSource, /\.node-composer-run-button\s*\{[\s\S]*width:\s*32px/);
    assert.match(cssSource, /\.node-composer-secondary-button,\n\.node-composer-more-button\s*\{[\s\S]*font-size:\s*12\.5px/);
    assert.match(cssSource, /\.node-composer-panel\s*\{[\s\S]*background:\s*color-mix\(in srgb, var\(--color-panel-elevated\)/);
    assert.match(cssSource, /\.node-composer-details-popover\s*\{[\s\S]*position:\s*fixed;/);
    assert.match(cssSource, /\.node-composer-details-header\s*\{[\s\S]*cursor:\s*grab;/);
    assert.match(cssSource, /\.node-composer-details-popover\s*\{[\s\S]*background:\s*color-mix\(in srgb, var\(--color-panel-elevated\)/);
    assert.doesNotMatch(cssSource, /\.node-composer-panel\s*\{[\s\S]*#272727/);
    assert.match(pageSource, /onDeleteInputEdge=\{deleteEdgeById\}/);
  });

  it("uses the same compact composer language for source material nodes", async () => {
    const propertyPanelSource = await readFile(
      new URL("../components/PropertyPanel.tsx", import.meta.url),
      "utf8",
    );
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(propertyPanelSource, /node-composer-source-panel/);
    assert.match(propertyPanelSource, /source-material-preview-card/);
    assert.match(propertyPanelSource, /source-material-readonly-note/);
    assert.doesNotMatch(propertyPanelSource, /<aside className="property-panel node-production-panel">[\s\S]*这是用户素材节点/);
    assert.match(cssSource, /\.node-composer-source-panel\s*\{[\s\S]*width:\s*min\(520px/);
    assert.match(cssSource, /\.source-material-preview-card\s*\{[\s\S]*border:\s*0;/);
    assert.match(cssSource, /\.source-material-readonly-note\s*\{[\s\S]*border-top:\s*1px solid var\(--border-subtle\)/);
  });

  it("keeps selected node composer below the rendered node even when media display size differs from stored canvas size", async () => {
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(pageSource, /renderedNodeRect/);
    assert.match(pageSource, /querySelector\(\s*`\.react-flow__node\[data-id=/);
    assert.match(pageSource, /nodeBottomY \+ 28/);
    assert.doesNotMatch(pageSource, /top:\s*Math\.round\(bottomLeft\.y \+ 28\)/);
    assert.doesNotMatch(pageSource, /top:\s*Math\.round\(clamp\(bottomLeft\.y \+ 28/);
  });

  it("supports double-click title editing on the floating media node title", async () => {
    const mediaSource = await readFile(mediaFlowNodeUrl, "utf8");
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const studioSource = await readFile(studioFlowCanvasUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(mediaSource, /onDoubleClick=\{/);
    assert.match(mediaSource, /media-node-title-input/);
    assert.match(mediaSource, /onRenameNode/);
    assert.match(surfaceSource, /onRenameNode/);
    assert.match(studioSource, /onRenameNode/);
    assert.match(pageSource, /onRenameNode=\{\(nodeId,\s*title\) =>/);
    assert.match(pageSource, /updateNodeMutation\.mutate\(\{ nodeId, patch: \{ title \} \}\)/);
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
    assert.match(surfaceSource, /onEdgeClick=\{[\s\S]{0,160}onSelectNode\(null\)/);
    assert.match(surfaceSource, /onSelectEdge\(edge\.id\)/);
    assert.doesNotMatch(
      surfaceSource,
      /node\.type === "group"[\s\S]{0,180}onSelectNode\(null\)/,
    );
  });

  it("wires React Flow edge creation and explicit delete mutations", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const studioSource = await readFile(studioFlowCanvasUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");
    const viewModelSource = await readFile(canvasViewModelUrl, "utf8");

    assert.match(surfaceSource, /onConnect=\{/);
    assert.match(surfaceSource, /onConnectNodes\?\./);
    assert.match(surfaceSource, /deleteKeyCode=\{null\}/);
    assert.match(surfaceSource, /onEdgeClick=\{/);
    assert.match(surfaceSource, /edges=\{derivedEdges\}/);
    assert.doesNotMatch(surfaceSource, /const \[edges,\s*setEdges\]/);
    assert.doesNotMatch(surfaceSource, /applyEdgeChanges/);
    assert.doesNotMatch(surfaceSource, /onEdgesChange=\{/);
    assert.doesNotMatch(surfaceSource, /change\.type !== "select"/);
    assert.doesNotMatch(viewModelSource, /selectable:\s*false/);
    assert.match(surfaceSource, /onSelectEdge\(edge\.id\)/);
    assert.match(studioSource, /onConnectNodes/);
    assert.match(pageSource, /deleteMediaNode/);
    assert.match(pageSource, /deleteNodeById/);
    assert.match(pageSource, /deleteEdgeById/);
    assert.match(pageSource, /case "EdgeCreated"/);
    assert.match(pageSource, /canvasEdgeFromEventPayload/);
    assert.match(pageSource, /appendCanvasEdge\(current,\s*edge\)/);
    assert.match(pageSource, /case "EdgeDeleted"/);
    assert.match(pageSource, /removeCanvasEdge\(current,\s*edgeId\)/);
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
    const apiSource = await readFile(new URL("../lib/api.ts", import.meta.url), "utf8");
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(surfaceSource, /connectionLineComponent=\{ConnectionLinePreview\}/);
    assert.match(surfaceSource, /boundaryAnchorFromFlowPoint/);
    assert.match(surfaceSource, /metadata:\s*targetAnchor\s*\?/);
    assert.match(surfaceSource, /anchors:\s*\{\s*target:\s*targetAnchor/s);
    assert.match(apiSource, /interface EdgeMetadata/);
    assert.match(edgeSource, /connection-overlay-flow/);
    assert.match(edgeSource, /pathLength=\{1\}/);
    assert.doesNotMatch(edgeSource, /strokeDashoffset/);
    assert.match(edgeSource, /useReactFlow/);
    assert.match(edgeSource, /sourcePosition:\s*Position\.Right/);
    assert.match(edgeSource, /targetPosition:\s*Position\.Left/);
    assert.match(edgeSource, /sourceX:\s*sourceRect\.x\s*\+\s*sourceRect\.w/);
    assert.match(edgeSource, /sourceY:\s*sourceRect\.y\s*\+\s*sourceRect\.h\s*\/\s*2/);
    assert.match(edgeSource, /targetX:\s*targetRect\.x/);
    assert.match(edgeSource, /targetY:\s*targetRect\.y\s*\+\s*targetRect\.h\s*\/\s*2/);
    assert.doesNotMatch(edgeSource, /metadata\?\.anchors\?\.target/);
    assert.doesNotMatch(edgeSource, /positionForAnchor/);
    assert.match(connectionLineSource, /connection-overlay-preview-flow/);
    assert.match(connectionLineSource, /pathLength=\{1\}/);
    assert.match(connectionLineSource, /getBezierPath/);
    assert.match(connectionLineSource, /useStore/);
    assert.match(connectionLineSource, /pointerToFlowPoint/);
    assert.match(connectionLineSource, /targetX:\s*targetPoint\.x/);
    assert.match(connectionLineSource, /targetY:\s*targetPoint\.y/);
    assert.doesNotMatch(connectionLineSource, /targetX:\s*pointer\.x/);
    assert.doesNotMatch(connectionLineSource, /targetY:\s*pointer\.y/);
    assert.match(cssSource, /\.connection-overlay-flow,[\s\S]*stroke-dasharray:\s*0\.075 1\.025/);
    assert.match(cssSource, /@keyframes connection-flow[\s\S]*stroke-dashoffset:\s*-1\.1/);
    assert.doesNotMatch(cssSource, /stroke-dasharray:\s*18 84/);
  });

  it("keeps React Flow controls readable and adds a canvas minimap", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(surfaceSource, /MiniMap/);
    assert.match(surfaceSource, /className="canvas-flow-minimap"/);
    assert.match(cssSource, /\.canvas-flow-surface \.react-flow__controls\s*\{[\s\S]*background:\s*color-mix\(in srgb, var\(--color-panel-elevated\)/);
    assert.match(cssSource, /\.canvas-flow-surface \.react-flow__controls button\s*\{[\s\S]*color:\s*var\(--fg-primary\)/);
    assert.match(cssSource, /\.canvas-flow-minimap\s*\{[\s\S]*background:\s*color-mix\(in srgb, var\(--color-panel-elevated\)/);
  });

  it("creates ordinary Studio nodes with generation operations by default", async () => {
    const pageSource = await readFile(workspacePageUrl, "utf8");

    assert.match(pageSource, /defaultOperationForNode/);
    assert.match(pageSource, /const operationType\s*=/);
    assert.match(pageSource, /input\?\.patch\?\.operation_type\s*\?\?/);
    assert.match(pageSource, /defaultOperationForNode\(\{\s*node_type:\s*nodeType/s);
    assert.match(pageSource, /operation_type:\s*operationType/);
    assert.match(pageSource, /node_type:\s*nodeType/);
    assert.doesNotMatch(
      pageSource,
      /operation_type:\s*input\?\.patch\?\.operation_type,\s*\n/,
    );
  });

  it("moves groups by persisting member node positions", async () => {
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");
    const studioSource = await readFile(studioFlowCanvasUrl, "utf8");
    const pageSource = await readFile(workspacePageUrl, "utf8");
    const viewModelSource = await readFile(canvasViewModelUrl, "utf8");
    const groupNodeSource = await readFile(groupFlowNodeUrl, "utf8");

    assert.match(surfaceSource, /onNodeDragStart/);
    assert.match(surfaceSource, /syncGroupNodeLayout/);
    assert.match(surfaceSource, /applyGroupDragDelta/);
    assert.match(surfaceSource, /change\.dragging/);
    assert.match(surfaceSource, /onGroupMove\?\./);
    assert.match(surfaceSource, /node\.type === "group"/);
    assert.match(surfaceSource, /node\.type !== "media"/);
    assert.match(studioSource, /onGroupMove/);
    assert.match(viewModelSource, /groupToFlowNode/);
    assert.match(viewModelSource, /dragHandle:\s*"\.group-flow-drag-handle"/);
    assert.match(groupNodeSource, /group-flow-drag-handle/);
    assert.match(pageSource, /getGroupMemberMovePositions/);
    assert.match(pageSource, /moveGroupMembers/);
  });

  it("restores group editing and deletion through the shared Studio panel", async () => {
    const propertyPanelSource = await readFile(
      new URL("../components/PropertyPanel.tsx", import.meta.url),
      "utf8",
    );
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
    assert.match(pageSource, /blurEditableTargetBeforeHide/);
    assert.match(propertyPanelSource, /commitGroupRename/);
    assert.match(propertyPanelSource, /event\.key === "Enter"/);
    assert.match(propertyPanelSource, /event\.currentTarget\.blur\(\)/);
  });
});
