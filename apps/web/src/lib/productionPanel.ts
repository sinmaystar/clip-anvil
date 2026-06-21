import type {
  GenerationJob,
  MediaNode,
  ModelCapability,
  NodeProductionState,
  OperationType,
} from "./api";
import {
  runDisabledReasonForPromptRefs,
  type InputReferenceState,
} from "./promptRefs.js";
import { nodeStatusForGenerationStatus } from "./canvasRunState.js";
import { isSourceMaterialNode } from "./sourceMaterial.js";

export const operationLabels: Record<OperationType, string> = {
  manual: "手动",
  upload: "上传素材",
  collect_references: "收集参考",
  text_generation: "文本生成",
  text_to_image: "文生图",
  image_to_image: "图生图",
  multi_image_to_image: "多图生图",
  text_to_video: "文生视频",
  image_to_video: "图生视频",
  video_to_video: "视频改写",
  multi_reference_to_video: "多参考生视频",
  extract_first_frame: "提取首帧",
  extract_last_frame: "提取尾帧",
};

export interface OperationOption {
  value: OperationType;
  label: string;
}

const defaultOperationByNodeType: Record<
  MediaNode["node_type"],
  OperationType
> = {
  text: "text_generation",
  image: "text_to_image",
  video: "text_to_video",
  audio: "manual",
  reference_pack: "collect_references",
};

const simplifiedOperationsByNodeType: Record<
  MediaNode["node_type"],
  OperationOption[]
> = {
  text: [{ value: "text_generation", label: "文本生成" }],
  image: [
    { value: "text_to_image", label: "图片生成" },
    { value: "extract_first_frame", label: "提取首帧" },
    { value: "extract_last_frame", label: "提取尾帧" },
  ],
  video: [{ value: "text_to_video", label: "视频生成" }],
  audio: [{ value: "manual", label: "手动" }],
  reference_pack: [{ value: "collect_references", label: "收集参考" }],
};

export function defaultOperationForNode(
  node: Pick<MediaNode, "node_type" | "operation_type">,
) {
  if (
    isKnownOperation(node.operation_type) &&
    simplifiedOperationsByNodeType[node.node_type].some(
      (option) => option.value === node.operation_type,
    )
  ) {
    return node.operation_type;
  }
  return defaultOperationByNodeType[node.node_type];
}

export function simplifiedOperationOptionsForNode(
  node: Pick<MediaNode, "node_type">,
): OperationOption[] {
  return simplifiedOperationsByNodeType[node.node_type];
}

export function capabilitiesForNode(
  node: Pick<MediaNode, "node_type">,
  capabilities: ModelCapability[],
  operation: string,
) {
  return capabilities.filter(
    (capability) =>
      capability.enabled &&
      capability.output_types.includes(node.node_type) &&
      capability.supported_operations.includes(operation),
  );
}

export function capabilityKey(
  capability: Pick<ModelCapability, "provider_id" | "model_id">,
) {
  return `${capability.provider_id}::${capability.model_id}`;
}

export function selectedCapabilityKey(
  node: Pick<
    MediaNode,
    "node_type" | "operation_type" | "model_provider" | "model_id"
  >,
  capabilities: ModelCapability[],
) {
  const operation = defaultOperationForNode(node);
  const compatible = capabilitiesForNode(node, capabilities, operation);
  const explicit = compatible.find(
    (capability) =>
      capability.provider_id === node.model_provider &&
      capability.model_id === node.model_id,
  );
  const selected = explicit ?? compatible[0];
  return selected ? capabilityKey(selected) : "";
}

export function splitCapabilityKey(key: string) {
  const [provider = "", modelId = ""] = key.split("::");
  return { provider, modelId };
}

export function modelParamsForCapability(
  node: Pick<MediaNode, "model_params">,
  capability?: Pick<ModelCapability, "defaults">,
) {
  return {
    ...(capability?.defaults ?? {}),
    ...objectParamMap(node.model_params),
  };
}

export function productionConfigPatchForOperation(
  node: Pick<
    MediaNode,
    "node_type" | "model_provider" | "model_id" | "model_params"
  >,
  capabilities: ModelCapability[],
  operation: OperationType | string,
) {
  const compatible = capabilitiesForNode(node, capabilities, operation);
  const explicit = compatible.find(
    (capability) =>
      capability.provider_id === node.model_provider &&
      capability.model_id === node.model_id,
  );
  const capability = explicit ?? compatible[0];
  return {
    operation_type: operation,
    model_provider: capability?.provider_id ?? "",
    model_id: capability?.model_id ?? "",
    model_params: modelParamsForCapability(node, capability),
  };
}

