import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type CSSProperties,
  type MouseEvent,
  type PointerEvent as ReactPointerEvent,
  type SyntheticEvent,
} from "react";
import { Navigate, useNavigate, useParams } from "react-router";
import {
  ApiError,
  batchUpdateNodePositions,
  createMediaGroup,
  createMediaEdge,
  createMediaNode,
  deleteMediaGroup,
  deleteMediaEdge,
  deleteMediaNode,
  fetchModelCapabilities,
  fetchNodeProductionState,
  fetchCanvas,
  fetchReferencePackItems,
  fetchWorkspace,
  replaceMediaGroupNodes,
  replaceReferencePackItems,
  retryJob,
  runNode,
  selectNodeVersion,
  updateMediaGroup,
  updateMediaNode,
  type CanvasPayload,
  type CreateMediaNodeRequest,
  type MediaEdge,
  type MediaGroup,
  type MediaNode,
  type MediaType,
  type NodeProductionState,
} from "../lib/api";
import { AutoLayoutControls } from "../components/AutoLayoutControls";
import {
  isActiveNodeRunStatus,
  nodeStatusForGenerationStatus,
  overlayActiveNodeStatuses,
  productionStateWithSubmittedJob,
} from "../lib/canvasRunState";
import { ConnectionStatus } from "../components/ConnectionStatus";
import {
  FileDropZone,
  useCanvasFileUpload,
} from "../components/FileDropZone";
import { PropertyPanel } from "../components/PropertyPanel";
import { ResourceTree } from "../components/ResourceTree";
import {
  connectCanvasSocket,
  type CanvasConnectionStatus,
  type CanvasEvent,
} from "../lib/ws";
import { computeDagreLayout, type LayoutDirection } from "../lib/layout";
import { getGroupMemberMovePositions } from "../lib/groupLayout";
import { StudioFlowCanvas } from "../components/canvas-flow/StudioFlowCanvas";
import {
  connectionFailureFeedback,
  type ConnectionFeedback,
} from "../lib/connectionFeedback";
import {
  promptRefRenamePatch,
  promptRefsAfterSelect,
} from "../lib/promptRefs";
import { mergeNodeUpdateResponse } from "../lib/productionPanel";
import { isReferencePackMemberDependency } from "../lib/referencePack";
import {
  openArtifactViewInNewTab,
  type ArtifactViewSource,
} from "../lib/artifactViewer";
import { useAppearanceStore } from "../stores/appearance";
import { useAuthStore } from "../stores/auth";
import { workspaceModeRoute } from "../lib/workspaceRoutes";

interface CanvasContextMenu {
  screenX: number;
  screenY: number;
  flowPoint: { x: number; y: number };
}

interface SelectNodeEvent {
  nodeId: string;
}

interface NodeReviewRequestEvent extends ArtifactViewSource {
  nodeId: string;
}

interface SelectGroupEvent {
  groupId: string;
}

type NodeDraftPatch = Partial<
  Pick<MediaNode, "title" | "prompt" | "prompt_refs" | "prompt_rich">
>;

interface NodeEditorPosition {
  left: number;
  top: number;
  width: number;
  maxHeight: number;
}

const nodeCreateOptions: Array<{
  type: MediaType;
  title: string;
  description: string;
  icon: string;
  defaultTitle: string;
}> = [
  {
    type: "text",
    title: "文本节点",
    description: "提示词 / 文案 / 旁白",
    icon: "文本",
    defaultTitle: "未命名文本",
  },
  {
    type: "image",
    title: "图片节点",
    description: "参考图 / 产品图 / 画面素材",
    icon: "图片",
    defaultTitle: "未命名图片",
  },
  {
    type: "video",
    title: "视频节点",
    description: "镜头 / 片段 / 成片",
    icon: "视频",
    defaultTitle: "未命名视频",
  },
  {
    type: "audio",
    title: "音频节点",
    description: "配乐 / 旁白 / 音效",
    icon: "音频",
    defaultTitle: "未命名音频",
  },
  {
    type: "reference_pack",
    title: "参考包",
    description: "商品 / 角色 / 风格参考集合",
    icon: "参考包",
    defaultTitle: "未命名参考包",
  },
];

const layoutSafeInset = {
  collapsed: { x: 120, y: 112 },
  expanded: { x: 360, y: 112 },
} as const;

const nodeEditorSafeLeft = {
  collapsed: 88,
  expanded: 336,
} as const;

