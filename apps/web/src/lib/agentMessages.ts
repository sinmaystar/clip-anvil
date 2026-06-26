export interface SequencedAgentMessage {
  id: string;
  thread_id?: string;
  seq: number;
  created_at?: string;
  message_type?: string;
  content?: unknown;
  raw_message?: {
    parent_tool_call_id?: unknown;
    source?: unknown;
    trigger?: unknown;
    tool_call_id?: unknown;
  };
}

export function isProducerThreadMessage(
  message: Pick<SequencedAgentMessage, "thread_id">,
  producerThreadID: string | undefined | null,
) {
  if (!producerThreadID || !message.thread_id) {
    return false;
  }
  return message.thread_id === producerThreadID;
}

export function mergeAgentMessages<T extends SequencedAgentMessage>(
  current: T[],
  incoming: T[],
) {
  const byID = new Map<string, T>();
  for (const message of current) {
    byID.set(message.id, message);
  }
  for (const message of incoming) {
    byID.set(message.id, message);
  }
  return orderNestedAgentMessages(Array.from(byID.values()).sort(compareAgentMessages));
}

export function visibleAgentMessages<T extends SequencedAgentMessage>(
  messages: T[],
) {
  const completedDecisionToolCalls = requestDecisionToolCallIDsByStatus(
    messages,
    "succeeded",
  );
  return messages.filter((message) => {
    if (message.message_type === "tool_result") {
      return false;
    }
    return !isObsoleteRunningRequestDecisionToolCall(
      message,
      completedDecisionToolCalls,
    );
  });
}

export function formatAgentMessageTime(
  createdAt: string | undefined | null,
  timeZone?: string,
) {
  if (!createdAt) {
    return "";
  }
  const date = new Date(createdAt);
  if (Number.isNaN(date.getTime())) {
    return "";
  }
  try {
    const formatter = new Intl.DateTimeFormat("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: false,
      timeZone,
    });
    const parts = Object.fromEntries(
      formatter
        .formatToParts(date)
        .filter((part) => part.type !== "literal")
        .map((part) => [part.type, part.value]),
    );
    if (
      !parts.year ||
      !parts.month ||
      !parts.day ||
      !parts.hour ||
      !parts.minute ||
      !parts.second
    ) {
      return formatter.format(date);
    }
    return `${parts.year}/${parts.month}/${parts.day} ${parts.hour}:${parts.minute}:${parts.second}`;
  } catch {
    return "";
  }
}

export function isSystemReminderMessage(
  message: Pick<SequencedAgentMessage, "content" | "raw_message">,
) {
  const source = stringValue(message.raw_message?.source);
  const trigger = stringValue(message.raw_message?.trigger);
  if (source === "system" && trigger) {
    return true;
  }
  return agentMessageText(message.content).includes("<system-reminder>");
}

function compareAgentMessages<T extends SequencedAgentMessage>(a: T, b: T) {
  const aCreatedAt = Date.parse(a.created_at ?? "");
  const bCreatedAt = Date.parse(b.created_at ?? "");
  if (!Number.isNaN(aCreatedAt) && !Number.isNaN(bCreatedAt)) {
    if (aCreatedAt !== bCreatedAt) {
      return aCreatedAt - bCreatedAt;
    }
  }
  if (a.seq !== b.seq) {
    return a.seq - b.seq;
  }
  return a.id.localeCompare(b.id);
}

function orderNestedAgentMessages<T extends SequencedAgentMessage>(messages: T[]) {
  const childrenByParent = new Map<string, T[]>();
  const childIDs = new Set<string>();
  for (const message of messages) {
    const parentToolCallID = stringValue(message.raw_message?.parent_tool_call_id);
    if (!parentToolCallID) {
      continue;
    }
    childIDs.add(message.id);
    const children = childrenByParent.get(parentToolCallID) ?? [];
    children.push(message);
    childrenByParent.set(parentToolCallID, children);
  }
  if (childIDs.size === 0) {
    return messages;
  }

  const ordered: T[] = [];
  const emittedChildren = new Set<string>();
  for (const message of messages) {
    if (childIDs.has(message.id)) {
      continue;
    }
    ordered.push(message);
    const toolCallID = stringValue(message.raw_message?.tool_call_id);
    if (!toolCallID) {
      continue;
    }
    for (const child of childrenByParent.get(toolCallID) ?? []) {
      if (emittedChildren.has(child.id)) {
        continue;
      }
      ordered.push(child);
      emittedChildren.add(child.id);
    }
  }
  for (const message of messages) {
    if (childIDs.has(message.id) && !emittedChildren.has(message.id)) {
      ordered.push(message);
    }
  }
  return ordered;
}

function stringValue(value: unknown) {
  return typeof value === "string" && value.trim() ? value.trim() : "";
}

function agentMessageText(content: unknown) {
  if (!content || typeof content !== "object") {
    return "";
  }
  const envelope = content as { schema?: unknown; blocks?: unknown };
  if (
    envelope.schema !== "clipanvil.agent.message.v1" ||
    !Array.isArray(envelope.blocks)
  ) {
    return "";
  }
  return envelope.blocks
    .map((block) => {
      if (!block || typeof block !== "object") {
        return "";
      }
      const value = block as { text?: unknown };
      return typeof value.text === "string" ? value.text : "";
    })
    .filter(Boolean)
    .join("\n");
}

function requestDecisionToolCallIDsByStatus<T extends SequencedAgentMessage>(
  messages: T[],
  status: "running" | "succeeded" | "failed",
) {
  const ids = new Set<string>();
  for (const message of messages) {
    const toolStatus = requestDecisionToolStatus(message);
    if (!toolStatus || toolStatus.status !== status) {
      continue;
    }
    const toolCallID =
      stringValue(toolStatus.tool_call_id) ||
      stringValue(message.raw_message?.tool_call_id);
    if (toolCallID) {
      ids.add(toolCallID);
    }
  }
  return ids;
}

function isObsoleteRunningRequestDecisionToolCall<T extends SequencedAgentMessage>(
  message: T,
  completedDecisionToolCalls: Set<string>,
) {
  const toolStatus = requestDecisionToolStatus(message);
  if (!toolStatus || toolStatus.status !== "running") {
    return false;
  }
  const toolCallID =
    stringValue(toolStatus.tool_call_id) ||
    stringValue(message.raw_message?.tool_call_id);
  return Boolean(toolCallID && completedDecisionToolCalls.has(toolCallID));
}

function requestDecisionToolStatus(message: { content?: unknown }) {
  const content = message.content;
  if (!content || typeof content !== "object") {
    return null;
  }
  const envelope = content as { schema?: unknown; blocks?: unknown };
  if (
    envelope.schema !== "clipanvil.agent.message.v1" ||
    !Array.isArray(envelope.blocks)
  ) {
    return null;
  }
  for (const block of envelope.blocks) {
    if (!block || typeof block !== "object") {
      continue;
    }
    const value = block as {
      type?: unknown;
      tool_call_id?: unknown;
      tool_name?: unknown;
      label?: unknown;
      status?: unknown;
    };
    const toolName = stringValue(value.tool_name) || stringValue(value.label);
    const status = toolStatusValue(value.status);
    if (
      value.type === "tool_status" &&
      toolName === "request_user_decision" &&
      status
    ) {
      return {
        tool_call_id: value.tool_call_id,
        status,
      };
    }
  }
  return null;
}

function toolStatusValue(value: unknown) {
  if (value === "running" || value === "succeeded" || value === "failed") {
    return value;
  }
  return "";
}
