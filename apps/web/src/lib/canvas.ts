import {
  MEDIA_SHAPE_TYPE,
  type MediaShape,
  type MediaShapeProps,
} from "@clip-anvil/canvas-schema";
import { createShapeId, type TLShapePartial } from "tldraw";
import type { MediaNode } from "./api";

export function shapeIdForNode(nodeId: string) {
  return createShapeId(`media-${nodeId}`);
}

export function nodeToShape(node: MediaNode): TLShapePartial<MediaShape> {
  return {
    id: shapeIdForNode(node.id),
    type: MEDIA_SHAPE_TYPE,
    x: node.canvas_x,
    y: node.canvas_y,
    props: nodeToShapeProps(node),
  };
}

export function nodeToShapeProps(node: MediaNode): MediaShapeProps {
  return {
    nodeId: node.id,
    nodeType: node.node_type,
    title: node.title || "未命名文本",
    prompt: node.prompt,
    status: node.status,
    w: node.canvas_w,
    h: node.canvas_h,
  };
}

export function isMediaShape(shape: unknown): shape is MediaShape {
  return (
    typeof shape === "object" &&
    shape !== null &&
    "type" in shape &&
    (shape as { type?: string }).type === MEDIA_SHAPE_TYPE
  );
}
