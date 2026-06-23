import type {
  ArtifactVersion,
  CanvasPayload,
  GenerationJob,
  MediaNode,
  NodeProductionState,
} from "./api";

export function nodeStatusForGenerationStatus(
  status: GenerationJob["status"] | string,
): MediaNode["status"] | null {
  switch (status) {
    case "pending":
    case "queued":
      return "queued";
    case "running":
      return "running";
    case "succeeded":
      return "succeeded";
    case "failed":
    case "cancelled":
      return "failed";
    default:
      return null;
  }
}

export function isActiveNodeRunStatus(status: MediaNode["status"]) {
  return status === "queued" || status === "running";
}

export function isTerminalGenerationStatus(status: GenerationJob["status"] | string) {
  return status === "succeeded" || status === "failed" || status === "cancelled";
}

export function shouldPollCanvasForProductionUpdates(
  payload: CanvasPayload | undefined,
) {
  if (!payload) {
    return false;
  }
  return payload.nodes.some((node) => {
    if (isActiveNodeRunStatus(node.status)) {
      return true;
    }
    return (
      node.status === "succeeded" &&
      Boolean(node.current_version_id) &&
      !node.production_preview
    );
  });
}

export function overlayActiveNodeStatuses(
  nodes: MediaNode[],
  statuses: Record<string, MediaNode["status"] | undefined>,
) {
  return nodes.map((node) => {
    const status = statuses[node.id];
    return status && isActiveNodeRunStatus(status)
      ? { ...node, status }
      : node;
  });
}

export function runningNodeForShapeUpdate(
  payload: CanvasPayload | undefined,
  nodeId: string,
  fallbackNode?: MediaNode | null,
) {
  const node =
    payload?.nodes.find((item) => item.id === nodeId) ??
    (fallbackNode?.id === nodeId ? fallbackNode : null);
  return node ? { ...node, status: "running" as const } : undefined;
}

export function productionStateWithSubmittedJob(
  state: NodeProductionState | undefined,
  job: GenerationJob,
  version?: ArtifactVersion,
) {
  if (!state) {
    return state;
  }
  const nodeStatus = nodeStatusForGenerationStatus(job.status) ?? "running";
  const versions = version
    ? [
        version,
        ...state.versions.filter((item) => item.id !== version.id),
      ].sort((a, b) => a.version_no - b.version_no)
    : state.versions;
  return {
    ...state,
    node: { ...state.node, status: nodeStatus },
    versions,
    latest_job: job,
  };
}
