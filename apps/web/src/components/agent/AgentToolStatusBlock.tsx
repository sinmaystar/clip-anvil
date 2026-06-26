import { useState } from "react";
import type { AgentObservedThread } from "../../lib/agentApi";
import type { AgentToolStatusBlock as AgentToolStatusBlockData } from "../../lib/agentMessageBlocks";
import { threadsForDispatchTool } from "../../lib/agentThreads";
import { AgentThreadLinkChip } from "./AgentThreadLinkChip";

export function AgentToolStatusBlock({
  block,
  actions,
}: {
  block: AgentToolStatusBlockData;
  actions?: {
    onSelectAgentThread?: (threadId: string) => void;
    observedAgentThreads?: AgentObservedThread[];
  };
}) {
  const [expanded, setExpanded] = useState(false);
  const statusLabel =
    block.status === "running"
      ? "执行中"
      : block.status === "failed"
        ? "失败"
        : "完成";
  const brief = toolBrief(block);
  return (
    <section className={`agent-tool-status agent-tool-status-${block.status}`}>
      <button
        className="agent-tool-status-toggle"
        onClick={() => setExpanded((current) => !current)}
        type="button"
      >
        <span aria-hidden="true" className="agent-tool-status-dot" />
        <span className="agent-tool-status-copy">
          <strong>{block.tool_name}</strong>
          {brief ? <small>{brief}</small> : null}
        </span>
        <span className="agent-tool-status-state">{statusLabel}</span>
        <span aria-hidden="true" className="agent-tool-status-chevron">
          {expanded ? "⌃" : "⌄"}
        </span>
      </button>
      {expanded ? (
        <div className="agent-tool-status-details">
          {block.error_message ? <p>{block.error_message}</p> : null}
          {block.arguments ? (
            <ToolPayloadDetails label="入参" value={block.arguments} />
          ) : null}
          {block.result ? (
            <ToolPayloadDetails label="出参" value={block.result} />
          ) : null}
        </div>
      ) : null}
      {actions?.onSelectAgentThread ? (
        <ToolThreadLinks
          block={block}
          observedThreads={actions.observedAgentThreads}
          onSelectThread={actions.onSelectAgentThread}
        />
      ) : null}
    </section>
  );
}

function toolBrief(block: AgentToolStatusBlockData) {
  if (block.summary?.trim()) {
    return block.summary.trim();
  }
  const brief = block.arguments?.brief;
  return typeof brief === "string" ? brief.trim() : "";
}

function ToolPayloadDetails({
  label,
  value,
}: {
  label: string;
  value: Record<string, unknown>;
}) {
  return (
    <details className="agent-tool-payload">
      <summary>{label}</summary>
      <pre>{JSON.stringify(value, null, 2)}</pre>
    </details>
  );
}

function ToolThreadLinks({
  block,
  observedThreads,
  onSelectThread,
}: {
  block: AgentToolStatusBlockData;
  observedThreads?: AgentObservedThread[];
  onSelectThread: (threadId: string) => void;
}) {
  const threads =
    observedThreads && observedThreads.length > 0
      ? threadsForDispatchTool(block.tool_name, observedThreads)
      : dispatchThreads(block.result);
  if (threads.length === 0) {
    return null;
  }
  return (
    <div className="agent-tool-thread-links">
      {threads.map((thread) => (
        <AgentThreadLinkChip
          key={thread.id}
          onSelect={onSelectThread}
          thread={thread}
        />
      ))}
    </div>
  );
}

function dispatchThreads(result: Record<string, unknown> | undefined) {
  const dispatched = Array.isArray(result?.dispatched)
    ? result.dispatched
    : [];
  return dispatched
    .map((item) => {
      if (!item || typeof item !== "object") {
        return null;
      }
      const value = item as Record<string, unknown>;
      const threadID =
        stringValue(value.craftsman_thread_id) ||
        stringValue(value.reviewer_thread_id);
      if (!threadID) {
        return null;
      }
      const label =
        stringValue(value.client_key) ||
        stringValue(value.scope_type) ||
        threadID.slice(0, 8);
      return {
        id: threadID,
        display_name: label,
        latest_task: { status: stringValue(value.status) || "queued" },
      } satisfies Pick<AgentObservedThread, "id" | "display_name"> & {
        latest_task: { status: string };
      };
    })
    .filter(
      (
        item,
      ): item is Pick<AgentObservedThread, "id" | "display_name"> & {
        latest_task: { status: string };
      } => Boolean(item),
    );
}

function stringValue(value: unknown) {
  return typeof value === "string" && value.trim() ? value.trim() : "";
}
