import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { URL } from "node:url";

const agentWorkbenchCanvasUrl = new URL(
  "../components/agent-workbench/AgentWorkbenchCanvas.tsx",
  import.meta.url,
);
const agentCanvasDetailPanelUrl = new URL(
  "../components/agent-workbench/AgentCanvasDetailPanel.tsx",
  import.meta.url,
);
const agentShotNodeUrl = new URL(
  "../components/agent-workbench/AgentShotNode.tsx",
  import.meta.url,
);
const agentWorkbenchViewModelUrl = new URL(
  "./agentWorkbenchViewModel.ts",
  import.meta.url,
);
const agentApiUrl = new URL("./agentApi.ts", import.meta.url);
const agentPageUrl = new URL(
  "../pages/AgentWorkspacePage.tsx",
  import.meta.url,
);
const canvasSurfaceUrl = new URL(
  "../components/canvas-flow/CanvasFlowSurface.tsx",
  import.meta.url,
);
const policyUrl = new URL(
  "../components/canvas-flow/flowModePolicy.ts",
  import.meta.url,
);
const guardUrl = new URL(
  "../../../server/internal/api/workspace_mode_guard.go",
  import.meta.url,
);
const canvasHandlerUrl = new URL(
  "../../../server/internal/api/canvas_handler.go",
  import.meta.url,
);
const nodeHandlerUrl = new URL(
  "../../../server/internal/api/node_handler.go",
  import.meta.url,
);
const smokeScriptUrl = new URL(
  "../../../../scripts/smoke-react-flow-canvas.sh",
  import.meta.url,
);
const mainCssUrl = new URL("../main.css", import.meta.url);

