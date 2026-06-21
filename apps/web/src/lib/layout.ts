import dagre from "@dagrejs/dagre";
import type { MediaEdge, MediaGroup, MediaNode } from "./api";
import { adaptiveMediaNodeSize } from "./nodePreviewLayout.js";

export type LayoutDirection = "LR" | "TB";

export interface LayoutPosition {
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

export interface LayoutOrigin {
  x: number;
  y: number;
}

export function computeDagreLayout(input: {
  nodes: MediaNode[];
  edges: MediaEdge[];
  groups: MediaGroup[];
  direction: LayoutDirection;
  origin?: LayoutOrigin;
}): { positions: LayoutPosition[]; groupBounds: GroupBounds[] } {
  const graph = new dagre.graphlib.Graph();
  graph.setDefaultEdgeLabel(() => ({}));
  graph.setGraph({
    rankdir: input.direction,
    nodesep: 40,
    ranksep: 80,
    marginx: 20,
    marginy: 20,
  });

  const groupBoundsById = new Map(
    input.groups.map((group) => [group.id, boundsForGroup(group, input.nodes)]),
  );
  const groupedNodeIds = new Set(input.groups.flatMap((group) => group.node_ids));
  const containerByNodeId = new Map<string, string>();

  for (const group of input.groups) {
    const bounds = groupBoundsById.get(group.id) ?? {
      groupId: group.id,
      x: 0,
      y: 0,
      w: 240,
      h: 120,
    };
    graph.setNode(containerIdForGroup(group.id), {
      width: bounds.w,
      height: bounds.h,
    });
    for (const nodeId of group.node_ids) {
      containerByNodeId.set(nodeId, containerIdForGroup(group.id));
    }
  }

  for (const node of input.nodes) {
    if (groupedNodeIds.has(node.id)) {
      continue;
    }
    const containerId = containerIdForNode(node.id);
    containerByNodeId.set(node.id, containerId);
    const size = displaySizeForNode(node);
    graph.setNode(containerId, {
      width: size.w,
      height: size.h,
    });
  }

  const seenEdges = new Set<string>();
  for (const edge of input.edges) {
    if (edge.edge_type === "dependency") {
      const fromContainer = containerByNodeId.get(edge.from_node_id);
      const toContainer = containerByNodeId.get(edge.to_node_id);
      if (!fromContainer || !toContainer || fromContainer === toContainer) {
        continue;
      }
      const edgeKey = `${fromContainer}->${toContainer}`;
      if (seenEdges.has(edgeKey)) {
        continue;
      }
      seenEdges.add(edgeKey);
      graph.setEdge(fromContainer, toContainer);
    }
  }

  dagre.layout(graph);

  const positions: LayoutPosition[] = [];
  const groupBounds: GroupBounds[] = [];

  for (const group of input.groups) {
    const containerId = containerIdForGroup(group.id);
    const layoutNode = graph.node(containerId) as { x: number; y: number };
    const oldBounds = groupBoundsById.get(group.id);
    if (!layoutNode || !oldBounds) {
      continue;
    }
    const nextBounds = {
      ...oldBounds,
      x: layoutNode.x - oldBounds.w / 2,
      y: layoutNode.y - oldBounds.h / 2,
    };
    const deltaX = nextBounds.x - oldBounds.x;
    const deltaY = nextBounds.y - oldBounds.y;
    groupBounds.push(nextBounds);
    const memberIds = new Set(group.node_ids);
    positions.push(
      ...input.nodes
        .filter((node) => memberIds.has(node.id))
        .map((node) => ({
          id: node.id,
          canvas_x: node.canvas_x + deltaX,
          canvas_y: node.canvas_y + deltaY,
        })),
    );
  }

  for (const node of input.nodes) {
    if (groupedNodeIds.has(node.id)) {
      continue;
    }
    const layoutNode = graph.node(containerIdForNode(node.id)) as
      | { x: number; y: number }
      | undefined;
    if (!layoutNode) {
      continue;
    }
    const size = displaySizeForNode(node);
    positions.push({
      id: node.id,
      canvas_x: layoutNode.x - size.w / 2,
      canvas_y: layoutNode.y - size.h / 2,
    });
  }

  return alignLayoutToOrigin({ groupBounds, origin: input.origin, positions });
}

function alignLayoutToOrigin(input: {
  groupBounds: GroupBounds[];
  origin?: LayoutOrigin;
  positions: LayoutPosition[];
}) {
  if (!input.origin || input.positions.length === 0) {
    return {
      positions: input.positions,
      groupBounds: input.groupBounds,
    };
  }

  const minNodeX = Math.min(
    ...input.positions.map((position) => position.canvas_x),
  );
  const minNodeY = Math.min(
    ...input.positions.map((position) => position.canvas_y),
  );
  const minGroupX =
    input.groupBounds.length > 0
      ? Math.min(...input.groupBounds.map((bounds) => bounds.x))
      : minNodeX;
  const minGroupY =
    input.groupBounds.length > 0
      ? Math.min(...input.groupBounds.map((bounds) => bounds.y))
      : minNodeY;
  const deltaX = input.origin.x - Math.min(minNodeX, minGroupX);
  const deltaY = input.origin.y - Math.min(minNodeY, minGroupY);

  return {
    positions: input.positions.map((position) => ({
      ...position,
      canvas_x: position.canvas_x + deltaX,
      canvas_y: position.canvas_y + deltaY,
    })),
    groupBounds: input.groupBounds.map((bounds) => ({
      ...bounds,
      x: bounds.x + deltaX,
      y: bounds.y + deltaY,
    })),
  };
}

function containerIdForGroup(groupId: string) {
  return `group:${groupId}`;
}

function containerIdForNode(nodeId: string) {
  return `node:${nodeId}`;
}

function boundsForGroup(group: MediaGroup, nodes: MediaNode[]): GroupBounds {
  const members = nodes.filter((node) => group.node_ids.includes(node.id));
  if (members.length === 0) {
    return { groupId: group.id, x: 0, y: 0, w: 240, h: 120 };
  }
  const minX = Math.min(...members.map((node) => node.canvas_x));
  const minY = Math.min(...members.map((node) => node.canvas_y));
  const maxX = Math.max(
    ...members.map((node) => node.canvas_x + displaySizeForNode(node).w),
  );
  const maxY = Math.max(
    ...members.map((node) => node.canvas_y + displaySizeForNode(node).h),
  );
  return {
    groupId: group.id,
    x: minX - 20,
    y: minY - 44,
    w: Math.max(240, maxX - minX + 40),
    h: Math.max(120, maxY - minY + 64),
  };
}

function displaySizeForNode(node: MediaNode) {
  return adaptiveMediaNodeSize(node);
}
