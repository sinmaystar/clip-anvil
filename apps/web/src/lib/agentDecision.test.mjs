import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  decisionCardFromMessage,
  decisionResolvedFromEventPayload,
} from "../../dist-test/lib/agentDecision.js";

describe("agent decision cards", () => {
  it("parses persisted decision request cards", () => {
    const card = decisionCardFromMessage({
      message_type: "ui_card",
      content: {
        schema: "clipanvil.agent.message.v1",
        blocks: [
          {
            id: "blk_decision",
            type: "decision_card",
            decision_id: "decision-1",
            title: "确认方向",
            message: "请选择一个方向",
            status: "pending",
            allow_free_text: false,
            options: [{ id: "a", label: "方案 A" }],
          },
        ],
      },
    });

    assert.equal(card?.decision_id, "decision-1");
    assert.equal(card?.options[0]?.label, "方案 A");
  });

  it("ignores non-card messages and extracts resolved decision ids", () => {
    assert.equal(
      decisionCardFromMessage({ message_type: "text", content: {} }),
      null,
    );
    assert.equal(
      decisionResolvedFromEventPayload({ decision_id: "decision-1" }),
      "decision-1",
    );
  });
});
