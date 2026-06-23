import { useMutation, useQueryClient } from "@tanstack/react-query";
import type { CanvasCamera, CanvasPayload } from "../../lib/api";
import {
  batchUpdateNodePositions,
  updateCamera,
} from "../../lib/api";
import { CanvasFlowSurface } from "./CanvasFlowSurface";

interface StudioFlowCanvasProps {
  workspaceId: string;
  canvas: CanvasPayload;
  selectedNodeId: string | null;
  selectedGroupId: string | null;
  selectedEdgeId: string | null;
  onSelectNode: (nodeId: string | null) => void;
  onSelectGroup: (groupId: string | null) => void;
  onSelectEdge: (edgeId: string | null) => void;
  onCreateNodeAtPoint: (input: {
    flowPoint: { x: number; y: number };
    screenX: number;
    screenY: number;
  }) => void;
  onConnectNodes: (input: { fromNodeId: string; toNodeId: string }) => void;
  onGroupMove: (input: {
    groupId: string;
    deltaX: number;
    deltaY: number;
  }) => void;
}

export function StudioFlowCanvas({
  workspaceId,
  canvas,
  selectedNodeId,
  selectedGroupId,
  selectedEdgeId,
  onSelectNode,
  onSelectGroup,
  onSelectEdge,
  onCreateNodeAtPoint,
  onConnectNodes,
  onGroupMove,
}: StudioFlowCanvasProps) {
  const queryClient = useQueryClient();
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
    <CanvasFlowSurface
      canvas={canvas}
      mode="studio"
      selectedEdgeId={selectedEdgeId}
      selectedGroupId={selectedGroupId}
      selectedNodeId={selectedNodeId}
      onConnectNodes={onConnectNodes}
      onCreateNodeAtPoint={onCreateNodeAtPoint}
      onGroupMove={onGroupMove}
      onNodePositionsChange={(positions) => positionMutation.mutate(positions)}
      onSelectEdge={onSelectEdge}
      onSelectGroup={onSelectGroup}
      onSelectNode={onSelectNode}
      onViewportChange={(camera) => cameraMutation.mutate(camera)}
    />
  );
}
