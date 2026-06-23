import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  Background,
  ConnectionMode,
  Controls,
  ReactFlow,
  ReactFlowProvider,
  applyEdgeChanges,
  applyNodeChanges,
  useReactFlow,
  type Connection,
  type EdgeChange,
  type EdgeTypes,
  type NodeChange,
  type NodeTypes,
  type OnConnectEnd,
  type OnConnectStart,
} from "@xyflow/react";
import type { CanvasCamera, CanvasPayload } from "../../lib/api";
import { isValidConnectionTarget } from "../../lib/connectionGeometry";
import { canvasToFlowEdges, canvasToFlowNodes } from "./canvasViewModel";
import { cameraToViewport, viewportToCamera } from "./canvasViewport";
import { ConnectionLinePreview } from "./ConnectionLinePreview";
import { DependencyFlowEdge } from "./DependencyFlowEdge";
import { GroupFlowNode } from "./GroupFlowNode";
import { MediaFlowNode } from "./MediaFlowNode";
import { NodeInspectorPopover } from "./NodeInspectorPopover";
import type { CanvasFlowEdge, CanvasFlowMode, CanvasFlowNode } from "./flowTypes";
import {
  CanvasFlowPolicyProvider,
  policyForCanvasMode,
} from "./flowModePolicy";

const nodeTypes: NodeTypes = {
  media: MediaFlowNode,
  group: GroupFlowNode,
};

const edgeTypes: EdgeTypes = {
  dependency: DependencyFlowEdge,
};

export interface CanvasFlowSurfaceProps {
  canvas: CanvasPayload;
  mode: CanvasFlowMode;
  selectedNodeId: string | null;
  selectedGroupId?: string | null;
  selectedEdgeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onSelectGroup?: (groupId: string | null) => void;
  onSelectEdge: (edgeId: string | null) => void;
  onNodePositionsChange?: (
    positions: Array<{ id: string; canvas_x: number; canvas_y: number }>,
  ) => void;
  onGroupMove?: (input: {
    groupId: string;
    deltaX: number;
    deltaY: number;
  }) => void;
  onViewportChange?: (camera: CanvasCamera) => void;
  onCreateNodeAtPoint?: (input: {
    flowPoint: { x: number; y: number };
    screenX: number;
    screenY: number;
  }) => void;
  onConnectNodes?: (input: { fromNodeId: string; toNodeId: string }) => void;
  renderInspector?: boolean;
}

export function CanvasFlowSurface(props: CanvasFlowSurfaceProps) {
  return (
    <ReactFlowProvider>
      <CanvasFlowSurfaceContent {...props} />
    </ReactFlowProvider>
  );
}

