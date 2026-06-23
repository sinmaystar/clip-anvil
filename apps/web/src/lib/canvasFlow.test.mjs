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

  it("keeps React Flow types independent from tldraw shape concepts", async () => {
    const source = await readCanvasFlowSource("flowTypes.ts");

    assert.match(source, /import type \{ Edge, Node \} from "@xyflow\/react"/);
    assert.match(source, /CanvasFlowMode = "studio" \| "agent"/);
    assert.match(source, /kind: "media"/);
    assert.match(source, /kind: "group"/);
    assert.doesNotMatch(source, /TLRecord|TLShape|ShapeUtil|createShapeId/);
  });

  it("maps canvas payload into React Flow nodes and edges from business ids", async () => {
    const source = await readCanvasFlowSource("canvasViewModel.ts");

    assert.match(source, /id:\s*node\.id/);
    assert.match(source, /position:\s*\{\s*x:\s*node\.canvas_x,\s*y:\s*node\.canvas_y\s*\}/s);
    assert.match(source, /id:\s*group\.id/);
    assert.match(source, /source:\s*edge\.from_node_id/);
    assert.match(source, /target:\s*edge\.to_node_id/);
    assert.match(source, /mediaNodeDisplaySize/);
    assert.doesNotMatch(source, /TLRecord|TLShape|ShapeUtil|createShapeId/);
  });

  it("round-trips backend camera through React Flow viewport shape", async () => {
    const source = await readCanvasFlowSource("canvasViewport.ts");

    assert.match(source, /cameraToViewport/);
    assert.match(source, /viewportToCamera/);
    assert.match(source, /x:\s*camera\.x/);
    assert.match(source, /zoom:\s*viewport\.zoom/);
  });
});
