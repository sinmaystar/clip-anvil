export type AgentWorkbenchObjectType =
  | "overview"
  | "key_element"
  | "key_element_state"
  | "scene"
  | "shot"
  | "artifact"
  | "render_plan"
  | "review"
  | "issue"
  | "final_output";

export interface AgentWorkbenchSelection {
  objectType: AgentWorkbenchObjectType;
  objectId: string;
  label?: string;
}

export function agentWorkbenchSelectionKey(
  selection: AgentWorkbenchSelection | null,
) {
  if (!selection) {
    return "";
  }
  return `${selection.objectType}:${selection.objectId}`;
}

export function agentWorkbenchSelectionEquals(
  selection: AgentWorkbenchSelection | null,
  objectType: AgentWorkbenchObjectType,
  objectId: string | undefined | null,
) {
  return (
    Boolean(objectId) &&
    selection?.objectType === objectType &&
    selection.objectId === objectId
  );
}

export function agentWorkbenchOverviewSelection(workspaceId: string) {
  return {
    objectType: "overview",
    objectId: workspaceId,
    label: "Project Overview",
  } satisfies AgentWorkbenchSelection;
}
