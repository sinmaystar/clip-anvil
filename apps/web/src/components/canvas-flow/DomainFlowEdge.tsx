import { BaseEdge, EdgeLabelRenderer, getBezierPath, type EdgeProps } from "@xyflow/react";
import type { CanvasFlowDomainEdge } from "./flowTypes";

export function DomainFlowEdge({
  id,
  data,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
}: EdgeProps<CanvasFlowDomainEdge>) {
  const [path, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    sourcePosition,
    targetX,
    targetY,
    targetPosition,
  });

  return (
    <>
      <BaseEdge id={id} className="domain-flow-edge" path={path} />
      {data?.edge.label ? (
        <EdgeLabelRenderer>
          <div
            className="domain-flow-edge-label"
            style={{ transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)` }}
          >
            {data.edge.label}
          </div>
        </EdgeLabelRenderer>
      ) : null}
    </>
  );
}
