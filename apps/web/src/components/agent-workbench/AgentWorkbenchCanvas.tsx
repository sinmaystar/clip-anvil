import { useCallback, useMemo, useState } from "react";
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
import type {
  AgentWorkbenchMediaDimensions,
  AgentWorkbenchMediaDimensionsByKey,
} from "../../lib/agentWorkbenchMediaLayout";
import { AgentProjectOverviewNode } from "./AgentProjectOverviewNode";
import { AgentFinalOutputNode } from "./AgentFinalOutputNode";
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
  agentFinalOutput: AgentFinalOutputNode,
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
  const [mediaDimensions, setMediaDimensions] =
    useState<AgentWorkbenchMediaDimensionsByKey>({});
  const [shotHeights, setShotHeights] = useState<
    Record<string, number | undefined>
  >({});
  const handleMediaDimensionsChange = useCallback(
    (key: string, dimensions: AgentWorkbenchMediaDimensions) => {
      setMediaDimensions((current) => {
        const previous = current[key];
        if (
          previous?.width === dimensions.width &&
          previous?.height === dimensions.height
        ) {
          return current;
        }
        return { ...current, [key]: dimensions };
      });
    },
    [],
  );
  const handleShotHeightChange = useCallback((shotId: string, height: number) => {
    setShotHeights((current) => {
      const previous = current[shotId];
      if (previous && Math.abs(previous - height) <= 1) {
        return current;
      }
      return { ...current, [shotId]: height };
    });
  }, []);
  const flow = useMemo(
    () => agentWorkbenchToFlow(workbench, mediaDimensions, shotHeights),
    [mediaDimensions, shotHeights, workbench],
  );
  const nodes = useMemo<AgentWorkbenchNode[]>(
    () =>
      flow.nodes.map((node) => {
        if (node.type === "agentShot") {
          return {
            ...node,
            data: {
              ...node.data,
              mediaDimensions,
              onMediaDimensionsChange: handleMediaDimensionsChange,
              onShotHeightChange: handleShotHeightChange,
            },
            selected: isFlowNodeSelected(node, selected),
          };
        }
        return {
          ...node,
          selected: isFlowNodeSelected(node, selected),
        };
      }),
    [
      flow.nodes,
      handleMediaDimensionsChange,
      handleShotHeightChange,
      mediaDimensions,
      selected,
    ],
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
  if (node.type === "agentFinalOutput") {
    return {
      objectType: "final_output",
      objectId: node.data.finalOutput.timeline_plan_id,
      label: "Final Output",
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
  if (node.type === "agentFinalOutput") {
    return (
      selected.objectType === "final_output" &&
      selected.objectId === node.data.finalOutput.timeline_plan_id
    );
  }
  return (
    selected.objectType === "shot" && selected.objectId === node.data.shot.id
  );
}
