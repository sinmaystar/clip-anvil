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
        style={{ strokeDashoffset: selected ? -24 : 0 }}
      />
    </g>
  );
}
