export interface AgentWorkbenchProjection {
  overview: AgentWorkbenchOverview;
  scenes: AgentWorkbenchScene[];
  counts: AgentWorkbenchCounts;
  final_output?: AgentWorkbenchFinalOutput | null;
}

export interface AgentWorkbenchOverview {
  workspace_id: string;
  brief?: AgentWorkbenchBrief;
  memory?: AgentWorkbenchMemory;
  audio_plan?: AgentWorkbenchAudioPlan | null;
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

export interface AgentWorkbenchAudioPlan {
  id: string;
  status: string;
  title: string;
  plan_kind?: string;
  language?: string;
  target_duration_sec?: number;
  voiceover_script?: string;
  voice_profile?: Record<string, unknown>;
  bgm_plan?: Record<string, unknown>;
  cue_plan?: unknown;
  voiceover_node_id?: string;
  voiceover_status?: string;
  voiceover_artifact?: AgentWorkbenchArtifactSlot;
  bgm_node_id?: string;
  bgm_status?: string;
  bgm_artifact?: AgentWorkbenchArtifactSlot;
  timeline_plan_id?: string;
  voiceover_render_plan_id?: string;
  bgm_render_plan_id?: string;
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
  width?: number;
  height?: number;
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

export interface AgentWorkbenchFinalOutput {
  id: string;
  timeline_plan_id: string;
  output_node_id?: string;
  artifact_version_id?: string;
  sandbox_job_id?: string;
  status: string;
  template_key: string;
  summary?: string;
  asset_url?: string;
  thumbnail_url?: string;
  asset_id?: string;
  mime?: string;
  audio_summary?: AgentWorkbenchAudioSummary | null;
  audio_tracks?: AgentWorkbenchAudioTrack[];
  final_review?: AgentWorkbenchReviewSummary | null;
  plan?: Record<string, unknown>;
  result?: Record<string, unknown>;
  updated_at?: string;
}

export interface AgentWorkbenchAudioSummary {
  has_voiceover: boolean;
  has_bgm: boolean;
  audio_codec?: string;
  track_count: number;
  ducking: boolean;
}

export interface AgentWorkbenchAudioTrack {
  role: string;
  asset_id?: string;
  workspace_path?: string;
  start_sec?: number;
  duration_sec?: number;
  volume?: number;
  ducking?: boolean;
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
  audio_ready: number;
  audio_missing: number;
  final_reviews: number;
}

export function agentWorkbenchVisibleNodeCount(
  workbench: AgentWorkbenchProjection,
) {
  return (
    1 +
    workbench.scenes.length +
    workbench.scenes.reduce((sum, scene) => sum + scene.shots.length, 0) +
    (workbench.final_output ? 1 : 0)
  );
}
