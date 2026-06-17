import dagre from "@dagrejs/dagre";
import type { MediaEdge, MediaGroup, MediaNode } from "./api";

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

export function computeDagreLayout(input: {
  nodes: MediaNode[];
  edges: MediaEdge[];
  groups: MediaGroup[];
  direction: LayoutDirection;
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

  for (const node of input.nodes) {
    graph.setNode(node.id, {
      width: node.canvas_w,
      height: node.canvas_h,
    });
  }

  for (const edge of input.edges) {
    if (edge.edge_type === "dependency") {
      graph.setEdge(edge.from_node_id, edge.to_node_id);
    }
  }

  dagre.layout(graph);

  const positions = input.nodes.map((node) => {
    const layoutNode = graph.node(node.id) as { x: number; y: number };
    return {
      id: node.id,
      canvas_x: layoutNode.x - node.canvas_w / 2,
      canvas_y: layoutNode.y - node.canvas_h / 2,
    };
  });
  const positionByID = new Map(
    positions.map((position) => [position.id, position]),
  );
  const positionedNodes = input.nodes.map((node) => {
    const position = positionByID.get(node.id);
    return position
      ? { ...node, canvas_x: position.canvas_x, canvas_y: position.canvas_y }
      : node;
  });

  return {
    positions,
    groupBounds: input.groups.map((group) =>
      boundsForGroup(group, positionedNodes),
    ),
  };
}

function boundsForGroup(group: MediaGroup, nodes: MediaNode[]): GroupBounds {
  const members = nodes.filter((node) => group.node_ids.includes(node.id));
  if (members.length === 0) {
    return { groupId: group.id, x: 0, y: 0, w: 240, h: 120 };
  }
  const minX = Math.min(...members.map((node) => node.canvas_x));
  const minY = Math.min(...members.map((node) => node.canvas_y));
  const maxX = Math.max(...members.map((node) => node.canvas_x + node.canvas_w));
  const maxY = Math.max(...members.map((node) => node.canvas_y + node.canvas_h));
  return {
    groupId: group.id,
    x: minX - 20,
    y: minY - 44,
    w: Math.max(240, maxX - minX + 40),
    h: Math.max(120, maxY - minY + 64),
  };
}
