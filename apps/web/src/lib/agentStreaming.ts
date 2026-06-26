export interface AgentStreamDelta {
  task_id: string;
  message_id?: string;
  block_id?: string;
  block_type?: "markdown" | "thinking" | string;
  delta: string;
  sequence?: number;
}

export interface AgentStreamBlock {
  id: string;
  type: "markdown" | "thinking" | string;
  text: string;
  sequence: number;
}

export interface AgentStreamState {
  task_id: string;
  message_id?: string;
  blocks: AgentStreamBlock[];
}

export interface AgentStreamFinalMessage {
  id?: string;
  role?: string;
  message_type?: string;
  task_id?: string | null;
  content?: unknown;
}

export function mergeAgentStreamDelta(
  current: AgentStreamState[],
  incoming: AgentStreamDelta,
  finalizedStreamKeys: Set<string> = new Set(),
): AgentStreamState[] {
  if (!incoming.task_id || !incoming.delta) {
    return current;
  }
  if (isFinalizedStream(incoming, finalizedStreamKeys)) {
    return current;
  }
  const blockID = incoming.block_id || "blk_answer";
  const blockType = incoming.block_type || "markdown";
  const sequence = incoming.sequence ?? 0;
  const index = current.findIndex((stream) => stream.task_id === incoming.task_id);
  if (index === -1) {
    const messageID = stringValue(incoming.message_id);
    return [
      ...current,
      {
        task_id: incoming.task_id,
        ...(messageID ? { message_id: messageID } : {}),
        blocks: [
          {
            id: blockID,
            type: blockType,
            text: incoming.delta,
            sequence,
          },
        ],
      },
    ];
  }
  return current.map((stream, streamIndex) =>
    streamIndex === index
      ? mergeStreamBlock(stream, blockID, blockType, incoming.delta, sequence)
      : stream,
  );
}

function mergeStreamBlock(
  stream: AgentStreamState,
  blockID: string,
  blockType: string,
  delta: string,
  sequence: number,
): AgentStreamState {
  const blockIndex = stream.blocks.findIndex((block) => block.id === blockID);
  if (blockIndex === -1) {
    return {
      ...stream,
      blocks: [
        ...stream.blocks,
        {
          id: blockID,
          type: blockType,
          text: delta,
          sequence,
        },
      ],
    };
  }
  return {
    ...stream,
    blocks: stream.blocks.map((block, index) =>
      index === blockIndex
        ? {
            ...block,
            text: block.text + delta,
            sequence,
          }
        : block,
    ),
  };
}

export function clearAgentStream(
  current: AgentStreamState[],
  taskId: string | null | undefined,
): AgentStreamState[] {
  if (!taskId) {
    return current;
  }
  return current.filter((stream) => stream.task_id !== taskId);
}

export function visibleAgentStreams(
  streams: AgentStreamState[],
  messages: AgentStreamFinalMessage[],
) {
  const finalizedStreamKeys = new Set<string>();
  const finalizedTexts = new Set<string>();
  for (const message of messages) {
    if (!isFinalAssistantTextMessage(message)) {
      continue;
    }
    addIfPresent(finalizedStreamKeys, message.task_id);
    addIfPresent(finalizedStreamKeys, message.id);
    addIfPresent(finalizedTexts, finalMessageMarkdownText(message.content));
  }
  if (finalizedStreamKeys.size === 0 && finalizedTexts.size === 0) {
    return streams;
  }
  return streams.filter(
    (stream) =>
      !finalizedStreamKeys.has(stream.task_id) &&
      !finalizedStreamKeys.has(stream.message_id ?? "") &&
      !finalizedTexts.has(streamMarkdownText(stream)),
  );
}

export function rememberFinalAgentMessage(
  current: Set<string>,
  message: AgentStreamFinalMessage,
) {
  if (!isFinalAssistantTextMessage(message)) {
    return current;
  }
  const next = new Set(current);
  addIfPresent(next, message.task_id);
  addIfPresent(next, message.id);
  return next;
}

export function shouldShowAgentThinkingIndicator(
  producerRunning: boolean,
  streams: AgentStreamState[],
) {
  return producerRunning && streams.length === 0;
}

function isFinalizedStream(
  incoming: Pick<AgentStreamDelta, "task_id" | "message_id">,
  finalizedStreamKeys: Set<string>,
) {
  return (
    finalizedStreamKeys.has(incoming.task_id) ||
    finalizedStreamKeys.has(stringValue(incoming.message_id))
  );
}

function isFinalAssistantTextMessage(message: AgentStreamFinalMessage) {
  return message.role === "assistant" && message.message_type === "text";
}

function finalMessageMarkdownText(content: unknown) {
  if (!content || typeof content !== "object") {
    return "";
  }
  const envelope = content as { blocks?: unknown };
  if (!Array.isArray(envelope.blocks)) {
    return "";
  }
  return normalizeText(
    envelope.blocks
      .map((block) => {
        if (!block || typeof block !== "object") {
          return "";
        }
        const value = block as { type?: unknown; text?: unknown };
        if (value.type !== "markdown" || typeof value.text !== "string") {
          return "";
        }
        return value.text;
      })
      .filter(Boolean)
      .join("\n"),
  );
}

function streamMarkdownText(stream: AgentStreamState) {
  return normalizeText(
    stream.blocks
      .filter((block) => block.type === "markdown")
      .sort((left, right) => left.sequence - right.sequence)
      .map((block) => block.text)
      .join("\n"),
  );
}

function normalizeText(value: string) {
  return value.replace(/\s+/g, " ").trim();
}

function addIfPresent(set: Set<string>, value: string | null | undefined) {
  const normalized = stringValue(value);
  if (normalized) {
    set.add(normalized);
  }
}

function stringValue(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}
