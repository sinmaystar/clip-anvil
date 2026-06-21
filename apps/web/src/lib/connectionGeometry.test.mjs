import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  connectionPath,
  inputAnchor,
  isValidConnectionTarget,
  mediaNodeBounds,
  outputAnchor,
} from "../../dist-test/lib/connectionGeometry.js";

describe("connection geometry", () => {
  it("uses right and left midpoints as node anchors", () => {
    const source = mediaNodeBounds({
      id: "a",
      canvas_x: 40,
      canvas_y: 60,
      canvas_w: 120,
      canvas_h: 80,
    });
    const target = mediaNodeBounds({
      id: "b",
      canvas_x: 300,
      canvas_y: 110,
      canvas_w: 140,
      canvas_h: 100,
    });

    assert.deepEqual(outputAnchor(source), { x: 160, y: 100 });
    assert.deepEqual(inputAnchor(target), { x: 300, y: 160 });
  });

  it("can use a live shape position while keeping saved dimensions", () => {
    const source = mediaNodeBounds(
      {
        id: "a",
        canvas_x: 40,
        canvas_y: 60,
        canvas_w: 120,
        canvas_h: 80,
      },
      { x: 120, y: 90 },
    );

    assert.deepEqual(source, { id: "a", x: 120, y: 90, w: 120, h: 80 });
    assert.deepEqual(outputAnchor(source), { x: 240, y: 130 });
  });

  it("uses adaptive preview dimensions when generated content expands a node", () => {
    const source = mediaNodeBounds({
      id: "a",
      node_type: "text",
      canvas_x: 40,
      canvas_y: 60,
      canvas_w: 220,
      canvas_h: 132,
      prompt: "",
      production_preview: {
        text: [
          "# 视频脚本",
          "",
          "## 核心镜头",
          "",
          "- 机场大厅里三只彩色行李箱按绿、红、蓝排列。",
          "- 地面有清晰反光，整体光线明亮。",
          "- 镜头慢慢推近，突出材质和颜色对比。",
          "",
          "```text",
          "主视觉：现代、干净、商业广告质感",
          "```",
        ].join("\n"),
      },
    });

    assert.equal(source.w, 340);
    assert.equal(outputAnchor(source).x, 380);
  });

  it("builds a long cubic path with horizontal pull", () => {
    const path = connectionPath({ x: 160, y: 100 }, { x: 300, y: 160 });

    assert.equal(path, "M 160 100 C 244 100, 216 160, 300 160");
  });

  it("keeps a visible curve when the target is close to the source", () => {
    const path = connectionPath({ x: 160, y: 100 }, { x: 178, y: 116 });

    assert.equal(path, "M 160 100 C 220 100, 118 116, 178 116");
  });

  it("accepts any other node as a drag release target", () => {
    assert.equal(isValidConnectionTarget("source", "target"), true);
    assert.equal(isValidConnectionTarget("source", "source"), false);
    assert.equal(isValidConnectionTarget("source", null), false);
  });
});
