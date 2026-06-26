import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentWorkbenchOverviewSelection,
  agentWorkbenchSelectionEquals,
  agentWorkbenchSelectionKey,
} from "../../dist-test/lib/agentWorkbenchSelection.js";

describe("agent workbench selection", () => {
  it("keys selection by object type and object id", () => {
    assert.equal(
      agentWorkbenchSelectionKey({
        objectType: "shot",
        objectId: "shot-1",
      }),
      "shot:shot-1",
    );
    assert.equal(
      agentWorkbenchSelectionKey({
        objectType: "artifact",
        objectId: "shot-1",
      }),
      "artifact:shot-1",
    );
  });

  it("distinguishes child artifact selection from parent shot selection", () => {
    const selection = {
      objectType: "artifact",
      objectId: "node-1",
      label: "Preview",
    };

    assert.equal(
      agentWorkbenchSelectionEquals(selection, "artifact", "node-1"),
      true,
    );
    assert.equal(agentWorkbenchSelectionEquals(selection, "shot", "node-1"), false);
  });

  it("builds overview selection from workspace id", () => {
    assert.deepEqual(agentWorkbenchOverviewSelection("workspace-1"), {
      objectType: "overview",
      objectId: "workspace-1",
      label: "Project Overview",
    });
  });
});
