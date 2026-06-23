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

export function hasRunningProducerTask(tasks: AgentTaskState[]) {
  return tasks.some(
    (task) =>
      (task.task_type === "producer_turn" ||
        task.task_type === "decision_resume") &&
      (task.status === "queued" || task.status === "running"),
  );
}

export function hasActiveAgentTask(tasks: AgentTaskState[]) {
	return tasks.some(
		(task) =>
			task.status === "queued" ||
			task.status === "running" ||
			task.status === "waiting_for_user",
	);
}

export function hasProcessingAgentTask(tasks: AgentTaskState[]) {
	return tasks.some(
		(task) => task.status === "queued" || task.status === "running",
	);
}

export function agentComposerDisabledReason(tasks: AgentTaskState[]) {
	void tasks;
	return "";
}