export function WorkspaceDetailPage() {
  const navigate = useNavigate();
  const { id } = useParams();
  const queryClient = useQueryClient();
  const canvasFrameRef = useRef<HTMLElement | null>(null);
  const nodeSnapshotsRef = useRef(new Map<string, MediaNode>());
  const titleSaveTimersRef = useRef(new Map<string, number>());
  const promptSaveTimersRef = useRef(new Map<string, number>());
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
  const [contextMenu, setContextMenu] = useState<CanvasContextMenu | null>(null);
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);
  const [nodeEditorPosition, setNodeEditorPosition] =
    useState<NodeEditorPosition | null>(null);
  const [connectionFeedback, setConnectionFeedback] =
    useState<ConnectionFeedback | null>(null);
  const [connectionStatus, setConnectionStatus] =
    useState<CanvasConnectionStatus>("offline");
  const [layoutDirection, setLayoutDirection] =
    useState<LayoutDirection>("LR");
  const [isLayouting, setIsLayouting] = useState(false);
  const [activeNodeRunStatuses, setActiveNodeRunStatuses] = useState<
    Record<string, MediaNode["status"] | undefined>
  >({});
  const toggleAppearance = useAppearanceStore((state) => state.toggleAppearance);
  const token = useAuthStore((state) => state.token);
  const account = useAuthStore((state) => state.account);
  const logout = useAuthStore((state) => state.logout);

  const workspaceQuery = useQuery({
    queryKey: ["workspace", id],
    queryFn: () => fetchWorkspace(id ?? ""),
    enabled: Boolean(id),
  });

  const canvasQuery = useQuery({
    queryKey: ["workspace", id, "canvas"],
    queryFn: () => fetchCanvas(id ?? ""),
    enabled: Boolean(id),
  });

  const modelCapabilitiesQuery = useQuery({
    queryKey: ["model-capabilities"],
    queryFn: fetchModelCapabilities,
  });

  const rawNodes = canvasQuery.data?.nodes ?? [];
  const nodes = useMemo(
    () => overlayActiveNodeStatuses(rawNodes, activeNodeRunStatuses),
    [activeNodeRunStatuses, rawNodes],
  );
  const effectiveCanvas = useMemo<CanvasPayload | undefined>(
    () => (canvasQuery.data ? { ...canvasQuery.data, nodes } : undefined),
    [canvasQuery.data, nodes],
  );
  const groups = canvasQuery.data?.groups ?? [];
  const selectedNode =
    nodes.find((node) => node.id === selectedNodeId) ?? null;
  const selectedGroup =
    groups.find((group) => group.id === selectedGroupId) ?? null;

  const selectedNodeProductionStateQuery = useQuery({
    queryKey: ["node", selectedNodeId, "production-state"],
    queryFn: () => fetchNodeProductionState(selectedNodeId ?? ""),
    enabled: Boolean(selectedNodeId),
  });

  const selectedReferencePackItemsQuery = useQuery({
    queryKey: ["reference-pack", selectedNodeId, "items"],
    queryFn: () => fetchReferencePackItems(selectedNodeId ?? ""),
    enabled: selectedNode?.node_type === "reference_pack",
  });

  useEffect(() => {
    for (const node of nodes) {
      nodeSnapshotsRef.current.set(node.id, node);
    }
  }, [nodes]);

  const screenToCanvasPoint = useCallback(
    (point: { x: number; y: number }) => {
      const frameRect = canvasFrameRef.current?.getBoundingClientRect();
      const camera = canvasQuery.data?.camera;
      if (!frameRect || !camera) {
        return null;
      }
      return {
        x: (point.x - frameRect.left - camera.x) / camera.zoom,
        y: (point.y - frameRect.top - camera.y) / camera.zoom,
      };
    },
    [canvasQuery.data?.camera],
  );

  const selectNode = useCallback((nodeId: string) => {
    setSelectedGroupId(null);
    setSelectedEdgeId(null);
    setSelectedNodeId(nodeId);
    announceActiveNode(nodeId);
  }, []);

  const selectGroup = useCallback((groupId: string) => {
    setSelectedNodeId(null);
    setSelectedEdgeId(null);
    setSelectedGroupId(groupId);
    announceActiveNode(null);
  }, []);

  const selectEdge = useCallback((edgeId: string) => {
    setSelectedNodeId(null);
    setSelectedGroupId(null);
    setSelectedEdgeId(edgeId);
    announceActiveNode(null);
  }, []);

  const hideActiveNodeEditor = useCallback(() => {
    setSelectedGroupId(null);
    setSelectedNodeId(null);
    setSelectedEdgeId(null);
    announceActiveNode(null);
  }, []);

  const createGroupMutation = useMutation({
    mutationFn: async () => {
      if (!id) {
        throw new Error("缺少工作区");
      }
      return createMediaGroup({
        workspace_id: id,
        name: "新建分组",
        node_ids: [],
      });
    },
    onSuccess: (response) => {
      const group = groupResponseToMediaGroup(response);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => appendCanvasGroup(current, group),
      );
      setSelectedGroupId(group.id);
      setSelectedNodeId(null);
      announceActiveNode(null);
    },
  });

  const replaceGroupNodesMutation = useMutation({
    mutationFn: async (input: { groupId: string; nodeIds: string[] }) =>
      replaceMediaGroupNodes(input.groupId, input.nodeIds),
    onSuccess: (response) => {
      const group = groupResponseToMediaGroup(response);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => appendCanvasGroup(current, group),
      );
      setSelectedGroupId(group.id);
    },
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
  });

  const renameGroupMutation = useMutation({
    mutationFn: async (input: { groupId: string; name: string }) =>
      updateMediaGroup(input.groupId, { name: input.name }),
    onSuccess: (groupWithoutMembers, input) => {
      const currentGroup = groups.find((group) => group.id === input.groupId);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) =>
          current
            ? appendCanvasGroup(current, {
                ...groupWithoutMembers,
                node_ids: currentGroup?.node_ids ?? [],
              })
            : current,
      );
    },
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
  });

  const deleteGroupMutation = useMutation({
    mutationFn: deleteMediaGroup,
    onMutate: (groupId) => {
      setSelectedGroupId(null);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => removeCanvasGroup(current, groupId),
      );
    },
    onError: () => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
  });

  const createNodeMutation = useMutation({
    mutationFn: async (input?: {
      point?: { x: number; y: number };
      nodeType?: MediaType;
      patch?: Partial<CreateMediaNodeRequest>;
    }) => {
      if (!id) {
        throw new Error("画布尚未准备好");
      }

      const nodeType = input?.nodeType ?? "text";
      const option =
        nodeCreateOptions.find((item) => item.type === nodeType) ??
        nodeCreateOptions[0];
      const position = input?.point ?? { x: 120, y: 120 };
      return createMediaNode({
        workspace_id: id,
        node_type: nodeType,
        title: input?.patch?.title ?? option.defaultTitle,
        prompt: input?.patch?.prompt,
        status: input?.patch?.status,
        operation_type: input?.patch?.operation_type,
        model_provider: input?.patch?.model_provider,
        model_id: input?.patch?.model_id,
        model_params: input?.patch?.model_params,
        canvas_x: position.x - 100,
        canvas_y: position.y - 60,
      });
    },
    onSuccess: (node) => {
      nodeSnapshotsRef.current.set(node.id, node);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => appendCanvasNode(current, node),
      );
      selectNode(node.id);
      setContextMenu(null);
    },
  });

  const updateNodeMutation = useMutation({
    mutationFn: async (input: {
      nodeId: string;
      patch: Parameters<typeof updateMediaNode>[1];
    }) => updateMediaNode(input.nodeId, input.patch),
    onSuccess: (node, input) => {
      const current = queryClient.getQueryData<CanvasPayload>([
        "workspace",
        id,
        "canvas",
      ]);
      const currentNode = current?.nodes.find((item) => item.id === node.id);
      const mergedNode = mergeNodeUpdateResponse(
        currentNode,
        node,
        input.patch,
      );
      nodeSnapshotsRef.current.set(mergedNode.id, mergedNode);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (payload) => replaceCanvasNode(payload, mergedNode),
      );
      void queryClient.invalidateQueries({
        queryKey: ["node", mergedNode.id, "production-state"],
      });
      if (
        "title" in input.patch &&
        current &&
        currentNode &&
        currentNode.title !== mergedNode.title
      ) {
        for (const item of promptRefRenamePatches(current.nodes, mergedNode)) {
          void updateMediaNode(item.nodeId, item.patch)
            .then((updatedNode) => {
              queryClient.setQueryData<CanvasPayload>(
                ["workspace", id, "canvas"],
                (payload) => replaceCanvasNode(payload, updatedNode),
              );
            })
            .catch(() => {
              void queryClient.invalidateQueries({
                queryKey: ["workspace", id, "canvas"],
              });
            });
        }
      }
	    },
    onError: (_error, input) => {
      void queryClient.invalidateQueries({
        queryKey: ["node", input.nodeId, "production-state"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
  });

  const setActiveNodeRunStatus = useCallback(
    (nodeId: string, status: MediaNode["status"]) => {
      setActiveNodeRunStatuses((current) => {
        const next = { ...current };
        if (isActiveNodeRunStatus(status)) {
          next[nodeId] = status;
        } else {
          delete next[nodeId];
        }
        return next;
      });
    },
    [],
  );

  const markNodeRunStatusOptimistically = useCallback(
    (nodeId: string, status: MediaNode["status"] = "running") => {
      setActiveNodeRunStatus(nodeId, status);
      const fallbackNode =
        selectedNode?.id === nodeId
          ? selectedNode
          : nodes.find((node) => node.id === nodeId) ?? null;
      let runningNode = fallbackNode ? { ...fallbackNode, status } : null;
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (payload) => {
          const node = payload?.nodes.find((item) => item.id === nodeId);
          runningNode = node ? { ...node, status } : runningNode;
          return payload
            ? updateCanvasNodeStatus(payload, nodeId, status)
            : payload;
        },
      );
    },
    [id, nodes, queryClient, selectedNode, setActiveNodeRunStatus],
  );

  const runNodeMutation = useMutation({
    mutationFn: async (nodeId: string) => runNode(nodeId),
    onMutate: (nodeId) => {
      markNodeRunStatusOptimistically(nodeId, "running");
    },
    onSuccess: (data, nodeId) => {
      markNodeRunStatusOptimistically(
        nodeId,
        nodeStatusForGenerationStatus(data.job.status) ?? "running",
      );
      queryClient.setQueryData<NodeProductionState>(
        ["node", nodeId, "production-state"],
        (state) => productionStateWithSubmittedJob(state, data.job, data.version),
      );
    },
    onError: (_error, nodeId) => {
      void queryClient.invalidateQueries({
        queryKey: ["node", nodeId, "production-state"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
  });

  const runNodeWithOptionalPatch = useCallback(
    (nodeId: string, patch?: Parameters<typeof updateMediaNode>[1]) => {
      if (patch && Object.keys(patch).length > 0) {
        markNodeRunStatusOptimistically(nodeId, "running");
        updateNodeMutation.mutate(
          { nodeId, patch },
          {
            onSuccess: () => runNodeMutation.mutate(nodeId),
          },
        );
        return;
      }
      runNodeMutation.mutate(nodeId);
    },
    [markNodeRunStatusOptimistically, runNodeMutation, updateNodeMutation],
  );

  const retryJobMutation = useMutation({
    mutationFn: async (jobId: string) => retryJob(jobId),
    onError: (error: unknown) => {
      setConnectionFeedback({
        title: "重试失败",
        description:
          error instanceof Error ? error.message : "任务重试失败，请稍后再试。",
        tone: "danger",
      });
    },
    onSettled: () => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["node", selectedNodeId, "production-state"],
      });
    },
  });

  const selectVersionMutation = useMutation({
    mutationFn: async (input: { nodeId: string; versionId: string }) =>
      selectNodeVersion(input.nodeId, input.versionId),
    onSuccess: (_data, input) => {
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["node", input.nodeId, "production-state"],
      });
    },
  });

  const replaceReferencePackItemsMutation = useMutation({
    mutationFn: async (input: {
      packNodeId: string;
      memberNodeIds: string[];
    }) => replaceReferencePackItems(input.packNodeId, input.memberNodeIds),
    onSuccess: (items, input) => {
      queryClient.setQueryData(
        ["reference-pack", input.packNodeId, "items"],
        items,
      );
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["node"],
      });
    },
    onError: (_error, input) => {
      void queryClient.invalidateQueries({
        queryKey: ["reference-pack", input.packNodeId, "items"],
      });
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
  });

  const createDependencyEdge = useCallback(
    (fromNodeId: string, toNodeId: string) => {
      if (!id || fromNodeId === toNodeId) {
        return;
      }
      const fromNode = nodes.find((node) => node.id === fromNodeId);
      if (
        fromNode?.node_type === "reference_pack" &&
        isReferencePackMemberDependency(
          fromNodeId,
          toNodeId,
          (fromNode.reference_pack_preview?.members ?? []).map((member) => ({
            id: member.id,
            pack_node_id: fromNodeId,
            member_node_id: member.id,
            position: 0,
          })),
        )
      ) {
        setConnectionFeedback({
          title: "不能连接参考包成员",
          description: "Reference Pack 不能作为其成员节点的输入。",
          tone: "danger",
        });
        return;
      }

      void createMediaEdge({
        workspace_id: id,
        from_node_id: fromNodeId,
        to_node_id: toNodeId,
      })
        .then((edge) => {
          queryClient.setQueryData<CanvasPayload>(
            ["workspace", id, "canvas"],
            (payload) => appendCanvasEdge(payload, edge),
          );
        })
        .catch((error: unknown) => {
          setConnectionFeedback(
            connectionFailureFeedback(
              error instanceof ApiError ? error.status : null,
            ),
          );
        });
    },
    [id, nodes, queryClient],
  );

  const deleteEdgeById = useCallback(
    (edgeId: string) => {
      if (!id) {
        return;
      }
      setSelectedEdgeId((current) => (current === edgeId ? null : current));
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => removeCanvasEdge(current, edgeId),
      );
      void deleteMediaEdge(edgeId).catch(() => {
        void queryClient.invalidateQueries({
          queryKey: ["workspace", id, "canvas"],
        });
      });
    },
    [id, queryClient],
  );

  const deleteNodeById = useCallback(
    (nodeId: string) => {
      if (!id) {
        return;
      }
      setSelectedNodeId((current) => (current === nodeId ? null : current));
      setSelectedEdgeId(null);
      nodeSnapshotsRef.current.delete(nodeId);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => removeCanvasNode(current, nodeId),
      );
      void deleteMediaNode(nodeId).catch(() => {
        void queryClient.invalidateQueries({
          queryKey: ["workspace", id, "canvas"],
        });
      });
    },
    [id, queryClient],
  );

  const appendAssetNodeToCanvas = useCallback(
    (node: MediaNode) => {
      nodeSnapshotsRef.current.set(node.id, node);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => appendCanvasNode(current, node),
      );
      selectNode(node.id);
    },
    [id, queryClient, selectNode],
  );

  const {
    isUploading: isUploadingAsset,
    uploadError: assetUploadError,
    uploadFiles: uploadAssetFiles,
  } = useCanvasFileUpload({
    workspaceId: id ?? "",
    onAssetNodeCreated: appendAssetNodeToCanvas,
  });

  const replaceGroupMembers = useCallback(
    (groupId: string, nodeIds: string[]) => {
      replaceGroupNodesMutation.mutate({ groupId, nodeIds });
    },
    [replaceGroupNodesMutation],
  );

  const addGroupMember = useCallback(
    (groupId: string, nodeId: string) => {
      const group = groups.find((item) => item.id === groupId);
      if (!group) {
        return;
      }
      replaceGroupMembers(groupId, Array.from(new Set([...group.node_ids, nodeId])));
    },
    [groups, replaceGroupMembers],
  );

  const removeGroupMember = useCallback(
    (groupId: string, nodeId: string) => {
      const group = groups.find((item) => item.id === groupId);
      if (!group) {
        return;
      }
      replaceGroupMembers(
        groupId,
        group.node_ids.filter((id) => id !== nodeId),
      );
    },
    [groups, replaceGroupMembers],
  );

  const moveGroupMembers = useCallback(
    (input: { groupId: string; deltaX: number; deltaY: number }) => {
      const group = groups.find((item) => item.id === input.groupId);
      if (!group || !id) {
        return;
      }
      const positions = getGroupMemberMovePositions({
        group,
        nodes,
        deltaX: input.deltaX,
        deltaY: input.deltaY,
      });
      if (positions.length === 0) {
        return;
      }
      const positionById = new Map(
        positions.map((position) => [position.id, position]),
      );
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => {
          if (!current) {
            return current;
          }
          const nextNodes = current.nodes.map((node) => {
            const position = positionById.get(node.id);
            return position
              ? {
                  ...node,
                  canvas_x: position.canvas_x,
                  canvas_y: position.canvas_y,
                }
              : node;
          });
          for (const node of nextNodes) {
            nodeSnapshotsRef.current.set(node.id, node);
          }
          return { ...current, nodes: nextNodes };
        },
      );
      void batchUpdateNodePositions(positions).catch(() => {
        void queryClient.invalidateQueries({
          queryKey: ["workspace", id, "canvas"],
        });
      });
    },
    [groups, id, nodes, queryClient],
  );

  const applyCanvasEvent = useCallback(
    (event: CanvasEvent) => {
      if (!id) {
        return;
      }
      switch (event.type) {
        case "NodeCreated":
        case "NodeUpdated": {
          const node = canvasNodeFromEventPayload(event.payload.node);
          if (node) {
            queryClient.setQueryData<CanvasPayload>(
              ["workspace", id, "canvas"],
              (current) => appendCanvasNode(current, node),
            );
          }
          void queryClient.invalidateQueries({
            queryKey: ["workspace", id, "canvas"],
          });
          break;
        }
        case "NodeDeleted":
        case "EdgeCreated":
        case "EdgeDeleted":
        case "GroupCreated":
        case "GroupUpdated":
        case "GroupDeleted":
          void queryClient.invalidateQueries({
            queryKey: ["workspace", id, "canvas"],
          });
          break;
        case "production.job.updated":
        case "production.model.delta": {
          const nodeStatus = nodeStatusForGenerationStatus(
            event.payload.status,
          );
          if (nodeStatus) {
            setActiveNodeRunStatus(event.payload.node_id, nodeStatus);
            queryClient.setQueryData<CanvasPayload>(
              ["workspace", id, "canvas"],
              (payload) =>
                updateCanvasNodeStatus(
                  payload,
                  event.payload.node_id,
                  nodeStatus,
                ),
            );
          }
          if (
            event.type === "production.job.updated" &&
            nodeStatus &&
            !isActiveNodeRunStatus(nodeStatus)
          ) {
            void queryClient.invalidateQueries({
              queryKey: ["workspace", id, "canvas"],
            });
            void queryClient.invalidateQueries({
              queryKey: ["node", event.payload.node_id, "production-state"],
            });
          }
          break;
        }
      }
    },
    [id, queryClient, setActiveNodeRunStatus],
  );

  const runAutoLayout = useCallback(() => {
    if (!id || !canvasQuery.data) {
      return;
    }
    const frameRect = canvasFrameRef.current?.getBoundingClientRect();
    const safeInset = isSidebarCollapsed
      ? layoutSafeInset.collapsed
      : layoutSafeInset.expanded;
    const origin = frameRect
      ? (screenToCanvasPoint({
          x: frameRect.left + safeInset.x,
          y: frameRect.top + safeInset.y,
        }) ?? safeInset)
      : safeInset;

    setIsLayouting(true);
    const result = computeDagreLayout({
      nodes: canvasQuery.data.nodes,
      edges: canvasQuery.data.edges,
      groups: canvasQuery.data.groups,
      direction: layoutDirection,
      origin,
    });

    const positionByID = new Map(
      result.positions.map((position) => [position.id, position]),
    );
    queryClient.setQueryData<CanvasPayload>(
      ["workspace", id, "canvas"],
      (current) => {
        if (!current) {
          return current;
        }
        const nodes = current.nodes.map((node) => {
          const position = positionByID.get(node.id);
          return position
            ? {
                ...node,
                canvas_x: position.canvas_x,
                canvas_y: position.canvas_y,
              }
            : node;
        });
        for (const node of nodes) {
          nodeSnapshotsRef.current.set(node.id, node);
        }
        return { ...current, nodes };
      },
    );

    void batchUpdateNodePositions(result.positions)
      .catch(() => {
        void queryClient.invalidateQueries({
          queryKey: ["workspace", id, "canvas"],
        });
      })
      .finally(() => setIsLayouting(false));
  }, [
    canvasQuery.data,
    id,
    isSidebarCollapsed,
    layoutDirection,
    queryClient,
    screenToCanvasPoint,
  ]);

  useEffect(() => {
    if (!selectedNode && !selectedGroup) {
      setNodeEditorPosition(null);
      return;
    }
    const frameRect = canvasFrameRef.current?.getBoundingClientRect();
    if (!frameRect) {
      setNodeEditorPosition(null);
      return;
    }

    const safeLeft = isSidebarCollapsed
      ? nodeEditorSafeLeft.collapsed
      : nodeEditorSafeLeft.expanded;
    const width = Math.min(720, Math.max(520, frameRect.width - safeLeft - 24));
    const maxHeight = Math.round(
      Math.min(720, Math.max(320, frameRect.height - 32)),
    );
    const maxTop = Math.max(16, frameRect.height - maxHeight - 16);
    if (!selectedNode || !effectiveCanvas) {
      setNodeEditorPosition({
        left: safeLeft,
        top: Math.round(clamp(112, 16, maxTop)),
        width: Math.round(width),
        maxHeight,
      });
      return;
    }
    const bottomLeft = {
      x: selectedNode.canvas_x * effectiveCanvas.camera.zoom + effectiveCanvas.camera.x,
      y:
        (selectedNode.canvas_y + selectedNode.canvas_h) *
          effectiveCanvas.camera.zoom +
        effectiveCanvas.camera.y,
    };
    setNodeEditorPosition({
      left: Math.round(
        clamp(bottomLeft.x, safeLeft, Math.max(12, frameRect.width - width - 12)),
      ),
      top: Math.round(clamp(bottomLeft.y + 12, 16, maxTop)),
      width: Math.round(width),
      maxHeight,
    });
  }, [effectiveCanvas, isSidebarCollapsed, selectedGroup, selectedNode]);

  const applyNodeDraft = useCallback(
    (nodeId: string, patch: NodeDraftPatch) => {
      const current = queryClient.getQueryData<CanvasPayload>([
        "workspace",
        id,
        "canvas",
      ]);
      const baseNode =
        nodeSnapshotsRef.current.get(nodeId) ??
        current?.nodes.find((node) => node.id === nodeId);
      if (!baseNode) {
        return null;
      }

      const nextNode = { ...baseNode, ...patch };
      nodeSnapshotsRef.current.set(nodeId, nextNode);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (payload) => replaceCanvasNode(payload, nextNode),
      );
      return nextNode;
    },
    [id, queryClient],
  );

  const commitNodePatch = useCallback(
    (nodeId: string, patch: NodeDraftPatch) => {
      if (!id) {
        return;
      }
      const nextNode = applyNodeDraft(nodeId, patch);
      if (!nextNode) {
        return;
      }

      if ("title" in patch) {
        window.clearTimeout(titleSaveTimersRef.current.get(nodeId));
        titleSaveTimersRef.current.delete(nodeId);
      }
      if ("prompt" in patch) {
        window.clearTimeout(promptSaveTimersRef.current.get(nodeId));
        promptSaveTimersRef.current.delete(nodeId);
      }

      void updateMediaNode(nodeId, patch)
        .then((node) => {
          const current = queryClient.getQueryData<CanvasPayload>([
            "workspace",
            id,
            "canvas",
          ]);
          const currentNode = current?.nodes.find(
            (item) => item.id === node.id,
          );
          const mergedNode = mergeNodeUpdateResponse(currentNode, node, patch);
          nodeSnapshotsRef.current.set(node.id, mergedNode);
          queryClient.setQueryData<CanvasPayload>(
            ["workspace", id, "canvas"],
            (payload) => replaceCanvasNode(payload, mergedNode),
          );
        })
        .catch(() => {
          void queryClient.invalidateQueries({
            queryKey: ["workspace", id, "canvas"],
          });
        });
    },
    [applyNodeDraft, id, queryClient],
  );

  const commitPromptRefSelection = useCallback(
    (targetNode: MediaNode, refNode: MediaNode, prompt: string) => {
      const hasEdge = (canvasQuery.data?.edges ?? []).some(
        (edge) =>
          edge.from_node_id === refNode.id && edge.to_node_id === targetNode.id,
      );
      if (!hasEdge && id) {
        createDependencyEdge(refNode.id, targetNode.id);
      }
      commitNodePatch(targetNode.id, {
        prompt,
        prompt_refs: promptRefsAfterSelect(targetNode.prompt_refs, refNode),
        prompt_rich: { version: 1, source: "textarea-at", text: prompt },
      });
    },
    [canvasQuery.data?.edges, commitNodePatch, createDependencyEdge, id],
  );

  const openCanvasMenu = useCallback((event: MouseEvent<HTMLElement>) => {
    if (
      event.target instanceof HTMLElement &&
      event.target.closest(".node-production-popover")
    ) {
      event.stopPropagation();
    }
  }, []);

  const stopCanvasEvent = useCallback((event: SyntheticEvent) => {
    event.stopPropagation();
  }, []);

  useEffect(() => {
    if (!id || !token) {
      setConnectionStatus("offline");
      return;
    }
    return connectCanvasSocket({
      workspaceId: id,
      token,
      onEvent: applyCanvasEvent,
      onReconnect: () => {
        void queryClient.invalidateQueries({
          queryKey: ["workspace", id, "canvas"],
        });
      },
      onStatusChange: setConnectionStatus,
    });
  }, [applyCanvasEvent, id, queryClient, token]);

  useEffect(() => {
    const onSelectNode = (event: Event) => {
      const detail = (event as CustomEvent<SelectNodeEvent>).detail;
      if (detail?.nodeId) {
        selectNode(detail.nodeId);
      }
    };

    window.addEventListener("clip-anvil:select-node", onSelectNode);
    return () => {
      window.removeEventListener("clip-anvil:select-node", onSelectNode);
    };
  }, [selectNode]);

  useEffect(() => {
    const onNodeReviewRequest = (event: Event) => {
      const detail = (event as CustomEvent<NodeReviewRequestEvent>).detail;
      if (!detail?.nodeId) {
        return;
      }
      selectNode(detail.nodeId);
      openArtifactViewInNewTab(detail);
    };

    window.addEventListener(
      "clip-anvil:node-review-request",
      onNodeReviewRequest,
    );
    return () => {
      window.removeEventListener(
        "clip-anvil:node-review-request",
        onNodeReviewRequest,
      );
    };
  }, [selectNode]);

  useEffect(() => {
    const onSelectGroup = (event: Event) => {
      const detail = (event as CustomEvent<SelectGroupEvent>).detail;
      if (detail?.groupId) {
        selectGroup(detail.groupId);
      }
    };

    window.addEventListener("clip-anvil:select-group", onSelectGroup);
    return () => {
      window.removeEventListener("clip-anvil:select-group", onSelectGroup);
    };
  }, [selectGroup]);

  useEffect(() => {
    if (!connectionFeedback) {
      return;
    }

    const timeout = window.setTimeout(() => {
      setConnectionFeedback(null);
    }, connectionFeedback.tone === "danger" ? 4800 : 3200);

    return () => window.clearTimeout(timeout);
  }, [connectionFeedback]);

  useEffect(() => {
    const hideOnOutsidePointerDown = (event: PointerEvent) => {
      if (!(event.target instanceof HTMLElement)) {
        return;
      }
      if (
        event.target.closest(
          ".media-node-shell, .node-editor-overlay, .resource-tree, .studio-context-menu, .connection-overlay",
        )
      ) {
        return;
      }
      hideActiveNodeEditor();
    };

    const hideOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !isEditableTarget(event.target)) {
        hideActiveNodeEditor();
      }
    };

    window.addEventListener("pointerdown", hideOnOutsidePointerDown, true);
    window.addEventListener("keydown", hideOnEscape);
    return () => {
      window.removeEventListener("pointerdown", hideOnOutsidePointerDown, true);
      window.removeEventListener("keydown", hideOnEscape);
    };
  }, [hideActiveNodeEditor]);

  useEffect(() => {
    const deleteSelectedCanvasItem = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) {
        return;
      }
      if (event.key !== "Delete" && event.key !== "Backspace") {
        return;
      }
      if (!selectedEdgeId && !selectedNodeId) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
      if (selectedEdgeId) {
        deleteEdgeById(selectedEdgeId);
        return;
      }
      if (selectedNodeId) {
        deleteNodeById(selectedNodeId);
      }
    };

    window.addEventListener("keydown", deleteSelectedCanvasItem, true);
    return () => {
      window.removeEventListener("keydown", deleteSelectedCanvasItem, true);
    };
  }, [deleteEdgeById, deleteNodeById, selectedEdgeId, selectedNodeId]);

  useEffect(() => {
    return () => {
      for (const timer of titleSaveTimersRef.current.values()) {
        window.clearTimeout(timer);
      }
      titleSaveTimersRef.current.clear();
      for (const timer of promptSaveTimersRef.current.values()) {
        window.clearTimeout(timer);
      }
      promptSaveTimersRef.current.clear();
      announceActiveNode(null);
    };
  }, []);

  useEffect(() => {
    if (selectedNodeId && !nodes.some((node) => node.id === selectedNodeId)) {
      setSelectedNodeId(null);
      announceActiveNode(null);
    }
  }, [nodes, selectedNodeId]);

  useEffect(() => {
    if (
      selectedGroupId &&
      !groups.some((group) => group.id === selectedGroupId)
    ) {
      setSelectedGroupId(null);
    }
  }, [groups, selectedGroupId]);

  useEffect(() => {
    if (
      selectedEdgeId &&
      !canvasQuery.data?.edges.some((edge) => edge.id === selectedEdgeId)
    ) {
      setSelectedEdgeId(null);
    }
  }, [canvasQuery.data?.edges, selectedEdgeId]);

  useEffect(() => {
    const blockToolShortcuts = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) {
        return;
      }
      if (isEditableTarget(event.target)) {
        return;
      }
      const key = event.key.toLowerCase();
      if (key !== "d" && key !== "e") {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
    };

    window.addEventListener("keydown", blockToolShortcuts, true);
    return () => {
      window.removeEventListener("keydown", blockToolShortcuts, true);
    };
  }, []);

  useEffect(() => {
    if (!contextMenu) {
      return;
    }

    const closeMenu = () => setContextMenu(null);
    window.addEventListener("click", closeMenu);
    window.addEventListener("keydown", closeMenu);
    return () => {
      window.removeEventListener("click", closeMenu);
      window.removeEventListener("keydown", closeMenu);
    };
  }, [contextMenu]);

  const closeCanvasMenuOnPointerDown = useCallback(
    (event: ReactPointerEvent<HTMLElement>) => {
      if (!contextMenu) {
        return;
      }
      if (
        event.target instanceof HTMLElement &&
        event.target.closest(".studio-context-menu")
      ) {
        return;
      }
      setContextMenu(null);
    },
    [contextMenu],
  );

  const handleLogout = () => {
    logout();
    navigate("/login", { replace: true });
  };

  if (workspaceQuery.data && workspaceQuery.data.mode !== "studio") {
    return (
      <Navigate
        to={workspaceModeRoute(workspaceQuery.data.id, workspaceQuery.data.mode)}
        replace
      />
    );
  }

  return (
    <main
      className="studio-shell"
      data-sidebar={isSidebarCollapsed ? "collapsed" : "expanded"}
    >
      <aside
        className="studio-sidebar"
        data-collapsed={isSidebarCollapsed}
      >
        {isSidebarCollapsed ? (
          <div className="studio-sidebar-collapsed">
            <button
              aria-label="展开侧边栏"
              className="studio-sidebar-peek"
              onClick={() => setIsSidebarCollapsed(false)}
              type="button"
            >
              <span className="studio-peek-mark">影</span>
              <span className="studio-peek-title">
                {workspaceQuery.data?.name ?? "Studio"}
              </span>
              <span className="studio-peek-arrow">›</span>
            </button>
          </div>
        ) : (
          <>
            <div className="studio-sidebar-header">
              <div className="studio-brand-row">
                <div className="studio-brand">
                  <p className="studio-brand-kicker">Studio</p>
                  <h1 className="studio-brand-title">
                    {workspaceQuery.data?.name ?? "加载中"}
                  </h1>
                  <ConnectionStatus status={connectionStatus} />
                </div>
                <button
                  aria-label="收起侧边栏"
                  className="studio-icon-button studio-sidebar-toggle"
                  onClick={() => setIsSidebarCollapsed(true)}
                  type="button"
                >
                  ‹
                </button>
              </div>
            </div>

            <div className="studio-sidebar-body">
              <div className="studio-action-row">
                <button
                  className="studio-secondary-button studio-nav-button"
                  onClick={() => navigate("/workspaces")}
                  type="button"
                >
                  <span className="studio-action-label">项目列表</span>
                </button>
                <button
                  aria-label="切换明暗模式"
                  className="studio-icon-button studio-theme-toggle"
                  onClick={toggleAppearance}
                  type="button"
                >
                  ◐
                </button>
              </div>

              <div className="mt-5">
                {nodes.length > 0 ? (
                  <ResourceTree
                    groups={groups}
                    nodes={nodes}
                    selectedGroupId={selectedGroupId}
                    selectedNodeId={selectedNodeId}
                    onCreateGroup={() => createGroupMutation.mutate()}
                    onSelectGroup={selectGroup}
                    onSelectNode={selectNode}
                  />
                ) : (
                  <div className="rounded-[var(--radius-md)] p-3 text-[12px] leading-5 text-[var(--fg-tertiary)]">
                    在画布空白处右键，选择节点类型开始。
                  </div>
                )}
              </div>

            </div>

            <div className="studio-sidebar-footer">
              <div className="flex items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate text-[12px] font-medium text-[var(--fg-secondary)]">
                    {account?.name ?? "未登录"}
                  </p>
                  <p className="mt-1 truncate text-[11.5px] text-[var(--fg-tertiary)]">
                    {account?.email}
                  </p>
                </div>
                <button
                  className="studio-secondary-button studio-logout-button"
                  onClick={handleLogout}
                  type="button"
                >
                  登出
                </button>
              </div>
            </div>
          </>
        )}
      </aside>

      <section
        className="studio-canvas-frame"
        onContextMenuCapture={openCanvasMenu}
        onPointerDownCapture={closeCanvasMenuOnPointerDown}
        ref={canvasFrameRef}
      >
        {workspaceQuery.isError || canvasQuery.isError ? (
          <div className="flex h-full items-center justify-center text-[13px] text-[var(--fg-tertiary)]">
            画布加载失败
          </div>
        ) : canvasQuery.data ? (
          <div className="studio-canvas-host">
            <StudioFlowCanvas
              canvas={effectiveCanvas ?? canvasQuery.data}
              onConnectNodes={({ fromNodeId, toNodeId }) => {
                createDependencyEdge(fromNodeId, toNodeId);
              }}
              onCreateNodeAtPoint={({ flowPoint, screenX, screenY }) => {
                setContextMenu({
                  flowPoint,
                  screenX,
                  screenY,
                });
              }}
              onGroupMove={moveGroupMembers}
              onSelectEdge={(edgeId) => {
                if (edgeId) {
                  selectEdge(edgeId);
                } else {
                  setSelectedEdgeId(null);
                }
              }}
              onSelectNode={(nodeId) => {
                if (nodeId) {
                  selectNode(nodeId);
                } else {
                  hideActiveNodeEditor();
                }
              }}
              onSelectGroup={(groupId) => {
                if (groupId) {
                  selectGroup(groupId);
                } else {
                  setSelectedGroupId(null);
                }
              }}
              selectedEdgeId={selectedEdgeId}
              selectedGroupId={selectedGroupId}
              selectedNodeId={selectedNodeId}
              workspaceId={id ?? ""}
            />
          </div>
        ) : (
          <div className="flex h-full items-center justify-center text-[13px] text-[var(--fg-tertiary)]">
            正在加载画布
          </div>
        )}

        {(selectedNode || selectedGroup) && nodeEditorPosition ? (
          <div
            className="node-editor-overlay node-production-popover"
            onClick={stopCanvasEvent}
            onContextMenu={stopCanvasEvent}
            onKeyDown={stopCanvasEvent}
            onPointerDown={stopCanvasEvent}
            onWheel={stopCanvasEvent}
            style={{
              left: nodeEditorPosition.left,
              top: nodeEditorPosition.top,
              width: nodeEditorPosition.width,
              maxHeight: nodeEditorPosition.maxHeight,
              "--node-editor-max-height": `${nodeEditorPosition.maxHeight}px`,
            } as CSSProperties}
          >
            <PropertyPanel
              edges={canvasQuery.data?.edges ?? []}
              groups={groups}
              isModelCapabilitiesLoading={modelCapabilitiesQuery.isLoading}
              isProductionStateLoading={
                selectedNodeProductionStateQuery.isLoading
              }
              isReferencePackItemsLoading={
                selectedReferencePackItemsQuery.isLoading
              }
              isRetryingJob={retryJobMutation.isPending}
              isRunningNode={runNodeMutation.isPending}
              isSelectingVersion={selectVersionMutation.isPending}
              isUpdatingGroupMembers={replaceGroupNodesMutation.isPending}
              isUpdatingNode={updateNodeMutation.isPending}
              isUpdatingReferencePackItems={
                replaceReferencePackItemsMutation.isPending
              }
              modelCapabilities={modelCapabilitiesQuery.data ?? []}
              nodeProductionState={selectedNodeProductionStateQuery.data ?? null}
              nodes={nodes}
              referencePackItems={selectedReferencePackItemsQuery.data ?? []}
              selectedEdgeId={selectedEdgeId}
              selectedGroupId={selectedGroupId}
              selectedNodeId={selectedNodeId}
              onAddGroupMember={addGroupMember}
              onDeleteEdge={deleteEdgeById}
              onDeleteGroup={(groupId) => deleteGroupMutation.mutate(groupId)}
              onRemoveGroupMember={removeGroupMember}
              onRenameGroup={(groupId, name) =>
                renameGroupMutation.mutate({ groupId, name })
              }
              onReplaceReferencePackItems={(packNodeId, memberNodeIds) =>
                replaceReferencePackItemsMutation.mutate({
                  packNodeId,
                  memberNodeIds,
                })
              }
              onPromptRefSelect={(targetNode, refNode, prompt) =>
                commitPromptRefSelection(targetNode, refNode, prompt)
              }
              onRetryJob={(jobId) => retryJobMutation.mutate(jobId)}
              onRunNode={runNodeWithOptionalPatch}
              onSelectVersion={(nodeId, versionId) =>
                selectVersionMutation.mutate({ nodeId, versionId })
              }
              onUpdateNode={(nodeId, patch) =>
                updateNodeMutation.mutate({ nodeId, patch })
              }
            />
          </div>
        ) : null}

        {id ? (
          <FileDropZone
            isUploading={isUploadingAsset}
            onUploadFiles={(files, point) => {
              void uploadAssetFiles(files, point);
            }}
            screenToCanvasPoint={screenToCanvasPoint}
            uploadError={assetUploadError}
          />
        ) : null}
        <AutoLayoutControls
          direction={layoutDirection}
          disabled={isLayouting || nodes.length === 0}
          onDirectionChange={setLayoutDirection}
          onRun={runAutoLayout}
        />

	        {contextMenu ? (
          <div
            className="studio-context-menu"
            onClick={(event) => event.stopPropagation()}
            style={{
              left: contextMenu.screenX,
              top: contextMenu.screenY,
            }}
          >
            {nodeCreateOptions.map((option) => (
              <button
                key={option.type}
                onClick={() =>
                  createNodeMutation.mutate({
                    point: contextMenu.flowPoint,
                    nodeType: option.type,
                  })
                }
                type="button"
              >
                <span className="studio-menu-icon">{option.icon}</span>
                <span>
                  <span className="block text-[13px] font-semibold tracking-[-0.01em]">
                    {option.title}
                  </span>
                  <span className="block text-[11.5px] text-[var(--fg-tertiary)]">
                    {option.description}
                  </span>
                </span>
              </button>
            ))}
          </div>
        ) : null}

        {createNodeMutation.isError ? (
          <div className="absolute right-4 top-4 rounded-[var(--radius-pill)] bg-[color-mix(in_srgb,var(--color-panel-elevated)_88%,transparent)] px-4 py-2 text-[12px] font-medium text-[var(--fg-secondary)] shadow-[var(--shadow-popover)] backdrop-blur-xl">
            创建节点失败
          </div>
        ) : null}
        {connectionFeedback ? (
          <div
            className="connection-feedback-toast"
            data-tone={connectionFeedback.tone}
            role={connectionFeedback.tone === "danger" ? "alert" : "status"}
          >
            <span className="connection-feedback-mark" aria-hidden="true" />
            <span className="connection-feedback-copy">
              <strong>{connectionFeedback.title}</strong>
              <span>{connectionFeedback.description}</span>
            </span>
          </div>
        ) : null}
      </section>

    </main>
  );
}

