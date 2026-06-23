import type {
  AgentProductionOverview,
  AgentProductionTimelineItem,
} from "../../lib/agentApi";
import {
  productionStatusTone,
  timelineStatusLabel,
} from "../../lib/agentProductionOverview";

export function AgentTaskTimeline({
  overview,
}: {
  overview: AgentProductionOverview | null;
}) {
  if (!overview) {
    return null;
  }
  return (
    <section className="agent-production-panel" aria-label="任务时间线">
      <header>
        <strong>Timeline</strong>
        <span>{overview.timeline.length} 条</span>
      </header>
      {overview.timeline.length > 0 ? (
        <div className="agent-task-timeline">
          {overview.timeline.slice(0, 8).map((item) => (
            <TimelineRow item={item} key={`${item.type}:${item.id}`} />
          ))}
        </div>
      ) : (
        <p className="agent-production-empty">还没有任务记录。</p>
      )}
    </section>
  );
}

function TimelineRow({ item }: { item: AgentProductionTimelineItem }) {
  return (
    <article className="agent-task-timeline-row">
      <span
        className={`agent-task-timeline-dot agent-task-timeline-dot-${productionStatusTone(
          item.status,
        )}`}
      />
      <div>
        <strong>{item.label}</strong>
        <small>
          {timelineStatusLabel(item.status)}
          {item.role ? ` · ${item.role}` : ""}
        </small>
        {item.diagnostics && Object.keys(item.diagnostics).length > 0 ? (
          <details>
            <summary>诊断信息</summary>
            <pre>{JSON.stringify(item.diagnostics, null, 2)}</pre>
          </details>
        ) : null}
      </div>
    </article>
  );
}
