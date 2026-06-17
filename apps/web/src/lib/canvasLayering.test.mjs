import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { describe, it } from "node:test";
import { URL } from "node:url";

const css = readFileSync(new URL("../main.css", import.meta.url), "utf8");
const mediaShapeUtil = readFileSync(
  new URL("../shapes/MediaShapeUtil.tsx", import.meta.url),
  "utf8",
);

describe("canvas layering", () => {
  it("keeps node inline editor above animated connection lines", () => {
    assert.ok(
      zIndex(".node-editor-overlay") > zIndex(".connection-overlay"),
      "node inline editor must be layered above connection overlay",
    );
  });

  it("renders node inline editor outside individual tldraw shapes", () => {
    assert.equal(
      mediaShapeUtil.includes("media-node-inline-editor"),
      false,
      "node inline editor should not be trapped inside a tldraw shape container",
    );
  });
});

function zIndex(selector) {
  const block = css.match(new RegExp(`${escapeRegExp(selector)}\\s*\\{([^}]*)\\}`));
  assert.ok(block, `missing CSS block for ${selector}`);
  const value = block[1].match(/z-index:\s*(\d+)\s*;/);
  assert.ok(value, `missing z-index for ${selector}`);
  return Number(value[1]);
}

function escapeRegExp(value) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}
