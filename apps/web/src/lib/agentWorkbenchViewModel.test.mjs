import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentWorkbenchToFlow,
  sceneNodeId,
  shotNodeId,
} from "../../dist-test/lib/agentWorkbenchViewModel.js";

const workbench = {
  overview: {
    workspace_id: "workspace-1",
    brief: {
      id: "brief-1",
      title: "行李箱广告",
      concept: "抖音种草",
      status: "active",
    },
    key_elements: [],
    key_element_states: [],
    source_materials: [],
  },
  scenes: [
    {
      id: "scene-1",
      title: "机场出行",
      status: "planned",
      shots: [
        {
          id: "shot-2",
          client_key: "shot_02",
          title: "第二镜",
          status: "planned",
          sequence_index: 2,
          creative_text: "展示耐磨",
          dependencies: [],
          key_elements: [],
          preview: { kind: "preview_image", status: "missing" },
          video: { kind: "shot_video", status: "missing" },
          artifacts: [],
          render_plans: [],
          issues: [],
        },
        {
          id: "shot-1",
          client_key: "shot_01",
          title: "第一镜",
          status: "planned",
          sequence_index: 1,
          creative_text: "机场亮相",
          dependencies: [],
          key_elements: [],
          preview: {
            kind: "preview_image",
            status: "succeeded",
            thumbnail_url: "preview.png",
          },
          video: { kind: "shot_video", status: "queued" },
          artifacts: [
            {
              kind: "preview_image",
              status: "succeeded",
              thumbnail_url: "preview-a.png",
            },
            {
              kind: "preview_image",
              status: "running",
            },
          ],
          render_plans: [],
          issues: [],
        },
      ],
    },
  ],
  counts: {
    scenes: 1,
    shots: 2,
    preview_succeeded: 1,
    preview_failed: 0,
    video_succeeded: 0,
    video_failed: 0,
    open_issues: 0,
    needs_reference: 0,
  },
};

describe("agent workbench view model", () => {
  it("creates overview, scene, and shot nodes without render plan node spam", () => {
    const flow = agentWorkbenchToFlow(workbench);

    assert.deepEqual(
      flow.nodes.map((node) => node.id),
      [
        "agent-workbench-overview",
        "agent-scene-scene-1",
        "agent-shot-shot-1",
        "agent-shot-shot-2",
      ],
    );
    assert.equal(flow.edges.length, 2);
    assert.equal(
      flow.nodes.some((node) => node.id.includes("render-plan")),
      false,
    );
  });

  it("sorts shots by sequence index and lays them inside the scene lane", () => {
    const flow = agentWorkbenchToFlow(workbench);
    const shot1 = flow.nodes.find((node) => node.id === shotNodeId("shot-1"));
    const shot2 = flow.nodes.find((node) => node.id === shotNodeId("shot-2"));

    assert.ok(shot1);
    assert.ok(shot2);
    assert.equal(shot1.parentId, "agent-scene-scene-1");
    assert.equal(shot2.parentId, "agent-scene-scene-1");
    assert.ok(shot1.position.x < shot2.position.x);
    assert.equal(shot1.position.y, shot2.position.y);
    assert.ok(shot1.position.y >= 100);
  });

  it("wraps many shots into scene rows instead of one horizontal strip", () => {
    const manyShotWorkbench = {
      ...workbench,
      scenes: [
        {
          ...workbench.scenes[0],
          shots: Array.from({ length: 5 }, (_, index) => ({
            ...workbench.scenes[0].shots[0],
            id: `shot-${index + 1}`,
            client_key: `shot_0${index + 1}`,
            sequence_index: index + 1,
          })),
        },
      ],
    };
    const flow = agentWorkbenchToFlow(manyShotWorkbench);
    const scene = flow.nodes.find((node) => node.id === sceneNodeId("scene-1"));
    const shot1 = flow.nodes.find((node) => node.id === shotNodeId("shot-1"));
    const shot2 = flow.nodes.find((node) => node.id === shotNodeId("shot-2"));
    const shot3 = flow.nodes.find((node) => node.id === shotNodeId("shot-3"));

    assert.ok(scene);
    assert.ok(shot1);
    assert.ok(shot2);
    assert.ok(shot3);
    assert.equal(shot1.position.y, shot2.position.y);
    assert.ok(shot3.position.y > shot1.position.y);
    assert.ok(Number(scene.height) > Number(shot1.height) * 2);
  });
});
