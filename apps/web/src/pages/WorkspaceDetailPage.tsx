import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
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
  deleteMediaEdge,
  deleteMediaNode,
  fetchModelCapabilities,
  fetchNodeProductionState,
  fetchCanvas,
  fetchReferencePackItems,
  fetchWorkspace,
  replaceReferencePackItems,
  retryJob,
  runNode,
  selectNodeVersion,
  updateCamera,
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
  groupToShape,
  isGroupContainerShape,
  isMediaShape,
  nodeToShape,
  nodeToShapeProps,
  shapeIdForGroup,
  shapeIdForNode,
} from "../lib/canvas";
import {
  isActiveNodeRunStatus,
  nodeStatusForGenerationStatus,
  overlayActiveNodeStatuses,
  productionStateWithSubmittedJob,
} from "../lib/canvasRunState";
import { ConnectionStatus } from "../components/ConnectionStatus";
import {
  ConnectionOverlay,
  type DragConnection,
} from "../components/ConnectionOverlay";
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
import { GroupContainerShapeUtil } from "../shapes/GroupContainerShapeUtil";
import { MediaShapeUtil } from "../shapes/MediaShapeUtil";
import {
  Tldraw,
  type Editor,
  type TLRecord,
  type TLUiComponents,
} from "tldraw";
import "tldraw/tldraw.css";
import {
  connectionFailureFeedback,
  type ConnectionFeedback,
} from "../lib/connectionFeedback";
import { nodeIdsWithout } from "../lib/canvasSelectors";
import { isValidConnectionTarget } from "../lib/connectionGeometry";
import {
  getContainingGroupId,
  getGroupMemberMovePositions,
  type GroupBounds as GroupHitBounds,
} from "../lib/groupLayout";
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
  pageX: number;
  pageY: number;
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

interface ConnectionStartEvent {
  clientX?: number;
  clientY?: number;
  fromNodeId: string;
  pointerId: number | null;
}

interface PendingConnection {
  fromNodeId: string;
  pointerId: number | null;
}

type NodeDraftPatch = Partial<
  Pick<MediaNode, "title" | "prompt" | "prompt_refs" | "prompt_rich">
>;

interface NodeEditorPosition {
  left: number;
  top: number;
  width: number;
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
  const editorRef = useRef<Editor | null>(null);
  const canvasFrameRef = useRef<HTMLElement | null>(null);
  const [editor, setEditor] = useState<Editor | null>(null);
  const nodeSnapshotsRef = useRef(new Map<string, MediaNode>());
  const titleSaveTimersRef = useRef(new Map<string, number>());
  const promptSaveTimersRef = useRef(new Map<string, number>());
  const restoringNodeIdsRef = useRef(new Set<string>());
  const pendingConnectionRef = useRef<PendingConnection | null>(null);
  const shapeUtils = useMemo(() => [GroupContainerShapeUtil, MediaShapeUtil], []);
  const tldrawComponents = useMemo<TLUiComponents>(
    () => ({
      ContextMenu: null,
      ActionsMenu: null,
      HelpMenu: null,
      ZoomMenu: null,
      MainMenu: null,
      Minimap: null,
      StylePanel: null,
      PageMenu: null,
      NavigationPanel: null,
      Toolbar: null,
      RichTextToolbar: null,
      ImageToolbar: null,
      VideoToolbar: null,
      KeyboardShortcutsDialog: null,
      QuickActions: null,
      HelperButtons: null,
      DebugPanel: null,
      DebugMenu: null,
      MenuPanel: null,
      TopPanel: null,
      SharePanel: null,
      PeopleMenu: null,
    }),
    [],
  );
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState(false);
  const [contextMenu, setContextMenu] = useState<CanvasContextMenu | null>(null);
  const [textCreateMenuOpen, setTextCreateMenuOpen] = useState(false);
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [selectedEdgeId, setSelectedEdgeId] = useState<string | null>(null);
  const [nodeEditorPosition, setNodeEditorPosition] =
    useState<NodeEditorPosition | null>(null);
  const [connectionSourceId, setConnectionSourceId] = useState<string | null>(
    null,
  );
  const [dragConnection, setDragConnection] =
    useState<DragConnection | null>(null);
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
  const appearance = useAppearanceStore((state) => state.appearance);
  const toggleAppearance = useAppearanceStore((state) => state.toggleAppearance);
  const token = useAuthStore((state) => state.token);
  const account = useAuthStore((state) => state.account);
  const logout = useAuthStore((state) => state.logout);

  useEffect(() => {
    editor?.user.updateUserPreferences({ colorScheme: appearance });
  }, [appearance, editor]);

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

  const selectNode = useCallback((nodeId: string) => {
    setSelectedGroupId(null);
    setSelectedEdgeId(null);
    setSelectedNodeId(nodeId);
    editorRef.current?.setSelectedShapes([shapeIdForNode(nodeId)]);
    announceActiveNode(nodeId);
  }, []);

  const selectGroup = useCallback((groupId: string) => {
    setSelectedNodeId(null);
    setSelectedEdgeId(null);
    setSelectedGroupId(groupId);
    announceActiveNode(null);
    editorRef.current?.setSelectedShapes([shapeIdForGroup(groupId)]);
  }, []);

