import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { URL } from "node:url";

describe("agent workbench audio nodes", () => {
  it("keeps audio players out of the project overview and renders standalone audio nodes", () => {
    const overviewSource = readFileSync(
      new URL("../components/agent-workbench/AgentProjectOverviewNode.tsx", import.meta.url),
      "utf8",
    );
    const audioNodeSource = readFileSync(
      new URL("../components/agent-workbench/AgentAudioNode.tsx", import.meta.url),
      "utf8",
    );
    const canvasSource = readFileSync(
      new URL("../components/agent-workbench/AgentWorkbenchCanvas.tsx", import.meta.url),
      "utf8",
    );

    assert.doesNotMatch(overviewSource, /<audio/);
    assert.match(audioNodeSource, /<audio/);
    assert.match(audioNodeSource, /controls/);
    assert.match(audioNodeSource, /src=\{artifact\.access_url\}/);
    assert.doesNotMatch(audioNodeSource, /<p>\{artifact\.title \|\| data\.planTitle\}<\/p>/);
    assert.match(audioNodeSource, /title=\{audioTitle\}/);
    assert.match(canvasSource, /agentAudio:\s*AgentAudioNode/);
  });

  it("keeps the project overview rows from compressing tag content", () => {
    const cssSource = readFileSync(
      new URL("../main.css", import.meta.url),
      "utf8",
    );

    assert.match(
      cssSource,
      /\.agent-workbench-overview-node\s*\{[\s\S]*grid-template-rows:\s*auto auto auto auto auto auto;/,
    );
    assert.doesNotMatch(cssSource, /grid-template-rows:\s*auto auto auto auto minmax\(0,\s*1fr\) auto/);
    assert.match(
      cssSource,
      /\.agent-workbench-overview-elements button,[\s\S]*line-height:\s*1\.2;/,
    );
  });
});
