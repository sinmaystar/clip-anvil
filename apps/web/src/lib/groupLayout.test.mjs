import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  getContainingGroupId,
  getGroupMemberMovePositions,
  getGroupMemberLayoutPositions,
} from "../../dist-test/lib/groupLayout.js";

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

const node = (id, canvas_x, canvas_y, group_id = "group-a") => ({
  ...baseNode,
  id,
  title: id,
  group_id,
  canvas_x,
  canvas_y,
});

const group = {
  id: "group-a",
  workspace_id: "workspace",
  name: "分组",
  sort_order: 0,
  node_ids: ["alpha", "beta"],
};

describe("group layout", () => {
  it("moves every group member by the group drag delta", () => {
    assert.deepEqual(
      getGroupMemberMovePositions({
        group,
        nodes: [node("alpha", 40, 60), node("beta", 280, 60), node("gamma", 0, 0, null)],
        deltaX: 24,
        deltaY: -12,
      }),
      [
        { id: "alpha", canvas_x: 64, canvas_y: 48 },
        { id: "beta", canvas_x: 304, canvas_y: 48 },
      ],
    );
  });

  it("lays out members inside the group instead of preserving far away coordinates", () => {
    assert.deepEqual(
      getGroupMemberLayoutPositions({
        group: { ...group, node_ids: ["alpha", "beta", "gamma"] },
        nodes: [
          node("alpha", 20, 80),
          node("beta", 300, 80),
          node("gamma", 2000, 2000),
        ],
        groupX: 0,
        groupY: 0,
      }),
      [
        { id: "alpha", canvas_x: 20, canvas_y: 44 },
        { id: "beta", canvas_x: 244, canvas_y: 44 },
        { id: "gamma", canvas_x: 20, canvas_y: 188 },
      ],
    );
  });

  it("finds the group containing a dragged node center", () => {
    assert.equal(
      getContainingGroupId({
        point: { x: 180, y: 120 },
        bounds: [
          { groupId: "group-a", x: 0, y: 0, w: 240, h: 180 },
          { groupId: "group-b", x: 300, y: 0, w: 240, h: 180 },
        ],
      }),
      "group-a",
    );
    assert.equal(
      getContainingGroupId({
        point: { x: 280, y: 120 },
        bounds: [{ groupId: "group-a", x: 0, y: 0, w: 240, h: 180 }],
      }),
      null,
    );
  });
});
