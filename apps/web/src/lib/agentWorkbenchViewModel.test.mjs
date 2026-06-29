import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  agentWorkbenchToFlow,
  audioNodeId,
  finalOutputNodeId,
  sceneNodeId,
  shotNodeId,
} from "../../dist-test/lib/agentWorkbenchViewModel.js";

function nodeMinHeight(node) {
  return Number(node?.style?.minHeight ?? node?.height ?? 0);
}

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

  it("adds a final output node when backend projection has final_output", () => {
    const finalWorkbench = {
      ...workbench,
      final_output: {
        id: "timeline-1",
        timeline_plan_id: "timeline-1",
        output_node_id: "final-node-1",
        artifact_version_id: "final-version-1",
        sandbox_job_id: "sandbox-job-1",
        status: "completed",
        template_key: "concat_with_fades",
        summary: "rendered with fades",
        asset_url: "final.mp4",
        updated_at: "2026-06-27T00:00:00Z",
      },
    };
    const flow = agentWorkbenchToFlow(finalWorkbench);
    const finalNode = flow.nodes.find(
      (node) => node.id === finalOutputNodeId("timeline-1"),
    );

    assert.ok(finalNode);
    assert.equal(finalNode.type, "agentFinalOutput");
    assert.equal(finalNode.data.finalOutput.status, "completed");
    assert.ok(finalNode.position.x > 0);
  });

  it("creates standalone audio nodes from the active audio plan", () => {
    const audioWorkbench = {
      ...workbench,
      overview: {
        ...workbench.overview,
        audio_plan: {
          id: "audio-plan-1",
          status: "composing",
          title: "音频方案",
          voiceover_status: "succeeded",
          bgm_status: "succeeded",
          voiceover_artifact: {
            kind: "voiceover_audio",
            status: "succeeded",
            node_id: "voice-node-1",
            title: "Voiceover",
            access_url: "voice.mp3",
          },
          bgm_artifact: {
            kind: "bgm_audio",
            status: "succeeded",
            node_id: "bgm-node-1",
            title: "BGM",
            access_url: "bgm.mp3",
          },
        },
      },
    };
    const flow = agentWorkbenchToFlow(audioWorkbench);
    const voiceNode = flow.nodes.find((node) => node.id === audioNodeId("voice-node-1"));
    const bgmNode = flow.nodes.find((node) => node.id === audioNodeId("bgm-node-1"));

    assert.ok(voiceNode);
    assert.ok(bgmNode);
    assert.equal(voiceNode.type, "agentAudio");
    assert.equal(bgmNode.type, "agentAudio");
    assert.equal(voiceNode.parentId, undefined);
    assert.equal(voiceNode.data.label, "VO");
    assert.ok(voiceNode.position.y > 0);
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
    assert.ok(Number(scene.height) > nodeMinHeight(shot1) * 2);
  });

  it("lays multiple compact scenes across columns instead of one vertical strip", () => {
    const multiSceneWorkbench = {
      ...workbench,
      scenes: Array.from({ length: 4 }, (_, index) => ({
        ...workbench.scenes[0],
        id: `scene-${index + 1}`,
        title: `场景 ${index + 1}`,
        shots: [
          {
            ...workbench.scenes[0].shots[0],
            id: `shot-${index + 1}`,
            client_key: `shot_0${index + 1}`,
            sequence_index: index + 1,
          },
        ],
      })),
    };
    const flow = agentWorkbenchToFlow(multiSceneWorkbench);
    const scene1 = flow.nodes.find((node) => node.id === sceneNodeId("scene-1"));
    const scene2 = flow.nodes.find((node) => node.id === sceneNodeId("scene-2"));
    const scene3 = flow.nodes.find((node) => node.id === sceneNodeId("scene-3"));

    assert.ok(scene1);
    assert.ok(scene2);
    assert.ok(scene3);
    assert.ok(scene2.position.x > scene1.position.x);
    assert.equal(scene2.position.y, scene1.position.y);
    assert.equal(scene3.position.x, scene1.position.x);
    assert.ok(scene3.position.y > scene1.position.y);
  });

  it("packs preview and video media side by side when a shot can fit them", () => {
    const denseMediaWorkbench = {
      ...workbench,
      scenes: [
        {
          ...workbench.scenes[0],
          shots: [
            {
              ...workbench.scenes[0].shots[0],
              id: "shot-dense",
              client_key: "shot_dense",
              sequence_index: 1,
              artifacts: [
                {
                  kind: "preview_image",
                  status: "succeeded",
                  node_id: "preview-dense",
                  width: 2048,
                  height: 2048,
                },
                {
                  kind: "shot_video",
                  status: "succeeded",
                  node_id: "video-dense",
                  width: 496,
                  height: 864,
                },
              ],
            },
          ],
        },
      ],
    };
    const flow = agentWorkbenchToFlow(denseMediaWorkbench);
    const shot = flow.nodes.find((node) => node.id === shotNodeId("shot-dense"));

    assert.ok(shot);
    assert.ok(
      nodeMinHeight(shot) <= 620,
      `shot minHeight ${nodeMinHeight(shot)} should stay dense`,
    );
  });

  it("uses measured shot heights to relayout masonry after rendered content expands", () => {
    const measuredWorkbench = {
      ...workbench,
      scenes: [
        {
          ...workbench.scenes[0],
          shots: [
            {
              ...workbench.scenes[0].shots[0],
              id: "shot-measured-1",
              client_key: "shot_01",
              sequence_index: 1,
            },
            {
              ...workbench.scenes[0].shots[0],
              id: "shot-measured-2",
              client_key: "shot_02",
              sequence_index: 2,
            },
            {
              ...workbench.scenes[0].shots[0],
              id: "shot-measured-3",
              client_key: "shot_03",
              sequence_index: 3,
            },
          ],
        },
      ],
    };
    const flow = agentWorkbenchToFlow(
      measuredWorkbench,
      {},
      { "shot-measured-1": 860 },
    );
    const scene = flow.nodes.find((node) => node.id === sceneNodeId("scene-1"));
    const shot1 = flow.nodes.find((node) => node.id === shotNodeId("shot-measured-1"));
    const shot2 = flow.nodes.find((node) => node.id === shotNodeId("shot-measured-2"));
    const shot3 = flow.nodes.find((node) => node.id === shotNodeId("shot-measured-3"));

    assert.ok(scene);
    assert.ok(shot1);
    assert.ok(shot2);
    assert.ok(shot3);
    assert.equal(shot1.height, 860);
    assert.ok(shot3.position.x > shot1.position.x);
    assert.equal(shot3.position.y, shot2.position.y + nodeMinHeight(shot2) + 32);
    assert.ok(Number(scene.height) > 860);
  });

  it("uses masonry layout so tall video shots do not stretch neighboring shots", () => {
    const masonryWorkbench = {
      ...workbench,
      scenes: [
        {
          ...workbench.scenes[0],
          shots: [
            {
              ...workbench.scenes[0].shots[0],
              id: "shot-1",
              client_key: "shot_01",
              sequence_index: 1,
              artifacts: [
                {
                  kind: "preview_image",
                  status: "succeeded",
                  node_id: "preview-1",
                  width: 1024,
                  height: 1024,
                },
              ],
            },
            {
              ...workbench.scenes[0].shots[0],
              id: "shot-2",
              client_key: "shot_02",
              sequence_index: 2,
              artifacts: [
                {
                  kind: "preview_image",
                  status: "succeeded",
                  node_id: "preview-2",
                  width: 1024,
                  height: 1024,
                },
                {
                  kind: "shot_video",
                  status: "succeeded",
                  node_id: "video-2",
                  width: 1280,
                  height: 720,
                },
              ],
            },
            {
              ...workbench.scenes[0].shots[0],
              id: "shot-3",
              client_key: "shot_03",
              sequence_index: 3,
              artifacts: [
                {
                  kind: "preview_image",
                  status: "succeeded",
                  node_id: "preview-3",
                  width: 1024,
                  height: 1024,
                },
              ],
            },
          ],
        },
      ],
    };

    const flow = agentWorkbenchToFlow(masonryWorkbench);
    const shot1 = flow.nodes.find((node) => node.id === shotNodeId("shot-1"));
    const shot2 = flow.nodes.find((node) => node.id === shotNodeId("shot-2"));
    const shot3 = flow.nodes.find((node) => node.id === shotNodeId("shot-3"));

    assert.ok(shot1);
    assert.ok(shot2);
    assert.ok(shot3);
    assert.ok(nodeMinHeight(shot2) > nodeMinHeight(shot1));
    assert.notEqual(nodeMinHeight(shot1), nodeMinHeight(shot2));
    assert.equal(shot1.position.y, shot2.position.y);
    assert.equal(shot3.position.x, shot1.position.x);
    assert.equal(
      shot3.position.y,
      shot1.position.y + nodeMinHeight(shot1) + 32,
    );
  });
});
