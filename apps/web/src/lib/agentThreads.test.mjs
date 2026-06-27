import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  mergeAgentThreadMessages,
  mergeObservedAgentThreads,
  threadsForDispatchTool,
  threadPreviewFromMessage,
  updateObservedThreadFromMessage,
  updateObservedThreadFromTask,
} from "../../dist-test/lib/agentThreads.js";

describe("agent thread observer helpers", () => {
  it("dedupes and sorts observed threads by active work and latest message", () => {
    const threads = mergeObservedAgentThreads(
      [
        {
          id: "a",
          role: "craftsman",
          status: "active",
          latest_task: {
            id: "ta",
            status: "succeeded",
            task_type: "craftsman_turn",
          },
          latest_message_at: "2026-06-26T09:00:00Z",
        },
      ],
      [
        {
          id: "b",
          role: "reviewer",
          status: "active",
          latest_task: {
            id: "tb",
            status: "running",
            task_type: "reviewer_turn",
          },
          latest_message_at: "2026-06-26T08:00:00Z",
        },
      ],
    );

    assert.deepEqual(
      threads.map((thread) => thread.id),
      ["b", "a"],
    );
  });

  it("merges messages into a per-thread cache", () => {
    const cache = mergeAgentThreadMessages({}, "thread-1", [
      { id: "m2", thread_id: "thread-1", seq: 2 },
      { id: "m1", thread_id: "thread-1", seq: 1 },
    ]);

    assert.deepEqual(
      cache["thread-1"].messages.map((message) => message.id),
      ["m1", "m2"],
    );
    assert.equal(cache["thread-1"].hasLoadedInitial, true);
  });

  it("extracts markdown preview from an agent message", () => {
    assert.equal(
      threadPreviewFromMessage({
        content: {
          schema: "clipanvil.agent.message.v1",
          blocks: [
            { id: "blk", type: "markdown", text: "已创建 RenderPlan" },
          ],
        },
      }),
      "已创建 RenderPlan",
    );
  });

  it("updates thread item from task and message events", () => {
    const threads = [{ id: "thread-1", latest_message_preview: "" }];
    const withTask = updateObservedThreadFromTask(threads, {
      id: "task-1",
      thread_id: "thread-1",
      status: "running",
      task_type: "craftsman_turn",
    });
    assert.equal(withTask[0].latest_task.status, "running");

    const withMessage = updateObservedThreadFromMessage(withTask, {
      id: "m1",
      thread_id: "thread-1",
      seq: 1,
      created_at: "2026-06-26T09:05:00Z",
      content: {
        schema: "clipanvil.agent.message.v1",
        blocks: [{ id: "blk", type: "markdown", text: "完成" }],
      },
    });
    assert.equal(withMessage[0].latest_message_preview, "完成");
    assert.equal(withMessage[0].latest_message_at, "2026-06-26T09:05:00Z");
  });

  it("maps dispatch tool cards to observable sub-agent threads", () => {
    const threads = [
      { id: "craftsman-thread", role: "craftsman" },
      { id: "reviewer-thread", role: "reviewer" },
    ];

    assert.deepEqual(
      threadsForDispatchTool("dispatch_craftsman", threads).map(
        (thread) => thread.id,
      ),
      ["craftsman-thread"],
    );
    assert.deepEqual(
      threadsForDispatchTool("dispatch_reviewer", threads).map(
        (thread) => thread.id,
      ),
      ["reviewer-thread"],
    );
    assert.deepEqual(
      threadsForDispatchTool("dispatch_custom_agent", threads).map(
        (thread) => thread.id,
      ),
      ["craftsman-thread", "reviewer-thread"],
    );
  });
});
