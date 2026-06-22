import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  clearAgentStream,
  mergeAgentStreamDelta,
  shouldShowAgentThinkingIndicator,
} from "../../dist-test/lib/agentStreaming.js";

describe("agent streaming", () => {
  it("appends stream deltas by task id", () => {
    const streams = mergeAgentStreamDelta([], {
      task_id: "task-1",
      block_id: "blk_answer",
      block_type: "markdown",
      delta: "第一段",
      sequence: 1,
    });
    const updated = mergeAgentStreamDelta(streams, {
      task_id: "task-1",
      block_id: "blk_answer",
      block_type: "markdown",
      delta: "，第二段",
      sequence: 2,
    });

    assert.deepEqual(updated, [
      {
        task_id: "task-1",
        blocks: [
          {
            id: "blk_answer",
            type: "markdown",
            text: "第一段，第二段",
            sequence: 2,
          },
        ],
      },
    ]);
  });

  it("keeps thinking and content deltas separate", () => {
    const streams = mergeAgentStreamDelta([], {
      task_id: "task-1",
      block_id: "blk_thinking",
      block_type: "thinking",
      delta: "先分析",
      sequence: 1,
    });
    const updated = mergeAgentStreamDelta(streams, {
      task_id: "task-1",
      block_id: "blk_answer",
      block_type: "markdown",
      delta: "结论",
      sequence: 2,
    });

    assert.deepEqual(updated, [
      {
        task_id: "task-1",
        blocks: [
          {
            id: "blk_thinking",
            type: "thinking",
            text: "先分析",
            sequence: 1,
          },
          {
            id: "blk_answer",
            type: "markdown",
            text: "结论",
            sequence: 2,
          },
        ],
      },
    ]);
  });

  it("clears the transient stream when the final message arrives", () => {
    const streams = [
      {
        task_id: "task-1",
        blocks: [
          {
            id: "blk_answer",
            type: "markdown",
            text: "临时文本",
            sequence: 1,
          },
        ],
      },
    ];

    assert.deepEqual(clearAgentStream(streams, "task-1"), []);
  });

  it("shows the standalone thinking indicator only before streaming starts", () => {
    assert.equal(shouldShowAgentThinkingIndicator(true, []), true);
    assert.equal(
      shouldShowAgentThinkingIndicator(true, [
        {
          task_id: "task-1",
          blocks: [
            {
              id: "blk_thinking",
              type: "thinking",
              text: "正在分析",
              sequence: 1,
            },
          ],
        },
      ]),
      false,
    );
    assert.equal(
      shouldShowAgentThinkingIndicator(true, [
        {
          task_id: "task-1",
          blocks: [
            {
              id: "blk_answer",
              type: "markdown",
              text: "正在输出",
              sequence: 1,
            },
          ],
        },
      ]),
      false,
    );
    assert.equal(shouldShowAgentThinkingIndicator(false, []), false);
  });
});
