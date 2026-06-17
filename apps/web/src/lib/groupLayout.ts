import type { MediaGroup, MediaNode } from "./api";

export interface CanvasPosition {
  id: string;
  canvas_x: number;
  canvas_y: number;
}

export interface GroupBounds {
  groupId: string;
  x: number;
  y: number;
  w: number;
  h: number;
}

export function getContainingGroupId(input: {
  point: { x: number; y: number };
  bounds: GroupBounds[];
}): string | null {
  const containing = input.bounds
    .filter(
      (bounds) =>
        input.point.x >= bounds.x &&
        input.point.x <= bounds.x + bounds.w &&
        input.point.y >= bounds.y &&
        input.point.y <= bounds.y + bounds.h,
    )
    .sort((a, b) => a.w * a.h - b.w * b.h);
  return containing[0]?.groupId ?? null;
}

export function getGroupMemberMovePositions(input: {
  group: MediaGroup;
  nodes: MediaNode[];
  deltaX: number;
  deltaY: number;
}): CanvasPosition[] {
  const memberIds = new Set(input.group.node_ids ?? []);
  return input.nodes
    .filter((node) => memberIds.has(node.id))
    .map((node) => ({
      id: node.id,
      canvas_x: node.canvas_x + input.deltaX,
      canvas_y: node.canvas_y + input.deltaY,
    }));
}

export function getGroupMemberLayoutPositions(input: {
  group: MediaGroup;
  nodes: MediaNode[];
  groupX: number;
  groupY: number;
}): CanvasPosition[] {
  const nodeById = new Map(input.nodes.map((node) => [node.id, node]));
  const members = (input.group.node_ids ?? [])
    .map((nodeId) => nodeById.get(nodeId))
    .filter((node): node is MediaNode => Boolean(node));
  const columns = Math.max(1, Math.min(2, members.length));
  const gap = 24;
  const paddingX = 20;
  const paddingTop = 44;

  return members.map((node, index) => {
    const column = index % columns;
    const row = Math.floor(index / columns);
    return {
      id: node.id,
      canvas_x: input.groupX + paddingX + column * (node.canvas_w + gap),
      canvas_y: input.groupY + paddingTop + row * (node.canvas_h + gap),
    };
  });
}
