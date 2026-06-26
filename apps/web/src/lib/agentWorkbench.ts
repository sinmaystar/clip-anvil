export interface AgentWorkbenchProjection {
  overview: AgentWorkbenchOverview;
  scenes: AgentWorkbenchScene[];
  counts: AgentWorkbenchCounts;
}

export interface AgentWorkbenchOverview {
  workspace_id: string;
  brief?: AgentWorkbenchBrief;
  memory?: AgentWorkbenchMemory;
  key_elements: AgentWorkbenchKeyElement[];
  key_element_states: AgentWorkbenchKeyElementState[];
  source_materials: AgentWorkbenchSourceMaterial[];
}

export interface AgentWorkbenchBrief {
  id: string;
  title: string;
  concept: string;
  status: string;
}

export interface AgentWorkbenchMemory {
  id: string;
  version: number;
  soul: string;
  status: string;
}

export interface AgentWorkbenchKeyElement {
  id: string;
  client_key: string;
  name: string;
  type: string;
  status: string;
}

export interface AgentWorkbenchKeyElementState {
  id: string;
  key_element_id: string;
  client_key: string;
  label: string;
  reference_status: string;
}

export interface AgentWorkbenchSourceMaterial {
  id: string;
  title: string;
  node_type: string;
  status: string;
}

export interface AgentWorkbenchScene {
  id: string;
  title: string;
  status: string;
  summary?: string;
  location?: string;
  shots: AgentWorkbenchShot[];
}

export interface AgentWorkbenchShot {
  id: string;
  client_key: string;
  title: string;
  status: string;
  sequence_index: number;
  creative_text: string;
  dependencies: AgentWorkbenchShotDependency[];
  key_elements: AgentWorkbenchShotKeyElementRef[];
  preview: AgentWorkbenchArtifactSlot;
  video: AgentWorkbenchArtifactSlot;
  artifacts?: AgentWorkbenchArtifactSlot[];
  render_plans: AgentWorkbenchRenderPlanSummary[];
  review?: AgentWorkbenchReviewSummary;
  issues: AgentWorkbenchIssueSummary[];
}

export interface AgentWorkbenchShotDependency {
  id: string;
  from_shot_id: string;
  to_shot_id: string;
  dependency_type: string;
}

export interface AgentWorkbenchShotKeyElementRef {
  id: string;
  key_element_id: string;
  key_element_state_id?: string;
  role: string;
}

export interface AgentWorkbenchArtifactSlot {
  kind: "preview_image" | "shot_video" | string;
  status:
    | "missing"
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "stale"
    | string;
  node_id?: string;
  title?: string;
  version_id?: string;
  thumbnail_url?: string;
  access_url?: string;
  error_code?: string;
  error_message?: string;
}

export interface AgentWorkbenchRenderPlanSummary {
  id: string;
  revision: number;
  target_phase: string;
  operation: string;
  status: string;
}

export interface AgentWorkbenchReviewSummary {
  id: string;
  review_task: string;
  target_phase: string;
  status: string;
  verdict: string;
  score?: number;
}

export interface AgentWorkbenchIssueSummary {
  id: string;
  title: string;
  severity: string;
  dimension: string;
  suggested_fix: string;
}

export interface AgentWorkbenchCounts {
  scenes: number;
  shots: number;
  preview_succeeded: number;
  preview_failed: number;
  video_succeeded: number;
  video_failed: number;
  open_issues: number;
  needs_reference: number;
}

export function agentWorkbenchVisibleNodeCount(
  workbench: AgentWorkbenchProjection,
) {
  return (
    1 +
    workbench.scenes.length +
    workbench.scenes.reduce((sum, scene) => sum + scene.shots.length, 0)
  );
}
