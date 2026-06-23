import type { AgentProductionOverview } from "../../lib/agentApi";
import {
  agentPhaseLabel,
  agentProductionProgressText,
  productionStatusTone,
} from "../../lib/agentProductionOverview";

export function AgentProductionStatusBar({
  isLoading,
  overview,
}: {
  isLoading: boolean;
  overview: AgentProductionOverview | null;
}) {
  if (isLoading && !overview) {
    return (
      <section className="agent-production-status-bar" aria-label="生产状态">
        <div>
          <span className="agent-production-pulse" />
          <strong>读取生产状态</strong>
        </div>
      </section>
    );
  }
  if (!overview) {
    return null;
  }
  return (
    <section className="agent-production-status-bar" aria-label="生产状态">
      <div>
        <span
          className={`agent-production-pulse agent-production-pulse-${productionStatusTone(
            overview.phase,
          )}`}
        />
        <strong>{agentPhaseLabel(overview.phase)}</strong>
      </div>
      <p>{agentProductionProgressText(overview.counts)}</p>
    </section>
  );
}
