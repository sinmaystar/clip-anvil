import { useState } from "react";
import type { AgentToolStatusBlock as AgentToolStatusBlockData } from "../../lib/agentMessageBlocks";

export function AgentToolStatusBlock({
  block,
}: {
  block: AgentToolStatusBlockData;
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
