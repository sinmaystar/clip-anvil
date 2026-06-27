import type { AgentMessage, AgentObservedThread, AgentTask } from "./agentApi";
import { mergeAgentMessages } from "./agentMessages.js";

export interface AgentThreadMessageCacheEntry {
  messages: AgentMessage[];
  hasLoadedInitial: boolean;
}

export type AgentThreadMessageCache = Record<
  string,
  AgentThreadMessageCacheEntry
>;

export function mergeObservedAgentThreads(
  current: AgentObservedThread[],
  incoming: AgentObservedThread[],
) {
  return mergeObservedAgentThreadPartials(current, incoming) as AgentObservedThread[];
}

function mergeObservedAgentThreadPartials(
  current: Partial<AgentObservedThread>[],
  incoming: Partial<AgentObservedThread>[],
) {
  const byId = new Map<string, Partial<AgentObservedThread>>();
  for (const thread of current) {
    if (thread.id) {
      byId.set(thread.id, thread);
    }
  }
  for (const thread of incoming) {
    if (thread.id) {
      byId.set(thread.id, { ...byId.get(thread.id), ...thread });
    }
  }
  return Array.from(byId.values()).sort(compareObservedThreads);
}

export function mergeAgentThreadMessages(
  current: AgentThreadMessageCache,
  threadId: string,
  incoming: AgentMessage[],
): AgentThreadMessageCache {
  const entry = current[threadId] ?? {
    messages: [],
    hasLoadedInitial: false,
  };
  return {
    ...current,
    [threadId]: {
      messages: mergeAgentMessages(entry.messages, incoming),
      hasLoadedInitial: true,
    },
  };
}

export function updateObservedThreadFromTask<
  T extends Partial<AgentObservedThread>,
>(threads: T[], task: Partial<AgentTask>): T[] {
  if (!task.thread_id) {
    return threads;
  }
  return threads.map((thread) =>
    thread.id === task.thread_id
      ? ({ ...thread, latest_task: task } as T)
      : thread,
  );
}

export function updateObservedThreadFromMessage<
  T extends Partial<AgentObservedThread>,
>(threads: T[], message: Partial<AgentMessage>): T[] {
  if (!message.thread_id) {
    return threads;
  }
  return threads.map((thread) =>
    thread.id === message.thread_id
      ? ({
          ...thread,
          latest_message_at: message.created_at,
          latest_message_preview: threadPreviewFromMessage(message),
        } as T)
      : thread,
  );
}

export function threadPreviewFromMessage(
  message: Pick<Partial<AgentMessage>, "content">,
) {
  const content = message.content;
  if (!content || typeof content !== "object") {
    return "";
  }
  const blocks = (content as { blocks?: unknown }).blocks;
  if (!Array.isArray(blocks)) {
    return "";
  }
  return blocks
    .map((block) => {
      if (!block || typeof block !== "object") {
        return "";
      }
      const text = (block as { text?: unknown }).text;
      return typeof text === "string" ? text.trim() : "";
    })
    .filter(Boolean)
    .join("\n")
    .slice(0, 160);
}

export function threadsForDispatchTool<T extends Partial<AgentObservedThread>>(
  toolName: string,
  threads: T[],
) {
  if (toolName === "dispatch_craftsman") {
    return threads.filter((thread) => thread.role === "craftsman");
  }
  if (toolName === "dispatch_reviewer") {
    return threads.filter((thread) => thread.role === "reviewer");
  }
  if (toolName.startsWith("dispatch_")) {
    return threads;
  }
  return [];
}

function compareObservedThreads(
  left: Partial<AgentObservedThread>,
  right: Partial<AgentObservedThread>,
) {
  const leftActive = activeRank(left.latest_task?.status);
  const rightActive = activeRank(right.latest_task?.status);
  if (leftActive !== rightActive) {
    return leftActive - rightActive;
  }
  const leftTime = Date.parse(
    left.latest_message_at ?? left.updated_at ?? left.created_at ?? "",
  );
  const rightTime = Date.parse(
    right.latest_message_at ?? right.updated_at ?? right.created_at ?? "",
  );
  if (
    !Number.isNaN(leftTime) &&
    !Number.isNaN(rightTime) &&
    leftTime !== rightTime
  ) {
    return rightTime - leftTime;
  }
  return String(left.display_name ?? left.id ?? "").localeCompare(
    String(right.display_name ?? right.id ?? ""),
  );
}

function activeRank(status: unknown) {
  if (status === "running") {
    return 0;
  }
  if (status === "queued") {
    return 1;
  }
  if (status === "failed") {
    return 2;
  }
  return 3;
}
