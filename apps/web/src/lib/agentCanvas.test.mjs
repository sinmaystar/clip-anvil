import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { URL } from "node:url";

const agentFlowCanvasUrl = new URL(
  "../components/canvas-flow/AgentFlowCanvas.tsx",
  import.meta.url,
);
const agentPageUrl = new URL("../pages/AgentWorkspacePage.tsx", import.meta.url);
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
  it("renders Agent through the shared React Flow canvas surface", async () => {
    const source = await readFile(agentFlowCanvasUrl, "utf8");
    const pageSource = await readFile(agentPageUrl, "utf8");

    assert.match(source, /CanvasFlowSurface/);
    assert.match(source, /mode="agent"/);
    assert.match(source, /batchUpdateNodePositions/);
    assert.match(source, /updateCamera/);
    assert.match(pageSource, /AgentFlowCanvas/);
    assert.doesNotMatch(pageSource, /AgentReadonlyCanvas/);
    assert.doesNotMatch(pageSource, /AgentNodeDetailDrawer/);
    assert.doesNotMatch(source, /from "tldraw"|from 'tldraw'|Tldraw|createTLStore/);
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
    assert.match(surfaceSource, /deleteKeyCode=\{\s*policy\.canDeleteNodes/);
  });

  it("persists Agent node drag layout and viewport through explicit layout APIs", async () => {
    const source = await readFile(canvasSurfaceUrl, "utf8");
    const agentSource = await readFile(agentFlowCanvasUrl, "utf8");
    const cssSource = await readFile(mainCssUrl, "utf8");

    assert.match(source, /onNodeDragStop/);
    assert.match(source, /onNodePositionsChange/);
    assert.match(source, /change\.type !== "position"/);
    assert.match(source, /settledPositions/);
    assert.match(source, /onMoveEnd/);
    assert.match(source, /onViewportChange/);
    assert.match(agentSource, /batchUpdateNodePositions/);
    assert.match(agentSource, /updateCamera/);
    assert.match(agentSource, /queryClient\.setQueryData<CanvasPayload>/);
    assert.match(cssSource, /\.canvas-flow-surface \.react-flow[\s\S]*min-height:\s*420px/);
  });

  it("lets backend Agent workspaces write only canvas layout endpoints", async () => {
    const guardSource = await readFile(guardUrl, "utf8");
    const canvasHandlerSource = await readFile(canvasHandlerUrl, "utf8");
    const nodeHandlerSource = await readFile(nodeHandlerUrl, "utf8");

    assert.match(guardSource, /requireCanvasLayoutWorkspace/);
    assert.match(guardSource, /WorkspaceModeAgent/);
    assert.match(canvasHandlerSource, /requireCanvasLayoutWorkspace/);
    assert.match(nodeHandlerSource, /BatchUpdatePosition[\s\S]*requireCanvasLayoutWorkspace/);
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
    assert.match(source, /nodeStatusForGenerationStatus/);
    assert.match(source, /updateCanvasNodeStatus/);
    assert.match(source, /isTerminalGenerationStatus/);
    assert.match(source, /status\s*===\s*"connected"[\s\S]{0,80}refreshCanvas\(\)/);
  });
});