function appendCanvasNode(current: CanvasPayload | undefined, node: MediaNode) {
  if (!current) {
    return current;
  }
  if (current.nodes.some((item) => item.id === node.id)) {
    return replaceCanvasNode(current, node);
  }
  return { ...current, nodes: [...current.nodes, node] };
}

function canvasNodeFromEventPayload(node: unknown): MediaNode | null {
  if (
    typeof node === "object" &&
    node !== null &&
    "id" in node &&
    typeof (node as { id?: unknown }).id === "string"
  ) {
    return node as MediaNode;
  }
  return null;
}

function appendCanvasEdge(current: CanvasPayload | undefined, edge: MediaEdge) {
  if (!current) {
    return current;
  }
  if (current.edges.some((item) => item.id === edge.id)) {
    return {
      ...current,
      edges: current.edges.map((item) => (item.id === edge.id ? edge : item)),
    };
  }
  return { ...current, edges: [...current.edges, edge] };
}

function appendCanvasGroup(
  current: CanvasPayload | undefined,
  group: MediaGroup,
) {
  if (!current) {
    return current;
  }
  const movingNodeIds = new Set(group.node_ids);
  const groups = current.groups.some((item) => item.id === group.id)
    ? current.groups.map((item) =>
        item.id === group.id
          ? group
          : {
              ...item,
              node_ids: item.node_ids.filter((id) => !movingNodeIds.has(id)),
            },
      )
    : [
        ...current.groups.map((item) => ({
          ...item,
          node_ids: item.node_ids.filter((id) => !movingNodeIds.has(id)),
        })),
        group,
      ];
  return {
    ...current,
    groups,
    nodes: current.nodes.map((node) =>
      movingNodeIds.has(node.id)
        ? { ...node, group_id: group.id }
        : node.group_id === group.id
          ? { ...node, group_id: null }
          : node,
    ),
  };
}

