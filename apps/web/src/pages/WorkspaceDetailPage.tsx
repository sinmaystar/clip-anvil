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
  batchUpdateNodePositions,
  createMediaNode,
  deleteMediaNode,
  fetchCanvas,
  fetchWorkspace,
  updateCamera,
  updateMediaNode,
  type CanvasPayload,
  type MediaNode,
} from "../lib/api";
import {
  isMediaShape,
  nodeToShape,
  shapeIdForNode,
} from "../lib/canvas";
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

interface PromptEvent {
  nodeId: string;
  prompt: string;
}

interface TitleEvent {
  nodeId: string;
  title: string;
}

type NodeDraftPatch = Partial<Pick<MediaNode, "title" | "prompt">>;

export function WorkspaceDetailPage() {
  const navigate = useNavigate();
  const { id } = useParams();
  const queryClient = useQueryClient();
  const editorRef = useRef<Editor | null>(null);
  const nodeSnapshotsRef = useRef(new Map<string, MediaNode>());
  const titleSaveTimersRef = useRef(new Map<string, number>());
  const promptSaveTimersRef = useRef(new Map<string, number>());
  const restoringNodeIdsRef = useRef(new Set<string>());
  const shapeUtils = useMemo(() => [MediaShapeUtil], []);
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
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const appearance = useAppearanceStore((state) => state.appearance);
  const toggleAppearance = useAppearanceStore((state) => state.toggleAppearance);
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

  useEffect(() => {
    for (const node of nodes) {
      nodeSnapshotsRef.current.set(node.id, node);
    }
  }, [nodes]);

  const selectNode = useCallback((nodeId: string) => {
    setSelectedNodeId(nodeId);
    editorRef.current?.setSelectedShapes([shapeIdForNode(nodeId)]);
    announceActiveNode(nodeId);
  }, []);

  const hideActiveNodeEditor = useCallback(() => {
    setSelectedNodeId(null);
    announceActiveNode(null);
    editorRef.current?.setSelectedShapes([]);
  }, []);

  const createNodeMutation = useMutation({
    mutationFn: async (point?: { x: number; y: number }) => {
      if (!id || !editorRef.current) {
        throw new Error("画布尚未准备好");
      }

      const center = editorRef.current.getViewportPageBounds().center;
      const position = point ?? center;
      return createMediaNode({
        workspace_id: id,
        node_type: "text",
        title: "未命名文本",
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
      const shapes = canvasQuery.data.nodes.map(nodeToShape);
      if (shapes.length > 0) {
        editor.store.mergeRemoteChanges(() => {
          editor.createShapes(shapes);
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
          ".media-node-shell, .studio-resource-item, .studio-context-menu",
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

          <div className="mt-5 studio-resource-list">
            <div className="studio-action-row">
              <p className="studio-section-label">Media Nodes</p>
              <span className="studio-node-count text-[11.5px] font-medium text-[var(--fg-tertiary)]">
                {nodes.length}
              </span>
            </div>
            {nodes.length > 0 ? (
              nodes.map((node) => (
                <button
                  className="studio-resource-item"
                  data-selected={node.id === selectedNodeId}
                  key={node.id}
                  onClick={() => selectNode(node.id)}
                  type="button"
                >
                  <span className="studio-resource-thumb">T</span>
                  <span className="studio-resource-name">
                    {node.title || "未命名文本"}
                  </span>
                  <span
                    className="studio-resource-status"
                    data-status={node.status}
                  />
                </button>
              ))
            ) : (
              <div className="rounded-[var(--radius-md)] p-3 text-[12px] leading-5 text-[var(--fg-tertiary)]">
                在画布空白处右键，选择文本节点开始。
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

        {contextMenu ? (
          <div
            className="studio-context-menu"
            onClick={(event) => event.stopPropagation()}
            style={{
              left: contextMenu.screenX,
              top: contextMenu.screenY,
            }}
          >
            <button
              onClick={() =>
                createNodeMutation.mutate({
                  x: contextMenu.pageX,
                  y: contextMenu.pageY,
                })
              }
              type="button"
            >
              <span className="studio-menu-icon">T</span>
              <span>
                <span className="block text-[13px] font-semibold tracking-[-0.01em]">
                  文本节点
                </span>
                <span className="block text-[11.5px] text-[var(--fg-tertiary)]">
                  提示词 / 文案 / 旁白
                </span>
              </span>
            </button>
          </div>
        ) : null}

        {createNodeMutation.isError ? (
          <div className="absolute right-4 top-4 rounded-[var(--radius-pill)] bg-[color-mix(in_srgb,var(--color-panel-elevated)_88%,transparent)] px-4 py-2 text-[12px] font-medium text-[var(--fg-secondary)] shadow-[var(--shadow-popover)] backdrop-blur-xl">
            创建节点失败
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
