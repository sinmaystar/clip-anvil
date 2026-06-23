import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { describe, it } from "node:test";
import { URL } from "node:url";

describe("agent readonly canvas", () => {
  it("reuses studio shape conversion and tldraw rendering", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /nodeToShape/);
    assert.match(source, /groupToShape/);
    assert.match(source, /edgeToArrow/);
    assert.match(source, /MediaShapeUtil/);
    assert.match(source, /Tldraw/);
    assert.match(source, /import "tldraw\/tldraw\.css"/);
  });

  it("locks readonly canvas shapes to prevent local editing drag state", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /isLocked: true/);
  });

  it("uses tldraw readonly store and a no-op tool instead of draggable tools", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /createTLStore/);
    assert.match(source, /defaultShapeUtils/);
    assert.match(source, /defaultBindingUtils/);
    assert.match(source, /defaultAssetUtils/);
    assert.match(source, /collaboration/);
    assert.match(source, /readonly/);
    assert.match(source, /class AgentReadonlyTool extends StateNode/);
    assert.match(source, /static override id = "agent_readonly"/);
    assert.match(source, /const readonlyTldrawTools = \[AgentReadonlyTool\] as const/);
    assert.match(source, /tools=\{readonlyTldrawTools\}/);
    assert.match(source, /initialState="agent_readonly"/);
    assert.match(source, /setCurrentTool\("agent_readonly"\)/);
    assert.match(source, /edgeScrollSpeed:\s*0/);
    assert.match(source, /stopReadonlyNodeTldrawGesture/);
    assert.match(source, /onPointerDownCapture=\{stopReadonlyNodeTldrawGesture\}/);
    assert.doesNotMatch(source, /initialState="hand"/);
    assert.doesNotMatch(source, /setCurrentTool\("hand"\)/);
    assert.doesNotMatch(source, /setCurrentTool\("select"\)/);
  });

  it("uses a lightweight readonly media shape instead of the full studio media shape", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );
    const readonlyShapeSource = await readFile(
      new URL(
        "../shapes/AgentReadonlyMediaShapeUtil.tsx",
        import.meta.url,
      ),
      "utf8",
    );

    assert.match(source, /AgentReadonlyMediaShapeUtil/);
    assert.doesNotMatch(source, /from "\.\.\/\.\.\/shapes\/MediaShapeUtil"/);
    assert.match(readonlyShapeSource, /decoding="async"/);
    assert.match(readonlyShapeSource, /loading="lazy"/);
    assert.match(readonlyShapeSource, /draggable=\{false\}/);
    assert.match(readonlyShapeSource, /onPointerDownCapture/);
    assert.match(readonlyShapeSource, /stopImmediatePropagation/);
    assert.doesNotMatch(readonlyShapeSource, /media-node-connect-button/);
    assert.doesNotMatch(readonlyShapeSource, /media-node-expand-button/);
  });

  it("temporarily opens the readonly store only while applying remote canvas snapshots", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /withReadonlyStoreWrite/);
    assert.match(source, /readonlyCollaborationMode\.set\("readwrite"\)/);
    assert.match(source, /readonlyCollaborationMode\.set\("readonly"\)/);
  });

  it("agent workspace no longer renders text-only node cards", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.doesNotMatch(source, /agent-node-card/);
    assert.match(source, /AgentReadonlyCanvas/);
  });

  it("canvas interactions do not collapse the floating chat panel", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.doesNotMatch(
      source,
      /className="agent-canvas-surface"[\s\S]{0,120}onPointerDown=\{collapseFromCanvas\}/,
    );
  });

  it("pins the readonly tldraw canvas to the viewport instead of min-content growth", async () => {
    const css = await readFile(new URL("../main.css", import.meta.url), "utf8");

    assert.match(
      css,
      /\.agent-workspace-shell\s*{[\s\S]*height:\s*100vh[\s\S]*overflow:\s*hidden/,
    );
    assert.match(
      css,
      /\.agent-workspace-shell\s*{[\s\S]*grid-template-rows:\s*auto minmax\(0,\s*1fr\)/,
    );
    assert.match(
      css,
      /\.agent-readonly-canvas\s*{[\s\S]*height:\s*100%[\s\S]*grid-template-rows:\s*auto minmax\(0,\s*1fr\)/,
    );
    assert.match(
      css,
      /\.agent-canvas-surface\s*{[\s\S]*height:\s*100%[\s\S]*min-height:\s*0/,
    );
    assert.match(
      css,
      /\.agent-readonly-tldraw\s*{[\s\S]*height:\s*100%[\s\S]*overflow:\s*hidden/,
    );
    assert.doesNotMatch(
      css,
      /\.agent-readonly-tldraw\s*{[^}]*min-height:\s*460px/,
    );
    assert.match(
      css,
      /\.agent-readonly-tldraw\s+\.tl-container\s*{[\s\S]*position:\s*absolute[\s\S]*height:\s*100%/,
    );
    assert.match(
      css,
      /\.agent-readonly-tldraw\s+\.tl-canvas\s*{[\s\S]*position:\s*absolute[\s\S]*height:\s*100%[\s\S]*overflow:\s*clip[\s\S]*contain:\s*strict/,
    );
    assert.match(
      css,
      /\.agent-readonly-tldraw\s+\.tl-canvas-overlays\s*{[\s\S]*position:\s*absolute[\s\S]*pointer-events:\s*none/,
    );
  });

  it("node detail drawer exposes production information without edit controls", async () => {
    const source = await readFile(
      new URL(
        "../components/agent/AgentNodeDetailDrawer.tsx",
        import.meta.url,
      ),
      "utf8",
    );

    assert.match(source, /Prompt/);
    assert.match(source, /Model/);
    assert.match(source, /Versions/);
    assert.match(source, /Status/);
    assert.match(source, /Shot ID/);
    assert.match(source, /shot_id/);
    assert.doesNotMatch(source, /onUpdateNode/);
    assert.doesNotMatch(source, /textarea/);
  });

  it("merges websocket node snapshots into the agent canvas cache", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /case "NodeUpdated"/);
    assert.match(source, /queryClient\.setQueryData<CanvasPayload>/);
    assert.match(source, /upsertCanvasNode\(current, node\)/);
  });

  it("updates agent canvas node status from production websocket events", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /nodeStatusForGenerationStatus/);
    assert.match(source, /updateCanvasNodeStatus/);
    assert.match(source, /event\.payload\.node_id/);
  });
});
