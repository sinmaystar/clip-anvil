import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  nodeStatusForGenerationStatus,
  overlayActiveNodeStatuses,
  productionStateWithSubmittedJob,
  runningNodeForShapeUpdate,
  shouldPollCanvasForProductionUpdates,
} from "../../dist-test/lib/canvasRunState.js";

const baseNode = {
  id: "node-1",
  workspace_id: "workspace",
  node_type: "text",
  title: "文案",
  prompt: "写一段介绍",
  status: "draft",
  canvas_x: 0,
  canvas_y: 0,
  canvas_w: 200,
  canvas_h: 120,
  created_at: "2026-06-20T00:00:00Z",
  updated_at: "2026-06-20T00:00:00Z",
};

describe("canvas run state", () => {
  it("uses the selected node as a fallback when the canvas query cache misses", () => {
    const running = runningNodeForShapeUpdate(undefined, "node-1", baseNode);

    assert.equal(running?.id, "node-1");
    assert.equal(running?.status, "running");
  });

  it("prefers the canvas cache node when present", () => {
    const running = runningNodeForShapeUpdate(
      {
        camera: { x: 0, y: 0, zoom: 1 },
        nodes: [{ ...baseNode, title: "cache node" }],
        edges: [],
        groups: [],
      },
      "node-1",
      { ...baseNode, title: "selected node" },
    );

    assert.equal(running?.title, "cache node");
    assert.equal(running?.status, "running");
  });

  it("maps queued and pending jobs to queued node display state", () => {
    assert.equal(nodeStatusForGenerationStatus("pending"), "queued");
    assert.equal(nodeStatusForGenerationStatus("queued"), "queued");
    assert.equal(nodeStatusForGenerationStatus("running"), "running");
    assert.equal(nodeStatusForGenerationStatus("succeeded"), "succeeded");
    assert.equal(nodeStatusForGenerationStatus("failed"), "failed");
    assert.equal(nodeStatusForGenerationStatus("cancelled"), "failed");
    assert.equal(nodeStatusForGenerationStatus("unknown"), null);
  });

  it("keeps a submitted generation job visible while the node is queued", () => {
    const submittedJob = {
      id: "job-2",
      workspace_id: "workspace",
      target_node_id: "node-1",
      operation_type: "text_generation",
      provider: "mock",
      model_id: "mock-text",
      intent: {},
      rendered_prompt: "new prompt",
      provider_request: {},
      provider_response: {},
      status: "queued",
      progress: 0,
      attempt: 1,
      max_attempts: 1,
      requested_by_type: "user",
      created_at: "2026-06-20T00:01:00Z",
    };

    const state = productionStateWithSubmittedJob(
      {
        node: { ...baseNode, status: "succeeded" },
        versions: [],
        latest_job: {
          ...submittedJob,
          id: "job-1",
          status: "succeeded",
          progress: 100,
          created_at: "2026-06-20T00:00:00Z",
        },
        active_stale_reasons: [],
        sandbox_jobs: [],
      },
      submittedJob,
    );

    assert.equal(state?.node.status, "queued");
    assert.equal(state?.latest_job?.id, "job-2");
    assert.equal(state?.latest_job?.status, "queued");
  });

  it("adds the submitted artifact version to production state immediately", () => {
    const submittedJob = {
      id: "job-2",
      workspace_id: "workspace",
      target_node_id: "node-1",
      operation_type: "text_generation",
      provider: "mock",
      model_id: "mock-text",
      intent: {},
      rendered_prompt: "new prompt",
      provider_request: {},
      provider_response: {},
      status: "queued",
      progress: 0,
      attempt: 1,
      max_attempts: 1,
      requested_by_type: "user",
      created_at: "2026-06-20T00:01:00Z",
    };
    const submittedVersion = {
      id: "version-2",
      workspace_id: "workspace",
      node_id: "node-1",
      job_id: "job-2",
      version_no: 2,
      winner: false,
      status: "queued",
      progress: 0,
      input_hash: "sha256:new",
      output: {},
      provider_request: {},
      provider_response: {},
      created_at: "2026-06-20T00:01:00Z",
    };

    const state = productionStateWithSubmittedJob(
      {
        node: { ...baseNode, status: "succeeded" },
        versions: [
          {
            ...submittedVersion,
            id: "version-1",
            job_id: "job-1",
            version_no: 1,
            winner: true,
            status: "succeeded",
            progress: 100,
          },
        ],
        active_stale_reasons: [],
        sandbox_jobs: [],
      },
      submittedJob,
      submittedVersion,
    );

    assert.deepEqual(
      state?.versions.map((version) => [
        version.id,
        version.version_no,
        version.status,
      ]),
      [
        ["version-1", 1, "succeeded"],
        ["version-2", 2, "queued"],
      ],
    );
  });

  it("overlays active async run state over stale canvas node state", () => {
    const [node] = overlayActiveNodeStatuses(
      [{ ...baseNode, status: "succeeded" }],
      { "node-1": "running" },
    );

    assert.equal(node.status, "running");
  });

  it("polls canvas while production nodes are active or missing previews", () => {
    assert.equal(
      shouldPollCanvasForProductionUpdates({
        camera: { x: 0, y: 0, zoom: 1 },
        nodes: [{ ...baseNode, status: "queued" }],
        edges: [],
        groups: [],
      }),
      true,
    );
    assert.equal(
      shouldPollCanvasForProductionUpdates({
        camera: { x: 0, y: 0, zoom: 1 },
        nodes: [
          {
            ...baseNode,
            status: "succeeded",
            current_version_id: "version-1",
          },
        ],
        edges: [],
        groups: [],
      }),
      true,
    );
    assert.equal(
      shouldPollCanvasForProductionUpdates({
        camera: { x: 0, y: 0, zoom: 1 },
        nodes: [
          {
            ...baseNode,
            status: "succeeded",
            current_version_id: "version-1",
            production_preview: { version_id: "version-1", version_no: 1 },
          },
        ],
        edges: [],
        groups: [],
      }),
      false,
    );
  });
});
