import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  mergeAgentMessages,
  visibleAgentMessages,
} from "../../dist-test/lib/agentMessages.js";

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

  it("sorts messages from multiple agent threads by created_at before seq", () => {
    const messages = mergeAgentMessages(
      [
        {
          id: "producer-tool",
          thread_id: "producer",
          seq: 3,
          created_at: "2026-06-25T10:00:03Z",
        },
        {
          id: "craftsman-tool",
          thread_id: "craftsman-shot-1",
          seq: 1,
          created_at: "2026-06-25T10:00:02Z",
        },
      ],
      [
        {
          id: "producer-text",
          thread_id: "producer",
          seq: 4,
          created_at: "2026-06-25T10:00:04Z",
        },
      ],
    );

    assert.deepEqual(
      messages.map((message) => message.id),
      ["craftsman-tool", "producer-tool", "producer-text"],
    );
  });

  it("keeps nested agent messages after their parent tool call", () => {
    const messages = mergeAgentMessages(
      [
        {
          id: "child-tool",
          seq: 1,
          created_at: "2026-06-25T10:00:02Z",
          raw_message: { parent_tool_call_id: "dispatch-1" },
        },
        {
          id: "producer-tool",
          seq: 10,
          created_at: "2026-06-25T10:00:10Z",
          raw_message: { tool_call_id: "dispatch-1" },
        },
      ],
      [
        {
          id: "producer-answer",
          seq: 11,
          created_at: "2026-06-25T10:00:11Z",
        },
      ],
    );

    assert.deepEqual(
      messages.map((message) => message.id),
      ["producer-tool", "child-tool", "producer-answer"],
    );
  });

  it("does not duplicate nested messages when a hidden tool result shares the parent tool_call_id", () => {
    const messages = visibleAgentMessages(
      mergeAgentMessages(
        [
          {
            id: "producer-tool",
            seq: 1,
            message_type: "tool_call",
            raw_message: { tool_call_id: "dispatch-1" },
          },
          {
            id: "producer-result",
            seq: 2,
            message_type: "tool_result",
            raw_message: { tool_call_id: "dispatch-1" },
          },
        ],
        [
          {
            id: "child-tool",
            seq: 1,
            message_type: "tool_call",
            raw_message: { parent_tool_call_id: "dispatch-1" },
          },
        ],
      ),
    );

    assert.deepEqual(
      messages.map((message) => message.id),
      ["producer-tool", "child-tool"],
    );
  });

  it("hides raw tool_result rows because their payload is folded into the tool_call row", () => {
    const messages = visibleAgentMessages([
      { id: "call", seq: 1, message_type: "tool_call" },
      { id: "result", seq: 2, message_type: "tool_result" },
      { id: "answer", seq: 3, message_type: "text" },
    ]);

    assert.deepEqual(
      messages.map((message) => message.id),
      ["call", "answer"],
    );
  });
});
