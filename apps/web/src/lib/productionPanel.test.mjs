import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
  capabilitiesForNode,
  defaultOperationForNode,
  formatJobAttempt,
  mergeNodeUpdateResponse,
  modelParamsForCapability,
  productionConfigPatchForOperation,
  productionConfigPatchForRun,
  productionConfigPatchForSelectedModel,
  simplifiedOperationOptionsForNode,
  retryDisabledReason,
  runDisabledReason,
  selectedCapabilityKey,
} from "../../dist-test/lib/productionPanel.js";

const node = {
  id: "node-1",
  workspace_id: "workspace",
  node_type: "text",
  title: "文案",
  prompt: "write a line",
  status: "draft",
  canvas_x: 0,
  canvas_y: 0,
  canvas_w: 220,
  canvas_h: 132,
  created_at: "2026-06-19T00:00:00Z",
  updated_at: "2026-06-19T00:00:00Z",
};

const capabilities = [
  {
    provider_id: "mock",
    model_id: "mock-text",
    display_name: "Mock Text",
    output_types: ["text"],
    supported_operations: ["text_generation"],
    supported_input_node_types: ["text"],
    limits: { max_attempts: 3 },
    pricing: {},
    defaults: { temperature: 0.2 },
    enabled: true,
  },
  {
    provider_id: "mock",
    model_id: "mock-video",
    display_name: "Mock Video",
    output_types: ["video"],
    supported_operations: ["text_to_video", "image_to_video"],
    supported_input_node_types: ["text", "image"],
    limits: { durations_sec: [4, 5, 8], max_attempts: 3 },
    pricing: {},
    defaults: { duration_sec: 5 },
    enabled: true,
  },
  {
    provider_id: "volcengine",
    model_id: "doubao-seedream-5-0-260128",
    display_name: "Doubao Seedream 5.0",
    output_types: ["image"],
    supported_operations: ["text_to_image", "image_to_image"],
    supported_input_node_types: ["text", "image"],
    limits: { max_attempts: 1 },
    pricing: {},
    defaults: { size: "2048x2048", response_format: "url" },
    enabled: true,
  },
];

