import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentModelSelectionPayload,
  agentModelSelectionValue,
  formatAgentModelOption,
} from "../../dist-test/lib/agentModelSelection.js";

describe("agent model selection", () => {
  it("builds stable select values and save payloads", () => {
    const value = agentModelSelectionValue({
      provider_id: "volcengine",
      model_id: "doubao-mini",
    });

    assert.equal(value, "volcengine:doubao-mini");
    assert.deepEqual(agentModelSelectionPayload(value), {
      producer: {
        provider_id: "volcengine",
        model_id: "doubao-mini",
      },
    });
    assert.deepEqual(agentModelSelectionPayload(value, "high"), {
      producer: {
        provider_id: "volcengine",
        model_id: "doubao-mini",
        reasoning_effort: "high",
      },
    });
  });

  it("formats model options with display name fallback", () => {
    assert.equal(
      formatAgentModelOption({
        provider_id: "volcengine",
        model_id: "doubao-mini",
        display_name: "Doubao Mini",
      }),
      "Doubao Mini",
    );
    assert.equal(
      formatAgentModelOption({
        provider_id: "volcengine",
        model_id: "doubao-lite",
      }),
      "doubao-lite",
    );
  });
});
