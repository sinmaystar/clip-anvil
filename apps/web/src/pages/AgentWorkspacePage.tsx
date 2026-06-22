import {
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useRef,
  useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useNavigate, useParams } from "react-router";
import {
  fetchCanvas,
  fetchNodeProductionState,
  fetchWorkspace,
} from "../lib/api";
import {
  type AgentAttachment,
  type AgentMessage,
  type AgentTask,
  fetchAgentModelSelection,
  fetchAgentMessages,
  fetchAgentThread,
  postAgentDecision,
  postAgentMessage,
  putAgentModelSelection,
  uploadAgentAttachment,
} from "../lib/agentApi";
import {
  agentAttachmentKindForFile,
  attachmentAccept,
  formatAgentAttachmentLabel,
} from "../lib/agentAttachments";
import {
  decisionCardFromMessage,
  decisionResolvedFromEventPayload,
  type AgentDecisionCard,
} from "../lib/agentDecision";
import {
  AgentMessageRenderer,
  type AgentMessageActions,
} from "../components/agent/AgentMessageRenderer";
import { AgentNodeDetailDrawer } from "../components/agent/AgentNodeDetailDrawer";
import { AgentReadonlyCanvas } from "../components/agent/AgentReadonlyCanvas";
import {
  type AgentFloatingPosition,
  type AgentPanelCorner,
  agentPanelHeightStorageKey,
  agentPanelWidthStorageKey,
  agentRestorePositionStorageKey,
  clampAgentPanelHeight,
  clampAgentPanelWidth,
  clampAgentRestorePosition,
  defaultAgentRestorePosition,
  isAgentMessageListNearBottom,
  resizeAgentPanelFromCorner,
} from "../lib/agentLayout";
import { mergeAgentMessages } from "../lib/agentMessages";
import { agentMessageSchemaV1 } from "../lib/agentMessageBlocks";
import {
  agentModelSelectionPayload,
  agentModelSelectionValue,
  formatAgentModelOption,
} from "../lib/agentModelSelection";
import {
  clearAgentStream,
  type AgentStreamState,
  mergeAgentStreamDelta,
  shouldShowAgentThinkingIndicator,
} from "../lib/agentStreaming";
import {
  agentModelSupportsThinking,
  agentThinkingEffortLabel,
  agentThinkingEffortOptions,
} from "../lib/agentThinking";
import {
  agentComposerDisabledReason,
  hasActiveAgentTask,
  hasRunningProducerTask,
  mergeAgentTasks,
} from "../lib/agentTasks";
import {
  type AgentConnectionStatus,
  connectAgentSocket,
} from "../lib/agentWs";
import { createClientMessageId } from "../lib/clientMessageId";
import { workspaceModeRoute } from "../lib/workspaceRoutes";
import { useAuthStore } from "../stores/auth";

