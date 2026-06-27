import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentProcessingLabel,
  hasActiveAgentTask,
  hasProcessingAgentTask,
  hasRunningProducerTask,
  mergeActiveAgentTaskSnapshot,
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

  it("treats the active task response as a backend snapshot", () => {
    const tasks = mergeActiveAgentTaskSnapshot(
      [
        { id: "task-1", status: "running", task_type: "producer_turn" },
        { id: "task-2", status: "succeeded", task_type: "producer_turn" },
      ],
      [],
    );

    assert.deepEqual(tasks, [
      { id: "task-2", status: "succeeded", task_type: "producer_turn" },
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

  it("distinguishes processing tasks from waiting decisions", () => {
    assert.equal(
      hasActiveAgentTask([
        { id: "task-1", status: "waiting_for_user", task_type: "producer_turn" },
      ]),
      true,
    );
    assert.equal(
      hasProcessingAgentTask([
        { id: "task-1", status: "waiting_for_user", task_type: "producer_turn" },
      ]),
      false,
    );
    assert.equal(
      hasProcessingAgentTask([
        { id: "task-2", status: "running", task_type: "producer_turn" },
      ]),
      true,
    );
    assert.equal(
      hasProcessingAgentTask([
        { id: "task-3", status: "running", task_type: "craftsman_turn" },
        { id: "task-4", status: "queued", task_type: "worker_generation" },
      ]),
      false,
    );
  });

  it("renders processing labels for the shimmer thinking indicator", () => {
    assert.equal(
      agentProcessingLabel([
        { id: "task-1", status: "running", task_type: "producer_turn" },
      ]),
      "ClipAnvil 正在思考",
    );
    assert.equal(
      agentProcessingLabel([
        { id: "task-2", status: "queued", task_type: "decision_resume" },
      ]),
      "ClipAnvil 正在处理你的选择",
    );
    assert.equal(
      agentProcessingLabel([
        { id: "task-3", status: "running", task_type: "worker_generation" },
      ]),
      "",
    );
    assert.equal(
      agentProcessingLabel([
        { id: "task-4", status: "waiting_for_user", task_type: "producer_turn" },
      ]),
      "",
    );
  });
});
