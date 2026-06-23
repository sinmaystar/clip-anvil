import type { AgentReviewCardBlock as AgentReviewCardBlockData } from "../../lib/agentMessageBlocks";

export function AgentReviewCardBlock({
  block,
}: {
  block: AgentReviewCardBlockData;
}) {
  const statusLabel =
    block.status === "accepted"
      ? "通过"
      : block.status === "rejected"
        ? "需要重做"
        : block.status === "failed"
          ? "评审失败"
          : "评审中";
  const score =
    typeof block.overall_score === "number"
      ? `${Math.round(block.overall_score * 100)}`
      : "-";
  return (
    <section className={`agent-review-card agent-review-card-${block.status}`}>
      <header>
        <div>
          <strong>{block.shot_ref || "分镜评审"}</strong>
          <span>{statusLabel}</span>
        </div>
        <b>{score}</b>
      </header>
      {block.critique ? <p>{block.critique}</p> : null}
      <small>
        {phaseLabel(block.target_phase)} · 第 {block.retry_count}/
        {block.max_attempts} 次
      </small>
      {block.fix_hints?.length ? (
        <details>
          <summary>修改建议</summary>
          <ul>
            {block.fix_hints.map((hint) => (
              <li key={hint}>{hint}</li>
            ))}
          </ul>
        </details>
      ) : null}
      <details>
        <summary>评审细则</summary>
        <pre>{JSON.stringify(block.rubric, null, 2)}</pre>
      </details>
    </section>
  );
}

function phaseLabel(phase: string) {
  if (phase === "preview_image") {
    return "预览图";
  }
  if (phase === "shot_video") {
    return "分镜视频";
  }
  if (phase === "final_video") {
    return "成片";
  }
  return phase;
}
