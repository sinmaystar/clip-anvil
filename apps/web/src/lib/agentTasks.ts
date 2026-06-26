import type { AgentTask } from "./agentApi";

type AgentTaskState = Pick<AgentTask, "id" | "status" | "task_type">;

export function mergeAgentTasks<T extends AgentTaskState>(
  current: T[],
  incoming: T[],
) {
  const byId = new Map<string, T>();
  for (const task of current) {
    byId.set(task.id, task);
  }
  for (const task of incoming) {
    byId.set(task.id, task);
  }
  return [...byId.values()];
}

export function mergeActiveAgentTaskSnapshot<T extends AgentTaskState>(
  current: T[],
  activeTasks: T[],
) {
  const activeIds = new Set(activeTasks.map((task) => task.id));
  const retained = current.filter(
    (task) =>
      !isActiveAgentTask(task) ||
      activeIds.has(task.id),
  );
  return mergeAgentTasks(retained, activeTasks);
}

export function hasRunningProducerTask(tasks: AgentTaskState[]) {
  return tasks.some(
    (task) =>
      (task.task_type === "producer_turn" ||
        task.task_type === "decision_resume") &&
      (task.status === "queued" || task.status === "running"),
  );
}

export function hasActiveAgentTask(tasks: AgentTaskState[]) {
  return tasks.some((task) => isActiveAgentTask(task));
}

export function hasProcessingAgentTask(tasks: AgentTaskState[]) {
  return tasks.some(
    (task) => task.status === "queued" || task.status === "running",
  );
}

export function agentComposerDisabledReason(tasks: AgentTaskState[]) {
  if (
    tasks.some(
      (task) =>
        task.task_type === "producer_turn" &&
        (task.status === "queued" || task.status === "running"),
    )
  ) {
    return "ClipAnvil 正在思考";
  }
  if (
    tasks.some(
      (task) =>
        task.task_type === "decision_resume" &&
        (task.status === "queued" || task.status === "running"),
    )
  ) {
    return "ClipAnvil 正在处理你的选择";
  }
  if (
    tasks.some((task) => task.status === "queued" || task.status === "running")
  ) {
    return "ClipAnvil 正在制作";
  }
  return "";
}

function isActiveAgentTask(task: AgentTaskState) {
  return (
    task.status === "queued" ||
    task.status === "running" ||
    task.status === "waiting_for_user"
  );
}
