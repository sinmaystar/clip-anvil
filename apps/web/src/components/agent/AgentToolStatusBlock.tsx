import type { AgentToolStatusBlock as AgentToolStatusBlockData } from "../../lib/agentMessageBlocks";

export function AgentToolStatusBlock({
  block,
}: {
  block: AgentToolStatusBlockData;
}) {
  const statusLabel =
    block.status === "running"
      ? "执行中"
      : block.status === "failed"
        ? "失败"
        : "完成";
  return (
    <section className={`agent-tool-status agent-tool-status-${block.status}`}>
      <div>
        <span aria-hidden="true" />
        <strong>{block.label || block.tool_name}</strong>
      </div>
      <small>{statusLabel}</small>
      {block.summary ? <p>{block.summary}</p> : null}
      {block.error_message ? <p>{block.error_message}</p> : null}
    </section>
  );
}