function removeCanvasEdge(current: CanvasPayload | undefined, edgeId: string) {
  if (!current) {
    return current;
  }
  return {
    ...current,
    edges: current.edges.filter((edge) => edge.id !== edgeId),
  };
}

function removeCanvasNode(current: CanvasPayload | undefined, nodeId: string) {
  if (!current) {
    return current;
  }
  return {
    ...current,
    nodes: current.nodes.filter((node) => node.id !== nodeId),
    edges: current.edges.filter(
      (edge) => edge.from_node_id !== nodeId && edge.to_node_id !== nodeId,
    ),
    groups: current.groups.map((group) => ({
      ...group,
      node_ids: group.node_ids.filter((id) => id !== nodeId),
    })),
  };
}

function removeCanvasGroup(current: CanvasPayload | undefined, groupId: string) {
  if (!current) {
    return current;
  }
  return {
    ...current,
    groups: current.groups.filter((group) => group.id !== groupId),
    nodes: current.nodes.map((node) =>
      node.group_id === groupId ? { ...node, group_id: null } : node,
    ),
  };
}

function groupResponseToMediaGroup(response: {
  group: Omit<MediaGroup, "node_ids">;
  node_ids: string[] | null;
}): MediaGroup {
  return {
    ...response.group,
    node_ids: response.node_ids ?? [],
  };
}

