import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type MouseEvent,
} from "react";
import { useNavigate, useParams } from "react-router";
import {
  ApiError,
  batchUpdateNodePositions,
  createMediaGroup,
  createMediaEdge,
  createMediaNode,
  deleteMediaEdge,
  deleteMediaGroup,
  deleteMediaNode,
  fetchCanvas,
  fetchWorkspace,
  updateCamera,
  updateMediaNode,
  type CanvasPayload,
  type MediaEdge,
  type MediaGroup,
  type MediaNode,
  type MediaType,
} from "../lib/api";
import { AutoLayoutControls } from "../components/AutoLayoutControls";
import {
  edgeToArrow,
  groupToShape,
  isMediaShape,
  nodeToShape,
  shapeIdForEdge,
  shapeIdForGroup,
  shapeIdForNode,
} from "../lib/canvas";
import { ConnectionStatus } from "../components/ConnectionStatus";
import {
  ConnectionOverlay,
  type DragConnection,
} from "../components/ConnectionOverlay";
import { FileDropZone } from "../components/FileDropZone";
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
import { useAppearanceStore } from "../stores/appearance";
import { useAuthStore } from "../stores/auth";

interface CanvasContextMenu {
  screenX: number;
  screenY: number;
  pageX: number;
  pageY: number;
}

interface SelectNodeEvent {
  nodeId: string;
}

interface SelectGroupEvent {
  groupId: string;
}

interface PromptEvent {
  nodeId: string;
  prompt: string;
}

interface TitleEvent {
  nodeId: string;
  title: string;
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

type NodeDraftPatch = Partial<Pick<MediaNode, "title" | "prompt">>;

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
    icon: "T",
    defaultTitle: "未命名文本",
  },
  {
    type: "image",
    title: "图片节点",
    description: "参考图 / 产品图 / 画面素材",
    icon: "I",
    defaultTitle: "未命名图片",
  },
  {
    type: "video",
    title: "视频节点",
    description: "镜头 / 片段 / 成片",
    icon: "V",
    defaultTitle: "未命名视频",
  },
  {
    type: "audio",
    title: "音频节点",
    description: "配乐 / 旁白 / 音效",
    icon: "A",
    defaultTitle: "未命名音频",
  },
];