export function AgentWorkspacePage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const token = useAuthStore((state) => state.token);
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [tasks, setTasks] = useState<AgentTask[]>([]);
  const [streams, setStreams] = useState<AgentStreamState[]>([]);
  const [attachments, setAttachments] = useState<AgentAttachment[]>([]);
  const [resolvedDecisionIds, setResolvedDecisionIds] = useState<Set<string>>(
    () => new Set(),
  );
  const [panelWidth, setPanelWidth] = useState(() => {
    if (typeof window === "undefined") {
      return 420;
    }
    const stored = Number.parseInt(
      window.localStorage.getItem(agentPanelWidthStorageKey) ?? "",
      10,
    );
    return Number.isFinite(stored)
      ? clampAgentPanelWidth(stored, window.innerWidth)
      : 420;
  });
  const [panelHeight, setPanelHeight] = useState(() => {
    if (typeof window === "undefined") {
      return 560;
    }
    const stored = Number.parseInt(
      window.localStorage.getItem(agentPanelHeightStorageKey) ?? "",
      10,
    );
    return Number.isFinite(stored)
      ? clampAgentPanelHeight(stored, window.innerHeight)
      : clampAgentPanelHeight(560, window.innerHeight);
  });
  const [restorePosition, setRestorePosition] = useState<AgentFloatingPosition>(
    () => {
      if (typeof window === "undefined") {
        return { x: 0, y: 0 };
      }
      const fallback = defaultAgentRestorePosition(
        window.innerWidth,
        window.innerHeight,
      );
      const raw = window.localStorage.getItem(agentRestorePositionStorageKey);
      if (!raw) {
        return fallback;
      }
      try {
        const parsed = JSON.parse(raw) as Partial<AgentFloatingPosition>;
        if (
          typeof parsed.x === "number" &&
          typeof parsed.y === "number" &&
          Number.isFinite(parsed.x) &&
          Number.isFinite(parsed.y)
        ) {
          return clampAgentRestorePosition(
            { x: parsed.x, y: parsed.y },
            window.innerWidth,
            window.innerHeight,
          );
        }
      } catch {
        window.localStorage.removeItem(agentRestorePositionStorageKey);
      }
      return fallback;
    },
  );
  const [draft, setDraft] = useState("");
  const [collapsed, setCollapsed] = useState(false);
  const [showJumpToLatest, setShowJumpToLatest] = useState(false);
  const [sendError, setSendError] = useState("");
  const [attachmentError, setAttachmentError] = useState("");
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null);
  const [connectionStatus, setConnectionStatus] =
    useState<AgentConnectionStatus>("offline");
  const lastSeqRef = useRef(0);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const messageListRef = useRef<HTMLDivElement>(null);
  const shouldPinToBottomRef = useRef(true);
  const restoreDragRef = useRef({
    moved: false,
    startClientX: 0,
    startClientY: 0,
    startX: 0,
    startY: 0,
  });
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
  const canvas = canvasQuery.data;
  const selectedNode =
    canvas?.nodes.find((node) => node.id === selectedNodeId) ?? null;
  const selectedNodeProductionStateQuery = useQuery({
    queryKey: ["node", selectedNodeId, "production-state"],
    queryFn: () => fetchNodeProductionState(selectedNodeId ?? ""),
    enabled: Boolean(selectedNodeId),
  });
  const agentEnabled = Boolean(id && workspaceQuery.data?.mode === "agent");
  const agentThreadQuery = useQuery({
    queryKey: ["agent", id, "thread"],
    queryFn: () => fetchAgentThread(id ?? ""),
    enabled: agentEnabled,
  });
  const agentMessagesQuery = useQuery({
    queryKey: ["agent", id, "messages"],
    queryFn: () => fetchAgentMessages(id ?? ""),
    enabled: agentEnabled,
  });
  const agentModelSelectionQuery = useQuery({
    queryKey: ["agent", id, "model-selection"],
    queryFn: () => fetchAgentModelSelection(id ?? ""),
    enabled: agentEnabled,
  });
  const modelSelectionMutation = useMutation({
    mutationFn: (input: { value: string; reasoningEffort?: string }) => {
      const payload = agentModelSelectionPayload(
        input.value,
        input.reasoningEffort,
      );
      if (!payload) {
        throw new Error("invalid model selection");
      }
      return putAgentModelSelection(id ?? "", payload);
    },
    onSuccess: (response) => {
      queryClient.setQueryData(["agent", id, "model-selection"], response);
      setSendError("");
    },
    onError: () => {
      setSendError("模型切换失败，请稍后再试。");
    },
  });
  const sendMessageMutation = useMutation({
    mutationFn: (text: string) =>
      postAgentMessage(id ?? "", {
        text,
        client_message_id: createClientMessageId(),
        attachments,
      }),
    onSuccess: (response) => {
      shouldPinToBottomRef.current = true;
      setMessages((current) =>
        mergeAgentMessages(current, [response.message]),
      );
      setTasks((current) => mergeAgentTasks(current, [response.task]));
      setDraft("");
      setAttachments([]);
      setSendError("");
    },
    onError: () => {
      setSendError("发送失败，请稍后再试。");
    },
  });
  const uploadAttachmentMutation = useMutation({
    mutationFn: (file: File) => uploadAgentAttachment(id ?? "", file),
    onSuccess: (response) => {
      setAttachments((current) => [...current, response.attachment]);
      setAttachmentError("");
      void queryClient.invalidateQueries({
        queryKey: ["workspace", id, "canvas"],
      });
    },
    onError: () => {
      setAttachmentError("附件上传失败，请选择图片、视频或 txt 文本。");
    },
  });
  const respondDecisionMutation = useMutation({
    mutationFn: (input: {
      eventId: string;
      selectedOptionId?: string;
      freeText?: string;
    }) =>
      postAgentDecision(id ?? "", input.eventId, {
        selected_option_id: input.selectedOptionId,
        free_text: input.freeText,
        client_response_id: createClientMessageId(),
      }),
    onSuccess: (response) => {
      shouldPinToBottomRef.current = true;
      setResolvedDecisionIds((current) => {
        const next = new Set(current);
        next.add(response.decision_event.id);
        return next;
      });
      setMessages((current) =>
        mergeAgentMessages(current, [response.message]),
      );
      setTasks((current) => mergeAgentTasks(current, [response.task]));
      setSendError("");
      scrollMessagesToBottom();
    },
    onError: () => {
      setSendError("提交选择失败，请稍后再试。");
    },
  });
  const pendingDecisionIds = messages
    .map((message) => decisionCardFromMessage(message))
    .filter((card): card is AgentDecisionCard => Boolean(card))
    .filter(
      (card) =>
        card.status === "pending" && !resolvedDecisionIds.has(card.decision_id),
    )
    .map((card) => card.decision_id);
  const agentBusy = hasActiveAgentTask(tasks) || pendingDecisionIds.length > 0;
  const composerDisabledReason =
    agentComposerDisabledReason(tasks) ||
    (pendingDecisionIds.length > 0 ? "请先完成当前决策" : "");
  const producerRunning = hasRunningProducerTask(tasks);
  const showThinkingIndicator = shouldShowAgentThinkingIndicator(
    producerRunning,
    streams,
  );
  const statusLabel = connectionStatusLabel(
    agentThreadQuery.isLoading ? "connecting" : connectionStatus,
  );
  const statusClass = connectionStatusClass(
    agentThreadQuery.isLoading ? "connecting" : connectionStatus,
  );

  function scrollMessagesToBottom() {
    window.requestAnimationFrame(() => {
      const element = messageListRef.current;
      if (!element) {
        return;
      }
      element.scrollTop = element.scrollHeight;
      setShowJumpToLatest(false);
      shouldPinToBottomRef.current = true;
    });
  }

  function persistRestorePosition(position: AgentFloatingPosition) {
    window.localStorage.setItem(
      agentRestorePositionStorageKey,
      JSON.stringify(position),
    );
  }

  useEffect(() => {
    if (agentMessagesQuery.data) {
      setMessages((current) =>
        mergeAgentMessages(current, agentMessagesQuery.data.messages),
      );
    }
  }, [agentMessagesQuery.data]);

  useEffect(() => {
    lastSeqRef.current =
      messages.length > 0 ? messages[messages.length - 1].seq : 0;
  }, [messages]);

  useEffect(() => {
    if (!shouldPinToBottomRef.current) {
      return;
    }
    scrollMessagesToBottom();
  }, [messages, streams, producerRunning]);

  useEffect(() => {
    if (!id || !token || workspaceQuery.data?.mode !== "agent") {
      return;
    }

    const fetchMissingMessages = () => {
      void fetchAgentMessages(id, lastSeqRef.current, 200).then((response) => {
        setMessages((current) =>
          mergeAgentMessages(current, response.messages),
        );
      });
    };

    return connectAgentSocket({
      workspaceId: id,
      token,
      onEvent: (event) => {
        if (
          event.type === "agent.message.created" &&
          event.payload.workspace_id === id
        ) {
          setMessages((current) =>
            mergeAgentMessages(current, [event.payload.message]),
          );
          setStreams((current) =>
            clearAgentStream(current, event.payload.message.task_id),
          );
        }
        if (
          event.type === "agent.message.delta" &&
          event.payload.workspace_id === id
        ) {
          setStreams((current) =>
            mergeAgentStreamDelta(current, {
              task_id: event.payload.task_id,
              block_id: event.payload.block_id,
              block_type: event.payload.block_type,
              delta: event.payload.delta,
              sequence: event.payload.sequence,
            }),
          );
        }
        if (
          event.type === "agent.task.updated" &&
          event.payload.workspace_id === id
        ) {
          setTasks((current) => mergeAgentTasks(current, [event.payload.task]));
        }
        if (
          event.type === "agent.event.created" &&
          event.payload.workspace_id === id
        ) {
          const agentEvent = event.payload.event;
          if (
            agentEvent.event_type === "decision_requested" &&
            agentEvent.status === "handled"
          ) {
            setResolvedDecisionIds((current) => {
              const next = new Set(current);
              next.add(agentEvent.id);
              return next;
            });
          }
          if (agentEvent.event_type === "decision_resolved") {
            const decisionID = decisionResolvedFromEventPayload(
              agentEvent.payload,
            );
            if (decisionID) {
              setResolvedDecisionIds((current) => {
                const next = new Set(current);
                next.add(decisionID);
                return next;
              });
            }
          }
        }
      },
      onReconnect: fetchMissingMessages,
      onStatusChange: setConnectionStatus,
    });
  }, [id, token, workspaceQuery.data?.mode]);

  if (workspaceQuery.isLoading) {
    return (
      <div className="app-route-loading" role="status" aria-label="正在加载" />
    );
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <main className="agent-workspace-shell">
        <p className="agent-empty-text">项目加载失败</p>
      </main>
    );
  }

  if (workspaceQuery.data.mode !== "agent") {
    return (
      <Navigate
        to={workspaceModeRoute(
          workspaceQuery.data.id,
          workspaceQuery.data.mode,
        )}
        replace
      />
    );
  }

  const trimmedDraft = draft.trim();
  const selectedModelValue = agentModelSelectionValue(
    agentModelSelectionQuery.data?.selection.producer,
  );
  const selectedModelOption = agentModelSelectionQuery.data?.options.find(
    (option) => agentModelSelectionValue(option) === selectedModelValue,
  );
  const selectedReasoningEffort =
    agentModelSelectionQuery.data?.selection.producer.reasoning_effort ||
    selectedModelOption?.default_reasoning_effort ||
    "";
  const thinkingEffortOptions =
    agentThinkingEffortOptions(selectedModelOption);
  const thinkingSelectorEnabled =
    agentModelSupportsThinking(selectedModelOption) &&
    thinkingEffortOptions.length > 0;
  const canSend =
    trimmedDraft.length > 0 &&
    !agentBusy &&
    !sendMessageMutation.isPending &&
    !uploadAttachmentMutation.isPending;
  const messageActions: AgentMessageActions = {
    disabled: respondDecisionMutation.isPending,
    resolvedDecisionIds,
    onSelectDecision: (decisionID, optionID) =>
      respondDecisionMutation.mutate({
        eventId: decisionID,
        selectedOptionId: optionID,
      }),
    onSubmitDecisionText: (decisionID, freeText) =>
      respondDecisionMutation.mutate({
        eventId: decisionID,
        freeText,
      }),
  };

  const submitMessage = () => {
    if (!canSend) {
      return;
    }
    shouldPinToBottomRef.current = true;
    scrollMessagesToBottom();
    sendMessageMutation.mutate(trimmedDraft);
  };
  const chooseAttachment = () => {
    fileInputRef.current?.click();
  };
  const uploadSelectedAttachment = (file: File | undefined) => {
    if (!file || !id) {
      return;
    }
    if (!agentAttachmentKindForFile(file)) {
      setAttachmentError("仅支持图片、视频或 txt 文本。");
      return;
    }
    uploadAttachmentMutation.mutate(file);
  };
  const updateMessageScrollState = () => {
    const element = messageListRef.current;
    if (!element) {
      return;
    }
    const nearBottom = isAgentMessageListNearBottom(element);
    shouldPinToBottomRef.current = nearBottom;
    setShowJumpToLatest(!nearBottom);
  };
  const beginWidthResizeFrom = (startX: number, startWidth: number) => {
    const move = (moveEvent: PointerEvent | MouseEvent) => {
      const nextWidth = clampAgentPanelWidth(
        startWidth + startX - moveEvent.clientX,
        window.innerWidth,
      );
      setPanelWidth(nextWidth);
      window.localStorage.setItem(agentPanelWidthStorageKey, String(nextWidth));
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  };
  const beginWidthPointerResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    beginWidthResizeFrom(event.clientX, panelWidth);
  };
  const beginWidthMouseResize = (event: ReactMouseEvent<HTMLDivElement>) => {
    event.preventDefault();
    beginWidthResizeFrom(event.clientX, panelWidth);
  };
  const beginHeightResizeFrom = (startY: number, startHeight: number) => {
    const move = (moveEvent: PointerEvent | MouseEvent) => {
      const nextHeight = clampAgentPanelHeight(
        startHeight + moveEvent.clientY - startY,
        window.innerHeight,
      );
      setPanelHeight(nextHeight);
      window.localStorage.setItem(
        agentPanelHeightStorageKey,
        String(nextHeight),
      );
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  };
  const beginHeightPointerResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    beginHeightResizeFrom(event.clientY, panelHeight);
  };
  const beginHeightMouseResize = (event: ReactMouseEvent<HTMLDivElement>) => {
    event.preventDefault();
    beginHeightResizeFrom(event.clientY, panelHeight);
  };
  const beginCornerResizeFrom = (
    corner: AgentPanelCorner,
    startX: number,
    startY: number,
    startWidth: number,
    startHeight: number,
  ) => {
    const move = (moveEvent: PointerEvent | MouseEvent) => {
      const nextSize = resizeAgentPanelFromCorner({
        corner,
        startClientX: startX,
        startClientY: startY,
        clientX: moveEvent.clientX,
        clientY: moveEvent.clientY,
        startWidth,
        startHeight,
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight,
      });
      setPanelWidth(nextSize.width);
      setPanelHeight(nextSize.height);
      window.localStorage.setItem(
        agentPanelWidthStorageKey,
        String(nextSize.width),
      );
      window.localStorage.setItem(
        agentPanelHeightStorageKey,
        String(nextSize.height),
      );
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  };
  const beginCornerPointerResize = (
    corner: AgentPanelCorner,
    event: ReactPointerEvent<HTMLDivElement>,
  ) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    beginCornerResizeFrom(
      corner,
      event.clientX,
      event.clientY,
      panelWidth,
      panelHeight,
    );
  };
  const beginCornerMouseResize = (
    corner: AgentPanelCorner,
    event: ReactMouseEvent<HTMLDivElement>,
  ) => {
    event.preventDefault();
    beginCornerResizeFrom(
      corner,
      event.clientX,
      event.clientY,
      panelWidth,
      panelHeight,
    );
  };
  const beginRestoreDragFrom = (clientX: number, clientY: number) => {
    restoreDragRef.current = {
      moved: false,
      startClientX: clientX,
      startClientY: clientY,
      startX: restorePosition.x,
      startY: restorePosition.y,
    };
    const move = (moveEvent: PointerEvent | MouseEvent) => {
      const deltaX = moveEvent.clientX - restoreDragRef.current.startClientX;
      const deltaY = moveEvent.clientY - restoreDragRef.current.startClientY;
      if (Math.abs(deltaX) + Math.abs(deltaY) > 4) {
        restoreDragRef.current.moved = true;
      }
      const nextPosition = clampAgentRestorePosition(
        {
          x: restoreDragRef.current.startX + deltaX,
          y: restoreDragRef.current.startY + deltaY,
        },
        window.innerWidth,
        window.innerHeight,
      );
      setRestorePosition(nextPosition);
      persistRestorePosition(nextPosition);
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("mousemove", move);
      window.removeEventListener("mouseup", up);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("mousemove", move);
    window.addEventListener("mouseup", up);
  };
  const beginRestorePointerDrag = (
    event: ReactPointerEvent<HTMLButtonElement>,
  ) => {
    event.currentTarget.setPointerCapture(event.pointerId);
    beginRestoreDragFrom(event.clientX, event.clientY);
  };
  const beginRestoreMouseDrag = (event: ReactMouseEvent<HTMLButtonElement>) => {
    beginRestoreDragFrom(event.clientX, event.clientY);
  };
  const collapseFromCanvas = () => {
    if (!collapsed) {
      setCollapsed(true);
    }
  };

  return (
    <main className="agent-workspace-shell">
      <header className="agent-topbar">
        <button
          className="studio-secondary-button"
          onClick={() => navigate("/workspaces")}
          type="button"
        >
          返回
        </button>
        <div>
          <p className="workspace-kicker">Agent Workspace</p>
          <h1>{workspaceQuery.data.name}</h1>
        </div>
      </header>

      <section
        className="agent-canvas-stage"
        aria-label="只读画布"
        onPointerDown={(event) => {
          if (event.target === event.currentTarget) {
            collapseFromCanvas();
          }
        }}
      >
        <section className="agent-readonly-canvas">
          <div className="agent-canvas-header">
            <div>
              <p className="workspace-kicker">Read Only Canvas</p>
              <h2>只读画布</h2>
            </div>
            <span>{canvas?.nodes.length ?? 0} 个节点</span>
          </div>
          <div className="agent-canvas-surface">
            {canvasQuery.isLoading ? (
              <p className="agent-empty-text">正在加载画布</p>
            ) : canvas && canvas.nodes.length > 0 ? (
              <AgentReadonlyCanvas
                canvas={canvas}
                onSelectNode={setSelectedNodeId}
                selectedNodeId={selectedNodeId}
              />
            ) : (
              <p className="agent-empty-text">Agent 尚未创建画布节点。</p>
            )}
          </div>
        </section>

        <AgentNodeDetailDrawer
          edges={canvas?.edges ?? []}
          isLoading={selectedNodeProductionStateQuery.isLoading}
          node={selectedNode}
          nodes={canvas?.nodes ?? []}
          onClose={() => setSelectedNodeId(null)}
          productionState={selectedNodeProductionStateQuery.data ?? null}
        />

        {collapsed ? (
          <button
            className="agent-chat-restore"
            aria-label="打开 ClipAnvil"
            onClick={() => {
              if (restoreDragRef.current.moved) {
                restoreDragRef.current.moved = false;
                return;
              }
              setCollapsed(false);
            }}
            onMouseDown={beginRestoreMouseDrag}
            onPointerDown={beginRestorePointerDrag}
            style={{
              left: restorePosition.x,
              top: restorePosition.y,
            }}
            type="button"
          >
            <span className={`agent-status-dot ${statusClass}`} />
            <span>CA</span>
          </button>
        ) : (
          <aside
            className="agent-chat-float"
            onPointerDown={(event) => event.stopPropagation()}
            style={{ height: panelHeight, width: panelWidth }}
          >
            <div
              aria-label="调整对话框宽度"
              className="agent-chat-resize-handle"
              onMouseDown={beginWidthMouseResize}
              onPointerDown={beginWidthPointerResize}
              role="separator"
            />
            <div
              aria-label="调整对话框高度"
              className="agent-chat-bottom-resize-handle"
              onMouseDown={beginHeightMouseResize}
              onPointerDown={beginHeightPointerResize}
              role="separator"
            />
            <div
              aria-label="从左上角同时调整对话框宽度和高度"
              className="agent-chat-corner-resize-handle agent-chat-corner-resize-handle-top-left"
              onMouseDown={(event) => beginCornerMouseResize("top-left", event)}
              onPointerDown={(event) =>
                beginCornerPointerResize("top-left", event)
              }
              role="separator"
            />
            <div
              aria-label="从左下角同时调整对话框宽度和高度"
              className="agent-chat-corner-resize-handle agent-chat-corner-resize-handle-bottom-left"
              onMouseDown={(event) =>
                beginCornerMouseResize("bottom-left", event)
              }
              onPointerDown={(event) =>
                beginCornerPointerResize("bottom-left", event)
              }
              role="separator"
            />
            <div className="agent-chat-header">
              <div>
                <h2>ClipAnvil</h2>
                <span className="agent-connection-state">
                  <span className={`agent-status-dot ${statusClass}`} />
                  <span className="agent-sr-only">{statusLabel}</span>
                </span>
              </div>
              <button
                className="agent-icon-button"
                onClick={() => setCollapsed(true)}
                title="收起"
                type="button"
              >
                ×
              </button>
            </div>

            <div
              className="agent-message-list"
              aria-live="polite"
              onScroll={updateMessageScrollState}
              ref={messageListRef}
            >
              {agentMessagesQuery.isLoading ? (
                <p className="agent-empty-text">正在加载对话</p>
              ) : messages.length > 0 ? (
                <>
                  {messages.map((message) => {
                    const decisionCard = decisionCardFromMessage(message);
                    const isDecisionResolved =
                      decisionCard !== null &&
                      (decisionCard.status === "handled" ||
                        resolvedDecisionIds.has(decisionCard.decision_id));
                    return (
                      <article
                        className={`agent-message agent-message-${messageClass(message)}`}
                        key={message.id}
                      >
                        <AgentMessageRenderer
                          actions={{
                            ...messageActions,
                            disabled:
                              isDecisionResolved ||
                              respondDecisionMutation.isPending,
                          }}
                          message={message}
                        />
                      </article>
                    );
                  })}
                  {streams.map((stream) => (
                    <article
                      className="agent-message agent-message-assistant agent-message-streaming"
                      key={stream.task_id}
                    >
                      <AgentMessageRenderer message={streamToMessage(stream)} />
                    </article>
                  ))}
                  {showThinkingIndicator ? (
                    <ThinkingIndicator />
                  ) : null}
                </>
              ) : (
                <>
                  {streams.map((stream) => (
                    <article
                      className="agent-message agent-message-assistant agent-message-streaming"
                      key={stream.task_id}
                    >
                      <AgentMessageRenderer message={streamToMessage(stream)} />
                    </article>
                  ))}
                  {streams.length === 0 ? (
                    showThinkingIndicator ? (
                      <ThinkingIndicator />
                    ) : (
                      <p className="agent-empty-text">还没有 ClipAnvil 对话。</p>
                    )
                  ) : null}
                </>
              )}
            </div>
            {showJumpToLatest ? (
              <button
                aria-label="跳到最新消息"
                className="agent-scroll-latest-button"
                onClick={scrollMessagesToBottom}
                type="button"
              >
                ↓
              </button>
            ) : null}

            {sendError ? <p className="agent-chat-error">{sendError}</p> : null}
            {attachmentError ? (
              <p className="agent-chat-error">{attachmentError}</p>
            ) : null}
            {composerDisabledReason ? (
              <p className="agent-chat-hint">{composerDisabledReason}</p>
            ) : null}

            <form
              className="agent-chat-composer"
              onSubmit={(event) => {
                event.preventDefault();
                submitMessage();
              }}
            >
              <input
                accept={attachmentAccept}
                className="agent-file-input"
                onChange={(event) => {
                  uploadSelectedAttachment(event.target.files?.[0]);
                  event.currentTarget.value = "";
                }}
                ref={fileInputRef}
                type="file"
              />
              {attachments.length > 0 ? (
                <div className="agent-composer-attachments">
                  {attachments.map((attachment) => (
                    <span
                      className="agent-attachment-chip"
                      key={attachment.node_id}
                    >
                      {formatAgentAttachmentLabel(attachment)}
                      <button
                        aria-label={`移除 ${attachment.name}`}
                        onClick={() =>
                          setAttachments((current) =>
                            current.filter(
                              (item) => item.node_id !== attachment.node_id,
                            ),
                          )
                        }
                        type="button"
                      >
                        ×
                      </button>
                    </span>
                  ))}
                </div>
              ) : null}
              <div className="agent-composer-box">
                <textarea
                  aria-label="发送给 ClipAnvil"
                  disabled={agentBusy}
                  onChange={(event) => setDraft(event.target.value)}
                  onKeyDown={(event) => {
                    if (event.key === "Enter" && !event.shiftKey) {
                      event.preventDefault();
                      submitMessage();
                    }
                  }}
                  placeholder={
                    agentBusy ? "等待当前任务完成" : "输入需求或反馈"
                  }
                  rows={3}
                  value={draft}
                />
                <div className="agent-composer-toolbar">
                  <div className="agent-composer-tools">
                    <button
                      aria-label="添加附件"
                      className="agent-composer-icon-button"
                      disabled={agentBusy || uploadAttachmentMutation.isPending}
                      onClick={chooseAttachment}
                      type="button"
                    >
                      +
                    </button>
                    {agentModelSelectionQuery.data ? (
                      <select
                        aria-label="对话模型"
                        className="agent-model-select"
                        disabled={agentBusy || modelSelectionMutation.isPending}
                        onChange={(event) => {
                          const nextValue = event.target.value;
                          const nextOption =
                            agentModelSelectionQuery.data?.options.find(
                              (option) =>
                                agentModelSelectionValue(option) === nextValue,
                            );
                          const nextEfforts =
                            agentThinkingEffortOptions(nextOption);
                          modelSelectionMutation.mutate({
                            value: nextValue,
                            reasoningEffort:
                              nextOption?.default_reasoning_effort ||
                              nextEfforts[0],
                          });
                        }}
                        value={selectedModelValue}
                      >
                        {agentModelSelectionQuery.data.options.map((option) => (
                          <option
                            key={`${option.provider_id}:${option.model_id}`}
                            value={agentModelSelectionValue(option)}
                          >
                            {formatAgentModelOption(option)}
                          </option>
                        ))}
                      </select>
                    ) : null}
                    {agentModelSelectionQuery.data && thinkingSelectorEnabled ? (
                      <select
                        aria-label="思考深度"
                        className="agent-thinking-select"
                        disabled={agentBusy || modelSelectionMutation.isPending}
                        onChange={(event) =>
                          modelSelectionMutation.mutate({
                            value: selectedModelValue,
                            reasoningEffort: event.target.value,
                          })
                        }
                        value={selectedReasoningEffort}
                      >
                        {thinkingEffortOptions.map((effort) => (
                          <option key={effort} value={effort}>
                            思考 {agentThinkingEffortLabel(effort)}
                          </option>
                        ))}
                      </select>
                    ) : null}
                  </div>
                  <button
                    aria-label="发送"
                    className="agent-composer-send-button"
                    disabled={!canSend}
                    type="submit"
                  >
                    ↑
                  </button>
                </div>
              </div>
            </form>
          </aside>
        )}
      </section>
    </main>
  );
}

