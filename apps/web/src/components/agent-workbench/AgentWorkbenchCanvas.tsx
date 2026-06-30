import { useCallback, useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  applyNodeChanges,
  Background,
  Controls,
  MiniMap,
  ReactFlow,
  ReactFlowProvider,
  type EdgeTypes,
  type NodeChange,
  type NodeTypes,
} from "@xyflow/react";
import {
  type AgentWorkbenchLayoutPosition,
  type AgentWorkbenchProjection,
} from "../../lib/agentWorkbench";
import { putAgentCanvasLayout } from "../../lib/agentApi";
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
import { AgentAudioNode } from "./AgentAudioNode";
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
  agentAudio: AgentAudioNode,
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
  const queryClient = useQueryClient();
  const workspaceId = workbench.overview.workspace_id;
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
  const baseNodes = useMemo<AgentWorkbenchNode[]>(
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
          };
        }
        return node;
      }),
    [
      flow.nodes,
      handleMediaDimensionsChange,
      handleShotHeightChange,
      mediaDimensions,
    ],
  );
  const [nodes, setNodes] = useState<AgentWorkbenchNode[]>(baseNodes);
  useEffect(() => {
    setNodes(baseNodes);
  }, [baseNodes]);
  const visibleNodes = useMemo(
    () =>
      nodes.map((node) => ({
        ...node,
        selected: isFlowNodeSelected(node, selected),
      })),
    [nodes, selected],
  );
  const layoutMutation = useMutation({
    mutationFn: (positions: AgentWorkbenchLayoutPosition[]) =>
      putAgentCanvasLayout(workspaceId, { positions }),
    onMutate: (positions) => {
      queryClient.setQueryData<AgentWorkbenchProjection>(
        ["workspace", workspaceId, "agent-workbench"],
        (current) => mergeWorkbenchLayoutPositions(current, positions),
      );
    },
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", workspaceId, "agent-workbench"],
      });
    },
  });
  const handleNodesChange = useCallback((changes: NodeChange<AgentWorkbenchNode>[]) => {
    setNodes((current) => applyNodeChanges(changes, current));
  }, []);
  const handleNodeDragStop = useCallback(
    (_: MouseEvent | TouchEvent, node: AgentWorkbenchNode) => {
      const position = layoutPositionForNode(node);
      if (!position) {
        return;
      }
      layoutMutation.mutate([position]);
    },
    [layoutMutation],
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
          nodes={visibleNodes}
          nodesConnectable={false}
          nodesDraggable
          nodesFocusable
          onNodeClick={(_, node) => onSelectObject(selectionForNode(node))}
          onNodeDragStop={handleNodeDragStop}
          onNodesChange={handleNodesChange}
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

function mergeWorkbenchLayoutPositions(
  workbench: AgentWorkbenchProjection | undefined,
  positions: AgentWorkbenchLayoutPosition[],
) {
  if (!workbench) {
    return workbench;
  }
  const byKey = new Map<string, AgentWorkbenchLayoutPosition>();
  for (const position of workbench.layout_positions ?? []) {
    byKey.set(layoutPositionKey(position), position);
  }
  for (const position of positions) {
    byKey.set(layoutPositionKey(position), position);
  }
  return {
    ...workbench,
    layout_positions: Array.from(byKey.values()),
  };
}

function layoutPositionKey(position: AgentWorkbenchLayoutPosition) {
  return `${position.object_type}:${position.object_id}`;
}

function layoutPositionForNode(
  node: AgentWorkbenchNode,
): AgentWorkbenchLayoutPosition | null {
  const selection = selectionForNode(node);
  if (!selection.objectId) {
    return null;
  }
  return {
    object_type: selection.objectType,
    object_id: selection.objectId,
    x: node.position.x,
    y: node.position.y,
  };
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
  if (node.type === "agentAudio") {
    return {
      objectType: "artifact",
      objectId: node.data.artifact.node_id || node.id,
      label: node.data.artifact.title || node.data.label,
    };
  }
  if (node.type === "agentFinalOutput") {
    return {
      objectType: "final_output",
      objectId:
        node.data.finalOutput.timeline_plan_id ||
        node.data.finalOutput.id ||
        node.data.finalOutput.output_node_id ||
        node.id,
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
  if (node.type === "agentAudio") {
    return (
      selected.objectType === "artifact" &&
      selected.objectId === node.data.artifact.node_id
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
