import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentThinkingEffortLabel,
  agentThinkingEffortOptions,
  agentModelSupportsThinking,
} from "../../dist-test/lib/agentThinking.js";

describe("agent thinking", () => {
  it("maps reasoning efforts to compact labels", () => {
    assert.equal(agentThinkingEffortLabel("minimal"), "关闭");
    assert.equal(agentThinkingEffortLabel("low"), "低");
    assert.equal(agentThinkingEffortLabel("medium"), "中");
    assert.equal(agentThinkingEffortLabel("high"), "高");
  });

  it("detects thinking-capable model options", () => {
    assert.equal(
      agentModelSupportsThinking({
        supports_thinking: true,
        reasoning_efforts: ["minimal", "low", "medium", "high"],
      }),
      true,
    );
    assert.equal(
      agentModelSupportsThinking({
        supports_thinking: true,
        reasoning_efforts: [],
      }),
      false,
    );
  });

  it("returns only supported reasoning effort options", () => {
    assert.deepEqual(
      agentThinkingEffortOptions({
        supports_thinking: true,
        reasoning_efforts: ["minimal", "low", "future", "high"],
      }),
      ["minimal", "low", "high"],
    );
  });
});
