import { useEffect, useMemo, useRef, useState } from "react";
import type { Editor } from "tldraw";
import type { MediaEdge, MediaNode } from "../lib/api";
import {
  connectionPath,
  inputAnchor,
  mediaNodeBounds,
  outputAnchor,
  type Point,
} from "../lib/connectionGeometry";

export interface DragConnection {
  fromNodeId: string;
  pointerPagePoint: Point;
}

interface ConnectionOverlayProps {
  dragConnection: DragConnection | null;
  editor: Editor | null;
  edges: MediaEdge[];
  hoveredTargetNodeId: string | null;
  nodes: MediaNode[];
}

interface ViewportSnapshot {
  cameraX: number;
  cameraY: number;
  cameraZ: number;
  height: number;
  width: number;
}

type ScreenProjector = Editor & {
  pageToScreen: (point: Point) => Point;
};

export function ConnectionOverlay({
  dragConnection,
  editor,
  edges,
  hoveredTargetNodeId,
  nodes,
}: ConnectionOverlayProps) {
  const overlayRef = useRef<SVGSVGElement | null>(null);
  const [viewport, setViewport] = useState<ViewportSnapshot>({
    cameraX: 0,
    cameraY: 0,
    cameraZ: 1,
    height: 0,
    width: 0,
  });
  const nodeById = useMemo(
    () => new Map(nodes.map((node) => [node.id, node])),
    [nodes],
  );

  useEffect(() => {
    if (!editor) {
      return;
    }

    let frame = 0;
    let previous = "";
    const tick = () => {
      const camera = editor.getCamera();
      const rect = overlayRef.current?.getBoundingClientRect();
      const next = JSON.stringify({
        cameraX: camera.x,
        cameraY: camera.y,
        cameraZ: camera.z,
        height: rect?.height ?? 0,
        width: rect?.width ?? 0,
      });
      if (next !== previous) {
        previous = next;
        setViewport(JSON.parse(next) as ViewportSnapshot);
      }
      frame = window.requestAnimationFrame(tick);
    };

    tick();
    return () => window.cancelAnimationFrame(frame);
  }, [editor]);

  const project = (point: Point) => {
    if (!editor || !overlayRef.current) {
      return point;
    }
    const screenPoint = (editor as ScreenProjector).pageToScreen(point);
    const rect = overlayRef.current.getBoundingClientRect();
    return {
      x: screenPoint.x - rect.left,
      y: screenPoint.y - rect.top,
    };
  };

  const renderedEdges = edges.flatMap((edge) => {
    const fromNode = nodeById.get(edge.from_node_id);
    const toNode = nodeById.get(edge.to_node_id);
    if (!fromNode || !toNode) {
      return [];
    }
    const start = project(outputAnchor(mediaNodeBounds(fromNode)));
    const end = project(inputAnchor(mediaNodeBounds(toNode)));
    return [
      {
        edge,
        path: connectionPath(start, end),
      },
    ];
  });

  const previewPath = (() => {
    if (!dragConnection) {
      return null;
    }
    const fromNode = nodeById.get(dragConnection.fromNodeId);
    if (!fromNode) {
      return null;
    }
    const start = project(outputAnchor(mediaNodeBounds(fromNode)));
    const end = project(dragConnection.pointerPagePoint);
    return connectionPath(start, end);
  })();

  const targetHighlights = dragConnection
    ? nodes
        .filter((node) => node.id !== dragConnection.fromNodeId)
        .map((node) => {
          const point = project(inputAnchor(mediaNodeBounds(node)));
          return {
            id: node.id,
            point,
            active: node.id === hoveredTargetNodeId,
          };
        })
    : [];

  return (
    <svg
      aria-hidden="true"
      className="connection-overlay"
      data-camera={`${viewport.cameraX}:${viewport.cameraY}:${viewport.cameraZ}`}
      ref={overlayRef}
    >
      <defs>
        <linearGradient
          gradientUnits="userSpaceOnUse"
          id="connection-overlay-gradient"
          x1="0"
          x2={viewport.width || 1}
          y1="0"
          y2={viewport.height || 1}
        >
          <stop offset="0%" stopColor="#22c55e" stopOpacity="0.45" />
          <stop offset="52%" stopColor="#3b82f6" stopOpacity="0.95" />
          <stop offset="100%" stopColor="#b26bff" stopOpacity="0.86" />
        </linearGradient>
        <marker
          id="connection-overlay-arrow"
          markerHeight="8"
          markerWidth="9"
          orient="auto"
          refX="8"
          refY="4"
          viewBox="0 0 9 8"
        >
          <path d="M 0 0 L 9 4 L 0 8 z" fill="#b26bff" />
        </marker>
      </defs>

      {renderedEdges.map(({ edge, path }) => (
        <g className="connection-overlay-edge" key={edge.id}>
          <path className="connection-overlay-path-shadow" d={path} />
          <path
            className="connection-overlay-path"
            d={path}
            markerEnd="url(#connection-overlay-arrow)"
          />
          <path className="connection-overlay-flow" d={path} />
        </g>
      ))}

      {previewPath ? (
        <g className="connection-overlay-preview-group">
          <path className="connection-overlay-preview-shadow" d={previewPath} />
          <path className="connection-overlay-preview" d={previewPath} />
          <path className="connection-overlay-preview-flow" d={previewPath} />
        </g>
      ) : null}

      {targetHighlights.map(({ active, id, point }) => (
        <circle
          className="connection-overlay-target"
          cx={point.x}
          cy={point.y}
          data-active={active}
          key={id}
          r={active ? 15 : 10}
        />
      ))}
    </svg>
  );
}
