import {
  BaseEdge,
  getBezierPath,
  type Edge,
  type EdgeProps,
} from "@xyflow/react";
import type { AgentWorkbenchEdgeData } from "../../lib/agentWorkbenchViewModel";

type WorkbenchEdge = Edge<AgentWorkbenchEdgeData, "agentWorkbench">;

export function AgentWorkbenchEdge({
  id,
  sourcePosition,
  sourceX,
  sourceY,
  targetPosition,
  targetX,
  targetY,
}: EdgeProps<WorkbenchEdge>) {
  const [path] = getBezierPath({
    sourcePosition,
    sourceX,
    sourceY,
    targetPosition,
    targetX,
    targetY,
  });
  return <BaseEdge className="agent-workbench-edge" id={id} path={path} />;
}