describe("production panel helpers", () => {
  it("chooses a node-type aware default operation", () => {
    assert.equal(
      defaultOperationForNode({ ...node, node_type: "text" }),
      "text_generation",
    );
    assert.equal(
      defaultOperationForNode({ ...node, node_type: "image" }),
      "text_to_image",
    );
    assert.equal(
      defaultOperationForNode({ ...node, node_type: "video" }),
      "text_to_video",
    );
    assert.equal(
      defaultOperationForNode({ ...node, node_type: "reference_pack" }),
      "collect_references",
    );
  });

  it("filters capabilities by output type and operation", () => {
    assert.deepEqual(
      capabilitiesForNode(
        { ...node, node_type: "video" },
        capabilities,
        "text_to_video",
      ).map((capability) => capability.model_id),
      ["mock-video"],
    );
    assert.deepEqual(
      capabilitiesForNode(node, capabilities, "text_generation").map(
        (capability) => capability.model_id,
      ),
      ["mock-text"],
    );
  });

  it("shows one primary generation operation per output type", () => {
    assert.deepEqual(
      simplifiedOperationOptionsForNode({ ...node, node_type: "image" }),
      [
        { value: "text_to_image", label: "图片生成" },
        { value: "extract_first_frame", label: "提取首帧" },
        { value: "extract_last_frame", label: "提取尾帧" },
      ],
    );
    assert.deepEqual(
      simplifiedOperationOptionsForNode({ ...node, node_type: "video" }),
      [{ value: "text_to_video", label: "视频生成" }],
    );
  });

  it("picks an explicit model when it remains compatible", () => {
    assert.equal(
      selectedCapabilityKey(
        { ...node, model_provider: "mock", model_id: "mock-text" },
        capabilities,
      ),
      "mock::mock-text",
    );
  });

  it("replaces an incompatible explicit model when operation changes", () => {
    assert.deepEqual(
      productionConfigPatchForOperation(
        {
          ...node,
          node_type: "video",
          model_provider: "mock",
          model_id: "mock-text",
        },
        capabilities,
        "text_to_video",
      ),
      {
        operation_type: "text_to_video",
        model_provider: "mock",
        model_id: "mock-video",
        model_params: { duration_sec: 5 },
      },
    );
  });

  it("patches model selection with the effective node operation", () => {
    assert.deepEqual(
      productionConfigPatchForSelectedModel(
        {
          ...node,
          node_type: "image",
          operation_type: "text_generation",
          model_provider: "mock",
          model_id: "mock-text",
        },
        capabilities,
        "volcengine::doubao-seedream-5-0-260128",
      ),
      {
        operation_type: "text_to_image",
        model_provider: "volcengine",
        model_id: "doubao-seedream-5-0-260128",
        model_params: { size: "2048x2048", response_format: "url" },
      },
    );
  });

  it("requires a pre-run patch when stored operation is incompatible", () => {
    assert.deepEqual(
      productionConfigPatchForRun(
        {
          ...node,
          node_type: "image",
          operation_type: "text_generation",
          model_provider: "volcengine",
          model_id: "doubao-seedream-5-0-260128",
          model_params: {},
        },
        capabilities,
      ),
      {
        operation_type: "text_to_image",
        model_provider: "volcengine",
        model_id: "doubao-seedream-5-0-260128",
        model_params: { size: "2048x2048", response_format: "url" },
      },
    );
  });

  it("merges model params over capability defaults", () => {
    assert.deepEqual(
      modelParamsForCapability(
        { ...node, model_params: { temperature: 0.7 } },
        capabilities[0],
      ),
      { temperature: 0.7 },
    );
  });

  it("preserves locally newer prompt when a production config response returns old text", () => {
    assert.equal(
      mergeNodeUpdateResponse(
        { ...node, prompt: "new prompt" },
        { ...node, prompt: "old prompt", model_id: "mock-text" },
        { model_id: "mock-text" },
      ).prompt,
      "new prompt",
    );
    assert.equal(
      mergeNodeUpdateResponse(
        { ...node, prompt: "new prompt" },
        { ...node, prompt: "server prompt" },
        { prompt: "server prompt" },
      ).prompt,
      "server prompt",
    );
  });

  it("prevents running reference packs and running nodes", () => {
    assert.equal(
      runDisabledReason(node, null, capabilities, {
        invalid: [
          { ref: { node_id: "missing", label: "Missing", node_type: "image" } },
        ],
      }),
      "Prompt 中有失效引用，请重新选择或移除后再运行。",
    );
    assert.equal(
      runDisabledReason(
        { ...node, node_type: "reference_pack" },
        null,
        capabilities,
      ),
      "Reference Pack 在 M5.4 管理成员，不在这里运行。",
    );
    assert.equal(
      runDisabledReason({ ...node, status: "running" }, null, capabilities),
      "节点正在运行。",
    );
    assert.equal(
      runDisabledReason({ ...node, status: "queued" }, null, capabilities),
      "节点正在运行。",
    );
    assert.equal(
      runDisabledReason(
        node,
        {
          latest_job: { status: "pending" },
        },
        capabilities,
      ),
      "节点正在运行。",
    );
  });

  it("prevents running source material nodes", () => {
    assert.equal(
      runDisabledReason(
        {
          ...node,
          node_type: "text",
          operation_type: "manual",
          asset_id: null,
          status: "succeeded",
          model_params: {},
        },
        null,
        [],
        { invalid: [] },
      ),
      "素材节点不需要运行模型。",
    );
    assert.equal(
      runDisabledReason(
        {
          ...node,
          node_type: "image",
          operation_type: "upload",
          asset_id: "asset-1",
          status: "succeeded",
          model_params: {},
        },
        null,
        [],
        { invalid: [] },
      ),
      "素材节点不需要运行模型。",
    );
  });

  it("formats retry attempts", () => {
    assert.equal(
      formatJobAttempt({ attempt: 2, max_attempts: 3 }),
      "Attempt 2 / 3",
    );
  });

  it("prevents retrying a failed job after max attempts are exhausted", () => {
    assert.equal(
      retryDisabledReason({ status: "failed", attempt: 1, max_attempts: 1 }),
      "重试次数已用完。",
    );
    assert.equal(
      retryDisabledReason({ status: "failed", attempt: 1, max_attempts: 2 }),
      null,
    );
  });
});
