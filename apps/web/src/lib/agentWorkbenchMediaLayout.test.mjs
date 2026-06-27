import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { agentWorkbenchMediaSize } from "../../dist-test/lib/agentWorkbenchMediaLayout.js";

describe("agent workbench media layout", () => {
  it("sizes vertical artifact previews by their original media ratio", () => {
    const size = agentWorkbenchMediaSize({
      kind: "preview_image",
      status: "succeeded",
      width: 900,
      height: 1600,
    });

    assert.ok(size.height > size.width, `${size.width}x${size.height}`);
    assert.ok(size.height <= 420, `height ${size.height} should stay readable`);
    assert.ok(Math.abs(size.width / size.height - 9 / 16) < 0.05);
    assert.equal(size.aspectRatio, "900 / 1600");
  });

  it("keeps horizontal media wide without forcing vertical dimensions", () => {
    const size = agentWorkbenchMediaSize({
      kind: "preview_image",
      status: "succeeded",
      width: 1600,
      height: 900,
    });

    assert.ok(size.width > size.height, `${size.width}x${size.height}`);
    assert.ok(Math.abs(size.width / size.height - 16 / 9) < 0.05);
  });

  it("uses measured natural dimensions when artifact metadata is absent", () => {
    const size = agentWorkbenchMediaSize(
      {
        kind: "preview_image",
        status: "succeeded",
      },
      { width: 1024, height: 1024 },
    );

    assert.equal(size.width, size.height);
    assert.equal(size.aspectRatio, "1024 / 1024");
  });
});
