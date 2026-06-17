import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  getEdgeDetail,
  getGroupMembers,
  getNodeDependencies,
  getResourceTreeSections,
  getUngroupedNodes,
  nodeIdsWith,
  nodeIdsWithout,
} from "../../dist-test/lib/canvasSelectors.js";

const baseNode = {
  workspace_id: "workspace",
  node_type: "text",
  title: "",
  prompt: "",
  status: "draft",
  canvas_x: 0,
  canvas_y: 0,
  canvas_w: 220,
  canvas_h: 132,
  created_at: "2026-06-17T00:00:00Z",
  updated_at: "2026-06-17T00:00:00Z",
};

const node = (id, title, group_id = null) => ({
  ...baseNode,
  id,
  title,
  group_id,
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

describe("canvas selectors", () => {
  const nodes = [
    node("copy", "文案", "group-a"),
    node("voice", "旁白", "group-a"),
    node("video", "成片"),
  ];
  const edges = [
    edge("copy-to-voice", "copy", "voice"),
    edge("voice-to-video", "voice", "video"),
  ];
  const groups = [
    {
      id: "group-a",
      workspace_id: "workspace",
      name: "素材组",
      sort_order: 0,
      node_ids: ["copy", "voice"],
    },
  ];

  it("returns upstream and downstream nodes for a selected node", () => {
    const dependencies = getNodeDependencies(nodes, edges, "voice");

    assert.deepEqual(
      dependencies.upstream.map((item) => item.id),
      ["copy"],
    );
    assert.deepEqual(
      dependencies.downstream.map((item) => item.id),
      ["video"],
    );
  });

  it("returns both endpoints for a selected edge", () => {
    const detail = getEdgeDetail(nodes, edges, "voice-to-video");

    assert.equal(detail?.edge.id, "voice-to-video");
    assert.equal(detail?.fromNode.title, "旁白");
    assert.equal(detail?.toNode.title, "成片");
  });

  it("returns group members and ungrouped nodes", () => {
    assert.deepEqual(
      getGroupMembers(nodes, groups[0]).map((item) => item.id),
      ["copy", "voice"],
    );
    assert.deepEqual(
      getUngroupedNodes(nodes, groups).map((item) => item.id),
      ["video"],
    );
  });

  it("treats null group node ids from the API as an empty member list", () => {
    const emptyGroup = { ...groups[0], node_ids: null };
    const sections = getResourceTreeSections(nodes, [emptyGroup], {
      query: "",
      type: "all",
    });

    assert.deepEqual(getGroupMembers(nodes, emptyGroup), []);
    assert.deepEqual(
      getUngroupedNodes(nodes, [emptyGroup]).map((item) => item.id),
      ["copy", "voice", "video"],
    );
    assert.equal(sections.groups[0].memberCount, 0);
    assert.deepEqual(sections.groups[0].nodes, []);
  });

  it("removes a node id from a group member list", () => {
    assert.deepEqual(nodeIdsWithout(["copy", "voice"], "copy"), ["voice"]);
    assert.deepEqual(nodeIdsWithout(["copy", "voice"], "video"), [
      "copy",
      "voice",
    ]);
  });

  it("adds a node id to a group member list without replacing existing members", () => {
    assert.deepEqual(nodeIdsWith(["copy"], "voice"), ["copy", "voice"]);
    assert.deepEqual(nodeIdsWith(["copy", "voice"], "voice"), [
      "copy",
      "voice",
    ]);
    assert.deepEqual(nodeIdsWith(null, "copy"), ["copy"]);
  });

  it("keeps matching groups visible when search matches the group name", () => {
    const sections = getResourceTreeSections(nodes, groups, {
      query: "素材",
      type: "all",
    });

    assert.deepEqual(
      sections.groups.map((section) => [
        section.group.id,
        section.nodes.map((item) => item.id),
      ]),
      [["group-a", ["copy", "voice"]]],
    );
    assert.deepEqual(sections.ungroupedNodes, []);
  });

  it("applies type filters to nodes without removing group headers", () => {
    const mixedNodes = [
      node("copy", "文案", "group-a"),
      { ...node("image", "产品图", "group-a"), node_type: "image" },
      { ...node("video", "成片"), node_type: "video" },
    ];
    const mixedGroups = [{ ...groups[0], node_ids: ["copy", "image"] }];
    const sections = getResourceTreeSections(mixedNodes, mixedGroups, {
      query: "",
      type: "image",
    });

    assert.equal(sections.groups[0].group.id, "group-a");
    assert.deepEqual(
      sections.groups[0].nodes.map((item) => item.id),
      ["image"],
    );
    assert.deepEqual(sections.ungroupedNodes, []);
  });
});
