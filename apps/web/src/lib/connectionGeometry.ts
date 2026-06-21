import { adaptiveMediaNodeSize } from "./nodePreviewLayout.js";
import type { MediaType, ProductionPreview, ReferencePackPreview } from "./api";

export interface ConnectionNodeBounds {
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export interface CanvasNodeLike {
  id: string;
  node_type?: MediaType | string;
  canvas_x: number;
  canvas_y: number;
  canvas_w: number;
  canvas_h: number;
  prompt?: string;
  production_preview?: ProductionPreview;
  reference_pack_preview?: ReferencePackPreview;
}

export interface Point {
  x: number;
  y: number;
}

export function mediaNodeBounds(
  node: CanvasNodeLike,
  livePosition?: Point | null,
): ConnectionNodeBounds {
  const size = displaySizeForNode(node);
  return {
    id: node.id,
    x: livePosition?.x ?? node.canvas_x,
    y: livePosition?.y ?? node.canvas_y,
    w: size.w,
    h: size.h,
  };
}

export function outputAnchor(bounds: ConnectionNodeBounds): Point {
  return {
    x: bounds.x + bounds.w,
    y: bounds.y + bounds.h / 2,
  };
}

export function inputAnchor(bounds: ConnectionNodeBounds): Point {
  return {
    x: bounds.x,
    y: bounds.y + bounds.h / 2,
  };
}

export function connectionPath(start: Point, end: Point): string {
  const distance = Math.abs(end.x - start.x);
  const pull = Math.max(60, Math.min(180, distance * 0.6));
  const c1 = { x: start.x + pull, y: start.y };
  const c2 = { x: end.x - pull, y: end.y };
  return `M ${round(start.x)} ${round(start.y)} C ${round(c1.x)} ${round(
    c1.y,
  )}, ${round(c2.x)} ${round(c2.y)}, ${round(end.x)} ${round(end.y)}`;
}

export function isValidConnectionTarget(
  fromNodeId: string,
  toNodeId: string | null | undefined,
): toNodeId is string {
  return Boolean(toNodeId && toNodeId !== fromNodeId);
}

function round(value: number) {
  return Math.round(value * 100) / 100;
}

function displaySizeForNode(node: CanvasNodeLike) {
  const adaptiveNode = toAdaptiveNode(node);
  if (adaptiveNode) {
    return adaptiveMediaNodeSize(adaptiveNode);
  }
  return { w: node.canvas_w, h: node.canvas_h };
}

function toAdaptiveNode(node: CanvasNodeLike): Parameters<typeof adaptiveMediaNodeSize>[0] | null {
  if (
    node.node_type === "text" ||
    node.node_type === "image" ||
    node.node_type === "video" ||
    node.node_type === "audio" ||
    node.node_type === "reference_pack"
  ) {
    return {
      node_type: node.node_type,
      canvas_w: node.canvas_w,
      canvas_h: node.canvas_h,
      prompt: node.prompt ?? "",
      production_preview: node.production_preview,
      reference_pack_preview: node.reference_pack_preview,
    };
  }
  return null;
}
