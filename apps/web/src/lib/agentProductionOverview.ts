export interface AgentProductionCountsLike {
  shots_total: number;
  previews_ready: number;
  reviews_accepted: number;
  videos_ready: number;
  final_outputs: number;
  running_tasks: number;
  failed_tasks: number;
  waiting_decisions: number;
}

const phaseLabels: Record<string, string> = {
  planning: "规划中",
  preview: "生成预览",
  review: "质量复核",
  video: "生成视频",
  final: "合成成片",
  waiting_confirmation: "等待确认",
  complete: "已完成",
  needs_attention: "需要处理",
  error: "异常",
};

const statusLabels: Record<string, string> = {
  pending: "等待中",
  queued: "排队中",
  running: "执行中",
  waiting_for_user: "等待确认",
  succeeded: "完成",
  handled: "完成",
  accepted: "通过",
  rejected: "需重试",
  failed: "失败",
  cancelled: "已取消",
};

export function agentPhaseLabel(phase: string) {
  return phaseLabels[phase] ?? "处理中";
}

export function timelineStatusLabel(status: string) {
  return statusLabels[status] ?? status;
}

export function agentProductionProgressText(counts: AgentProductionCountsLike) {
  const parts = [`${counts.shots_total} 个分镜`];
  if (counts.previews_ready > 0) {
    parts.push(`${counts.previews_ready} 张预览`);
  }
  if (counts.reviews_accepted > 0) {
    parts.push(`${counts.reviews_accepted} 个通过评审`);
  }
  if (counts.videos_ready > 0) {
    parts.push(`${counts.videos_ready} 段视频`);
  }
  if (counts.final_outputs > 0) {
    parts.push(`${counts.final_outputs} 个成片`);
  }
  if (counts.running_tasks > 0) {
    parts.push(`${counts.running_tasks} 个任务运行中`);
  }
  if (counts.failed_tasks > 0) {
    parts.push(`${counts.failed_tasks} 个失败`);
  }
  if (counts.waiting_decisions > 0) {
    parts.push(`${counts.waiting_decisions} 个待确认`);
  }
  return parts.join(" · ");
}

export function shouldRefreshAgentProductionOverview(eventType: string) {
  return (
    eventType === "agent.task.updated" ||
    eventType === "agent.event.created" ||
    eventType === "agent.message.created" ||
    eventType === "agent.message.updated" ||
    eventType === "production.job.updated" ||
    eventType === "NodeCreated" ||
    eventType === "NodeUpdated"
  );
}

export function productionStatusLabel(status: string) {
  if (status === "none") {
    return "未开始";
  }
  if (status === "ready") {
    return "完成";
  }
  return timelineStatusLabel(status);
}

export function productionStatusTone(status: string) {
  if (status === "ready" || status === "succeeded" || status === "accepted") {
    return "ready";
  }
  if (status === "running" || status === "queued" || status === "pending") {
    return "active";
  }
  if (status === "failed" || status === "rejected") {
    return "failed";
  }
  return "muted";
}