export function productionConfigPatchForSelectedModel(
  node: Pick<
    MediaNode,
    "node_type" | "operation_type" | "model_provider" | "model_id" | "model_params"
  >,
  capabilities: ModelCapability[],
  selectedKey: string,
) {
  const operation = defaultOperationForNode(node);
  const { provider, modelId } = splitCapabilityKey(selectedKey);
  const compatible = capabilitiesForNode(node, capabilities, operation);
  const capability = compatible.find(
    (item) => item.provider_id === provider && item.model_id === modelId,
  );
  return {
    operation_type: operation,
    model_provider: provider,
    model_id: modelId,
    model_params: modelParamsForCapability(node, capability),
  };
}

export function productionConfigPatchForRun(
  node: Pick<
    MediaNode,
    "node_type" | "operation_type" | "model_provider" | "model_id" | "model_params"
  >,
  capabilities: ModelCapability[],
) {
  const patch = productionConfigPatchForOperation(
    node,
    capabilities,
    defaultOperationForNode(node),
  );
  const changed =
    patch.operation_type !== node.operation_type ||
    patch.model_provider !== node.model_provider ||
    patch.model_id !== node.model_id ||
    JSON.stringify(patch.model_params) !==
      JSON.stringify(objectParamMap(node.model_params));
  return changed ? patch : null;
}

export function runDisabledReason(
  node: MediaNode,
  state: NodeProductionState | null,
  capabilities: ModelCapability[],
  promptRefState?: Pick<InputReferenceState, "invalid">,
) {
  const promptRefReason = promptRefState
    ? runDisabledReasonForPromptRefs(promptRefState)
    : null;
  if (promptRefReason) {
    return promptRefReason;
  }
  if (node.node_type === "reference_pack") {
    return "Reference Pack 在 M5.4 管理成员，不在这里运行。";
  }
  if (isSourceMaterialNode(node)) {
    return "素材节点不需要运行模型。";
  }
  const latestJobNodeStatus = state?.latest_job
    ? nodeStatusForGenerationStatus(state.latest_job.status)
    : null;
  if (
    node.status === "queued" ||
    node.status === "running" ||
    latestJobNodeStatus === "queued" ||
    latestJobNodeStatus === "running"
  ) {
    return "节点正在运行。";
  }
  const operation = defaultOperationForNode(node);
  if (capabilitiesForNode(node, capabilities, operation).length === 0) {
    return "没有兼容当前节点类型和 Operation 的模型。";
  }
  return null;
}

export function formatJobAttempt(
  job: Pick<GenerationJob, "attempt" | "max_attempts">,
) {
  return `Attempt ${job.attempt} / ${job.max_attempts}`;
}

export function retryDisabledReason(
  job: Pick<GenerationJob, "status" | "attempt" | "max_attempts"> | null,
) {
  if (!job || job.status !== "failed") {
    return null;
  }
  return job.attempt >= job.max_attempts ? "重试次数已用完。" : null;
}

export function mergeNodeUpdateResponse(
  currentNode: MediaNode | undefined,
  responseNode: MediaNode,
  patch: Partial<
    Pick<MediaNode, "title" | "prompt" | "prompt_refs" | "prompt_rich">
  >,
) {
  if (!currentNode) {
    return responseNode;
  }
  return {
    ...responseNode,
    canvas_x: currentNode.canvas_x,
    canvas_y: currentNode.canvas_y,
    title: "title" in patch ? responseNode.title : currentNode.title,
    prompt: "prompt" in patch ? responseNode.prompt : currentNode.prompt,
    prompt_refs:
      "prompt_refs" in patch ? responseNode.prompt_refs : currentNode.prompt_refs,
    prompt_rich:
      "prompt_rich" in patch ? responseNode.prompt_rich : currentNode.prompt_rich,
  };
}

function isKnownOperation(value: unknown): value is OperationType {
  return typeof value === "string" && value in operationLabels;
}

function objectParamMap(value: unknown) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return value as Record<string, unknown>;
}
