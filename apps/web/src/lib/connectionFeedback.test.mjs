import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { connectionFailureFeedback } from "../../dist-test/lib/connectionFeedback.js";

describe("connection feedback", () => {
  it("explains circular dependency failures in user-facing language", () => {
    const feedback = connectionFailureFeedback(422);

    assert.equal(feedback.title, "这条线会形成循环");
    assert.equal(
      feedback.description,
      "Node 不能依赖自己的下游节点。请改为从上游节点连到下游节点。",
    );
    assert.equal(feedback.tone, "danger");
  });

  it("uses a generic message for other connection failures", () => {
    const feedback = connectionFailureFeedback(500);

    assert.equal(feedback.title, "连线失败");
    assert.equal(feedback.description, "请稍后再试，或者换一个目标节点。");
    assert.equal(feedback.tone, "warning");
  });
});