function replaceCanvasNode(current: CanvasPayload | undefined, node: MediaNode) {
  if (!current) {
    return current;
  }
  return {
    ...current,
    nodes: current.nodes.map((item) => (item.id === node.id ? node : item)),
  };
}

function promptRefRenamePatches(nodes: MediaNode[], renamedNode: MediaNode) {
  const patches: Array<{
    nodeId: string;
    patch: NonNullable<ReturnType<typeof promptRefRenamePatch>>;
  }> = [];
  for (const node of nodes) {
    if (node.id === renamedNode.id) {
      continue;
    }
    const patch = promptRefRenamePatch(node, renamedNode);
    if (patch) {
      patches.push({ nodeId: node.id, patch });
    }
  }
  return patches;
}

function updateCanvasNodeStatus(
  current: CanvasPayload | undefined,
  nodeId: string,
  status: MediaNode["status"],
) {
  if (!current) {
    return current;
  }
  return {
    ...current,
    nodes: current.nodes.map((node) =>
      node.id === nodeId ? { ...node, status } : node,
    ),
  };
}

function announceActiveNode(nodeId: string | null) {
  window.dispatchEvent(
    new CustomEvent("clip-anvil:active-node-changed", {
      detail: { nodeId },
    }),
  );
}

function isEditableTarget(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) {
    return false;
  }
  return Boolean(
    target.closest("input, textarea, select, [contenteditable='true']"),
  );
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}
