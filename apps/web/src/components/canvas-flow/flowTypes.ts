import type { Edge, Node } from "@xyflow/react";
import type {
  DomainCanvasEdge,
  DomainCanvasNode,
  MediaEdge,
  MediaGroup,
  MediaNode,
} from "../../lib/api";
import type { MediaDimensions } from "../../lib/nodePreviewLayout";

export type CanvasFlowMode = "studio" | "agent";

export interface CanvasFlowNodeData extends Record<string, unknown> {
  kind: "media";
  node: MediaNode;
  onMediaDimensionsChange?: (
    nodeId: string,
    dimensions: MediaDimensions,
  ) => void;
  onRenameNode?: (nodeId: string, title: string) => void;
}

export interface CanvasFlowGroupData extends Record<string, unknown> {
  kind: "group";
  group: MediaGroup;
  nodeIds: string[];
}

export interface CanvasFlowDomainData extends Record<string, unknown> {
  kind: "domain";
  node: DomainCanvasNode;
}

export interface CanvasFlowEdgeData extends Record<string, unknown> {
  edge: MediaEdge;
}

export interface CanvasFlowDomainEdgeData extends Record<string, unknown> {
  edge: DomainCanvasEdge;
}

export type CanvasFlowNode =
  | Node<CanvasFlowNodeData, "media">
  | Node<CanvasFlowGroupData, "group">
  | Node<CanvasFlowDomainData, "domain">;

export type CanvasFlowDomainEdge = Edge<CanvasFlowDomainEdgeData, "domain">;

export type CanvasFlowEdge =
  | Edge<CanvasFlowEdgeData, "dependency">
  | CanvasFlowDomainEdge;