function CanvasFlowSurfaceContent({
  canvas,
  mode,
  selectedNodeId,
  selectedGroupId,
  selectedEdgeId,
  onSelectNode,
  onSelectGroup,
  onSelectEdge,
  onNodePositionsChange,
  onGroupMove,
  onViewportChange,
  onCreateNodeAtPoint,
  onConnectNodes,
  renderInspector = true,
}: CanvasFlowSurfaceProps) {
  const policy = policyForCanvasMode(mode);
  const { screenToFlowPosition } = useReactFlow<CanvasFlowNode, CanvasFlowEdge>();
  const dragStartPositionsRef = useRef(new Map<string, { x: number; y: number }>());
  const connectionSourceRef = useRef<string | null>(null);
  const connectedOnHandleRef = useRef(false);
  const derivedNodes = useMemo(
    () =>
      canvasToFlowNodes(canvas).map((node) => ({
        ...node,
        selected:
          node.type === "group"
            ? node.id === selectedGroupId
            : node.id === selectedNodeId,
      })),
    [canvas, selectedGroupId, selectedNodeId],
  );
  const derivedEdges = useMemo(
    () =>
      canvasToFlowEdges(canvas).map((edge) => ({
        ...edge,
        selected: edge.id === selectedEdgeId,
      })),
    [canvas, selectedEdgeId],
  );
  const [nodes, setNodes] = useState<CanvasFlowNode[]>(derivedNodes);
  const [edges, setEdges] = useState<CanvasFlowEdge[]>(derivedEdges);
  const selectedNode = canvas.nodes.find((node) => node.id === selectedNodeId);

  useEffect(() => {
    setNodes(derivedNodes);
  }, [derivedNodes]);

  useEffect(() => {
    setEdges(derivedEdges);
  }, [derivedEdges]);

  const handleNodesChange = useCallback(
    (changes: NodeChange<CanvasFlowNode>[]) => {
      setNodes((current) => applyNodeChanges(changes, current));
      const settledPositions = changes.flatMap((change) => {
        if (
          change.type !== "position" ||
          change.dragging ||
          !change.position
        ) {
          return [];
        }
        const node = nodes.find((item) => item.id === change.id);
        if (!node || node.type !== "media") {
          return [];
        }
        return [
          {
            id: change.id,
            canvas_x: change.position.x,
            canvas_y: change.position.y,
          },
        ];
      });
      if (settledPositions.length > 0) {
        onNodePositionsChange?.(settledPositions);
      }
    },
    [nodes, onNodePositionsChange],
  );

  const handleEdgesChange = useCallback((changes: EdgeChange<CanvasFlowEdge>[]) => {
    setEdges((current) => applyEdgeChanges(changes, current));
  }, []);

  const handleConnect = useCallback(
    (connection: Connection) => {
      if (!policy.canCreateEdges || !connection.source || !connection.target) {
        return;
      }
      connectedOnHandleRef.current = true;
      connectionSourceRef.current = null;
      onConnectNodes?.({
        fromNodeId: connection.source,
        toNodeId: connection.target,
      });
    },
    [onConnectNodes, policy.canCreateEdges],
  );

  const handleConnectStart = useCallback<OnConnectStart>(
    (_, params) => {
      if (!policy.canCreateEdges || params.handleType !== "source") {
        connectionSourceRef.current = null;
        return;
      }
      connectionSourceRef.current = params.nodeId ?? null;
      connectedOnHandleRef.current = false;
    },
    [policy.canCreateEdges],
  );

  const handleConnectEnd = useCallback<OnConnectEnd>(
    (event: MouseEvent | TouchEvent, connectionState) => {
      const fromNodeId =
        connectionState.fromHandle?.nodeId ?? connectionSourceRef.current;
      connectionSourceRef.current = null;
      if (!policy.canCreateEdges || !fromNodeId) {
        return;
      }
      if (connectedOnHandleRef.current) {
        connectedOnHandleRef.current = false;
        return;
      }
      const point = clientPointFromConnectionEvent(event);
      if (!point) {
        return;
      }
      const target = mediaNodeShellFromConnectionPoint(point);
      if (!target) {
        return;
      }
      const toNodeId = target.dataset.nodeId;
      if (!isValidConnectionTarget(fromNodeId, toNodeId)) {
        return;
      }
      onConnectNodes?.({ fromNodeId, toNodeId });
    },
    [onConnectNodes, policy.canCreateEdges],
  );

  return (
    <CanvasFlowPolicyProvider value={policy}>
      <div className="canvas-flow-surface" data-mode={mode}>
        <ReactFlow
          connectOnClick={false}
          connectionDragThreshold={0}
          connectionLineComponent={ConnectionLinePreview}
          connectionMode={ConnectionMode.Strict}
          connectionRadius={96}
          defaultViewport={cameraToViewport(canvas.camera)}
          deleteKeyCode={null}
          edgeTypes={edgeTypes}
          edges={edges}
          edgesFocusable={policy.canSelect}
          edgesReconnectable={policy.canCreateEdges}
          elementsSelectable={policy.canSelect}
          nodeTypes={nodeTypes}
          nodes={nodes}
          nodesConnectable={policy.canCreateEdges}
          nodesDraggable={policy.canDragNodes}
          nodesFocusable={policy.canSelect}
          onConnect={handleConnect}
          onConnectEnd={handleConnectEnd}
          onConnectStart={handleConnectStart}
          onEdgesChange={handleEdgesChange}
          onEdgeClick={(_, edge) => {
            onSelectEdge(edge.id);
          }}
          onMoveEnd={(_, viewport) => {
            if (policy.canPersistViewport) {
              onViewportChange?.(viewportToCamera(viewport));
            }
          }}
          onNodeDragStop={(_, node) => {
            if (node.type === "media") {
              onNodePositionsChange?.([
                {
                  id: node.id,
                  canvas_x: node.position.x,
                  canvas_y: node.position.y,
                },
              ]);
              return;
            }
            if (node.type === "group") {
              const start = dragStartPositionsRef.current.get(node.id);
              dragStartPositionsRef.current.delete(node.id);
              if (!start) {
                return;
              }
              const deltaX = node.position.x - start.x;
              const deltaY = node.position.y - start.y;
              if (deltaX !== 0 || deltaY !== 0) {
                onGroupMove?.({ groupId: node.id, deltaX, deltaY });
              }
            }
          }}
          onNodeDragStart={(_, node) => {
            dragStartPositionsRef.current.set(node.id, {
              x: node.position.x,
              y: node.position.y,
            });
          }}
          onNodesChange={handleNodesChange}
          onNodeClick={(_, node) => {
            if (node.type === "group") {
              onSelectGroup?.(node.id);
            } else {
              onSelectNode(node.id);
            }
            onSelectEdge(null);
          }}
          onPaneClick={() => {
            onSelectNode(null);
            onSelectGroup?.(null);
            onSelectEdge(null);
          }}
          onPaneContextMenu={(event) => {
            if (!policy.canCreateNodes || !onCreateNodeAtPoint) {
              return;
            }
            event.preventDefault();
            onCreateNodeAtPoint({
              flowPoint: screenToFlowPosition({
                x: event.clientX,
                y: event.clientY,
              }),
              screenX: event.clientX,
              screenY: event.clientY,
            });
          }}
          panOnDrag={policy.canPanZoom}
          zoomOnDoubleClick={policy.canPanZoom}
          zoomOnPinch={policy.canPanZoom}
          zoomOnScroll={policy.canPanZoom}
        >
          <Background />
          <Controls showInteractive={false} />
        </ReactFlow>
        {renderInspector && selectedNode ? (
          <NodeInspectorPopover
            mode={mode}
            policy={policy}
            node={selectedNode}
            edges={canvas.edges}
            groups={canvas.groups}
            onRunNode={noopRunNode}
            onUpdateNode={noopUpdateNode}
          />
        ) : null}
      </div>
    </CanvasFlowPolicyProvider>
  );
}

function noopRunNode() {}

function noopUpdateNode() {}

function clientPointFromConnectionEvent(event: MouseEvent | TouchEvent) {
  if ("changedTouches" in event) {
    const touch = event.changedTouches[0];
    return touch ? { x: touch.clientX, y: touch.clientY } : null;
  }
  return { x: event.clientX, y: event.clientY };
}

function mediaNodeShellFromConnectionPoint(point: { x: number; y: number }) {
  const shell = document
    .elementsFromPoint(point.x, point.y)
    .find(
      (element): element is HTMLElement =>
        element instanceof HTMLElement &&
        Boolean(element.closest(".media-node-shell")),
    )
    ?.closest(".media-node-shell");
  return shell instanceof HTMLElement ? shell : null;
}
