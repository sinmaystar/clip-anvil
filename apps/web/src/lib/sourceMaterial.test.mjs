import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  canRunProductionNode,
  isManualTextMaterialNode,
  isSourceMaterialNode,
  isUploadMaterialNode,
  materialKindLabel,
  materialStatusLabel,
} from "../../dist-test/lib/sourceMaterial.js";

describe("source material node helpers", () => {
  it("classifies uploaded asset nodes as source material", () => {
    const node = {
      node_type: "image",
      operation_type: "upload",
      asset_id: "asset-1",
      status: "succeeded",
    };

    assert.equal(isSourceMaterialNode(node), true);
    assert.equal(isUploadMaterialNode(node), true);
    assert.equal(isManualTextMaterialNode(node), false);
    assert.equal(canRunProductionNode(node), false);
    assert.equal(materialKindLabel(node), "图片素材");
    assert.equal(materialStatusLabel(node), "可用");
  });

  it("classifies manual text nodes as source material", () => {
    const node = {
      node_type: "text",
      operation_type: "manual",
      asset_id: null,
      status: "succeeded",
    };

    assert.equal(isSourceMaterialNode(node), true);
    assert.equal(isUploadMaterialNode(node), false);
    assert.equal(isManualTextMaterialNode(node), true);
    assert.equal(canRunProductionNode(node), false);
    assert.equal(materialKindLabel(node), "文本素材");
    assert.equal(materialStatusLabel(node), "可用");
  });

  it("keeps normal generation nodes runnable", () => {
    const node = {
      node_type: "image",
      operation_type: "text_to_image",
      asset_id: null,
      status: "draft",
    };

    assert.equal(isSourceMaterialNode(node), false);
    assert.equal(canRunProductionNode(node), true);
    assert.equal(materialKindLabel(node), "图片");
    assert.equal(materialStatusLabel(node), "");
  });
});
