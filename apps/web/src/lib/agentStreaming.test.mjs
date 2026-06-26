import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  clearAgentStream,
  mergeAgentStreamDelta,
  rememberFinalAgentMessage,
  shouldShowAgentThinkingIndicator,
  visibleAgentStreams,
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

  it("hides a transient stream when an assistant text message for the same task is visible", () => {
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
      {
        task_id: "task-2",
        blocks: [
          {
            id: "blk_answer",
            type: "markdown",
            text: "另一个任务",
            sequence: 1,
          },
        ],
      },
    ];

    assert.deepEqual(
      visibleAgentStreams(streams, [
        {
          id: "final-1",
          role: "assistant",
          message_type: "text",
          task_id: "task-1",
        },
      ]),
      [streams[1]],
    );
  });

  it("hides a transient stream when its text matches the final assistant message", () => {
    const streams = [
      {
        task_id: "stream-task",
        blocks: [
          {
            id: "blk_answer",
            type: "markdown",
            text: "已完成前期创作规划，并已开始推进视觉生成：",
            sequence: 1,
          },
        ],
      },
    ];

    assert.deepEqual(
      visibleAgentStreams(streams, [
        {
          id: "final-1",
          role: "assistant",
          message_type: "text",
          task_id: "different-task",
          content: {
            schema: "clipanvil.agent.message.v1",
            blocks: [
              {
                id: "blk_answer",
                type: "markdown",
                text: "已完成前期创作规划，并已开始推进视觉生成：",
              },
            ],
          },
        },
      ]),
      [],
    );
  });

  it("does not recreate a stream after the final message for that task arrived", () => {
    const finalized = rememberFinalAgentMessage(new Set(), {
      id: "final-1",
      role: "assistant",
      message_type: "text",
      task_id: "task-1",
    });

    assert.deepEqual(
      mergeAgentStreamDelta(
        [],
        {
          task_id: "task-1",
          block_id: "blk_answer",
          block_type: "markdown",
          delta: "迟到的增量",
          sequence: 3,
        },
        finalized,
      ),
      [],
    );
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
