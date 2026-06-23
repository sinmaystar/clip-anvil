import { useCallback, useEffect, useMemo, useState } from "react";
import {
  Background,
  Controls,
  ReactFlow,
  ReactFlowProvider,
  applyEdgeChanges,
  applyNodeChanges,
  type EdgeChange,
  type EdgeTypes,
  type NodeChange,
  type NodeTypes,
} from "@xyflow/react";
import type { CanvasCamera, CanvasPayload } from "../../lib/api";
import { canvasToFlowEdges, canvasToFlowNodes } from "./canvasViewModel";
import { cameraToViewport, viewportToCamera } from "./canvasViewport";
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
  selectedEdgeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onSelectEdge: (edgeId: string | null) => void;
  onNodePositionsChange?: (
    positions: Array<{ id: string; canvas_x: number; canvas_y: number }>,
  ) => void;
  onViewportChange?: (camera: CanvasCamera) => void;
}

export function CanvasFlowSurface({
  canvas,
  mode,
  selectedNodeId,
  selectedEdgeId,
  onSelectNode,
  onSelectEdge,
  onNodePositionsChange,
  onViewportChange,
}: CanvasFlowSurfaceProps) {
  const policy = policyForCanvasMode(mode);
  const derivedNodes = useMemo(
    () =>
      canvasToFlowNodes(canvas).map((node) => ({
        ...node,
        selected: node.id === selectedNodeId,
      })),
    [canvas, selectedNodeId],
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

  const handleNodesChange = useCallback((changes: NodeChange<CanvasFlowNode>[]) => {
    setNodes((current) => applyNodeChanges(changes, current));
  }, []);

  const handleEdgesChange = useCallback((changes: EdgeChange<CanvasFlowEdge>[]) => {
    setEdges((current) => applyEdgeChanges(changes, current));
  }, []);

  return (
    <ReactFlowProvider>
      <CanvasFlowPolicyProvider value={policy}>
        <div className="canvas-flow-surface" data-mode={mode}>
          <ReactFlow
            defaultViewport={cameraToViewport(canvas.camera)}
            deleteKeyCode={
              policy.canDeleteNodes || policy.canDeleteEdges ? "Backspace" : null
            }
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
            onEdgesChange={handleEdgesChange}
            onEdgeClick={(_, edge) => {
              onSelectEdge(edge.id);
              onSelectNode(null);
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
              }
            }}
            onNodesChange={handleNodesChange}
            onNodeClick={(_, node) => {
              onSelectNode(node.id);
              onSelectEdge(null);
            }}
            onPaneClick={() => {
              onSelectNode(null);
              onSelectEdge(null);
            }}
            panOnDrag={policy.canPanZoom}
            zoomOnDoubleClick={policy.canPanZoom}
            zoomOnPinch={policy.canPanZoom}
            zoomOnScroll={policy.canPanZoom}
          >
            <Background />
            <Controls showInteractive={false} />
          </ReactFlow>
          {selectedNode ? (
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
    </ReactFlowProvider>
  );
}

function noopRunNode() {}

function noopUpdateNode() {}