function ThinkingIndicator() {
  return (
    <p className="agent-thinking-indicator" aria-label="ClipAnvil 正在思考">
      <span aria-hidden="true">ClipAnvil 正在思考</span>
    </p>
  );
}

function messageClass(message: AgentMessage) {
  if (message.message_type === "error") {
    return "error";
  }
  return message.role;
}

function streamToMessage(
  stream: AgentStreamState,
): Pick<AgentMessage, "id" | "message_type" | "content"> {
  return {
    id: `stream-${stream.task_id}`,
    message_type: "text",
    content: {
      schema: agentMessageSchemaV1,
      blocks: [...stream.blocks]
        .sort((left, right) => streamBlockSort(left) - streamBlockSort(right))
        .map((block) =>
          block.type === "thinking"
            ? {
                id: block.id,
                type: "thinking",
                text: block.text,
                status: "streaming",
                default_collapsed: false,
              }
            : {
                id: block.id,
                type: "markdown",
                text: block.text,
              },
        ),
    },
  };
}

function streamBlockSort(block: AgentStreamState["blocks"][number]) {
  if (block.type === "thinking") {
    return -1;
  }
  return block.sequence;
}

function connectionStatusLabel(status: AgentConnectionStatus) {
  if (status === "connected") {
    return "已连接";
  }
  if (status === "connecting" || status === "reconnecting") {
    return "连接中";
  }
  return "离线";
}

function connectionStatusClass(status: AgentConnectionStatus) {
  if (status === "connected") {
    return "agent-status-connected";
  }
  if (status === "connecting" || status === "reconnecting") {
    return "agent-status-pending";
  }
  return "agent-status-offline";
}
