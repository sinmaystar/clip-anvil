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

  it("renders readonly video nodes as playable media with poster support", async () => {
    const readonlyShapeSource = await readFile(
      new URL(
        "../shapes/AgentReadonlyMediaShapeUtil.tsx",
        import.meta.url,
      ),
      "utf8",
    );

    assert.match(readonlyShapeSource, /nodeType === "video"/);
    assert.match(readonlyShapeSource, /<video/);
    assert.match(readonlyShapeSource, /controls/);
    assert.match(readonlyShapeSource, /poster=\{previewThumbnailUrl \|\| thumbnailUrl\}/);
    assert.match(readonlyShapeSource, /src=\{previewAssetUrl\}/);
  });

  it("keeps readonly video controls interactive while disabling image drag", async () => {
    const css = await readFile(new URL("../main.css", import.meta.url), "utf8");

    assert.match(
      css,
      /\.agent-readonly-tldraw\s+\.media-node-media-frame\s+img\s*{[\s\S]*pointer-events:\s*none/,
    );
    assert.doesNotMatch(
      css,
      /\.agent-readonly-tldraw\s+\.media-node-media-frame\s+video\s*{[\s\S]*pointer-events:\s*none/,
    );
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

  it("updates locked readonly shapes when remote canvas snapshots change", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /ignoreShapeLock:\s*true/);
  });

  it("skips tldraw shape updates when the readonly snapshot is unchanged", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /readonlyShapeChanged/);
    assert.match(source, /JSON\.stringify/);
    assert.match(source, /readonlyShapeChanged\(existing, shape\)/);
  });

  it("syncs readonly arrow bindings so edge lines stay attached to nodes", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /edgeBindings/);
    assert.match(source, /syncReadonlyArrowBinding/);
    assert.match(source, /getBindingsFromShape\(binding\.fromId,\s*"arrow"\)/);
    assert.match(source, /createBinding\(binding\)/);
    assert.match(source, /updateBinding/);
  });

  it("ignores presigned URL query churn when comparing readonly shapes", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /readonlyComparableShapeValue/);
    assert.match(source, /stableReadonlyAssetURL/);
    assert.match(source, /previewAssetUrl/);
    assert.match(source, /previewThumbnailUrl/);
    assert.match(source, /thumbnailUrl/);
    assert.match(source, /\.search = ""/);
    assert.match(source, /\.hash = ""/);
  });

  it("does not repeatedly zoom-to-fit when only production status changes", async () => {
    const source = await readFile(
      new URL("../components/agent/AgentReadonlyCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /lastFitSignatureRef/);
    assert.match(source, /readonlyFitSignature/);
    assert.match(source, /signature === lastFitSignatureRef\.current/);
    assert.match(source, /\.map\(\(node\) =>\s*\[\s*node\.id/);
    assert.doesNotMatch(source, /readonlyFitSignature[\s\S]{0,800}current_version_id/);
    assert.doesNotMatch(source, /readonlyFitSignature[\s\S]{0,800}status/);
  });

  it("agent workspace no longer renders text-only node cards", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.doesNotMatch(source, /agent-node-card/);
    assert.match(source, /AgentReadonlyCanvas/);
  });

  it("applies a readonly automatic layout before rendering agent canvas snapshots", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /computeDagreLayout/);
    assert.match(source, /readonlyAutoLayoutCanvas/);
    assert.match(source, /direction:\s*"TB"/);
    assert.match(source, /canvas=\{readonlyCanvas\}/);
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

  it("forces a canvas refetch after terminal production websocket events", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /isTerminalGenerationStatus/);
    assert.match(source, /queryClient\.refetchQueries\(\{\s*queryKey:\s*\["workspace",\s*id,\s*"canvas"\]/);
  });

  it("does not refetch the full agent canvas for non-terminal production websocket events", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    const productionCase = source.match(
      /case "production\.job\.updated":[\s\S]*?case "production\.model\.delta":[\s\S]*?break;/,
    )?.[0];

    assert.ok(productionCase, "expected production websocket case");
    assert.match(productionCase, /isTerminalGenerationStatus/);
    assert.doesNotMatch(productionCase, /refreshCanvas\(\)/);
    assert.doesNotMatch(productionCase, /invalidateQueries\(\{\s*queryKey:\s*\["workspace",\s*id,\s*"canvas"\]/);
  });

  it("refreshes agent canvas after the websocket first connects", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /onStatusChange:\s*\(status\)\s*=>\s*\{/);
    assert.match(source, /status\s*===\s*"connected"[\s\S]{0,80}refreshCanvas\(\)/);
  });

  it("polls agent canvas while async production previews are incomplete", async () => {
    const source = await readFile(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(source, /refetchInterval:\s*\(query\)\s*=>/);
    assert.match(source, /canvasConnectionStatus\s*!==\s*"connected"/);
    assert.match(source, /shouldPollCanvasForProductionUpdates\(query\.state\.data\)/);
  });
});
