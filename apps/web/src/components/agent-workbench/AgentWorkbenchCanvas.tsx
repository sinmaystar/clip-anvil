import { useMemo } from "react";
import {
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  type EdgeTypes,
  type NodeTypes,
} from "@xyflow/react";
import type { AgentWorkbenchProjection } from "../../lib/agentWorkbench";
import type { AgentWorkbenchSelection } from "../../lib/agentWorkbenchSelection";
import {
  agentWorkbenchToFlow,
  type AgentWorkbenchEdge as AgentWorkbenchFlowEdge,
  type AgentWorkbenchNode,
} from "../../lib/agentWorkbenchViewModel";
import { AgentProjectOverviewNode } from "./AgentProjectOverviewNode";
import { AgentSceneGroupNode } from "./AgentSceneGroupNode";
import { AgentShotNode } from "./AgentShotNode";
import { AgentWorkbenchEdge } from "./AgentWorkbenchEdge";
import { AgentWorkbenchSelectionProvider } from "./AgentWorkbenchSelectionContext";

interface AgentWorkbenchCanvasProps {
  workbench: AgentWorkbenchProjection;
  selected: AgentWorkbenchSelection | null;
  onSelectObject: (selection: AgentWorkbenchSelection | null) => void;
}

const nodeTypes: NodeTypes = {
  agentOverview: AgentProjectOverviewNode,
  agentScene: AgentSceneGroupNode,
  agentShot: AgentShotNode,
};

const edgeTypes: EdgeTypes = {
  agentWorkbench: AgentWorkbenchEdge,
};

export function AgentWorkbenchCanvas(props: AgentWorkbenchCanvasProps) {
  return (
    <ReactFlowProvider>
      <AgentWorkbenchCanvasContent {...props} />
    </ReactFlowProvider>
  );
}

function AgentWorkbenchCanvasContent({
  workbench,
  selected,
  onSelectObject,
}: AgentWorkbenchCanvasProps) {
  const flow = useMemo(() => agentWorkbenchToFlow(workbench), [workbench]);
  const nodes = useMemo(
    () =>
      flow.nodes.map((node) => ({
        ...node,
        selected: isFlowNodeSelected(node, selected),
      })),
    [flow.nodes, selected],
  );

  return (
    <div className="agent-workbench-surface">
      <AgentWorkbenchSelectionProvider
        selected={selected}
        onSelect={(selection) => onSelectObject(selection)}
      >
        <ReactFlow<AgentWorkbenchNode, AgentWorkbenchFlowEdge>
          defaultViewport={{ x: 24, y: 24, zoom: 0.78 }}
          deleteKeyCode={null}
          edgeTypes={edgeTypes}
          edges={flow.edges}
          edgesFocusable={false}
          elementsSelectable
          maxZoom={1.4}
          minZoom={0.2}
          nodeTypes={nodeTypes}
          nodes={nodes}
          nodesConnectable={false}
          nodesDraggable={false}
          nodesFocusable
          onNodeClick={(_, node) => onSelectObject(selectionForNode(node))}
          onPaneClick={() => onSelectObject(null)}
          panOnDrag
          zoomOnDoubleClick
          zoomOnPinch
          zoomOnScroll
        >
          <Background />
          <MiniMap
            className="canvas-flow-minimap"
            pannable
            position="bottom-left"
            zoomable
          />
          <Controls position="bottom-right" showInteractive={false} />
        </ReactFlow>
      </AgentWorkbenchSelectionProvider>
    </div>
  );
}

function selectionForNode(node: AgentWorkbenchNode): AgentWorkbenchSelection {
  if (node.type === "agentOverview") {
    return {
      objectType: "overview",
      objectId: node.data.workbench.overview.workspace_id,
      label: "Project Overview",
    };
  }
  if (node.type === "agentScene") {
    return {
      objectType: "scene",
      objectId: node.data.scene.id,
      label: node.data.scene.title,
    };
  }
  return {
    objectType: "shot",
    objectId: node.data.shot.id,
    label: node.data.shot.title,
  };
}

function isFlowNodeSelected(
  node: AgentWorkbenchNode,
  selected: AgentWorkbenchSelection | null,
) {
  if (!selected) {
    return false;
  }
  if (node.type === "agentOverview") {
    return (
      selected.objectType === "overview" &&
      selected.objectId === node.data.workbench.overview.workspace_id
    );
  }
  if (node.type === "agentScene") {
    return (
      selected.objectType === "scene" &&
      selected.objectId === node.data.scene.id
    );
  }
  return (
    selected.objectType === "shot" && selected.objectId === node.data.shot.id
  );
}
