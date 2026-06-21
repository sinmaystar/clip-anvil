import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { apiErrorMessage } from "../../dist-test/lib/apiErrors.js";

describe("api errors", () => {
  it("uses failed run job messages before generic text", () => {
    const payload = {
      job: {
        id: "job-1",
        error_code: "capability_mismatch",
        error_message: "model does not support input node type video",
      },
    };

    assert.equal(
      apiErrorMessage(payload, "请求失败"),
      "model does not support input node type video",
    );
  });

  it("uses normal error payload messages", () => {
    assert.equal(apiErrorMessage({ error: "invalid request" }, "请求失败"), "invalid request");
  });
});
