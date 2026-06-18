import type { Workspace, WorkspaceMode } from "./api";

export function workspaceRoute(workspace: Pick<Workspace, "id" | "mode">) {
  return workspaceModeRoute(workspace.id, workspace.mode);
}

export function workspaceModeRoute(id: string, mode: WorkspaceMode) {
  return `/workspaces/${id}/${mode}`;
}
