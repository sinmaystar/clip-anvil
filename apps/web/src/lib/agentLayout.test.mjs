import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  clampAgentPanelHeight,
  clampAgentPanelWidth,
  clampAgentRestorePosition,
  defaultAgentRestorePosition,
  isAgentMessageListNearBottom,
  resizeAgentPanelFromCorner,
} from "../../dist-test/lib/agentLayout.js";

describe("agent layout", () => {
  it("keeps desktop panel width inside useful bounds", () => {
    assert.equal(clampAgentPanelWidth(200, 1280), 360);
    assert.equal(clampAgentPanelWidth(520, 1280), 520);
    assert.equal(clampAgentPanelWidth(900, 1280), 720);
  });

  it("leaves room for the canvas on smaller viewports", () => {
    assert.equal(clampAgentPanelWidth(700, 760), 680);
  });

  it("keeps panel height inside useful viewport bounds", () => {
    assert.equal(clampAgentPanelHeight(200, 900), 360);
    assert.equal(clampAgentPanelHeight(620, 900), 620);
    assert.equal(clampAgentPanelHeight(900, 900), 868);
  });

  it("detects when the message list should stay pinned to newest content", () => {
    assert.equal(
      isAgentMessageListNearBottom({
        scrollHeight: 1000,
        clientHeight: 400,
        scrollTop: 560,
      }),
      true,
    );
    assert.equal(
      isAgentMessageListNearBottom({
        scrollHeight: 1000,
        clientHeight: 400,
        scrollTop: 420,
      }),
      false,
    );
  });

  it("keeps the restore bubble inside the viewport", () => {
    assert.deepEqual(
      clampAgentRestorePosition({ x: -20, y: 900 }, 1280, 720),
      { x: 16, y: 650 },
    );
    assert.deepEqual(
      clampAgentRestorePosition({ x: 500, y: 400 }, 1280, 720),
      { x: 500, y: 400 },
    );
  });

  it("places the default restore bubble in the lower right corner", () => {
    assert.deepEqual(defaultAgentRestorePosition(1280, 720), {
      x: 1210,
      y: 650,
    });
  });

  it("resizes from left corners with the expected vertical direction", () => {
    assert.deepEqual(
      resizeAgentPanelFromCorner({
        corner: "bottom-left",
        startClientX: 500,
        startClientY: 500,
        clientX: 460,
        clientY: 540,
        startWidth: 420,
        startHeight: 560,
        viewportWidth: 1280,
        viewportHeight: 900,
      }),
      { width: 460, height: 600 },
    );

    assert.deepEqual(
      resizeAgentPanelFromCorner({
        corner: "top-left",
        startClientX: 500,
        startClientY: 500,
        clientX: 460,
        clientY: 460,
        startWidth: 420,
        startHeight: 560,
        viewportWidth: 1280,
        viewportHeight: 900,
      }),
      { width: 460, height: 600 },
    );
  });
});
