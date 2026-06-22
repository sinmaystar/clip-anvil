import type { AgentErrorBlock as AgentErrorBlockData } from "../../lib/agentMessageBlocks";

export function AgentErrorBlock({ block }: { block: AgentErrorBlockData }) {
  return (
    <section className="agent-error-block">
      <strong>{block.title}</strong>
      <p>{block.message}</p>
      {block.code ? <small>{block.code}</small> : null}
    </section>
  );
}
