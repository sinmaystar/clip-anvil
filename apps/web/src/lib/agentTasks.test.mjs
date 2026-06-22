import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentComposerDisabledReason,
  hasActiveAgentTask,
  hasRunningProducerTask,
  mergeAgentTasks,
} from "../../dist-test/lib/agentTasks.js";

describe("agent tasks", () => {
  it("dedupes task updates by id", () => {
    const tasks = mergeAgentTasks(
      [{ id: "task-1", status: "queued", task_type: "producer_turn" }],
      [{ id: "task-1", status: "running", task_type: "producer_turn" }],
    );

    assert.deepEqual(tasks, [
      { id: "task-1", status: "running", task_type: "producer_turn" },
    ]);
  });

  it("detects running producer turns", () => {
    assert.equal(
      hasRunningProducerTask([
        { id: "task-1", status: "running", task_type: "producer_turn" },
      ]),
      true,
    );
    assert.equal(
      hasRunningProducerTask([
        { id: "task-1", status: "succeeded", task_type: "producer_turn" },
      ]),
      false,
    );
  });

  it("detects active tasks and composer disabled reason", () => {
    assert.equal(
      hasActiveAgentTask([
        { id: "task-1", status: "waiting_for_user", task_type: "producer_turn" },
      ]),
      true,
    );
    assert.equal(
      agentComposerDisabledReason([
        { id: "task-1", status: "waiting_for_user", task_type: "producer_turn" },
      ]),
      "请先完成当前决策",
    );
    assert.equal(
      agentComposerDisabledReason([
        { id: "task-2", status: "running", task_type: "producer_turn" },
      ]),
      "",
    );
  });
});
