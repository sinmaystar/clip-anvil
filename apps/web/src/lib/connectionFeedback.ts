export type ConnectionFeedbackTone = "danger" | "info" | "warning";

export interface ConnectionFeedback {
  description: string;
  title: string;
  tone: ConnectionFeedbackTone;
}

export function connectionFailureFeedback(
  status: number | null | undefined,
): ConnectionFeedback {
  if (status === 422) {
    return {
      title: "这条线会形成循环",
      description:
        "Node 不能依赖自己的下游节点。请改为从上游节点连到下游节点。",
      tone: "danger",
    };
  }

  return {
    title: "连线失败",
    description: "请稍后再试，或者换一个目标节点。",
    tone: "warning",
  };
}
