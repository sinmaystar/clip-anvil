import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { mergeAgentMessages } from "../../dist-test/lib/agentMessages.js";

describe("agent messages", () => {
  it("dedupes by id and sorts by seq", () => {
    const messages = mergeAgentMessages(
      [
        { id: "b", seq: 2 },
        { id: "a", seq: 1 },
      ],
      [
        { id: "b", seq: 2 },
        { id: "c", seq: 3 },
      ],
    );

    assert.deepEqual(
      messages.map((message) => message.id),
      ["a", "b", "c"],
    );
  });

  it("replaces an existing message when an updated payload has the same id", () => {
    const messages = mergeAgentMessages(
      [{ id: "tool-1", seq: 4, status: "running" }],
      [{ id: "tool-1", seq: 4, status: "succeeded" }],
    );

    assert.equal(messages.length, 1);
    assert.equal(messages[0].status, "succeeded");
  });
});