describe("agent React Flow canvas", () => {
  it("renders Agent through the grouped Workbench canvas by default", async () => {
    const source = await readFile(agentWorkbenchCanvasUrl, "utf8");
    const viewModelSource = await readFile(agentWorkbenchViewModelUrl, "utf8");
    const pageSource = await readFile(agentPageUrl, "utf8");
    const apiSource = await readFile(agentApiUrl, "utf8");

    assert.match(source, /ReactFlowProvider/);
    assert.match(source, /nodeTypes/);
    assert.match(source, /agentOverview/);
    assert.match(source, /agentScene/);
    assert.match(source, /agentShot/);
    assert.match(viewModelSource, /parentId:\s*currentSceneNodeId/);
    assert.match(viewModelSource, /extent:\s*"parent"/);
    assert.match(viewModelSource, /agentWorkbenchToFlow/);
    assert.match(pageSource, /AgentWorkbenchCanvas/);
    assert.match(pageSource, /fetchAgentCanvasWorkbench/);
    assert.match(pageSource, /\["workspace", id, "agent-workbench"\]/);
    assert.match(
      apiSource,
      /\/agent\/workspaces\/\$\{workspaceId\}\/canvas\/workbench/,
    );
    assert.doesNotMatch(pageSource, /AgentFlowCanvas/);
    assert.doesNotMatch(pageSource, /AgentReadonlyCanvas/);
    assert.doesNotMatch(pageSource, /AgentNodeDetailDrawer/);
  });

  it("uses one visible Agent canvas title", async () => {
    const pageSource = await readFile(agentPageUrl, "utf8");

    assert.match(
      pageSource,
      /<p className="workspace-kicker">Agent Canvas<\/p>/,
    );
    assert.doesNotMatch(pageSource, /<h2>Agent 画布<\/h2>/);
  });

  it("uses selected Workbench detail objects instead of the old property panel", async () => {
    const pageSource = await readFile(agentPageUrl, "utf8");
    const source = await readFile(agentWorkbenchCanvasUrl, "utf8");
    const apiSource = await readFile(agentApiUrl, "utf8");
    const detailPanelSource = await readFile(agentCanvasDetailPanelUrl, "utf8");

    assert.match(pageSource, /selectedWorkbenchSelection/);
    assert.match(pageSource, /setSelectedWorkbenchSelection/);
    assert.match(pageSource, /AgentCanvasDetailPanel/);
    assert.match(pageSource, /fetchAgentCanvasDetail/);
    assert.match(pageSource, /shouldClearAgentWorkbenchSelection/);
    assert.match(source, /selectionForNode/);
    assert.match(source, /AgentWorkbenchSelectionProvider/);
    assert.match(source, /onPaneClick=\{\(\) => onSelectObject\(null\)\}/);
    assert.match(
      apiSource,
      /\/agent\/workspaces\/\$\{workspaceId\}\/canvas\/details/,
    );
    assert.match(detailPanelSource, /RenderPlan/);
    assert.match(detailPanelSource, /RubricGrid/);
    assert.match(detailPanelSource, /agent-canvas-detail-body/);
    assert.doesNotMatch(pageSource, /selectedWorkbenchObjectId/);
    assert.doesNotMatch(pageSource, /setSelectedWorkbenchObjectId/);
    assert.doesNotMatch(pageSource, /agentWorkbenchOverviewSelection/);
    assert.doesNotMatch(pageSource, /PropertyPanel/);
    assert.doesNotMatch(detailPanelSource, /PropertyPanel/);
    assert.doesNotMatch(pageSource, /agent-node-production-popover/);
    assert.doesNotMatch(pageSource, /selectedNodeProductionStateQuery/);
    assert.doesNotMatch(pageSource, /fetchModelCapabilities/);
    assert.doesNotMatch(pageSource, /fetchReferencePackItems/);
    assert.match(pageSource, /preserveCanvasAssetUrls/);
  });

  it("keeps Agent layout interactive while blocking edit and execution capabilities", async () => {
    const policySource = await readFile(policyUrl, "utf8");
    const surfaceSource = await readFile(canvasSurfaceUrl, "utf8");

    assert.match(policySource, /canDragNodes:\s*true/);
    assert.match(policySource, /canPersistViewport:\s*true/);
    assert.match(policySource, /canCreateNodes:\s*false/);
    assert.match(policySource, /canDeleteNodes:\s*false/);
    assert.match(policySource, /canCreateEdges:\s*false/);
    assert.match(policySource, /canDeleteEdges:\s*false/);
    assert.match(policySource, /canRunNodes:\s*false/);
    assert.match(surfaceSource, /nodesDraggable=\{policy\.canDragNodes\}/);
    assert.match(surfaceSource, /nodesConnectable=\{policy\.canCreateEdges\}/);
    assert.match(surfaceSource, /deleteKeyCode=\{null\}/);
  });

  it("persists Agent node drag layout and viewport through explicit layout APIs", async () => {
    const source = await readFile(canvasSurfaceUrl, "utf8");
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(source, /onNodeDragStop/);
    assert.match(source, /onNodePositionsChange/);
    assert.match(source, /change\.type !== "position"/);
    assert.match(source, /settledPositions/);
    assert.match(source, /onMoveEnd/);
    assert.match(source, /onViewportChange/);
    assert.match(source, /<Controls position="bottom-right"/);
    assert.match(source, /<MiniMap[\s\S]*position="bottom-left"/);
    assert.match(
      cssSource,
      /\.canvas-flow-surface \.react-flow[\s\S]*min-height:\s*420px/,
    );
  });

  it("lets backend Agent workspaces write only canvas layout endpoints", async () => {
    const guardSource = await readFile(guardUrl, "utf8");
    const canvasHandlerSource = await readFile(canvasHandlerUrl, "utf8");
    const nodeHandlerSource = await readFile(nodeHandlerUrl, "utf8");

    assert.match(guardSource, /requireCanvasLayoutWorkspace/);
    assert.match(guardSource, /WorkspaceModeAgent/);
    assert.match(canvasHandlerSource, /requireCanvasLayoutWorkspace/);
    assert.match(
      nodeHandlerSource,
      /BatchUpdatePosition[\s\S]*requireCanvasLayoutWorkspace/,
    );
    assert.match(nodeHandlerSource, /Create[\s\S]*requireStudioWorkspace/);
    assert.match(nodeHandlerSource, /Update[\s\S]*requireStudioWorkspace/);
    assert.match(nodeHandlerSource, /Delete[\s\S]*requireStudioWorkspace/);
  });

  it("seeds Agent E2E with an Agent-owned node without opening public node creation", async () => {
    const source = await readFile(smokeScriptUrl, "utf8");

    assert.match(source, /\/agent\/workspaces\/\$\{agent\.id\}\/attachments/);
    assert.match(source, /FormData/);
    assert.match(source, /Agent Layout Brief/);
    assert.match(source, /agentCanvas\.nodes\.length !== 1/);
    assert.doesNotMatch(source, /createNode\(agent/);
  });

  it("keeps existing Agent websocket canvas refresh behavior", async () => {
    const source = await readFile(agentPageUrl, "utf8");

    assert.match(source, /case "NodeCreated"/);
    assert.match(source, /case "NodeUpdated"/);
    assert.match(source, /queryClient\.setQueryData<CanvasPayload>/);
    assert.match(source, /upsertCanvasNode\(current, node\)/);
    assert.match(source, /\["workspace", id, "agent-workbench"\]/);
    assert.match(source, /nodeStatusForGenerationStatus/);
    assert.match(source, /updateCanvasNodeStatus/);
    assert.match(source, /isTerminalGenerationStatus/);
    assert.match(
      source,
      /status\s*===\s*"connected"[\s\S]{0,80}refreshCanvas\(\)/,
    );
  });

  it("styles the Agent Workbench as grouped scene and shot cards", async () => {
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(cssSource, /\.agent-workbench-surface/);
    assert.match(cssSource, /\.agent-workbench-scene-node/);
    assert.match(cssSource, /\.agent-workbench-shot-node/);
    assert.match(cssSource, /\.agent-workbench-shot-media-card/);
    assert.match(cssSource, /\.agent-workbench-shot-status-row/);
    assert.match(cssSource, /\.agent-workbench-edge/);
  });

  it("organizes Shot details as script, dependencies, and output drilldown cards", async () => {
    const detailSource = await readFile(agentCanvasDetailPanelUrl, "utf8");
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(detailSource, /function ShotDetail/);
    assert.match(detailSource, /title="分镜脚本"/);
    assert.match(detailSource, /title="引用依赖"/);
    assert.match(detailSource, /title="生成产物"/);
    assert.match(detailSource, /agent-canvas-output-card/);
    assert.match(detailSource, /objectType:\s*"artifact"/);
    assert.doesNotMatch(detailSource, /<DetailSection title="生产状态">/);
    assert.match(detailSource, /outputCardPreviewStyle/);
    assert.match(detailSource, /onLoadedMetadata/);
    assert.match(detailSource, /videoWidth/);
    assert.match(detailSource, /videoHeight/);
    assert.match(cssSource, /\.agent-canvas-output-grid/);
    assert.match(cssSource, /\.agent-canvas-output-card/);
    assert.doesNotMatch(
      cssSource,
      /\.agent-canvas-output-card-preview\s*\{[^}]*aspect-ratio:\s*16\s*\/\s*9/,
    );
  });

  it("does not render a broken image preview for audio artifact details", async () => {
    const detailSource = await readFile(agentCanvasDetailPanelUrl, "utf8");

    assert.match(detailSource, /artifactVisualPreview/);
    assert.match(detailSource, /const visualPreview = artifactVisualPreview\(artifact\)/);
    assert.match(detailSource, /visualPreview \? \(/);
    assert.doesNotMatch(
      detailSource,
      /artifact\.node\.node_type === "video" \? \([\s\S]*?\) : \(\s*<img/,
    );
    assert.match(detailSource, /mime\.startsWith\("image\/"\)/);
    assert.match(detailSource, /mime\.startsWith\("video\/"\)/);
    assert.match(detailSource, /return null/);
  });

  it("renders Agent shots as modern media cards instead of dense artifact grids", async () => {
    const shotSource = await readFile(agentShotNodeUrl, "utf8");
    const canvasSource = await readFile(agentWorkbenchCanvasUrl, "utf8");
    const viewModelSource = await readFile(agentWorkbenchViewModelUrl, "utf8");
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(shotSource, /agent-workbench-shot-media-card/);
    assert.match(shotSource, /agent-workbench-shot-media-stack/);
    assert.match(shotSource, /visibleMediaSlots/);
    assert.match(shotSource, /agent-workbench-shot-status-row/);
    assert.doesNotMatch(shotSource, /agent-workbench-shot-play-badge/);
    assert.doesNotMatch(shotSource, /agent-workbench-shot-grid/);
    assert.match(cssSource, /\.agent-workbench-shot-media-card/);
    assert.match(shotSource, /agentWorkbenchMediaSize/);
    assert.match(shotSource, /style=\{mediaSlotStyle\(slot,\s*measuredDimensions\)\}/);
    assert.match(shotSource, /naturalWidth/);
    assert.match(shotSource, /onMediaDimensionsChange/);
    assert.match(shotSource, /ResizeObserver/);
    assert.match(shotSource, /scrollHeight/);
    assert.match(shotSource, /onShotHeightChange/);
    assert.match(canvasSource, /mediaDimensions/);
    assert.match(canvasSource, /setMediaDimensions/);
    assert.match(canvasSource, /shotHeights/);
    assert.match(canvasSource, /handleShotHeightChange/);
    assert.doesNotMatch(
      cssSource,
      /\.agent-workbench-shot-media-button\s*\{[^}]*aspect-ratio:\s*16\s*\/\s*9/,
    );
    assert.match(
      cssSource,
      /\.agent-workbench-shot-media-card\s*\{[^}]*background:\s*transparent;/,
    );
    assert.match(shotSource, /videoWidth/);
    assert.match(shotSource, /videoHeight/);
    assert.match(viewModelSource, /height:\s*layout\.height/);
    assert.match(viewModelSource, /style:\s*\{\s*width:\s*SHOT_WIDTH,\s*height:\s*layout\.height\s*\}/);
    assert.match(cssSource, /grid-template-rows:\s*auto auto auto auto;/);
    assert.match(
      cssSource,
      /\.agent-workbench-shot-media-button img,\s*\.agent-workbench-shot-media-button video\s*\{[^}]*object-fit:\s*contain;/,
    );
  });

  it("keeps Agent Workbench cards readable instead of fitting the full board", async () => {
    const source = await readFile(agentWorkbenchCanvasUrl, "utf8");

    assert.match(source, /defaultViewport=\{\{ x: 24, y: 24, zoom: 0\.78 \}\}/);
    assert.doesNotMatch(source, /\sfitView\s/);
  });

  it("does not mount the old production overview panels in the Agent chat", async () => {
    const pageSource = await readFile(agentPageUrl, "utf8");

    assert.doesNotMatch(pageSource, /AgentProductionStatusBar/);
    assert.doesNotMatch(pageSource, /AgentStoryboardPanel/);
    assert.doesNotMatch(pageSource, /AgentTaskTimeline/);
    assert.doesNotMatch(pageSource, /fetchAgentProductionOverview/);
    assert.doesNotMatch(pageSource, /agent-production-overview-stack/);
    assert.doesNotMatch(pageSource, /shouldRefreshAgentProductionOverview/);
    assert.doesNotMatch(pageSource, /agentComposerDisabledReason/);
    assert.doesNotMatch(pageSource, /agent-chat-hint/);
  });

  it("keeps the Agent chat composer out of the flexible grid row", async () => {
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(
      cssSource,
      /\.agent-chat-float\s*\{[\s\S]*grid-template-rows:\s*auto minmax\(96px,\s*1fr\) auto;/,
    );
    assert.doesNotMatch(
      cssSource,
      /\.agent-chat-float\s*\{[\s\S]*grid-template-rows:\s*auto auto minmax\(96px,\s*1fr\) auto;/,
    );
  });
});
