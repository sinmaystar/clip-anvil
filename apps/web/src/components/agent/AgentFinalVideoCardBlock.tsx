import type { AgentFinalVideoCardBlock as AgentFinalVideoCardBlockData } from "../../lib/agentMessageBlocks";

export function AgentFinalVideoCardBlock({
  block,
}: {
  block: AgentFinalVideoCardBlockData;
}) {
  return (
    <section className={`agent-final-video-card agent-final-video-card-${block.status}`}>
      <header>
        <div>
          <strong>{block.title || "成片"}</strong>
          <span>{statusLabel(block.status)}</span>
        </div>
        {block.decision_id ? <small>等待确认</small> : null}
      </header>
      {block.url ? (
        <video
          className="agent-final-video-card-video"
          controls
          poster={block.thumbnail_url}
          preload="metadata"
          src={block.url}
        />
      ) : (
        <div className="agent-final-video-card-placeholder">
          {block.status === "failed" ? "成片生成失败" : "成片正在生成"}
        </div>
      )}
      <div className="agent-final-video-card-meta">
        <span>版本 {shortID(block.version_id)}</span>
        <span>节点 {shortID(block.node_id)}</span>
      </div>
      {block.source_shots.length > 0 ? (
        <div className="agent-final-video-card-shots">
          {block.source_shots.map((shot) => (
            <span key={shot}>{shot}</span>
          ))}
        </div>
      ) : null}
    </section>
  );
}

function statusLabel(status: AgentFinalVideoCardBlockData["status"]) {
  switch (status) {
    case "ready":
      return "已生成";
    case "waiting_for_confirmation":
      return "待确认";
    case "failed":
      return "失败";
    case "running":
      return "合成中";
    case "queued":
      return "排队中";
  }
}

function shortID(value: string) {
  return value.length > 8 ? value.slice(0, 8) : value;
}
