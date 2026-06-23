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
      {block.arguments ? (
        <ToolPayloadDetails label="参数" value={block.arguments} />
      ) : null}
      {block.result ? (
        <ToolPayloadDetails label="结果" value={block.result} />
      ) : null}
    </section>
  );
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
