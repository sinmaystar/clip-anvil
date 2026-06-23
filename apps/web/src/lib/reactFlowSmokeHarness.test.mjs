import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { URL } from "node:url";

const smokeScriptUrl = new URL(
  "../../../../scripts/smoke-react-flow-canvas.sh",
  import.meta.url,
);

describe("React Flow canvas smoke harness", () => {
  it("creates reusable Studio and Agent E2E entry URLs", async () => {
    const source = await readFile(smokeScriptUrl, "utf8");

    assert.match(source, /CLIPANVIL_API_BASE/);
    assert.match(source, /CLIPANVIL_WEB_BASE/);
    assert.match(source, /createWorkspace\("React Flow Studio Smoke",\s*"studio"\)/);
    assert.match(source, /createWorkspace\("React Flow Agent Smoke",\s*"agent"\)/);
    assert.match(source, /studio_url/);
    assert.match(source, /agent_url/);
    assert.match(source, /\/workspaces\/\$\{studio\.id\}\/studio/);
    assert.match(source, /\/workspaces\/\$\{agent\.id\}\/agent/);
  });

  it("seeds baseline canvas nodes, dependency edge, and group", async () => {
    const source = await readFile(smokeScriptUrl, "utf8");

    assert.match(source, /Smoke Script/);
    assert.match(source, /Smoke Image/);
    assert.match(source, /\/edges/);
    assert.match(source, /edge_type:\s*"dependency"/);
    assert.match(source, /\/groups/);
    assert.match(source, /Smoke Group/);
    assert.match(source, /studioCanvas\.nodes\.length/);
    assert.match(source, /studioCanvas\.edges\.length/);
    assert.match(source, /studioCanvas\.groups\.length/);
  });

  it("seeds Agent baseline through Agent-owned attachment APIs", async () => {
    const source = await readFile(smokeScriptUrl, "utf8");

    assert.doesNotMatch(source, /createNode\(agent/);
    assert.match(source, /\/agent\/workspaces\/\$\{agent\.id\}\/attachments/);
    assert.match(source, /Agent Layout Brief/);
    assert.match(source, /agentCanvas\.nodes\.length !== 1/);
    assert.match(source, /agent_canvas_nodes/);
  });
});
