import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { computeDagreLayout } from "../../dist-test/lib/layout.js";

const baseNode = {
  workspace_id: "workspace",
  node_type: "text",
  title: "",
  prompt: "",
  status: "draft",
  canvas_w: 200,
  canvas_h: 120,
  created_at: "2026-06-17T00:00:00Z",
  updated_at: "2026-06-17T00:00:00Z",
};

const node = (id, canvas_x, canvas_y, group_id = null) => ({
  ...baseNode,
  id,
  title: id,
  group_id,
  canvas_x,
  canvas_y,
});

const edge = (id, from_node_id, to_node_id) => ({
  id,
  workspace_id: "workspace",
  from_node_id,
  to_node_id,
  edge_type: "dependency",
  source: "user",
  created_at: "2026-06-17T00:00:00Z",
});

describe("layout", () => {
  it("lays out groups as containers so ungrouped nodes do not land inside them", () => {
    const result = computeDagreLayout({
      direction: "LR",
      edges: [edge("alpha-gamma", "alpha", "gamma"), edge("gamma-beta", "gamma", "beta")],
      groups: [
        {
          id: "group-a",
          workspace_id: "workspace",
          name: "分组",
          sort_order: 0,
          node_ids: ["alpha", "beta"],
        },
      ],
      nodes: [
        node("alpha", 0, 0, "group-a"),
        node("beta", 240, 0, "group-a"),
        node("gamma", 100, 40),
      ],
    });

    const groupBounds = result.groupBounds[0];
    const gamma = result.positions.find((position) => position.id === "gamma");

    assert.ok(gamma);
    assert.equal(isPointInside(gamma, groupBounds), false);
  });
});

function isPointInside(point, bounds) {
  return (
    point.canvas_x >= bounds.x &&
    point.canvas_x <= bounds.x + bounds.w &&
    point.canvas_y >= bounds.y &&
    point.canvas_y <= bounds.y + bounds.h
  );
}
