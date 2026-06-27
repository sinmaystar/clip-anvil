import type { AgentWorkbenchArtifactSlot } from "./agentWorkbench";

export interface AgentWorkbenchMediaSize {
  width: number;
  height: number;
  aspectRatio: string;
}

export interface AgentWorkbenchMediaDimensions {
  width: number;
  height: number;
}

export type AgentWorkbenchMediaDimensionsByKey = Record<
  string,
  AgentWorkbenchMediaDimensions
>;

const mediaLimits = {
  minW: 180,
  minH: 160,
  defaultW: 480,
  defaultH: 270,
  maxW: 480,
  maxH: 380,
};

export function agentWorkbenchMediaSize(
  slot: AgentWorkbenchArtifactSlot | undefined,
  measuredDimensions?: AgentWorkbenchMediaDimensions | null,
): AgentWorkbenchMediaSize {
  const width = slot?.width ?? measuredDimensions?.width;
  const height = slot?.height ?? measuredDimensions?.height;
  const ratio =
    width && height && width > 0 && height > 0
      ? width / height
      : mediaLimits.defaultW / mediaLimits.defaultH;
  let displayWidth = mediaLimits.maxW;
  let displayHeight = Math.round(displayWidth / ratio);
  if (displayHeight > mediaLimits.maxH) {
    displayHeight = mediaLimits.maxH;
    displayWidth = Math.round(displayHeight * ratio);
  }

  return {
    width: clamp(displayWidth, mediaLimits.minW, mediaLimits.maxW),
    height: clamp(displayHeight, mediaLimits.minH, mediaLimits.maxH),
    aspectRatio: `${width && width > 0 ? width : mediaLimits.defaultW} / ${
      height && height > 0 ? height : mediaLimits.defaultH
    }`,
  };
}

export function agentWorkbenchMediaKey(
  slot: AgentWorkbenchArtifactSlot | undefined,
) {
  if (!slot) {
    return "";
  }
  return (
    slot.node_id ||
    slot.version_id ||
    slot.access_url ||
    slot.thumbnail_url ||
    `${slot.kind}:${slot.title || ""}`
  );
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, Math.round(value)));
}
