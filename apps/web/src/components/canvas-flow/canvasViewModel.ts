import type { CanvasPayload, MediaGroup, MediaNode } from "../../lib/api";
import { mediaNodeDisplaySize } from "../../lib/canvas";
import type { CanvasFlowEdge, CanvasFlowNode } from "./flowTypes";

const GROUP_PADDING_X = 20;
const GROUP_HEADER_HEIGHT = 44;
const GROUP_MIN_WIDTH = 240;
const GROUP_MIN_HEIGHT = 120;

export function canvasToFlowNodes(canvas: CanvasPayload): CanvasFlowNode[] {
  return [
    ...canvas.groups.map((group) => groupToFlowNode(group, canvas.nodes)),
    ...canvas.nodes.map(nodeToFlowNode),
  ];
}

export function canvasToFlowEdges(canvas: CanvasPayload): CanvasFlowEdge[] {
  return canvas.edges.map((edge) => ({
    id: edge.id,
    type: "dependency",
    source: edge.from_node_id,
    target: edge.to_node_id,
    data: { edge },
  }));
}

export function nodeToFlowNode(node: MediaNode): CanvasFlowNode {
  const size = mediaNodeDisplaySize(node);
  return {
    id: node.id,
    type: "media",
    position: { x: node.canvas_x, y: node.canvas_y },
    width: size.w,
    height: size.h,
    data: { kind: "media", node },
  };
}

export function groupToFlowNode(
  group: MediaGroup,
  nodes: MediaNode[],
): CanvasFlowNode {
  const bounds = boundsForGroup(group, nodes);
  return {
    id: group.id,
    type: "group",
    position: { x: bounds.x, y: bounds.y },
    width: bounds.w,
    height: bounds.h,
    data: { kind: "group", group, nodeIds: group.node_ids },
    dragHandle: ".group-flow-drag-handle",
    draggable: true,
    selectable: true,
  };
}

function boundsForGroup(group: MediaGroup, nodes: MediaNode[]) {
  const members = nodes.filter((node) => group.node_ids.includes(node.id));
  if (members.length === 0) {
    return { x: 0, y: 0, w: GROUP_MIN_WIDTH, h: GROUP_MIN_HEIGHT };
  }

  const minX = Math.min(...members.map((node) => node.canvas_x));
  const minY = Math.min(...members.map((node) => node.canvas_y));
  const maxX = Math.max(
    ...members.map((node) => node.canvas_x + mediaNodeDisplaySize(node).w),
  );
  const maxY = Math.max(
    ...members.map((node) => node.canvas_y + mediaNodeDisplaySize(node).h),
  );

  return {
    x: minX - GROUP_PADDING_X,
    y: minY - GROUP_HEADER_HEIGHT,
    w: Math.max(GROUP_MIN_WIDTH, maxX - minX + GROUP_PADDING_X * 2),
    h: Math.max(GROUP_MIN_HEIGHT, maxY - minY + GROUP_HEADER_HEIGHT + 20),
  };
}
