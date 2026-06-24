import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { URL } from "node:url";

async function readCanvasFlowSource(fileName) {
  return readFile(
    new URL(`../components/canvas-flow/${fileName}`, import.meta.url),
    "utf8",
  );
}

describe("React Flow canvas foundation", () => {
  it("defines Studio and Agent mode policies without making Agent a weak readonly canvas", async () => {
    const source = await readCanvasFlowSource("flowModePolicy.ts");

    assert.match(source, /canPanZoom:\s*true/);
    assert.match(source, /canSelect:\s*true/);
    assert.match(source, /canDragNodes:\s*true/);
    assert.match(source, /canPersistViewport:\s*true/);
    assert.match(source, /canCreateNodes:\s*false/);
    assert.match(source, /canCreateEdges:\s*false/);
    assert.match(source, /canDeleteNodes:\s*false/);
    assert.match(source, /canRunNodes:\s*false/);
    assert.match(source, /policyForCanvasMode/);
  });

  it("keeps React Flow types independent from legacy canvas record concepts", async () => {
    const source = await readCanvasFlowSource("flowTypes.ts");

    assert.match(source, /import type \{ Edge, Node \} from "@xyflow\/react"/);
    assert.match(source, /CanvasFlowMode = "studio" \| "agent"/);
    assert.match(source, /kind: "media"/);
    assert.match(source, /kind: "group"/);
  });

  it("maps canvas payload into React Flow nodes and edges from business ids", async () => {
    const source = await readCanvasFlowSource("canvasViewModel.ts");

    assert.match(source, /id:\s*node\.id/);
    assert.match(source, /position:\s*\{\s*x:\s*node\.canvas_x,\s*y:\s*node\.canvas_y\s*\}/s);
    assert.match(source, /id:\s*group\.id/);
    assert.match(source, /source:\s*edge\.from_node_id/);
    assert.match(source, /target:\s*edge\.to_node_id/);
    assert.match(source, /mediaNodeDisplaySize/);
  });

  it("round-trips backend camera through React Flow viewport shape", async () => {
    const source = await readCanvasFlowSource("canvasViewport.ts");

    assert.match(source, /cameraToViewport/);
    assert.match(source, /viewportToCamera/);
    assert.match(source, /x:\s*camera\.x/);
    assert.match(source, /zoom:\s*viewport\.zoom/);
  });

  it("renders Studio and Agent through one shared React Flow surface", async () => {
    const source = await readCanvasFlowSource("CanvasFlowSurface.tsx");

    assert.match(source, /ReactFlow/);
    assert.match(source, /canvasToFlowNodes/);
    assert.match(source, /canvasToFlowEdges/);
    assert.match(source, /policyForCanvasMode/);
    assert.match(source, /nodesDraggable=\{policy\.canDragNodes\}/);
    assert.match(source, /nodesConnectable=\{policy\.canCreateEdges\}/);
    assert.match(source, /nodeTypes/);
    assert.match(source, /edgeTypes/);
  });

  it("keeps shared media nodes policy-driven instead of Agent-specific", async () => {
    const source = await readCanvasFlowSource("MediaFlowNode.tsx");

    assert.match(source, /CanvasFlowPolicy/);
    assert.match(source, /media-node/);
    assert.match(source, /Handle/);
    assert.match(source, /policy\.canCreateEdges/);
    assert.doesNotMatch(source, /mode === "agent"/);
  });

  it("uses a shared inspector while honoring policy-gated actions", async () => {
    const source = await readCanvasFlowSource("NodeInspectorPopover.tsx");

    assert.match(source, /NodeInspectorPopoverProps/);
    assert.match(source, /mode:\s*CanvasFlowMode/);
    assert.match(source, /policy:\s*CanvasFlowPolicy/);
    assert.match(source, /onRunNode/);
    assert.match(source, /policy\.canRunNodes/);
    assert.match(source, /policy\.canEditNodeContent/);
    assert.doesNotMatch(source, /AgentReadonly/);
  });

  it("defines group and dependency edge renderers for the shared surface", async () => {
    const groupSource = await readCanvasFlowSource("GroupFlowNode.tsx");
    const edgeSource = await readCanvasFlowSource("DependencyFlowEdge.tsx");

    assert.match(groupSource, /group-flow-node/);
    assert.match(groupSource, /nodeCount/);
    assert.match(edgeSource, /BaseEdge/);
    assert.match(edgeSource, /getBezierPath/);
    assert.match(edgeSource, /dependency-flow-edge/);
  });
});
