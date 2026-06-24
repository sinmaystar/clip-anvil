import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { CanvasCamera, CanvasPayload } from "../../lib/api";
import {
  batchUpdateNodePositions,
  updateCamera,
} from "../../lib/api";
import { CanvasFlowSurface } from "./CanvasFlowSurface";

interface AgentFlowCanvasProps {
  workspaceId: string;
  canvas: CanvasPayload;
  selectedNodeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
}

export function AgentFlowCanvas({
  workspaceId,
  canvas,
  selectedNodeId,
  onSelectNode,
}: AgentFlowCanvasProps) {
  const queryClient = useQueryClient();
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);
  const positionMutation = useMutation({
    mutationFn: batchUpdateNodePositions,
    onSuccess: (_result, positions) => {
      const positionById = new Map(positions.map((position) => [position.id, position]));
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", workspaceId, "canvas"],
        (current) =>
          current
            ? {
                ...current,
                nodes: current.nodes.map((node) => {
                  const position = positionById.get(node.id);
                  return position
                    ? {
                        ...node,
                        canvas_x: position.canvas_x,
                        canvas_y: position.canvas_y,
                      }
                    : node;
                }),
              }
            : current,
      );
    },
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", workspaceId, "canvas"],
      });
    },
  });
  const cameraMutation = useMutation({
    mutationFn: (camera: CanvasCamera) => updateCamera(workspaceId, camera),
    onSuccess: (_result, camera) => {
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", workspaceId, "canvas"],
        (current) => (current ? { ...current, camera } : current),
      );
    },
  });

  return (
    <AgentFlowCanvasInner
      canvas={canvas}
      mode="agent"
      selectedEdgeId={selectedEdgeId}
      selectedNodeId={selectedNodeId}
      onNodePositionsChange={(positions) => positionMutation.mutate(positions)}
      onSelectEdge={setSelectedEdgeId}
      onSelectNode={onSelectNode}
      onViewportChange={(camera) => cameraMutation.mutate(camera)}
      renderInspector={false}
    />
  );
}

const AgentFlowCanvasInner = CanvasFlowSurface;
