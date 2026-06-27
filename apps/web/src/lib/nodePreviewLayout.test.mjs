import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  adaptiveMediaNodeSize,
  mediaNodePreviewLimits,
} from "../../dist-test/lib/nodePreviewLayout.js";

const baseNode = {
  node_type: "text",
  canvas_w: 0,
  canvas_h: 0,
  prompt: "",
  production_preview: undefined,
  reference_pack_preview: undefined,
};

describe("adaptive media node layout", () => {
  it("grows text nodes for long generated markdown within max bounds", () => {
    const markdown = Array.from(
      { length: 30 },
      (_, index) => `## Scene ${index + 1}\n- action\n- camera`,
    ).join("\n\n");
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      production_preview: { text: markdown },
    });

    assert.equal(size.w, 340);
    assert.ok(size.h <= 300, `height ${size.h} should stay compact`);
    assert.ok(size.h <= mediaNodePreviewLimits.text.maxH);
  });

  it("fits image nodes to preview aspect ratio inside max bounds", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "image",
      production_preview: {
        asset_type: "image",
        width: 1600,
        height: 900,
      },
    });

    assert.ok(size.w <= mediaNodePreviewLimits.image.maxW);
    assert.ok(size.h <= mediaNodePreviewLimits.image.maxH);
    assert.ok(size.w <= 440, `width ${size.w} should stay inspectable`);
    assert.ok(Math.abs(size.w / size.h - 16 / 9) < 0.05);
  });

  it("fits vertical images without cropping by height", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "image",
      production_preview: {
        asset_type: "image",
        width: 900,
        height: 1600,
      },
    });

    assert.ok(size.h <= mediaNodePreviewLimits.image.maxH);
    assert.ok(size.h <= 380, `height ${size.h} should stay inspectable`);
    assert.ok(Math.abs(size.w / size.h - 9 / 16) < 0.05);
  });

  it("uses measured image dimensions when preview metadata is absent", () => {
    const horizontal = adaptiveMediaNodeSize(
      {
        ...baseNode,
        node_type: "image",
      },
      { width: 1800, height: 900 },
    );
    const vertical = adaptiveMediaNodeSize(
      {
        ...baseNode,
        node_type: "image",
      },
      { width: 900, height: 1800 },
    );

    assert.ok(
      horizontal.w > horizontal.h,
      `horizontal image should stay wide, got ${horizontal.w}x${horizontal.h}`,
    );
    assert.ok(
      vertical.h > vertical.w,
      `vertical image should stay tall, got ${vertical.w}x${vertical.h}`,
    );
  });

  it("keeps obviously persisted larger sizes", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "text",
      canvas_w: 700,
      canvas_h: 560,
      production_preview: { text: "short" },
    });

    assert.deepEqual(size, { w: 700, h: 560, sizeMode: "persisted" });
  });

  it("uses a stable video ratio", () => {
    const size = adaptiveMediaNodeSize({
      ...baseNode,
      node_type: "video",
      production_preview: {
        asset_type: "video",
        width: 1280,
        height: 720,
      },
    });

    assert.ok(size.w <= 480, `width ${size.w} should stay inspectable`);
    assert.ok(Math.abs(size.w / size.h - 16 / 9) < 0.05);
  });

  it("uses measured video dimensions when preview metadata is absent", () => {
    const size = adaptiveMediaNodeSize(
      {
        ...baseNode,
        node_type: "video",
      },
      { width: 496, height: 864 },
    );

    assert.ok(
      size.h > size.w,
      `vertical video should stay tall, got ${size.w}x${size.h}`,
    );
    assert.ok(size.h <= mediaNodePreviewLimits.video.maxH);
    assert.ok(Math.abs(size.w / size.h - 496 / 864) < 0.05);
  });
});
