import type { Edge, Node } from "@xyflow/react";
import type { MediaEdge, MediaGroup, MediaNode } from "../../lib/api";

export type CanvasFlowMode = "studio" | "agent";

export interface CanvasFlowNodeData extends Record<string, unknown> {
  kind: "media";
  node: MediaNode;
}

export interface CanvasFlowGroupData extends Record<string, unknown> {
  kind: "group";
  group: MediaGroup;
  nodeIds: string[];
}

export interface CanvasFlowEdgeData extends Record<string, unknown> {
  edge: MediaEdge;
}

export type CanvasFlowNode =
  | Node<CanvasFlowNodeData, "media">
  | Node<CanvasFlowGroupData, "group">;

export type CanvasFlowEdge = Edge<CanvasFlowEdgeData, "dependency">;
