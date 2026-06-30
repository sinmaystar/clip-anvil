import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { URL } from "node:url";

describe("agent thread observer components", () => {
  it("defines observer panel, read-only drawer, and link chip", () => {
    const panel = readFileSync(
      new URL("../components/agent/AgentThreadObserverPanel.tsx", import.meta.url),
      "utf8",
    );
    const drawer = readFileSync(
      new URL("../components/agent/AgentThreadDrawer.tsx", import.meta.url),
      "utf8",
    );
    const chip = readFileSync(
      new URL("../components/agent/AgentThreadLinkChip.tsx", import.meta.url),
      "utf8",
    );

    assert.match(panel, /agent-thread-selector/);
    assert.doesNotMatch(panel, /<select/);
    assert.match(panel, /选择子 Agent/);
    assert.match(panel, /agent-thread-selector-trigger/);
    assert.match(panel, /agent-thread-selector-menu/);
    assert.match(panel, /agent-thread-selector-option/);
    assert.match(panel, /agent-thread-selector-status/);
    assert.doesNotMatch(panel, /agent-thread-strip/);
    assert.doesNotMatch(panel, /agent-thread-observer-item/);
    assert.match(drawer, /agent-thread-drawer/);
    assert.match(drawer, /只读/);
    assert.match(chip, /agent-thread-link-chip/);
  });

  it("wires the observer panel and drawer into AgentWorkspacePage", () => {
    const page = readFileSync(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(page, /fetchAgentThreads/);
    assert.match(page, /fetchAgentThreadMessages/);
    assert.match(page, /AgentThreadObserverPanel/);
    assert.match(page, /AgentThreadDrawer/);
    assert.match(page, /agent-chat-header-actions/);
    assert.doesNotMatch(page, /agent-chat-with-observer/);
  });

  it("persists selected sub-agent thread in the URL query string", () => {
    const page = readFileSync(
      new URL("../pages/AgentWorkspacePage.tsx", import.meta.url),
      "utf8",
    );

    assert.match(page, /useSearchParams/);
    assert.match(page, /agentThread/);
    assert.match(page, /selectAgentThread/);
    assert.match(page, /setSearchParams/);
  });
});
