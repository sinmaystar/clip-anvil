import {
  getBezierPath,
  useStore,
  type ConnectionLineComponentProps,
} from "@xyflow/react";

export function ConnectionLinePreview({
  fromX,
  fromY,
  fromPosition,
  pointer,
  toPosition,
}: ConnectionLineComponentProps) {
  const transform = useStore((state) => state.transform);
  const targetPoint = pointerToFlowPoint(pointer, transform);
  const [edgePath] = getBezierPath({
    sourceX: fromX,
    sourceY: fromY,
    sourcePosition: fromPosition,
    targetX: targetPoint.x,
    targetY: targetPoint.y,
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

function pointerToFlowPoint(
  pointer: { x: number; y: number },
  transform: [number, number, number],
) {
  const [translateX, translateY, zoom] = transform;
  return {
    x: (pointer.x - translateX) / zoom,
    y: (pointer.y - translateY) / zoom,
  };
}
