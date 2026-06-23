import {
  BaseEdge,
  getBezierPath,
  type EdgeProps,
} from "@xyflow/react";
import type { CanvasFlowEdge } from "./flowTypes";

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
}: EdgeProps<CanvasFlowEdge>) {
  const [edgePath] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  return (
    <BaseEdge
      id={id}
      className="connection-overlay-path dependency-flow-edge"
      data-selected={selected}
      interactionWidth={24}
      markerEnd={markerEnd}
      path={edgePath}
    />
  );
}
