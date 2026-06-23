import type { AgentProductionOverview, AgentProductionShot } from "../../lib/agentApi";
import {
  productionStatusLabel,
  productionStatusTone,
} from "../../lib/agentProductionOverview";

export function AgentStoryboardPanel({
  onSelectNode,
  overview,
}: {
  onSelectNode: (nodeId: string) => void;
  overview: AgentProductionOverview | null;
}) {
  if (!overview) {
    return null;
  }
  return (
    <section className="agent-production-panel" aria-label="分镜进度">
      <header>
        <strong>Storyboard</strong>
        <span>{overview.shots.length} 个分镜</span>
      </header>
      {overview.shots.length > 0 ? (
        <div className="agent-storyboard-list">
          {overview.shots.map((shot) => (
            <StoryboardRow key={shot.id} onSelectNode={onSelectNode} shot={shot} />
          ))}
        </div>
      ) : (
        <p className="agent-production-empty">还没有分镜。</p>
      )}
    </section>
  );
}

function StoryboardRow({
  onSelectNode,
  shot,
}: {
  onSelectNode: (nodeId: string) => void;
  shot: AgentProductionShot;
}) {
  const nodeId = shot.video_node_id || shot.preview_node_id || "";
  return (
    <article className="agent-storyboard-row">
      <button
        disabled={!nodeId}
        onClick={() => nodeId && onSelectNode(nodeId)}
        type="button"
      >
        <span>{shot.sort_order}</span>
        <strong>{shot.title || shot.client_key}</strong>
      </button>
      <div>
        <StatusChip label="预览" status={shot.preview_status} />
        <StatusChip label="评审" status={shot.review_status} />
        <StatusChip label="视频" status={shot.video_status} />
      </div>
    </article>
  );
}

function StatusChip({ label, status }: { label: string; status: string }) {
  return (
    <span
      className={`agent-production-chip agent-production-chip-${productionStatusTone(
        status,
      )}`}
    >
      {label} {productionStatusLabel(status)}
    </span>
  );
}
