import {
  getBezierPath,
  type ConnectionLineComponentProps,
} from "@xyflow/react";

export function ConnectionLinePreview({
  fromX,
  fromY,
  fromPosition,
  toX,
  toY,
  toPosition,
}: ConnectionLineComponentProps) {
  const [edgePath] = getBezierPath({
    sourceX: fromX,
    sourceY: fromY,
    sourcePosition: fromPosition,
    targetX: toX,
    targetY: toY,
    targetPosition: toPosition,
  });

  return (
    <g className="connection-overlay-preview-edge">
      <path className="connection-overlay-preview-shadow" d={edgePath} />
      <path className="connection-overlay-preview" d={edgePath} />
      <path className="connection-overlay-preview-flow" d={edgePath} />
    </g>
  );
}
