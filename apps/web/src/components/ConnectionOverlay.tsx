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
import { isMediaShape, shapeIdForNode } from "../lib/canvas";

export interface DragConnection {
  fromNodeId: string;
  pointerPagePoint: Point;
}

interface ConnectionOverlayProps {
  dragConnection: DragConnection | null;
  editor: Editor | null;
  edges: MediaEdge[];
  nodes: MediaNode[];
  onSelectEdge: (edgeId: string) => void;
  selectedEdgeId: string | null;
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
  nodes,
  onSelectEdge,
  selectedEdgeId,
}: ConnectionOverlayProps) {
  const overlayRef = useRef<SVGSVGElement | null>(null);
  const [liveShapeSignature, setLiveShapeSignature] = useState("");
  const [viewport, setViewport] = useState<ViewportSnapshot>({
    cameraX: 0,
    cameraY: 0,
    cameraZ: 1,
    height: 0,
    width: 0,
  });
  const nodeBoundsById = useMemo(
    () =>
      new Map(
        nodes.map((node) => {
          const shape = editor?.getShape(shapeIdForNode(node.id));
          if (isMediaShape(shape)) {
            return [
              node.id,
              mediaNodeBounds(node, { x: shape.x, y: shape.y }),
            ] as const;
          }
          return [node.id, mediaNodeBounds(node)] as const;
        }),
      ),
    [editor, liveShapeSignature, nodes],
  );

  useEffect(() => {
    if (!editor) {
      return;
    }

    let frame = 0;
    let previousViewport = "";
    let previousShapeSignature = "";
    const tick = () => {
      const camera = editor.getCamera();
      const rect = overlayRef.current?.getBoundingClientRect();
      const nextViewport = JSON.stringify({
        cameraX: camera.x,
        cameraY: camera.y,
        cameraZ: camera.z,
        height: rect?.height ?? 0,
        width: rect?.width ?? 0,
      });
      if (nextViewport !== previousViewport) {
        previousViewport = nextViewport;
        setViewport(JSON.parse(nextViewport) as ViewportSnapshot);
      }

      const nextShapeSignature = nodes
        .map((node) => {
          const shape = editor.getShape(shapeIdForNode(node.id));
          return isMediaShape(shape)
            ? `${node.id}:${shape.x}:${shape.y}`
            : `${node.id}:missing`;
        })
        .join("|");
      if (nextShapeSignature !== previousShapeSignature) {
        previousShapeSignature = nextShapeSignature;
        setLiveShapeSignature(nextShapeSignature);
      }
      frame = window.requestAnimationFrame(tick);
    };

    tick();
    return () => window.cancelAnimationFrame(frame);
  }, [editor, nodes]);

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
    const fromNode = nodeBoundsById.get(edge.from_node_id);
    const toNode = nodeBoundsById.get(edge.to_node_id);
    if (!fromNode || !toNode) {
      return [];
    }
    const start = project(outputAnchor(fromNode));
    const end = project(inputAnchor(toNode));
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
    const fromNode = nodeBoundsById.get(dragConnection.fromNodeId);
    if (!fromNode) {
      return null;
    }
    const start = project(outputAnchor(fromNode));
    const end = project(dragConnection.pointerPagePoint);
    return connectionPath(start, end);
  })();

  return (
    <svg
      aria-hidden="true"
      className="connection-overlay"
      data-camera={`${viewport.cameraX}:${viewport.cameraY}:${viewport.cameraZ}`}
      ref={overlayRef}
    >
      <defs>
        <marker
          id="connection-overlay-arrow"
          markerHeight="5"
          markerWidth="6"
          orient="auto"
          refX="5.5"
          refY="2.5"
          viewBox="0 0 6 5"
        >
          <path d="M 0 0 L 6 2.5 L 0 5 z" fill="var(--accent)" />
        </marker>
      </defs>

      {renderedEdges.map(({ edge, path }) => (
        <g
          className="connection-overlay-edge"
          data-selected={edge.id === selectedEdgeId}
          key={edge.id}
          onPointerDown={(event) => {
            event.preventDefault();
            event.stopPropagation();
            onSelectEdge(edge.id);
          }}
        >
          <path className="connection-overlay-path-shadow" d={path} />
          <path
            className="connection-overlay-path"
            d={path}
            markerEnd="url(#connection-overlay-arrow)"
          />
          <path className="connection-overlay-flow" d={path} />
          <path className="connection-overlay-hit" d={path} />
        </g>
      ))}

      {previewPath ? (
        <g className="connection-overlay-preview-group">
          <path className="connection-overlay-preview-shadow" d={previewPath} />
          <path className="connection-overlay-preview" d={previewPath} />
          <path className="connection-overlay-preview-flow" d={previewPath} />
        </g>
      ) : null}

    </svg>
  );
}
