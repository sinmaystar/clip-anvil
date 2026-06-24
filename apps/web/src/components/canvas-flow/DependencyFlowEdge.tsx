import {
  BaseEdge,
  getBezierPath,
  Position,
  useReactFlow,
  type EdgeProps,
} from "@xyflow/react";
import type { CanvasFlowEdge, CanvasFlowNode } from "./flowTypes";

export function DependencyFlowEdge({
  id,
  selected,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  markerEnd,
  source,
  target,
}: EdgeProps<CanvasFlowEdge>) {
  const { getNode } = useReactFlow<CanvasFlowNode, CanvasFlowEdge>();
  const sourceNode = getNode(source);
  const targetNode = getNode(target);
  const edgeAnchors = edgeAnchorPoints({
    fallback: {
      sourceX,
      sourceY,
      targetX,
      targetY,
      sourcePosition,
      targetPosition,
    },
    sourceNode,
    targetNode,
  });
  const [edgePath] = getBezierPath({
    sourceX: edgeAnchors.sourceX,
    sourceY: edgeAnchors.sourceY,
    sourcePosition: edgeAnchors.sourcePosition,
    targetX: edgeAnchors.targetX,
    targetY: edgeAnchors.targetY,
    targetPosition: edgeAnchors.targetPosition,
  });

  return (
    <g className="dependency-flow-edge-layer" data-selected={selected}>
      <path className="connection-overlay-path-shadow" d={edgePath} />
      <BaseEdge
        id={id}
        className="connection-overlay-path dependency-flow-edge"
        interactionWidth={24}
        markerEnd={markerEnd}
        path={edgePath}
      />
      <path
        className="connection-overlay-flow"
        d={edgePath}
        pathLength={1}
      />
    </g>
  );
}

function edgeAnchorPoints({
  fallback,
  sourceNode,
  targetNode,
}: {
  fallback: {
    sourceX: number;
    sourceY: number;
    targetX: number;
    targetY: number;
    sourcePosition: Position;
    targetPosition: Position;
  };
  sourceNode?: CanvasFlowNode;
  targetNode?: CanvasFlowNode;
}) {
  if (!sourceNode || !targetNode) {
    return fallback;
  }
  const sourceRect = nodeRect(sourceNode);
  const targetRect = nodeRect(targetNode);
  if (!sourceRect || !targetRect) {
    return fallback;
  }

  return {
    sourceX: sourceRect.x + sourceRect.w,
    sourceY: sourceRect.y + sourceRect.h / 2,
    sourcePosition: Position.Right,
    targetX: targetRect.x,
    targetY: targetRect.y + targetRect.h / 2,
    targetPosition: Position.Left,
  };
}

function nodeRect(node: CanvasFlowNode) {
  const width = node.width ?? (node.type === "media" ? node.data.node.canvas_w : 0);
  const height =
    node.height ?? (node.type === "media" ? node.data.node.canvas_h : 0);
  if (!width || !height) {
    return null;
  }
  return { x: node.position.x, y: node.position.y, w: width, h: height };
}
