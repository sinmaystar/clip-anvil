import {
  type MouseEvent as ReactMouseEvent,
  type PointerEvent as ReactPointerEvent,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Navigate,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router";
import {
  type CanvasPayload,
  fetchCanvas,
  fetchWorkspace,
  type MediaNode,
} from "../lib/api";
import {
  type AgentAttachment,
  type AgentMessage,
  type AgentObservedThread,
  type AgentTask,
  fetchAgentCanvasDetail,
  fetchAgentModelSelection,
  fetchAgentMessages,
  fetchAgentCanvasWorkbench,
  fetchAgentTasks,
  fetchAgentThread,
  fetchAgentThreadMessages,
  fetchAgentThreads,
  postAgentDecision,
  postAgentMessage,
  putAgentModelSelection,
  uploadAgentAttachment,
} from "../lib/agentApi";
import {
  attachmentAccept,
  formatAgentAttachmentLabel,
  validAgentAttachmentFiles,
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
import { AgentCanvasDetailPanel } from "../components/agent-workbench/AgentCanvasDetailPanel";
import { AgentWorkbenchCanvas } from "../components/agent-workbench/AgentWorkbenchCanvas";
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
import {
  formatAgentMessageTime,
  isProducerThreadMessage,
  messageDisplayClass,
  mergeAgentMessages,
  visibleAgentMessages,
} from "../lib/agentMessages";
import { AgentThreadDrawer } from "../components/agent/AgentThreadDrawer";
import { AgentThreadObserverPanel } from "../components/agent/AgentThreadObserverPanel";
import { agentMessageSchemaV1 } from "../lib/agentMessageBlocks";
import {
  agentModelSelectionPayload,
  agentModelSelectionValue,
  formatAgentModelOption,
} from "../lib/agentModelSelection";
import {
  finalizeAgentStreamWithMessage,
  type AgentStreamState,
  mergeAgentStreamDelta,
  shouldShowAgentThinkingIndicator,
  visibleAgentStreams,
} from "../lib/agentStreaming";
import {
  agentModelSupportsThinking,
  agentThinkingEffortLabel,
  agentThinkingEffortOptions,
} from "../lib/agentThinking";
import {
  agentProcessingLabel,
  hasProcessingAgentTask,
  mergeActiveAgentTaskSnapshot,
  mergeAgentTasks,
} from "../lib/agentTasks";
import {
  type AgentThreadMessageCache,
  mergeAgentThreadMessages,
  mergeObservedAgentThreads,
  updateObservedThreadFromMessage,
  updateObservedThreadFromTask,
} from "../lib/agentThreads";
import {
  isTerminalGenerationStatus,
  nodeStatusForGenerationStatus,
  shouldPollCanvasForProductionUpdates,
} from "../lib/canvasRunState";
import { type AgentConnectionStatus, connectAgentSocket } from "../lib/agentWs";
import { connectCanvasSocket } from "../lib/ws";
import { createClientMessageId } from "../lib/clientMessageId";
import { preserveCanvasAssetUrls } from "../lib/canvasAssetUrls";
import {
  agentWorkbenchVisibleNodeCount,
  type AgentWorkbenchProjection,
} from "../lib/agentWorkbench";
import type { AgentWorkbenchSelection } from "../lib/agentWorkbenchSelection";
import { workspaceModeRoute } from "../lib/workspaceRoutes";
import { useAuthStore } from "../stores/auth";

export function AgentWorkspacePage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  const token = useAuthStore((state) => state.token);
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [tasks, setTasks] = useState<AgentTask[]>([]);
  const [streams, setStreams] = useState<AgentStreamState[]>([]);
  const [observedThreads, setObservedThreads] = useState<AgentObservedThread[]>(
    [],
  );
  const [selectedAgentThreadId, setSelectedAgentThreadId] = useState(
    () => searchParams.get("agentThread") ?? "",
  );
  const [agentThreadMessageCache, setAgentThreadMessageCache] =
    useState<AgentThreadMessageCache>({});
  const [subThreadStreams, setSubThreadStreams] = useState<
    Record<string, AgentStreamState[]>
  >({});
  const finalizedStreamKeysRef = useRef<Set<string>>(new Set());
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
  const [selectedWorkbenchSelection, setSelectedWorkbenchSelection] =
    useState<AgentWorkbenchSelection | null>(null);
  const [connectionStatus, setConnectionStatus] =
    useState<AgentConnectionStatus>("offline");
  const [canvasConnectionStatus, setCanvasConnectionStatus] =
    useState<AgentConnectionStatus>("offline");
  const lastMessageCreatedAtRef = useRef("");
  const fileInputRef = useRef<HTMLInputElement>(null);
  const agentCanvasSurfaceRef = useRef<HTMLDivElement>(null);
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
  useQuery<CanvasPayload>({
    queryKey: ["workspace", id, "canvas"],
    queryFn: () => fetchCanvas(id ?? ""),
    enabled: Boolean(id),
    refetchInterval: (query) =>
      canvasConnectionStatus !== "connected" &&
      shouldPollCanvasForProductionUpdates(query.state.data)
        ? 2_000
        : false,
    structuralSharing: (oldData, newData) =>
      preserveCanvasAssetUrls(
        oldData as CanvasPayload | undefined,
        newData as CanvasPayload,
      ),
  });
  const agentEnabled = Boolean(id && workspaceQuery.data?.mode === "agent");
  const workbenchQuery = useQuery<AgentWorkbenchProjection>({
    queryKey: ["workspace", id, "agent-workbench"],
    queryFn: () => fetchAgentCanvasWorkbench(id ?? ""),
    enabled: agentEnabled,
    refetchInterval: (query) =>
      canvasConnectionStatus !== "connected" &&
      hasActiveWorkbenchProduction(query.state.data)
        ? 2_000
        : false,
  });
  const agentCanvasNodeCount = workbenchQuery.data
    ? agentWorkbenchVisibleNodeCount(workbenchQuery.data)
    : 0;
  const detailQuery = useQuery({
    queryKey: [
      "workspace",
      id,
      "agent-canvas-detail",
      selectedWorkbenchSelection?.objectType,
      selectedWorkbenchSelection?.objectId,
    ],
    queryFn: () =>
      fetchAgentCanvasDetail(id ?? "", selectedWorkbenchSelection!),
    enabled: agentEnabled && Boolean(selectedWorkbenchSelection),
  });
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
  const agentThreadsQuery = useQuery({
    queryKey: ["agent", id, "threads"],
    queryFn: () => fetchAgentThreads(id ?? ""),
    enabled: agentEnabled,
  });
  const selectedAgentThread = observedThreads.find(
    (thread) => thread.id === selectedAgentThreadId,
  );
  const selectedAgentThreadMessagesQuery = useQuery({
    queryKey: ["agent", id, "threads", selectedAgentThreadId, "messages"],
    queryFn: () => fetchAgentThreadMessages(id ?? "", selectedAgentThreadId),
    enabled: agentEnabled && Boolean(selectedAgentThreadId),
  });
  const agentTasksQuery = useQuery({
    queryKey: ["agent", id, "tasks"],
    queryFn: () => fetchAgentTasks(id ?? ""),
    enabled: agentEnabled,
    refetchInterval: (query) =>
      query.state.data?.tasks.some(
        (task) => task.status === "queued" || task.status === "running",
      )
        ? 2_000
        : false,
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
      setMessages((current) => mergeAgentMessages(current, [response.message]));
      setTasks((current) => mergeAgentTasks(current, [response.task]));
      const decisionEvent = response.decision_event;
      if (decisionEvent) {
        setResolvedDecisionIds((current) => {
          const next = new Set(current);
          next.add(decisionEvent.id);
          return next;
        });
      }
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
      setMessages((current) => mergeAgentMessages(current, [response.message]));
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
  const visibleMessages = useMemo(
    () => visibleAgentMessages(messages),
    [messages],
  );
  const visibleStreams = useMemo(
    () => visibleAgentStreams(streams, visibleMessages),
    [streams, visibleMessages],
  );
  const hasPendingDecision = pendingDecisionIds.length > 0;
  const agentBusy = hasProcessingAgentTask(tasks);
  const processingLabel = agentProcessingLabel(tasks);
  const activityLabel =
    processingLabel || (hasPendingDecision ? "ClipAnvil 等待你的确认" : "");
  const showThinkingIndicator = shouldShowAgentThinkingIndicator(
    Boolean(activityLabel),
    visibleStreams,
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

  function selectAgentThread(threadID: string) {
    setSelectedAgentThreadId(threadID);
    setSearchParams((current) => {
      const next = new URLSearchParams(current);
      if (threadID) {
        next.set("agentThread", threadID);
      } else {
        next.delete("agentThread");
      }
      return next;
    });
  }

  useEffect(() => {
    if (agentMessagesQuery.data) {
      setMessages((current) =>
        mergeAgentMessages(current, agentMessagesQuery.data.messages),
      );
    }
  }, [agentMessagesQuery.data]);

  useEffect(() => {
    if (agentThreadsQuery.data) {
      setObservedThreads(
        mergeObservedAgentThreads([], agentThreadsQuery.data.threads),
      );
    }
  }, [agentThreadsQuery.data]);

  useEffect(() => {
    setSelectedAgentThreadId(searchParams.get("agentThread") ?? "");
  }, [searchParams]);

  useEffect(() => {
    if (selectedAgentThreadId && selectedAgentThreadMessagesQuery.data) {
      setAgentThreadMessageCache((current) =>
        mergeAgentThreadMessages(
          current,
          selectedAgentThreadId,
          selectedAgentThreadMessagesQuery.data.messages,
        ),
      );
    }
  }, [selectedAgentThreadId, selectedAgentThreadMessagesQuery.data]);

  useEffect(() => {
    if (agentTasksQuery.data) {
      setTasks((current) =>
        mergeActiveAgentTaskSnapshot(current, agentTasksQuery.data.tasks),
      );
    }
  }, [agentTasksQuery.data]);

  useEffect(() => {
    lastMessageCreatedAtRef.current =
      messages.length > 0 ? messages[messages.length - 1].created_at : "";
  }, [messages]);

  useEffect(() => {
    if (!shouldPinToBottomRef.current) {
      return;
    }
    scrollMessagesToBottom();
  }, [messages, visibleStreams, activityLabel]);

  useEffect(() => {
    if (!id || !token || workspaceQuery.data?.mode !== "agent") {
      return;
    }
    const producerThreadID = agentThreadQuery.data?.thread.id;

    const fetchMissingMessages = () => {
      void fetchAgentMessages(id, lastMessageCreatedAtRef.current, 1000).then(
        (response) => {
          setMessages((current) =>
            mergeAgentMessages(current, response.messages),
          );
        },
      );
      void queryClient.invalidateQueries({ queryKey: ["agent", id, "threads"] });
    };
    const refetchAgentCanvas = () => {
      void queryClient.refetchQueries({
        queryKey: ["workspace", id, "canvas"],
      });
      void queryClient.refetchQueries({
        queryKey: ["workspace", id, "agent-workbench"],
      });
      void queryClient.refetchQueries({
        queryKey: ["workspace", id, "agent-canvas-detail"],
      });
    };

    return connectAgentSocket({
      workspaceId: id,
      token,
      onEvent: (event) => {
        if (
          (event.type === "agent.message.created" ||
            event.type === "agent.message.updated") &&
          event.payload.workspace_id === id
        ) {
          const message = event.payload.message;
          if (isProducerThreadMessage(message, producerThreadID)) {
            setMessages((current) => mergeAgentMessages(current, [message]));
            if (event.type === "agent.message.created") {
              setStreams((current) => {
                const finalized = finalizeAgentStreamWithMessage(
                  current,
                  finalizedStreamKeysRef.current,
                  message,
                );
                finalizedStreamKeysRef.current = finalized.finalizedStreamKeys;
                return finalized.streams;
              });
            } else {
              finalizedStreamKeysRef.current = finalizeAgentStreamWithMessage(
                [],
                finalizedStreamKeysRef.current,
                message,
              ).finalizedStreamKeys;
            }
            refetchAgentCanvas();
          } else {
            setAgentThreadMessageCache((current) =>
              mergeAgentThreadMessages(current, message.thread_id, [message]),
            );
            setObservedThreads((current) => {
              if (!current.some((thread) => thread.id === message.thread_id)) {
                void queryClient.invalidateQueries({
                  queryKey: ["agent", id, "threads"],
                });
              }
              return updateObservedThreadFromMessage(current, message);
            });
            setSubThreadStreams((current) => ({
              ...current,
              [message.thread_id]: finalizeAgentStreamWithMessage(
                current[message.thread_id] ?? [],
                new Set(),
                message,
              ).streams,
            }));
          }
        }
        if (
          event.type === "agent.message.delta" &&
          event.payload.workspace_id === id
        ) {
          const deltaPayload = {
            task_id: event.payload.task_id,
            block_id: event.payload.block_id,
            block_type: event.payload.block_type,
            delta: event.payload.delta,
            message_id: event.payload.message_id,
            sequence: event.payload.sequence,
          };
          if (event.payload.thread_id === producerThreadID) {
            setStreams((current) =>
              mergeAgentStreamDelta(
                current,
                deltaPayload,
                finalizedStreamKeysRef.current,
              ),
            );
          } else if (event.payload.thread_id === selectedAgentThreadId) {
            setSubThreadStreams((current) => ({
              ...current,
              [event.payload.thread_id]: mergeAgentStreamDelta(
                current[event.payload.thread_id] ?? [],
                deltaPayload,
              ),
            }));
          }
        }
        if (
          event.type === "agent.task.updated" &&
          event.payload.workspace_id === id
        ) {
          setTasks((current) => mergeAgentTasks(current, [event.payload.task]));
          setObservedThreads((current) =>
            updateObservedThreadFromTask(current, event.payload.task),
          );
          if (
            event.payload.task.thread_id &&
            event.payload.task.thread_id !== producerThreadID
          ) {
            void queryClient.invalidateQueries({
              queryKey: ["agent", id, "threads"],
            });
          }
          refetchAgentCanvas();
        }
        if (
          event.type === "agent.event.created" &&
          event.payload.workspace_id === id
        ) {
          refetchAgentCanvas();
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
  }, [
    id,
    token,
    workspaceQuery.data?.mode,
    queryClient,
    agentThreadQuery.data?.thread.id,
  ]);

  useEffect(() => {
    if (!id || !token || workspaceQuery.data?.mode !== "agent") {
      return;
    }

    const refetchCanvasSnapshot = () => {
      void queryClient.refetchQueries({
        queryKey: ["workspace", id, "canvas"],
      });
      void queryClient.refetchQueries({
        queryKey: ["workspace", id, "agent-workbench"],
      });
    };
    const refetchWorkbench = () => {
      void queryClient.refetchQueries({
        queryKey: ["workspace", id, "agent-workbench"],
      });
      void queryClient.refetchQueries({
        queryKey: ["workspace", id, "agent-canvas-detail"],
      });
    };
    const refreshCanvas = () => {
      refetchCanvasSnapshot();
    };

    return connectCanvasSocket({
      workspaceId: id,
      token,
      onEvent: (event) => {
        switch (event.type) {
          case "NodeCreated":
          case "NodeUpdated": {
            const node = canvasNodeFromEventPayload(event.payload.node);
            if (node) {
              queryClient.setQueryData<CanvasPayload>(
                ["workspace", id, "canvas"],
                (current) => upsertCanvasNode(current, node),
              );
              refetchWorkbench();
            }
            break;
          }
          case "NodeDeleted":
          case "EdgeCreated":
          case "EdgeDeleted":
          case "GroupCreated":
          case "GroupUpdated":
          case "GroupDeleted":
            refreshCanvas();
            break;
          case "production.job.updated":
          case "production.model.delta": {
            const nodeStatus = nodeStatusForGenerationStatus(
              event.payload.status,
            );
            if (nodeStatus) {
              queryClient.setQueryData<CanvasPayload>(
                ["workspace", id, "canvas"],
                (current) =>
                  updateCanvasNodeStatus(
                    current,
                    event.payload.node_id,
                    nodeStatus,
                  ),
              );
              refetchWorkbench();
            }
            if (
              event.type === "production.job.updated" &&
              isTerminalGenerationStatus(event.payload.status)
            ) {
              refetchCanvasSnapshot();
            }
            break;
          }
        }
      },
      onReconnect: refreshCanvas,
      onStatusChange: (status) => {
        setCanvasConnectionStatus(status);
        if (status === "connected") {
          refreshCanvas();
        }
      },
    });
  }, [id, queryClient, token, workspaceQuery.data?.mode]);

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
  const thinkingEffortOptions = agentThinkingEffortOptions(selectedModelOption);
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
  const uploadSelectedAttachments = (files: FileList | null | undefined) => {
    if (!files || !id) {
      return;
    }
    const validFiles = validAgentAttachmentFiles(files);
    if (validFiles.length === 0) {
      setAttachmentError("仅支持图片、视频或 txt 文本。");
      return;
    }
    const availableSlots = Math.max(0, 12 - attachments.length);
    if (availableSlots === 0) {
      setAttachmentError("最多可添加 12 个附件。");
      return;
    }
    const uploadFiles = validFiles.slice(0, availableSlots);
    if (uploadFiles.length < validFiles.length) {
      setAttachmentError("最多可添加 12 个附件，已上传前面的文件。");
    }
    uploadFiles.forEach((file) => uploadAttachmentMutation.mutate(file));
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
  const beginWidthPointerResize = (
    event: ReactPointerEvent<HTMLDivElement>,
  ) => {
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
  const beginHeightPointerResize = (
    event: ReactPointerEvent<HTMLDivElement>,
  ) => {
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
    setSelectedWorkbenchSelection(null);
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
        aria-label="Agent 画布"
        onPointerDown={(event) => {
          if (
            shouldClearAgentWorkbenchSelection(
              event.target,
              event.currentTarget,
            )
          ) {
            setSelectedWorkbenchSelection(null);
          }
          if (event.target === event.currentTarget) {
            collapseFromCanvas();
          }
        }}
      >
        <section className="agent-flow-canvas-panel">
          <div className="agent-canvas-header">
            <div>
              <p className="workspace-kicker">Agent Canvas</p>
            </div>
            <span>{agentCanvasNodeCount} 个节点</span>
          </div>
          <div className="agent-canvas-surface" ref={agentCanvasSurfaceRef}>
            {workbenchQuery.isLoading ? (
              <p className="agent-empty-text">正在加载画布</p>
            ) : workbenchQuery.data && agentCanvasNodeCount > 1 ? (
              <AgentWorkbenchCanvas
                onSelectObject={setSelectedWorkbenchSelection}
                selected={selectedWorkbenchSelection}
                workbench={workbenchQuery.data}
              />
            ) : (
              <p className="agent-empty-text">Agent 尚未创建场景或分镜。</p>
            )}
          </div>
        </section>

        <AgentCanvasDetailPanel
          detail={detailQuery.data}
          error={detailQuery.error instanceof Error ? detailQuery.error : null}
          isLoading={detailQuery.isLoading}
          onClose={() => setSelectedWorkbenchSelection(null)}
          onRetry={() => {
            void detailQuery.refetch();
          }}
          onSelectObject={setSelectedWorkbenchSelection}
          selection={selectedWorkbenchSelection}
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
              <div className="agent-chat-header-actions">
                <AgentThreadObserverPanel
                  threads={observedThreads}
                  selectedThreadId={selectedAgentThreadId}
                  onSelectThread={selectAgentThread}
                />
                <button
                  className="agent-icon-button"
                  onClick={() => setCollapsed(true)}
                  title="收起"
                  type="button"
                >
                  ×
                </button>
              </div>
            </div>

            <div className="agent-chat-main-column">
              <div
                className="agent-message-list"
                aria-live="polite"
                onScroll={updateMessageScrollState}
                ref={messageListRef}
              >
                {agentMessagesQuery.isLoading ? (
                  <p className="agent-empty-text">正在加载对话</p>
                ) : visibleMessages.length > 0 ? (
                  <>
                    {visibleMessages.map((message) => {
                      const decisionCard = decisionCardFromMessage(message);
                      const isDecisionResolved =
                        decisionCard !== null &&
                        (decisionCard.status === "handled" ||
                          resolvedDecisionIds.has(decisionCard.decision_id));
                      const messageTime = formatAgentMessageTime(
                        message.created_at,
                      );
                      return (
                        <article
                          className={`agent-message agent-message-${messageClass(message)}${isNestedAgentMessage(message) ? " agent-message-nested" : ""}`}
                          key={message.id}
                        >
                          <AgentMessageRenderer
                            actions={{
                              ...messageActions,
                              observedAgentThreads: observedThreads,
                              onSelectAgentThread: selectAgentThread,
                              disabled:
                                isDecisionResolved ||
                                respondDecisionMutation.isPending,
                            }}
                            message={message}
                          />
                          {messageTime ? (
                            <time
                              className="agent-message-time"
                              dateTime={message.created_at}
                            >
                              {messageTime}
                            </time>
                          ) : null}
                        </article>
                      );
                    })}
                    {visibleStreams.map((stream) => (
                      <article
                        className="agent-message agent-message-assistant agent-message-streaming"
                        key={stream.task_id}
                      >
                        <AgentMessageRenderer message={streamToMessage(stream)} />
                      </article>
                    ))}
                    {showThinkingIndicator ? (
                      <ThinkingIndicator label={activityLabel} />
                    ) : null}
                  </>
                ) : (
                  <>
                    {visibleStreams.map((stream) => (
                      <article
                        className="agent-message agent-message-assistant agent-message-streaming"
                        key={stream.task_id}
                      >
                        <AgentMessageRenderer message={streamToMessage(stream)} />
                      </article>
                    ))}
                    {visibleStreams.length === 0 ? (
                      showThinkingIndicator ? (
                        <ThinkingIndicator label={activityLabel} />
                      ) : (
                        <p className="agent-empty-text">
                          还没有 ClipAnvil 对话。
                        </p>
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
            </div>

            {sendError ? <p className="agent-chat-error">{sendError}</p> : null}
            {attachmentError ? (
              <p className="agent-chat-error">{attachmentError}</p>
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
                multiple
                onChange={(event) => {
                  uploadSelectedAttachments(event.target.files);
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
                    agentBusy
                      ? "等待当前任务完成"
                      : hasPendingDecision
                        ? "回复当前决策，或点击上方选项"
                        : "输入需求或反馈"
                  }
                  rows={3}
                  value={draft}
                />
                <div className="agent-composer-toolbar">
                  <div className="agent-composer-tools">
                    <button
                      aria-label="添加附件"
                      className="agent-composer-icon-button"
                      disabled={
                        agentBusy ||
                        hasPendingDecision ||
                        uploadAttachmentMutation.isPending
                      }
                      onClick={chooseAttachment}
                      type="button"
                    >
                      +
                    </button>
                    {agentModelSelectionQuery.data ? (
                      <select
                        aria-label="对话模型"
                        className="agent-model-select"
                        disabled={
                          agentBusy ||
                          hasPendingDecision ||
                          modelSelectionMutation.isPending
                        }
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
                    {agentModelSelectionQuery.data &&
                    thinkingSelectorEnabled ? (
                      <select
                        aria-label="思考深度"
                        className="agent-thinking-select"
                        disabled={
                          agentBusy ||
                          hasPendingDecision ||
                          modelSelectionMutation.isPending
                        }
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
            <AgentThreadDrawer
              isLoading={selectedAgentThreadMessagesQuery.isLoading}
              messages={
                selectedAgentThreadId
                  ? (agentThreadMessageCache[selectedAgentThreadId]?.messages ??
                    [])
                  : []
              }
              onClose={() => selectAgentThread("")}
              streams={
                selectedAgentThreadId
                  ? (subThreadStreams[selectedAgentThreadId] ?? [])
                  : []
              }
              thread={selectedAgentThread}
            />
          </aside>
        )}
      </section>
    </main>
  );
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

function shouldClearAgentWorkbenchSelection(
  target: EventTarget | null,
  currentTarget: HTMLElement,
) {
  if (!(target instanceof Element) || !currentTarget.contains(target)) {
    return false;
  }
  if (
    target.closest(
      [
        ".agent-canvas-detail-panel",
        ".agent-chat-float",
        ".agent-chat-restore",
        ".react-flow__node",
        ".react-flow__controls",
        ".react-flow__minimap",
        "button",
        "a",
        "input",
        "textarea",
        "select",
      ].join(","),
    )
  ) {
    return false;
  }
  return Boolean(
    target.closest(
      ".agent-canvas-surface, .agent-workbench-surface, .react-flow__pane",
    ),
  );
}

function hasActiveWorkbenchProduction(
  workbench: AgentWorkbenchProjection | undefined,
) {
  if (!workbench) {
    return false;
  }
  return workbench.scenes.some((scene) =>
    scene.shots.some(
      (shot) =>
        shot.preview.status === "queued" ||
        shot.preview.status === "running" ||
        shot.video.status === "queued" ||
        shot.video.status === "running",
    ),
  );
}

function upsertCanvasNode(current: CanvasPayload | undefined, node: MediaNode) {
  if (!current) {
    return current;
  }
  if (current.nodes.some((item) => item.id === node.id)) {
    return {
      ...current,
      nodes: current.nodes.map((item) => (item.id === node.id ? node : item)),
    };
  }
  return { ...current, nodes: [...current.nodes, node] };
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

function ThinkingIndicator({ label }: { label: string }) {
  const text = label || "ClipAnvil 正在思考";
  return (
    <p className="agent-thinking-indicator" aria-label={text}>
      <span aria-hidden="true">{text}</span>
    </p>
  );
}

function messageClass(message: AgentMessage) {
  return messageDisplayClass(message);
}

function isNestedAgentMessage(message: AgentMessage) {
  return typeof message.raw_message?.parent_tool_call_id === "string";
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