  const selectEdge = useCallback((edgeId: string) => {
    setSelectedNodeId(null);
    setSelectedGroupId(null);
    setSelectedEdgeId(edgeId);
    announceActiveNode(null);
    editorRef.current?.setSelectedShapes([]);
  }, []);

  const beginDependencyConnection = useCallback(
    (
      fromNodeId: string,
      pointer?: { clientX: number; clientY: number; pointerId: number | null },
    ) => {
      pendingConnectionRef.current = {
        fromNodeId,
        pointerId: pointer?.pointerId ?? null,
      };
      setConnectionSourceId(fromNodeId);
      setContextMenu(null);

      if (pointer && editorRef.current) {
        setConnectionFeedback(null);
        setDragConnection({
          fromNodeId,
          pointerPagePoint: editorRef.current.screenToPage({
            x: pointer.clientX,
            y: pointer.clientY,
          }),
        });
        return;
      }

      setDragConnection(null);
      setConnectionFeedback({
        title: "选择目标节点",
        description: "点击另一个 Node 完成连线。",
        tone: "info",
      });
    },
    [],
  );

  const hideActiveNodeEditor = useCallback(() => {
    setSelectedGroupId(null);
    setSelectedNodeId(null);
    setSelectedEdgeId(null);
    announceActiveNode(null);
    editorRef.current?.setSelectedShapes([]);
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
      const current = queryClient.getQueryData<CanvasPayload>([
        "workspace",
        id,
        "canvas",
      ]);
      const editor = editorRef.current;
      editor?.store.mergeRemoteChanges(() => {
        editor.createShapes([groupToShape(group, current?.nodes ?? [])]);
        editor.setSelectedShapes([shapeIdForGroup(group.id)]);
      });
      setSelectedGroupId(group.id);
      setSelectedNodeId(null);
      announceActiveNode(null);
    },
  });

  const createNodeMutation = useMutation({
    mutationFn: async (input?: {
      point?: { x: number; y: number };
      nodeType?: MediaType;
      patch?: Partial<CreateMediaNodeRequest>;
    }) => {
      if (!id || !editorRef.current) {
        throw new Error("画布尚未准备好");
      }

      const nodeType = input?.nodeType ?? "text";
      const option =
        nodeCreateOptions.find((item) => item.type === nodeType) ??
        nodeCreateOptions[0];
      const center = editorRef.current.getViewportPageBounds().center;
      const position = input?.point ?? center;
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
      const editor = editorRef.current;
      editor?.store.mergeRemoteChanges(() => {
        editor.createShapes([nodeToShape(node)]);
        editor.setSelectedShapes([shapeIdForNode(node.id)]);
      });
      selectNode(node.id);
      setContextMenu(null);
    },
  });

  const createNodeAtViewportCenter = useCallback(
    (nodeType: MediaType, patch?: Partial<CreateMediaNodeRequest>) => {
      createNodeMutation.mutate({ nodeType, patch });
    },
    [createNodeMutation],
  );

  const createManualTextSourceAtViewportCenter = useCallback(() => {
    createNodeAtViewportCenter("text", {
      title: "视频脚本",
      prompt: "",
      status: "succeeded",
      operation_type: "manual",
    });
  }, [createNodeAtViewportCenter]);

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
      editorRef.current?.store.mergeRemoteChanges(() => {
        editorRef.current?.updateShapes([
          {
            id: shapeIdForNode(mergedNode.id),
            type: "media",
            props: {
              prompt: mergedNode.prompt,
              status: mergedNode.status,
              title: mergedNode.title,
            },
          },
        ]);
      });
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
              editorRef.current?.store.mergeRemoteChanges(() => {
                editorRef.current?.updateShapes([
                  {
                    id: shapeIdForNode(updatedNode.id),
                    type: "media",
                    props: {
                      prompt: updatedNode.prompt,
                      status: updatedNode.status,
                      title: updatedNode.title,
                    },
                  },
                ]);
              });
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
      const node = runningNode;
      if (node) {
        editorRef.current?.store.mergeRemoteChanges(() => {
          editorRef.current?.updateShapes([
            {
              id: shapeIdForNode(nodeId),
              type: "media",
              props: nodeToShapeProps(node),
            },
          ]);
        });
      }
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

  const startToolbarConnection = useCallback(() => {
    if (selectedNodeId) {
      beginDependencyConnection(selectedNodeId);
      return;
    }
    setConnectionFeedback({
      title: "选择起点节点",
      description: "先选中一个节点，再点击连接并选择目标节点。",
      tone: "info",
    });
  }, [beginDependencyConnection, selectedNodeId]);

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

  const appendAssetNodeToCanvas = useCallback(
    (node: MediaNode) => {
      nodeSnapshotsRef.current.set(node.id, node);
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => appendCanvasNode(current, node),
      );
      const canvasEditor = editorRef.current;
      canvasEditor?.store.mergeRemoteChanges(() => {
        canvasEditor.createShapes([nodeToShape(node)]);
        canvasEditor.setSelectedShapes([shapeIdForNode(node.id)]);
      });
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

  const applyCanvasEvent = useCallback(
    (event: CanvasEvent) => {
      if (!id) {
        return;
      }
      switch (event.type) {
        case "NodeCreated":
        case "NodeUpdated":
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
            editorRef.current?.store.mergeRemoteChanges(() => {
              editorRef.current?.updateShapes([
                {
                  id: shapeIdForNode(event.payload.node_id),
                  type: "media",
                  props: { status: nodeStatus },
                },
              ]);
            });
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
    if (!id || !canvasQuery.data || !editorRef.current) {
      return;
    }
    const frameRect = canvasFrameRef.current?.getBoundingClientRect();
    const safeInset = isSidebarCollapsed
      ? layoutSafeInset.collapsed
      : layoutSafeInset.expanded;
    const origin = frameRect
      ? editorRef.current.screenToPage({
          x: frameRect.left + safeInset.x,
          y: frameRect.top + safeInset.y,
        })
      : safeInset;

    setIsLayouting(true);
    const result = computeDagreLayout({
      nodes: canvasQuery.data.nodes,
      edges: canvasQuery.data.edges,
      groups: canvasQuery.data.groups,
      direction: layoutDirection,
      origin,
    });

    const editor = editorRef.current;
    editor.store.mergeRemoteChanges(() => {
      editor.updateShapes(
        result.positions.map((position) => ({
          id: shapeIdForNode(position.id),
          type: "media",
          x: position.canvas_x,
          y: position.canvas_y,
        })),
      );
      editor.updateShapes(
        result.groupBounds.map((bounds) => ({
          id: shapeIdForGroup(bounds.groupId),
          type: "group-container",
          x: bounds.x,
          y: bounds.y,
          props: {
            w: bounds.w,
            h: bounds.h,
          },
        })),
      );
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
  }, [canvasQuery.data, id, isSidebarCollapsed, layoutDirection, queryClient]);

  useEffect(() => {
    if (!editor || !effectiveCanvas) {
      return;
    }
    syncEditorWithCanvas(editor, effectiveCanvas);
  }, [effectiveCanvas, editor]);

  useEffect(() => {
    if (!editor || !selectedNodeId) {
      setNodeEditorPosition(null);
      return;
    }

    let frame = 0;
    let previous = "";
    const tick = () => {
      const frameRect = canvasFrameRef.current?.getBoundingClientRect();
      const shape = editor.getShape(shapeIdForNode(selectedNodeId));
      if (!frameRect || !isMediaShape(shape)) {
        if (previous !== "hidden") {
          previous = "hidden";
          setNodeEditorPosition(null);
        }
        frame = window.requestAnimationFrame(tick);
        return;
      }

      const bottomLeft = editor.pageToScreen({
        x: shape.x,
        y: shape.y + shape.props.h,
      });
      const safeLeft = isSidebarCollapsed
        ? nodeEditorSafeLeft.collapsed
        : nodeEditorSafeLeft.expanded;
      const width = Math.min(
        720,
        Math.max(520, frameRect.width - safeLeft - 24),
      );
      const left = clamp(
        bottomLeft.x - frameRect.left,
        safeLeft,
        Math.max(12, frameRect.width - width - 12),
      );
      const top = bottomLeft.y - frameRect.top + 12;
      const next = JSON.stringify({
        left: Math.round(left),
        top: Math.round(top),
        width: Math.round(width),
      });
      if (next !== previous) {
        previous = next;
        setNodeEditorPosition(JSON.parse(next) as NodeEditorPosition);
      }
      frame = window.requestAnimationFrame(tick);
    };

    tick();
    return () => window.cancelAnimationFrame(frame);
  }, [editor, isSidebarCollapsed, selectedNodeId]);

  const selectOrConnectNode = useCallback(
    (nodeId: string) => {
      const fromNodeId =
        pendingConnectionRef.current?.fromNodeId ?? connectionSourceId;
      if (fromNodeId && fromNodeId !== nodeId) {
        pendingConnectionRef.current = null;
        setConnectionSourceId(null);
        setDragConnection(null);
        setConnectionFeedback(null);
        createDependencyEdge(fromNodeId, nodeId);
      }
      selectNode(nodeId);
    },
    [connectionSourceId, createDependencyEdge, selectNode],
  );

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
      const editor = editorRef.current;
      const shapeProps: Partial<Pick<MediaNode, "title" | "prompt">> = {};
      if (typeof patch.title === "string") {
        shapeProps.title = patch.title;
      }
      if (typeof patch.prompt === "string") {
        shapeProps.prompt = patch.prompt;
      }
      if (Object.keys(shapeProps).length > 0) {
        editor?.store.mergeRemoteChanges(() => {
          editor.updateShapes([
            {
              id: shapeIdForNode(nodeId),
              type: "media",
              props: shapeProps,
            },
          ]);
        });
      }
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
          const editor = editorRef.current;
          editor?.store.mergeRemoteChanges(() => {
            editor.updateShapes([
              {
                id: shapeIdForNode(node.id),
                type: "media",
                props: {
                  prompt: mergedNode.prompt,
                  status: mergedNode.status,
                  title: mergedNode.title,
                },
              },
            ]);
          });
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

  const handleMount = useCallback(
    (editor: Editor) => {
      if (!id || !effectiveCanvas) {
        return;
      }

      editorRef.current = editor;
      setEditor(editor);
      editor.registerExternalContentHandler("files", async ({ files, point }) => {
        await uploadAssetFiles(
          files,
          point ?? editor.getViewportPageBounds().center,
        );
      });
      const groupShapes = effectiveCanvas.groups.map((group) =>
        groupToShape(group, effectiveCanvas.nodes),
      );
      const shapes = [
        ...groupShapes,
        ...effectiveCanvas.nodes.map(nodeToShape),
      ];
      if (shapes.length > 0) {
        editor.store.mergeRemoteChanges(() => {
          editor.createShapes(shapes);
        });
      }
      editor.setCurrentTool("select");
      editor.setCamera({
        x: effectiveCanvas.camera.x,
        y: effectiveCanvas.camera.y,
        z: effectiveCanvas.camera.zoom,
      });

      const pendingPositions = new Map<
        string,
        { id: string; canvas_x: number; canvas_y: number }
      >();
      const pendingGroupAssignments = new Map<string, string>();
      const groupMovedNodeIds = new Set<string>();
      let positionTimer: number | undefined;

      const flushPositions = () => {
        window.clearTimeout(positionTimer);
        positionTimer = undefined;
        const positions = Array.from(pendingPositions.values());
        const groupAssignments = Array.from(pendingGroupAssignments.entries());
        pendingPositions.clear();
        pendingGroupAssignments.clear();
        if (positions.length > 0) {
          void batchUpdateNodePositions(positions).catch(() => {
            void queryClient.invalidateQueries({
              queryKey: ["workspace", id, "canvas"],
            });
          });
        }
        for (const [nodeId, groupId] of groupAssignments) {
          const current = queryClient.getQueryData<CanvasPayload>([
            "workspace",
            id,
            "canvas",
          ]);
          const currentNode = current?.nodes.find((node) => node.id === nodeId);
          if (!currentNode || currentNode.group_id === groupId) {
            continue;
          }
          const optimisticNode = { ...currentNode, group_id: groupId };
          nodeSnapshotsRef.current.set(nodeId, optimisticNode);
          queryClient.setQueryData<CanvasPayload>(
            ["workspace", id, "canvas"],
            (currentPayload) =>
              replaceCanvasNodeAndGroupMembership(
                currentPayload,
                optimisticNode,
              ),
          );
          flashGroupMembers(groupId, [nodeId]);
          void updateMediaNode(nodeId, { group_id: groupId })
            .then((node) => {
              const current = queryClient.getQueryData<CanvasPayload>([
                "workspace",
                id,
                "canvas",
              ]);
              const currentNode = current?.nodes.find(
                (item) => item.id === node.id,
              );
              const mergedNode = currentNode
                ? {
                    ...node,
                    canvas_x: currentNode.canvas_x,
                    canvas_y: currentNode.canvas_y,
                    prompt: currentNode.prompt,
                    title: currentNode.title,
                  }
                : node;
              nodeSnapshotsRef.current.set(node.id, mergedNode);
              queryClient.setQueryData<CanvasPayload>(
                ["workspace", id, "canvas"],
                (currentPayload) =>
                  replaceCanvasNodeAndGroupMembership(
                    currentPayload,
                    mergedNode,
                  ),
              );
            })
            .catch(() => {
              void queryClient.invalidateQueries({
                queryKey: ["workspace", id, "canvas"],
              });
            });
        }
      };

      const removeStoreListener = editor.store.listen(
        (entry) => {
          for (const record of Object.values(entry.changes.added)) {
            const shape = recordAsMediaShape(record);
            if (!shape || restoringNodeIdsRef.current.has(shape.props.nodeId)) {
              continue;
            }

            const current = queryClient.getQueryData<CanvasPayload>([
              "workspace",
              id,
              "canvas",
            ]);
            if (
              current?.nodes.some((node) => node.id === shape.props.nodeId)
            ) {
              continue;
            }

            restoringNodeIdsRef.current.add(shape.props.nodeId);
            const snapshot = nodeSnapshotsRef.current.get(shape.props.nodeId);
            void createMediaNode({
              id: shape.props.nodeId,
              workspace_id: id,
              node_type: snapshot?.node_type ?? shape.props.nodeType,
              title: snapshot?.title ?? shape.props.title,
              prompt: snapshot?.prompt ?? shape.props.prompt,
              status: snapshot?.status ?? shape.props.status,
              canvas_x: shape.x,
              canvas_y: shape.y,
            })
              .then((node) => {
                nodeSnapshotsRef.current.set(node.id, node);
                queryClient.setQueryData<CanvasPayload>(
                  ["workspace", id, "canvas"],
                  (currentPayload) => appendCanvasNode(currentPayload, node),
                );
                setSelectedNodeId(node.id);
                announceActiveNode(node.id);
                editor.store.mergeRemoteChanges(() => {
                  editor.updateShapes([
                    {
                      id: shape.id,
                      type: "media",
                      props: {
                        title: node.title,
                        prompt: node.prompt,
                        status: node.status,
                      },
                    },
                  ]);
                });
              })
              .catch(() => {
                editor.store.mergeRemoteChanges(() => {
                  editor.deleteShapes([shape.id]);
                });
                void queryClient.invalidateQueries({
                  queryKey: ["workspace", id, "canvas"],
                });
              })
              .finally(() => {
                restoringNodeIdsRef.current.delete(shape.props.nodeId);
              });
          }

          for (const [fromRecord, toRecord] of Object.values(
            entry.changes.updated,
          )) {
            const fromGroupShape = recordAsGroupContainerShape(fromRecord);
            const toGroupShape = recordAsGroupContainerShape(toRecord);
            if (fromGroupShape && toGroupShape) {
              const deltaX = toGroupShape.x - fromGroupShape.x;
              const deltaY = toGroupShape.y - fromGroupShape.y;
              if (deltaX !== 0 || deltaY !== 0) {
                const current = queryClient.getQueryData<CanvasPayload>([
                  "workspace",
                  id,
                  "canvas",
                ]);
                const group = current?.groups.find(
                  (item) => item.id === toGroupShape.props.groupId,
                );
                const sourceNodes =
                  current?.nodes.map(
                    (node) => nodeSnapshotsRef.current.get(node.id) ?? node,
                  ) ?? [];
                const positions =
                  group && current
                    ? getGroupMemberMovePositions({
                        group,
                        nodes: sourceNodes,
                        deltaX,
                        deltaY,
                      })
                    : [];
                if (positions.length > 0) {
                  editor.store.mergeRemoteChanges(() => {
                    editor.updateShapes(
                      positions.map((position) => ({
                        id: shapeIdForNode(position.id),
                        type: "media",
                        x: position.canvas_x,
                        y: position.canvas_y,
                      })),
                    );
                  });
                  for (const position of positions) {
                    groupMovedNodeIds.add(position.id);
                  }
                  window.setTimeout(() => {
                    for (const position of positions) {
                      groupMovedNodeIds.delete(position.id);
                    }
                  }, 0);
                  for (const position of positions) {
                    pendingPositions.set(position.id, position);
                    const currentSnapshot = nodeSnapshotsRef.current.get(
                      position.id,
                    );
                    if (currentSnapshot) {
                      nodeSnapshotsRef.current.set(position.id, {
                        ...currentSnapshot,
                        canvas_x: position.canvas_x,
                        canvas_y: position.canvas_y,
                      });
                    }
                  }
                }
              }
            }

            const fromShape = recordAsMediaShape(fromRecord);
            const toShape = recordAsMediaShape(toRecord);

            if (!fromShape || !toShape) {
              continue;
            }
            if (fromShape.x === toShape.x && fromShape.y === toShape.y) {
              continue;
            }
            if (groupMovedNodeIds.has(toShape.props.nodeId)) {
              continue;
            }

            pendingPositions.set(toShape.props.nodeId, {
              id: toShape.props.nodeId,
              canvas_x: toShape.x,
              canvas_y: toShape.y,
            });
            queryClient.setQueryData<CanvasPayload>(
              ["workspace", id, "canvas"],
              (current) =>
                updateCanvasNodePosition(
                  current,
                  toShape.props.nodeId,
                  toShape.x,
                  toShape.y,
                ),
            );
            const currentSnapshot = nodeSnapshotsRef.current.get(
              toShape.props.nodeId,
            );
            if (currentSnapshot) {
              const nextSnapshot = {
                ...currentSnapshot,
                canvas_x: toShape.x,
                canvas_y: toShape.y,
              };
              nodeSnapshotsRef.current.set(toShape.props.nodeId, nextSnapshot);
            }
            const current = queryClient.getQueryData<CanvasPayload>([
              "workspace",
              id,
              "canvas",
            ]);
            const targetGroupId = getContainingGroupId({
              bounds: getCurrentGroupBounds(editor, current?.groups ?? []),
              point: {
                x: toShape.x + toShape.props.w / 2,
                y: toShape.y + toShape.props.h / 2,
              },
            });
            const currentNode = current?.nodes.find(
              (node) => node.id === toShape.props.nodeId,
            );
            if (targetGroupId && currentNode?.group_id !== targetGroupId) {
              pendingGroupAssignments.set(toShape.props.nodeId, targetGroupId);
            } else {
              pendingGroupAssignments.delete(toShape.props.nodeId);
            }
          }

          for (const record of Object.values(entry.changes.removed)) {
            const edgeId = edgeIdFromRecord(record);
            if (edgeId) {
              queryClient.setQueryData<CanvasPayload>(
                ["workspace", id, "canvas"],
                (current) => removeCanvasEdge(current, edgeId),
              );
              void deleteMediaEdge(edgeId).catch(() => {
                void queryClient.invalidateQueries({
                  queryKey: ["workspace", id, "canvas"],
                });
              });
              continue;
            }

            const shape = recordAsMediaShape(record);
            if (shape) {
              window.clearTimeout(
                titleSaveTimersRef.current.get(shape.props.nodeId),
              );
              titleSaveTimersRef.current.delete(shape.props.nodeId);
              window.clearTimeout(
                promptSaveTimersRef.current.get(shape.props.nodeId),
              );
              promptSaveTimersRef.current.delete(shape.props.nodeId);
              const current = queryClient.getQueryData<CanvasPayload>([
                "workspace",
                id,
                "canvas",
              ]);
              const currentNode = current?.nodes.find(
                (node) => node.id === shape.props.nodeId,
              );
              if (currentNode) {
                nodeSnapshotsRef.current.set(currentNode.id, currentNode);
              }
              queryClient.setQueryData<CanvasPayload>(
                ["workspace", id, "canvas"],
                (current) => removeCanvasNode(current, shape.props.nodeId),
              );
              setSelectedNodeId((current) => {
                if (current !== shape.props.nodeId) {
                  return current;
                }
                announceActiveNode(null);
                return null;
              });
              void deleteMediaNode(shape.props.nodeId).catch(() => {
                void queryClient.invalidateQueries({
                  queryKey: ["workspace", id, "canvas"],
                });
              });
            }
          }

          if (pendingPositions.size > 0) {
            window.clearTimeout(positionTimer);
            positionTimer = window.setTimeout(flushPositions, 500);
          }
        },
        { source: "user", scope: "document" },
      );

      let lastCamera = { ...effectiveCanvas.camera };
      const cameraTimer = window.setInterval(() => {
        const camera = editor.getCamera();
        if (
          camera.x === lastCamera.x &&
          camera.y === lastCamera.y &&
          camera.z === lastCamera.zoom
        ) {
          return;
        }
        lastCamera = { x: camera.x, y: camera.y, zoom: camera.z };
        void updateCamera(id, lastCamera);
      }, 800);

      return () => {
        flushPositions();
        removeStoreListener();
        window.clearInterval(cameraTimer);
        editorRef.current = null;
        setEditor(null);
      };
    },
    [effectiveCanvas, id, queryClient, uploadAssetFiles],
  );

  const openCanvasMenu = useCallback((event: MouseEvent<HTMLElement>) => {
    if (!editorRef.current) {
      return;
    }
	      if (
	        event.target instanceof HTMLElement &&
	        event.target.closest(".node-production-popover")
	      ) {
	        return;
	      }

    const point = editorRef.current.screenToPage({
      x: event.clientX,
      y: event.clientY,
    });
    event.preventDefault();

    if (editorRef.current.getShapeAtPoint(point, { hitInside: true })) {
      setContextMenu(null);
      return;
    }

    setContextMenu({
      screenX: event.clientX,
      screenY: event.clientY,
      pageX: point.x,
      pageY: point.y,
    });
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
        selectOrConnectNode(detail.nodeId);
      }
    };

    window.addEventListener("clip-anvil:select-node", onSelectNode);
    return () => {
      window.removeEventListener("clip-anvil:select-node", onSelectNode);
    };
  }, [selectOrConnectNode]);

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
    const startConnectionFromTarget = (
      target: EventTarget | null,
      pointerId: number | null,
      clientX?: number,
      clientY?: number,
    ) => {
      if (!(target instanceof HTMLElement)) {
        return false;
      }
      const port = target.closest(".media-node-connect-button");
      if (!(port instanceof HTMLElement)) {
        return false;
      }
      const shell = port.closest(".media-node-shell");
      if (!(shell instanceof HTMLElement) || !shell.dataset.nodeId) {
        return false;
      }
      if (pointerId !== null && clientX !== undefined && clientY !== undefined) {
        beginDependencyConnection(shell.dataset.nodeId, {
          clientX,
          clientY,
          pointerId,
        });
      } else {
        beginDependencyConnection(shell.dataset.nodeId);
      }
      return true;
    };

    const onOutputPortPointerDown = (event: PointerEvent) => {
      if (
        startConnectionFromTarget(
          event.target,
          event.pointerId,
          event.clientX,
          event.clientY,
        )
      ) {
        event.preventDefault();
        event.stopPropagation();
        event.stopImmediatePropagation();
      }
    };

    const onOutputPortClick = (event: globalThis.MouseEvent) => {
      if (startConnectionFromTarget(event.target, null)) {
        event.preventDefault();
        event.stopPropagation();
        event.stopImmediatePropagation();
      }
    };

    const onConnectionTargetClick = (event: globalThis.MouseEvent) => {
      if (
        event.target instanceof HTMLElement &&
        event.target.closest(".media-node-connect-button")
      ) {
        return;
      }
      const pending = pendingConnectionRef.current;
      if (!pending || !(event.target instanceof HTMLElement)) {
        return;
      }
      const target = event.target.closest(".media-node-shell");
      if (!(target instanceof HTMLElement)) {
        return;
      }
      const toNodeId = target.dataset.nodeId;
      if (!isValidConnectionTarget(pending.fromNodeId, toNodeId)) {
        return;
      }
      pendingConnectionRef.current = null;
      setConnectionSourceId(null);
      setDragConnection(null);
      setConnectionFeedback(null);
      createDependencyEdge(pending.fromNodeId, toNodeId);
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
    };

    const onConnectionStart = (event: Event) => {
      const detail = (event as CustomEvent<ConnectionStartEvent>).detail;
      if (!detail?.fromNodeId) {
        return;
      }
      if (
        detail.pointerId !== null &&
        detail.clientX !== undefined &&
        detail.clientY !== undefined
      ) {
        beginDependencyConnection(detail.fromNodeId, {
          clientX: detail.clientX,
          clientY: detail.clientY,
          pointerId: detail.pointerId,
        });
        return;
      }
      beginDependencyConnection(detail.fromNodeId);
    };

    const onConnectionMove = (event: PointerEvent) => {
      const pending = pendingConnectionRef.current;
      const editor = editorRef.current;
      if (
        !pending ||
        pending.pointerId === null ||
        pending.pointerId !== event.pointerId ||
        !editor
      ) {
        return;
      }
      setDragConnection({
        fromNodeId: pending.fromNodeId,
        pointerPagePoint: editor.screenToPage({
          x: event.clientX,
          y: event.clientY,
        }),
      });
      event.preventDefault();
    };

    const onConnectionEnd = (event: PointerEvent) => {
      const pending = pendingConnectionRef.current;
      if (
        !pending ||
        pending.pointerId === null ||
        pending.pointerId !== event.pointerId
      ) {
        return;
      }
      pendingConnectionRef.current = null;
      setConnectionSourceId(null);
      setDragConnection(null);
      const target = document
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest(".media-node-shell");
      if (!(target instanceof HTMLElement)) {
        setConnectionFeedback(null);
        return;
      }
      const toNodeId = target.dataset.nodeId;
      if (!isValidConnectionTarget(pending.fromNodeId, toNodeId)) {
        setConnectionFeedback(null);
        return;
      }
      setConnectionFeedback(null);
      createDependencyEdge(pending.fromNodeId, toNodeId);
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
    };

    window.addEventListener("pointerdown", onOutputPortPointerDown, true);
    window.addEventListener("click", onOutputPortClick, true);
    window.addEventListener("click", onConnectionTargetClick, true);
    window.addEventListener(
      "clip-anvil:connection-start",
      onConnectionStart,
    );
    window.addEventListener("pointermove", onConnectionMove, true);
    window.addEventListener("pointerup", onConnectionEnd, true);
    return () => {
      window.removeEventListener("pointerdown", onOutputPortPointerDown, true);
      window.removeEventListener("click", onOutputPortClick, true);
      window.removeEventListener("click", onConnectionTargetClick, true);
      window.removeEventListener(
        "clip-anvil:connection-start",
        onConnectionStart,
      );
      window.removeEventListener("pointermove", onConnectionMove, true);
      window.removeEventListener("pointerup", onConnectionEnd, true);
    };
  }, [beginDependencyConnection, createDependencyEdge]);

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
    const deleteSelectedEdge = (event: KeyboardEvent) => {
      if (isEditableTarget(event.target)) {
        return;
      }
      if (event.key !== "Delete" && event.key !== "Backspace") {
        return;
      }
      if (!selectedEdgeId) {
        return;
      }
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
      deleteEdgeById(selectedEdgeId);
    };

    window.addEventListener("keydown", deleteSelectedEdge, true);
    return () => {
      window.removeEventListener("keydown", deleteSelectedEdge, true);
    };
  }, [deleteEdgeById, selectedEdgeId]);

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
      editorRef.current?.setCurrentTool("select");
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
                    onSelectNode={selectOrConnectNode}
                    onStartConnection={beginDependencyConnection}
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
        data-connection-dragging={dragConnection ? "true" : "false"}
        onContextMenuCapture={openCanvasMenu}
        onPointerDownCapture={closeCanvasMenuOnPointerDown}
        ref={canvasFrameRef}
      >
        <div className="studio-floating-toolbar" aria-label="创建节点工具栏">
          <button
            onClick={() => createNodeAtViewportCenter("video")}
            type="button"
          >
            视频
          </button>
          <div
            className="studio-toolbar-item"
            onPointerDown={(event) => event.stopPropagation()}
          >
            <button
              onPointerDown={(event) => {
                event.preventDefault();
                event.stopPropagation();
                setTextCreateMenuOpen((open) => !open);
              }}
              type="button"
            >
              文本
            </button>
            {textCreateMenuOpen ? (
              <div className="studio-context-menu studio-toolbar-menu">
                <button
                  onPointerDown={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    createNodeAtViewportCenter("text");
                    setTextCreateMenuOpen(false);
                  }}
                  type="button"
                >
                  生成文本
                </button>
                <button
                  onPointerDown={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    createManualTextSourceAtViewportCenter();
                    setTextCreateMenuOpen(false);
                  }}
                  type="button"
                >
                  文本素材
                </button>
              </div>
            ) : null}
          </div>
          <button
            onClick={() => createNodeAtViewportCenter("image")}
            type="button"
          >
            图片
          </button>
          <button
            onClick={() => createNodeAtViewportCenter("audio")}
            type="button"
          >
            音频
          </button>
          <button
            onClick={() => createNodeAtViewportCenter("reference_pack")}
            type="button"
          >
            参考包
          </button>
          <button onClick={startToolbarConnection} type="button">
            连接
          </button>
        </div>

        {workspaceQuery.isError || canvasQuery.isError ? (
          <div className="flex h-full items-center justify-center text-[13px] text-[var(--fg-tertiary)]">
            画布加载失败
          </div>
        ) : canvasQuery.data ? (
          <div className="studio-canvas-host">
            <Tldraw
              autoFocus
              components={tldrawComponents}
              key={id}
              onMount={handleMount}
              options={{ enableToolbarKeyboardShortcuts: false }}
              shapeUtils={shapeUtils}
            />
          </div>
        ) : (
          <div className="flex h-full items-center justify-center text-[13px] text-[var(--fg-tertiary)]">
            正在加载画布
          </div>
        )}

        <ConnectionOverlay
          dragConnection={dragConnection}
          editor={editor}
          edges={canvasQuery.data?.edges ?? []}
          nodes={nodes}
          onSelectEdge={selectEdge}
          selectedEdgeId={selectedEdgeId}
        />

        {selectedNode && nodeEditorPosition ? (
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
            }}
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
              isUpdatingGroupMembers={false}
              isUpdatingNode={updateNodeMutation.isPending}
              isUpdatingReferencePackItems={
                replaceReferencePackItemsMutation.isPending
              }
              modelCapabilities={modelCapabilitiesQuery.data ?? []}
              nodeProductionState={selectedNodeProductionStateQuery.data ?? null}
              nodes={nodes}
              referencePackItems={selectedReferencePackItemsQuery.data ?? []}
              selectedEdgeId={null}
              selectedGroupId={null}
              selectedNodeId={selectedNodeId}
              onDeleteEdge={deleteEdgeById}
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
            editor={editor}
            isUploading={isUploadingAsset}
            onUploadFiles={(files, point) => {
              void uploadAssetFiles(files, point);
            }}
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
                    point: { x: contextMenu.pageX, y: contextMenu.pageY },
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

