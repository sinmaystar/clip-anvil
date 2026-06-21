import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  candidateReferencePackMembers,
  isReferencePackMemberDependency,
  memberIdsAfterToggle,
  memberNodesForPack,
  referencePackSummaryText,
} from "../../dist-test/lib/referencePack.js";
import { winnerPreviewText } from "../../dist-test/lib/productionPreview.js";

const pack = {
  id: "pack",
  node_type: "reference_pack",
  title: "商品参考包",
  status: "draft",
};

const nodes = [
  pack,
  { id: "image-a", node_type: "image", title: "主图", status: "succeeded" },
  { id: "image-b", node_type: "image", title: "细节图", status: "succeeded" },
  { id: "text-a", node_type: "text", title: "卖点", status: "succeeded" },
  { id: "pack-2", node_type: "reference_pack", title: "另一个包", status: "draft" },
];

const items = [
  { id: "item-2", pack_node_id: "pack", member_node_id: "image-b", position: 1 },
  { id: "item-1", pack_node_id: "pack", member_node_id: "image-a", position: 0 },
];

describe("reference pack helpers", () => {
  it("returns member nodes in position order", () => {
    assert.deepEqual(
      memberNodesForPack(items, nodes).map((node) => node.id),
      ["image-a", "image-b"],
    );
  });

  it("filters candidates to non-pack non-member nodes", () => {
    assert.deepEqual(
      candidateReferencePackMembers(pack, nodes, items).map((node) => node.id),
      ["text-a"],
    );
  });

  it("detects dependency from a pack to one of its members", () => {
    assert.equal(isReferencePackMemberDependency("pack", "image-a", items), true);
    assert.equal(isReferencePackMemberDependency("pack", "text-a", items), false);
  });

  it("builds toggle payloads while preserving order", () => {
    assert.deepEqual(memberIdsAfterToggle(items, "image-a", false), [
      "image-b",
    ]);
    assert.deepEqual(memberIdsAfterToggle(items, "text-a", true), [
      "image-a",
      "image-b",
      "text-a",
    ]);
  });

  it("summarizes pack preview members", () => {
    assert.equal(
      referencePackSummaryText({
        member_count: 2,
        members: [
          {
            id: "image-a",
            node_type: "image",
            title: "主图",
            status: "succeeded",
          },
          {
            id: "image-b",
            node_type: "image",
            title: "细节图",
            status: "succeeded",
          },
        ],
      }),
      "2 members · 图片 主图, 图片 细节图",
    );
  });

  it("summarizes source material members with material labels", () => {
    assert.equal(
      referencePackSummaryText({
        member_count: 1,
        members: [
          {
            id: "image-source",
            node_type: "image",
            title: "商品主图",
            status: "succeeded",
            operation_type: "upload",
            asset_id: "asset-1",
          },
        ],
      }),
      "1 member · 图片素材 商品主图",
    );
  });

  it("uses reference pack preview in winner preview text", () => {
    assert.equal(
      winnerPreviewText({
        node_type: "reference_pack",
        prompt: "",
        reference_pack_preview: {
          member_count: 1,
          members: [
            {
              id: "image-a",
              node_type: "image",
              title: "主图",
              status: "succeeded",
            },
          ],
        },
      }),
      "1 member · 图片 主图",
    );
  });
});
