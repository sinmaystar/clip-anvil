import type { MediaNode, MediaType } from "./api";
import { winnerPreviewText } from "./productionPreview.js";

export interface AdaptiveNodeSize {
  w: number;
  h: number;
  sizeMode: "auto" | "persisted";
}

export interface MediaDimensions {
  width: number;
  height: number;
}

interface NodePreviewLimit {
  minW: number;
  minH: number;
  defaultW: number;
  defaultH: number;
  maxW: number;
  maxH: number;
}

export const mediaNodePreviewLimits: Record<MediaType, NodePreviewLimit> = {
  text: {
    minW: 280,
    minH: 160,
    defaultW: 340,
    defaultH: 220,
    maxW: 380,
    maxH: 300,
  },
  image: {
    minW: 190,
    minH: 190,
    defaultW: 360,
    defaultH: 280,
    maxW: 440,
    maxH: 380,
  },
  video: {
    minW: 280,
    minH: 188,
    defaultW: 400,
    defaultH: 260,
    maxW: 480,
    maxH: 330,
  },
  audio: {
    minW: 320,
    minH: 120,
    defaultW: 360,
    defaultH: 140,
    maxW: 460,
    maxH: 180,
  },
  reference_pack: {
    minW: 320,
    minH: 180,
    defaultW: 360,
    defaultH: 220,
    maxW: 520,
    maxH: 360,
  },
};

type SizableNode = Pick<
  MediaNode,
  | "node_type"
  | "canvas_w"
  | "canvas_h"
  | "prompt"
  | "production_preview"
  | "reference_pack_preview"
>;

export function adaptiveMediaNodeSize(
  node: SizableNode,
  measuredMediaDimensions?: MediaDimensions | null,
): AdaptiveNodeSize {
  const limits = mediaNodePreviewLimits[node.node_type];
  if (hasPersistedDisplaySize(node.canvas_w, node.canvas_h, limits)) {
    return {
      w: Math.round(node.canvas_w),
      h: Math.round(node.canvas_h),
      sizeMode: "persisted",
    };
  }

  if (node.node_type === "text") {
    return textNodeSize(node, limits);
  }
  if (node.node_type === "image") {
    return mediaRatioSize(
      node.production_preview?.width ?? measuredMediaDimensions?.width,
      node.production_preview?.height ?? measuredMediaDimensions?.height,
      limits,
      4 / 3,
    );
  }
  if (node.node_type === "video") {
    return mediaRatioSize(
      node.production_preview?.width,
      node.production_preview?.height,
      limits,
      16 / 9,
    );
  }
  return {
    w: limits.defaultW,
    h: limits.defaultH,
    sizeMode: "auto",
  };
}

function hasPersistedDisplaySize(
  width: number,
  height: number,
  limits: NodePreviewLimit,
) {
  return width > limits.defaultW + 24 || height > limits.defaultH + 24;
}

function textNodeSize(
  node: Pick<
    MediaNode,
    "node_type" | "prompt" | "production_preview" | "reference_pack_preview"
  >,
  limits: NodePreviewLimit,
): AdaptiveNodeSize {
  const text = winnerPreviewText(node);
  const lines = Math.max(1, text.split("\n").length);
  const chars = text.length;
  const estimatedWrappedLines = Math.ceil(chars / 42);
  const bodyLines = Math.max(lines, estimatedWrappedLines);
  const h = clamp(102 + bodyLines * 18, limits.minH, limits.maxH);
  return { w: limits.defaultW, h, sizeMode: "auto" };
}

function mediaRatioSize(
  width: number | undefined,
  height: number | undefined,
  limits: NodePreviewLimit,
  fallbackRatio: number,
): AdaptiveNodeSize {
  const ratio = width && height && width > 0 && height > 0 ? width / height : fallbackRatio;
  let w = limits.maxW;
  let h = Math.round(w / ratio);
  if (h > limits.maxH) {
    h = limits.maxH;
    w = Math.round(h * ratio);
  }
  return {
    w: clamp(w, limits.minW, limits.maxW),
    h: clamp(h, limits.minH, limits.maxH),
    sizeMode: "auto",
  };
}

function clamp(value: number, min: number, max: number) {
  return Math.min(max, Math.max(min, Math.round(value)));
}
