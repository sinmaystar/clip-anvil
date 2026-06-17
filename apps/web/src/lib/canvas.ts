import {
  MEDIA_SHAPE_TYPE,
  GROUP_CONTAINER_SHAPE_TYPE,
  type MediaShape,
  type MediaShapeProps,
  type GroupContainerShape,
} from "@clip-anvil/canvas-schema";
import {
  createBindingId,
  createShapeId,
  toRichText,
  type TLArrowBinding,
  type TLArrowShape,
  type TLBindingCreate,
  type TLShapePartial,
} from "tldraw";
import type { MediaEdge, MediaNode } from "./api";
import type { MediaGroup } from "./api";

export function shapeIdForNode(nodeId: string) {
  return createShapeId(`media-${nodeId}`);
}

export function shapeIdForEdge(edgeId: string) {
  return createShapeId(`edge-${edgeId}`);
}

export function shapeIdForGroup(groupId: string) {
  return createShapeId(`group-${groupId}`);
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

export function groupToShape(
  group: MediaGroup,
  nodes: MediaNode[],
): TLShapePartial<GroupContainerShape> {
  const bounds = boundsForNodes(
    nodes.filter((node) => group.node_ids.includes(node.id)),
  );
  return {
    id: shapeIdForGroup(group.id),
    type: GROUP_CONTAINER_SHAPE_TYPE,
    x: bounds.x - 20,
    y: bounds.y - 44,
    props: {
      groupId: group.id,
      name: group.name,
      nodeCount: group.node_ids.length,
      w: Math.max(240, bounds.w + 40),
      h: Math.max(120, bounds.h + 64),
    },
  };
}

export function edgeToArrow(
  edge: MediaEdge,
  nodes: MediaNode[],
): {
  arrow: TLShapePartial<TLArrowShape>;
  bindings: TLBindingCreate<TLArrowBinding>[];
} | null {
  const fromNode = nodes.find((node) => node.id === edge.from_node_id);
  const toNode = nodes.find((node) => node.id === edge.to_node_id);
  if (!fromNode || !toNode) {
    return null;
  }

  const arrowId = shapeIdForEdge(edge.id);
  const fromShapeId = shapeIdForNode(edge.from_node_id);
  const toShapeId = shapeIdForNode(edge.to_node_id);
  const start = {
    x: fromNode.canvas_w,
    y: fromNode.canvas_h / 2,
  };
  const end = {
    x: toNode.canvas_x - (fromNode.canvas_x + fromNode.canvas_w),
    y: toNode.canvas_y + toNode.canvas_h / 2 - (fromNode.canvas_y + fromNode.canvas_h / 2),
  };

  return {
    arrow: {
      id: arrowId,
      type: "arrow",
      x: fromNode.canvas_x,
      y: fromNode.canvas_y,
      props: {
        kind: "arc",
        labelColor: "black",
        color: "blue",
        fill: "none",
        dash: "draw",
        size: "m",
        arrowheadStart: "none",
        arrowheadEnd: "arrow",
        font: "draw",
        start,
        end,
        bend: 84,
        richText: toRichText(""),
        labelPosition: 0.5,
        scale: 1,
        elbowMidPoint: 0.5,
      },
      meta: {
        edgeId: edge.id,
        edgeType: edge.edge_type,
      },
    },
    bindings: [
      {
        id: createBindingId(),
        type: "arrow",
        fromId: arrowId,
        toId: fromShapeId,
        props: {
          terminal: "start",
          normalizedAnchor: { x: 1, y: 0.5 },
          isExact: false,
          isPrecise: true,
          snap: "edge",
        },
      },
      {
        id: createBindingId(),
        type: "arrow",
        fromId: arrowId,
        toId: toShapeId,
        props: {
          terminal: "end",
          normalizedAnchor: { x: 0, y: 0.5 },
          isExact: false,
          isPrecise: true,
          snap: "edge",
        },
      },
    ],
  };
}

export function nodeToShapeProps(node: MediaNode): MediaShapeProps {
  return {
    nodeId: node.id,
    nodeType: node.node_type,
    title: node.title || `未命名${nodeTypeLabel(node.node_type)}`,
    prompt: node.prompt,
    status: node.status,
    thumbnailUrl: node.thumbnail_url,
    w: node.canvas_w,
    h: node.canvas_h,
  };
}

function nodeTypeLabel(nodeType: MediaNode["node_type"]) {
  switch (nodeType) {
    case "image":
      return "图片";
    case "video":
      return "视频";
    case "audio":
      return "音频";
    case "text":
    default:
      return "文本";
  }
}

function boundsForNodes(nodes: MediaNode[]) {
  if (nodes.length === 0) {
    return { x: 0, y: 0, w: 240, h: 120 };
  }
  const minX = Math.min(...nodes.map((node) => node.canvas_x));
  const minY = Math.min(...nodes.map((node) => node.canvas_y));
  const maxX = Math.max(...nodes.map((node) => node.canvas_x + node.canvas_w));
  const maxY = Math.max(...nodes.map((node) => node.canvas_y + node.canvas_h));
  return { x: minX, y: minY, w: maxX - minX, h: maxY - minY };
}

export function isMediaShape(shape: unknown): shape is MediaShape {
  return (
    typeof shape === "object" &&
    shape !== null &&
    "type" in shape &&
    (shape as { type?: string }).type === MEDIA_SHAPE_TYPE
  );
}
