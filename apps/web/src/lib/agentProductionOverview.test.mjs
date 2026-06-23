import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentPhaseLabel,
  agentProductionProgressText,
  shouldRefreshAgentProductionOverview,
  timelineStatusLabel,
} from "../../dist-test/lib/agentProductionOverview.js";

describe("agent production overview", () => {
  it("maps internal phases to user-facing labels", () => {
    assert.equal(agentPhaseLabel("planning"), "规划中");
    assert.equal(agentPhaseLabel("waiting_confirmation"), "等待确认");
    assert.equal(agentPhaseLabel("complete"), "已完成");
    assert.equal(agentPhaseLabel("future_phase"), "处理中");
  });

  it("summarizes storyboard progress from counts", () => {
    const text = agentProductionProgressText({
      shots_total: 3,
      previews_ready: 2,
      reviews_accepted: 1,
      videos_ready: 0,
      final_outputs: 0,
      running_tasks: 1,
      failed_tasks: 0,
      waiting_decisions: 1,
    });

    assert.equal(text, "3 个分镜 · 2 张预览 · 1 个通过评审 · 1 个任务运行中 · 1 个待确认");
  });

  it("refreshes overview for persisted agent and canvas events", () => {
    assert.equal(shouldRefreshAgentProductionOverview("agent.task.updated"), true);
    assert.equal(shouldRefreshAgentProductionOverview("agent.event.created"), true);
    assert.equal(shouldRefreshAgentProductionOverview("agent.message.delta"), false);
    assert.equal(shouldRefreshAgentProductionOverview("production.job.updated"), true);
    assert.equal(shouldRefreshAgentProductionOverview("NodeUpdated"), true);
  });

  it("labels timeline statuses without leaking raw status first", () => {
    assert.equal(timelineStatusLabel("running"), "执行中");
    assert.equal(timelineStatusLabel("succeeded"), "完成");
    assert.equal(timelineStatusLabel("waiting_for_user"), "等待确认");
  });
});