function recordAsMediaShape(record: TLRecord) {
  if (record.typeName !== "shape" || !isMediaShape(record)) {
    return null;
  }

  return record;
}

function recordAsGroupContainerShape(record: TLRecord) {
  if (record.typeName !== "shape" || !isGroupContainerShape(record)) {
    return null;
  }

  return record;
}

function syncEditorWithCanvas(editor: Editor, canvas: CanvasPayload) {
  const groupShapes = canvas.groups.map((group) =>
    groupToShape(group, canvas.nodes),
  );
  const nodeShapes = canvas.nodes.map(nodeToShape);
  const canvasShapes = [...groupShapes, ...nodeShapes];
  const desiredShapeIds = new Set([
    ...canvas.groups.map((group) => shapeIdForGroup(group.id)),
    ...canvas.nodes.map((node) => shapeIdForNode(node.id)),
  ]);

  editor.store.mergeRemoteChanges(() => {
    const existingPageShapes = editor.getCurrentPageShapes();
    const staleShapeIds = existingPageShapes
      .filter((shape) => {
        if (
          shape.type !== "media" &&
          shape.type !== "group-container" &&
          !edgeIdFromRecord(shape)
        ) {
          return false;
        }
        return !desiredShapeIds.has(shape.id);
      })
      .map((shape) => shape.id);

    if (staleShapeIds.length > 0) {
      editor.deleteShapes(staleShapeIds);
    }

    const shapesToCreate = canvasShapes.filter(
      (shape) => !editor.getShape(shape.id),
    );
    const shapesToUpdate = canvasShapes.filter((shape) =>
      editor.getShape(shape.id),
    );

    if (shapesToCreate.length > 0) {
      editor.createShapes(shapesToCreate);
    }
    if (shapesToUpdate.length > 0) {
      editor.updateShapes(shapesToUpdate);
    }
  });
}

