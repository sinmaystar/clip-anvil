import { useMemo } from "react";
import {
  Background,
  Controls,
  ReactFlow,
  ReactFlowProvider,
  type EdgeTypes,
  type NodeTypes,
} from "@xyflow/react";
import type { CanvasPayload } from "../../lib/api";
import { canvasToFlowEdges, canvasToFlowNodes } from "./canvasViewModel";
import { cameraToViewport } from "./canvasViewport";
import { DependencyFlowEdge } from "./DependencyFlowEdge";
import { GroupFlowNode } from "./GroupFlowNode";
import { MediaFlowNode } from "./MediaFlowNode";
import { NodeInspectorPopover } from "./NodeInspectorPopover";
import type { CanvasFlowMode } from "./flowTypes";
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
}

export function CanvasFlowSurface({
  canvas,
  mode,
  selectedNodeId,
  selectedEdgeId,
  onSelectNode,
  onSelectEdge,
}: CanvasFlowSurfaceProps) {
  const policy = policyForCanvasMode(mode);
  const nodes = useMemo(
    () =>
      canvasToFlowNodes(canvas).map((node) => ({
        ...node,
        selected: node.id === selectedNodeId,
      })),
    [canvas, selectedNodeId],
  );
  const edges = useMemo(
    () =>
      canvasToFlowEdges(canvas).map((edge) => ({
        ...edge,
        selected: edge.id === selectedEdgeId,
      })),
    [canvas, selectedEdgeId],
  );
  const selectedNode = canvas.nodes.find((node) => node.id === selectedNodeId);

  return (
    <ReactFlowProvider>
      <CanvasFlowPolicyProvider value={policy}>
        <div className="canvas-flow-surface" data-mode={mode}>
          <ReactFlow
            defaultViewport={cameraToViewport(canvas.camera)}
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
            onEdgeClick={(_, edge) => {
              onSelectEdge(edge.id);
              onSelectNode(null);
            }}
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