export function WorkspaceDetailPage() {
  const navigate = useNavigate();
  const { id } = useParams();
  const queryClient = useQueryClient();
  const editorRef = useRef<Editor | null>(null);
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
  const [selectedGroupId, setSelectedGroupId] = useState<string | null>(null);
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [connectionSourceId, setConnectionSourceId] = useState<string | null>(
    null,
  );
  const [dragConnection, setDragConnection] =
    useState<DragConnection | null>(null);
  const [hoveredConnectionTargetId, setHoveredConnectionTargetId] =
    useState<string | null>(null);
  const [connectionError, setConnectionError] = useState<string | null>(null);
  const [connectionStatus, setConnectionStatus] =
    useState<CanvasConnectionStatus>("offline");
  const [layoutDirection, setLayoutDirection] =
    useState<LayoutDirection>("LR");
  const [isLayouting, setIsLayouting] = useState(false);
  const appearance = useAppearanceStore((state) => state.appearance);
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

  const nodes = canvasQuery.data?.nodes ?? [];
  const groups = canvasQuery.data?.groups ?? [];

  useEffect(() => {
    for (const node of nodes) {
      nodeSnapshotsRef.current.set(node.id, node);
    }
  }, [nodes]);

  const selectNode = useCallback((nodeId: string) => {
    setSelectedGroupId(null);
    setSelectedNodeId(nodeId);
    editorRef.current?.setSelectedShapes([shapeIdForNode(nodeId)]);
    announceActiveNode(nodeId);
  }, []);

  const selectGroup = useCallback((groupId: string) => {
    setSelectedNodeId(null);
    setSelectedGroupId(groupId);
    announceActiveNode(null);
    editorRef.current?.setSelectedShapes([shapeIdForGroup(groupId)]);
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
      setHoveredConnectionTargetId(null);
      setContextMenu(null);

      if (pointer && editorRef.current) {
        setConnectionError(null);
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
      setConnectionError("选择目标节点");
    },
    [],
  );

  const hideActiveNodeEditor = useCallback(() => {
    setSelectedGroupId(null);
    setSelectedNodeId(null);
    announceActiveNode(null);
    editorRef.current?.setSelectedShapes([]);
  }, []);

  const createGroupMutation = useMutation({
    mutationFn: async () => {
      if (!id || !selectedNodeId) {
        throw new Error("请选择节点");
      }
      return createMediaGroup({
        workspace_id: id,
        name: "新建分组",
        node_ids: [selectedNodeId],
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

  const deleteGroupMutation = useMutation({
    mutationFn: deleteMediaGroup,
    onSuccess: (_, groupId) => {
      queryClient.setQueryData<CanvasPayload>(
        ["workspace", id, "canvas"],
        (current) => removeCanvasGroup(current, groupId),
      );
      editorRef.current?.store.mergeRemoteChanges(() => {
        editorRef.current?.deleteShapes([shapeIdForGroup(groupId)]);
      });
      setSelectedGroupId(null);
    },
  });

  const createNodeMutation = useMutation({
    mutationFn: async (input?: {
      point?: { x: number; y: number };
      nodeType?: MediaType;
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
        title: option.defaultTitle,
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

  const createDependencyEdge = useCallback(
    (fromNodeId: string, toNodeId: string) => {
      if (!id || fromNodeId === toNodeId) {
        return;
      }

      void createMediaEdge({
        workspace_id: id,
        from_node_id: fromNodeId,
        to_node_id: toNodeId,
      })
        .then((edge) => {
          const current = queryClient.getQueryData<CanvasPayload>([
            "workspace",
            id,
            "canvas",
          ]);
          const nodesForArrow = current?.nodes ?? [];
          queryClient.setQueryData<CanvasPayload>(
            ["workspace", id, "canvas"],
            (payload) => appendCanvasEdge(payload, edge),
          );
          const record = edgeToArrow(edge, nodesForArrow);
          if (!record) {
            return;
          }
          const editor = editorRef.current;
          editor?.store.mergeRemoteChanges(() => {
            editor.createShapes([record.arrow]);
            editor.createBindings(record.bindings);
          });
        })
        .catch((error: unknown) => {
          setConnectionError(
            error instanceof ApiError && error.status === 422
              ? "不能形成循环依赖"
              : "连线失败",
          );
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
      }
    },
    [id, queryClient],
  );

  const runAutoLayout = useCallback(() => {
    if (!id || !canvasQuery.data || !editorRef.current) {
      return;
    }
    setIsLayouting(true);
    const result = computeDagreLayout({
      nodes: canvasQuery.data.nodes,
      edges: canvasQuery.data.edges,
      groups: canvasQuery.data.groups,
      direction: layoutDirection,
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
  }, [canvasQuery.data, id, layoutDirection, queryClient]);

  useEffect(() => {
    if (!editor || !canvasQuery.data) {
      return;
    }
    syncEditorWithCanvas(editor, canvasQuery.data);
  }, [canvasQuery.data, editor]);

  const selectOrConnectNode = useCallback(
    (nodeId: string) => {
      const fromNodeId =
        pendingConnectionRef.current?.fromNodeId ?? connectionSourceId;
      if (fromNodeId && fromNodeId !== nodeId) {
        pendingConnectionRef.current = null;
        setConnectionSourceId(null);
        setDragConnection(null);
        setHoveredConnectionTargetId(null);
        setConnectionError(null);
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
      editor?.store.mergeRemoteChanges(() => {
        editor.updateShapes([
          {
            id: shapeIdForNode(nodeId),
            type: "media",
            props: patch,
          },
        ]);
      });
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
          const mergedNode = currentNode
            ? {
                ...node,
                title: currentNode.title,
                prompt: currentNode.prompt,
              }
            : node;
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

  const scheduleTitleSave = useCallback(
    (nodeId: string, title: string) => {
      applyNodeDraft(nodeId, { title });
      window.clearTimeout(titleSaveTimersRef.current.get(nodeId));
      const timer = window.setTimeout(() => {
        commitNodePatch(nodeId, { title });
      }, 650);
      titleSaveTimersRef.current.set(nodeId, timer);
    },
    [applyNodeDraft, commitNodePatch],
  );

  const schedulePromptSave = useCallback(
    (nodeId: string, prompt: string) => {
      applyNodeDraft(nodeId, { prompt });
      window.clearTimeout(promptSaveTimersRef.current.get(nodeId));
      const timer = window.setTimeout(() => {
        commitNodePatch(nodeId, { prompt });
      }, 650);
      promptSaveTimersRef.current.set(nodeId, timer);
    },
    [applyNodeDraft, commitNodePatch],
  );

  const handleMount = useCallback(
    (editor: Editor) => {
      if (!id || !canvasQuery.data) {
        return;
      }

      editorRef.current = editor;
      setEditor(editor);
      const groupShapes = canvasQuery.data.groups.map((group) =>
        groupToShape(group, canvasQuery.data.nodes),
      );
      const shapes = [
        ...groupShapes,
        ...canvasQuery.data.nodes.map(nodeToShape),
      ];
      if (shapes.length > 0) {
        editor.store.mergeRemoteChanges(() => {
          editor.createShapes(shapes);
        });
      }
      const edgeRecords = canvasQuery.data.edges
        .map((edge) => edgeToArrow(edge, canvasQuery.data.nodes))
        .filter((record): record is NonNullable<typeof record> =>
          Boolean(record),
        );
      if (edgeRecords.length > 0) {
        editor.store.mergeRemoteChanges(() => {
          editor.createShapes(edgeRecords.map((record) => record.arrow));
          editor.createBindings(
            edgeRecords.flatMap((record) => record.bindings),
          );
        });
      }
      editor.setCurrentTool("select");
      editor.setCamera({
        x: canvasQuery.data.camera.x,
        y: canvasQuery.data.camera.y,
        z: canvasQuery.data.camera.zoom,
      });

      const pendingPositions = new Map<
        string,
        { id: string; canvas_x: number; canvas_y: number }
      >();
      let positionTimer: number | undefined;

      const flushPositions = () => {
        window.clearTimeout(positionTimer);
        positionTimer = undefined;
        const positions = Array.from(pendingPositions.values());
        pendingPositions.clear();
        if (positions.length > 0) {
          void batchUpdateNodePositions(positions);
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
            const fromShape = recordAsMediaShape(fromRecord);
            const toShape = recordAsMediaShape(toRecord);

            if (!fromShape || !toShape) {
              continue;
            }
            if (fromShape.x === toShape.x && fromShape.y === toShape.y) {
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

      let lastCamera = { ...canvasQuery.data.camera };
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
    [canvasQuery.data, id, queryClient],
  );

  const openCanvasMenu = useCallback((event: MouseEvent<HTMLElement>) => {
    if (!editorRef.current) {
      return;
    }
    if (
      event.target instanceof HTMLElement &&
      event.target.closest(".media-node-inline-editor")
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
      const port = target.closest(".media-node-port-output");
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
        event.target.closest(".media-node-port-output")
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
      if (!toNodeId || toNodeId === pending.fromNodeId) {
        return;
      }
      pendingConnectionRef.current = null;
      setConnectionSourceId(null);
      setDragConnection(null);
      setHoveredConnectionTargetId(null);
      setConnectionError(null);
      createDependencyEdge(pending.fromNodeId, toNodeId);
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
    };

    const updateHoveredConnectionTarget = (
      clientX: number,
      clientY: number,
      fromNodeId: string,
    ) => {
      const target = document
        .elementFromPoint(clientX, clientY)
        ?.closest(".media-node-shell");
      if (!(target instanceof HTMLElement)) {
        setHoveredConnectionTargetId(null);
        return;
      }
      const toNodeId = target.dataset.nodeId;
      setHoveredConnectionTargetId(
        toNodeId && toNodeId !== fromNodeId ? toNodeId : null,
      );
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
      updateHoveredConnectionTarget(
        event.clientX,
        event.clientY,
        pending.fromNodeId,
      );
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
      setHoveredConnectionTargetId(null);
      const target = document
        .elementFromPoint(event.clientX, event.clientY)
        ?.closest(".media-node-shell");
      if (!(target instanceof HTMLElement)) {
        setConnectionError(null);
        return;
      }
      const toNodeId = target.dataset.nodeId;
      if (!toNodeId || toNodeId === pending.fromNodeId) {
        setConnectionError(null);
        return;
      }
      setConnectionError(null);
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
    const onTitleChange = (event: Event) => {
      const detail = (event as CustomEvent<TitleEvent>).detail;
      if (!detail?.nodeId) {
        return;
      }
      scheduleTitleSave(detail.nodeId, detail.title);
    };
    const onTitleCommit = (event: Event) => {
      const detail = (event as CustomEvent<TitleEvent>).detail;
      if (!detail?.nodeId) {
        return;
      }
      commitNodePatch(detail.nodeId, { title: detail.title });
    };
    const onPromptChange = (event: Event) => {
      const detail = (event as CustomEvent<PromptEvent>).detail;
      if (!detail?.nodeId) {
        return;
      }
      schedulePromptSave(detail.nodeId, detail.prompt);
    };
    const onPromptCommit = (event: Event) => {
      const detail = (event as CustomEvent<PromptEvent>).detail;
      if (!detail?.nodeId) {
        return;
      }
      commitNodePatch(detail.nodeId, { prompt: detail.prompt });
    };

    window.addEventListener("clip-anvil:title-change", onTitleChange);
    window.addEventListener("clip-anvil:title-commit", onTitleCommit);
    window.addEventListener("clip-anvil:prompt-change", onPromptChange);
    window.addEventListener("clip-anvil:prompt-commit", onPromptCommit);
    return () => {
      window.removeEventListener("clip-anvil:title-change", onTitleChange);
      window.removeEventListener("clip-anvil:title-commit", onTitleCommit);
      window.removeEventListener("clip-anvil:prompt-change", onPromptChange);
      window.removeEventListener("clip-anvil:prompt-commit", onPromptCommit);
    };
  }, [commitNodePatch, schedulePromptSave, scheduleTitleSave]);

  useEffect(() => {
    const hideOnOutsidePointerDown = (event: PointerEvent) => {
      if (!(event.target instanceof HTMLElement)) {
        return;
      }
      if (
        event.target.closest(
          ".media-node-shell, .resource-tree, .property-panel, .studio-context-menu",
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

  const handleLogout = () => {
    logout();
    navigate("/login", { replace: true });
  };

  return (
    <main
      className="studio-shell"
      data-sidebar={isSidebarCollapsed ? "collapsed" : "expanded"}
    >
      <aside
        className="studio-sidebar"
        data-collapsed={isSidebarCollapsed}
      >
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
              aria-label={isSidebarCollapsed ? "展开侧边栏" : "收起侧边栏"}
              className="studio-icon-button"
              onClick={() => setIsSidebarCollapsed((value) => !value)}
              type="button"
            >
              {isSidebarCollapsed ? "›" : "‹"}
            </button>
          </div>
        </div>

        <div className="studio-sidebar-body">
          <div className="studio-action-row">
            <button
              className="studio-secondary-button"
              onClick={() => navigate("/workspaces")}
              type="button"
            >
              <span className="studio-action-label">项目列表</span>
              {isSidebarCollapsed ? "⌂" : null}
            </button>
            <button
              aria-label="切换明暗模式"
              className="studio-icon-button"
              onClick={toggleAppearance}
              type="button"
            >
              {appearance === "dark" ? "☾" : "☼"}
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
              className="studio-secondary-button"
              onClick={handleLogout}
              type="button"
            >
              登出
            </button>
          </div>
        </div>
      </aside>

      <section
        className="studio-canvas-frame"
        onContextMenuCapture={openCanvasMenu}
      >
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
          hoveredTargetNodeId={hoveredConnectionTargetId}
          nodes={nodes}
        />

        {id ? (
          <FileDropZone
            editor={editor}
            onAssetNodeCreated={appendAssetNodeToCanvas}
            workspaceId={id}
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
        {connectionError ? (
          <div className="absolute right-4 top-14 rounded-[var(--radius-pill)] bg-[color-mix(in_srgb,var(--color-panel-elevated)_88%,transparent)] px-4 py-2 text-[12px] font-medium text-[var(--fg-secondary)] shadow-[var(--shadow-popover)] backdrop-blur-xl">
            {connectionError}
          </div>
        ) : null}
      </section>

      <PropertyPanel
        groups={groups}
        nodes={nodes}
        selectedGroupId={selectedGroupId}
        selectedNodeId={selectedNodeId}
        onDeleteGroup={(groupId) => deleteGroupMutation.mutate(groupId)}
      />
    </main>
  );
}

function recordAsMediaShape(record: TLRecord) {
  if (record.typeName !== "shape" || !isMediaShape(record)) {
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
  const edgeRecords = canvas.edges
    .map((edge) => edgeToArrow(edge, canvas.nodes))
    .filter((record): record is NonNullable<typeof record> => Boolean(record));
  const desiredShapeIds = new Set([
    ...canvas.groups.map((group) => shapeIdForGroup(group.id)),
    ...canvas.nodes.map((node) => shapeIdForNode(node.id)),
    ...canvas.edges.map((edge) => shapeIdForEdge(edge.id)),
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

    const existingEdgeIds = existingPageShapes
      .filter((shape) => edgeIdFromRecord(shape))
      .map((shape) => shape.id);
    if (existingEdgeIds.length > 0) {
      editor.deleteShapes(existingEdgeIds);
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
    if (edgeRecords.length > 0) {
      editor.createShapes(edgeRecords.map((record) => record.arrow));
      editor.createBindings(edgeRecords.flatMap((record) => record.bindings));
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
  const groups = current.groups.some((item) => item.id === group.id)
    ? current.groups.map((item) => (item.id === group.id ? group : item))
    : [...current.groups, group];
  return {
    ...current,
    groups,
    nodes: current.nodes.map((node) =>
      group.node_ids.includes(node.id) ? { ...node, group_id: group.id } : node,
    ),
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
  node_ids: string[];
}): MediaGroup {
  return {
    ...response.group,
    node_ids: response.node_ids,
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
