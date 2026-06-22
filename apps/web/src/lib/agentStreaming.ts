export interface AgentStreamDelta {
  task_id: string;
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
  blocks: AgentStreamBlock[];
}

export function mergeAgentStreamDelta(
  current: AgentStreamState[],
  incoming: AgentStreamDelta,
): AgentStreamState[] {
  if (!incoming.task_id || !incoming.delta) {
    return current;
  }
  const blockID = incoming.block_id || "blk_answer";
  const blockType = incoming.block_type || "markdown";
  const sequence = incoming.sequence ?? 0;
  const index = current.findIndex((stream) => stream.task_id === incoming.task_id);
  if (index === -1) {
    return [
      ...current,
      {
        task_id: incoming.task_id,
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

export function shouldShowAgentThinkingIndicator(
  producerRunning: boolean,
  streams: AgentStreamState[],
) {
  return producerRunning && streams.length === 0;
}