function edgeIdFromRecord(record: TLRecord) {
  if (record.typeName !== "shape" || !("meta" in record)) {
    return null;
  }
  const edgeId = (record as { meta?: { edgeId?: unknown } }).meta?.edgeId;
  return typeof edgeId === "string" ? edgeId : null;
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

function removeCanvasNode(current: CanvasPayload | undefined, nodeId: string) {
  if (!current) {
    return current;
  }
  return {
    ...current,
    nodes: current.nodes.filter((node) => node.id !== nodeId),
  };
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

function replaceCanvasNodeAndGroupMembership(
  current: CanvasPayload | undefined,
  node: MediaNode,
) {
  const next = replaceCanvasNode(current, node);
  if (!next) {
    return next;
  }
  return {
    ...next,
    groups: next.groups.map((group) => {
      const nodeIds = nodeIdsWithout(group.node_ids, node.id);
      return {
        ...group,
        node_ids: node.group_id === group.id ? [...nodeIds, node.id] : nodeIds,
      };
    }),
  };
}

function updateCanvasNodePosition(
  current: CanvasPayload | undefined,
  nodeId: string,
  canvasX: number,
  canvasY: number,
) {
  if (!current) {
    return current;
  }
  return {
    ...current,
    nodes: current.nodes.map((node) =>
      node.id === nodeId
        ? { ...node, canvas_x: canvasX, canvas_y: canvasY }
        : node,
    ),
  };
}

function getCurrentGroupBounds(
  editor: Editor,
  groups: MediaGroup[],
): GroupHitBounds[] {
  return groups
    .map((group) => {
      const shape = editor.getShape(shapeIdForGroup(group.id));
      if (!shape || !isGroupContainerShape(shape)) {
        return null;
      }
      return {
        groupId: group.id,
        x: shape.x,
        y: shape.y,
        w: shape.props.w,
        h: shape.props.h,
      };
    })
    .filter((bounds): bounds is GroupHitBounds => Boolean(bounds));
}

function flashGroupMembers(groupId: string, nodeIds: string[]) {
  window.dispatchEvent(
    new CustomEvent("clip-anvil:group-flash", {
      detail: { groupId },
    }),
  );
  window.dispatchEvent(
    new CustomEvent("clip-anvil:node-flash", {
      detail: { nodeIds },
    }),
  );
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
