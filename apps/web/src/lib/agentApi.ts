import { apiFetch, type MediaAsset, type MediaNode } from "./api";
import type {
  AgentWorkbenchAudioPlan,
  AgentWorkbenchLayoutPosition,
  AgentWorkbenchAudioSummary,
  AgentWorkbenchAudioTrack,
  AgentWorkbenchIssueSummary,
  AgentWorkbenchProjection,
  AgentWorkbenchReviewSummary,
} from "./agentWorkbench";
import type { AgentWorkbenchSelection } from "./agentWorkbenchSelection";

export type AgentJsonValue =
  | null
  | string
  | number
  | boolean
  | AgentJsonValue[]
  | { [key: string]: AgentJsonValue };

export interface AgentThread {
  id: string;
  workspace_id: string;
  role: "producer" | "craftsman" | "reviewer" | "composer";
  scope_type:
    | "workspace"
    | "shot"
    | "final_output"
    | "render_plan"
    | "key_element_state";
  scope_id?: string | null;
  runtime_provider: string;
  runtime_agent_name: string;
  current_checkpoint_key?: string | null;
  status: "active" | "paused" | "archived" | "failed";
  summary: string;
  created_at: string;
  updated_at: string;
}

export interface AgentMessage {
  id: string;
  workspace_id: string;
  thread_id: string;
  seq: number;
  role: "user" | "assistant" | "tool" | "system";
  message_type:
    | "text"
    | "tool_call"
    | "tool_result"
    | "ui_card"
    | "error"
    | "status";
  content: Record<string, unknown>;
  raw_message: Record<string, unknown>;
  task_id?: string | null;
  event_id?: string | null;
  created_at: string;
}

export interface AgentEvent {
  id: string;
  workspace_id: string;
  thread_id?: string | null;
  task_id?: string | null;
  event_type: string;
  source_role:
    | "user"
    | "producer"
    | "craftsman"
    | "reviewer"
    | "composer"
    | "worker"
    | "system";
  target_role?: string | null;
  scope: Record<string, unknown>;
  payload: Record<string, unknown>;
  status: "pending" | "handled" | "failed" | "cancelled";
  created_at: string;
  handled_at?: string | null;
}

export interface AgentTask {
  id: string;
  workspace_id: string;
  thread_id?: string | null;
  role:
    | "producer"
    | "craftsman"
    | "reviewer"
    | "composer"
    | "worker"
    | "system";
  scope_type: "workspace" | "shot" | "node" | "job" | "final_output";
  scope_id?: string | null;
  task_type:
    | "producer_turn"
    | "tool_call"
    | "decision_resume"
    | "craftsman_turn"
    | "reviewer_turn"
    | "worker_generation"
    | "composer_turn"
    | "dependency_scheduler";
  status:
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "cancelled"
    | "waiting_for_user";
  attempt: number;
  max_attempts: number;
  input: Record<string, unknown>;
  output: Record<string, unknown>;
  error_code?: string | null;
  error_message?: string | null;
  created_at: string;
  started_at?: string | null;
  completed_at?: string | null;
}

export interface AgentAttachment {
  asset_id: string;
  node_id: string;
  kind: "image" | "video" | "text";
  name: string;
  mime: string;
  size_bytes: number;
  url?: string;
  thumbnail_url?: string;
}

export interface AgentThreadResponse {
  thread: AgentThread;
}

export interface AgentMessagesResponse {
  thread: AgentThread;
  messages: AgentMessage[];
}

export interface AgentObservedThread extends AgentThread {
  display_name: string;
  scope_label: string;
  scope_title?: string;
  latest_task?: AgentTask;
  latest_message_at?: string;
  latest_message_preview?: string;
  metadata?: Record<string, unknown>;
}

export interface AgentThreadsResponse {
  threads: AgentObservedThread[];
}

export interface AgentTasksResponse {
  tasks: AgentTask[];
}

export interface PostAgentMessageResponse {
  message: AgentMessage;
  event: AgentEvent;
  task: AgentTask;
  decision_event?: AgentEvent;
  resolved_event?: AgentEvent;
}

export interface PostAgentAttachmentResponse {
  attachment: AgentAttachment;
  node: MediaNode;
  asset: MediaAsset;
}

export interface AgentModelRef {
  provider_id: string;
  model_id: string;
  reasoning_effort?: string;
}

export interface AgentModelOption extends AgentModelRef {
  display_name: string;
  limits: Record<string, unknown>;
  pricing: Record<string, unknown>;
  supports_thinking: boolean;
  reasoning_efforts: string[];
  default_reasoning_effort: string;
}

export interface AgentModelSelectionResponse {
  selection: {
    producer: AgentModelRef;
  };
  defaults: {
    producer: AgentModelRef;
  };
  options: AgentModelOption[];
}

export interface AgentCanvasDetail {
  object_type: AgentWorkbenchSelection["objectType"];
  object_id: string;
  title: string;
  status?: string;
  updated_at?: string;
  overview?: AgentCanvasOverviewDetail;
  key_element?: AgentCanvasKeyElementDetail;
  key_element_state?: AgentCanvasKeyElementStateDetail;
  scene?: AgentCanvasSceneDetail;
  shot?: AgentCanvasShotDetail;
  artifact?: AgentCanvasArtifactDetail;
  render_plan?: AgentCanvasRenderPlanDetail;
  review?: AgentCanvasReviewDetail;
  issue?: AgentCanvasIssueDetail;
  final_output?: AgentCanvasFinalOutputDetail;
}

export interface AgentCanvasOverviewDetail {
  workspace_id: string;
  brief?: AgentCanvasCreativeBriefDetail;
  memory?: AgentCanvasProjectMemoryDetail;
  audio_plan?: AgentWorkbenchAudioPlan | null;
  key_elements: AgentCanvasKeyElementSummary[];
  key_element_states: AgentCanvasKeyElementStateSummary[];
  source_materials: AgentCanvasSourceMaterialSummary[];
}

export interface AgentCanvasFinalOutputDetail {
  timeline_plan_id: string;
  output_node?: AgentCanvasMediaNodeDetail;
  output_version?: AgentCanvasArtifactVersion;
  production_job_id?: string;
  artifact_version_id?: string;
  sandbox_job_id?: string;
  status: string;
  template_key: string;
  audio_summary?: AgentWorkbenchAudioSummary | null;
  audio_tracks?: AgentWorkbenchAudioTrack[];
  final_reviews: AgentWorkbenchReviewSummary[];
  issues: AgentWorkbenchIssueSummary[];
  plan?: AgentJsonValue;
  result?: AgentJsonValue;
  error_message?: string;
  created_at?: string;
  updated_at?: string;
}

export interface AgentCanvasCreativeBriefDetail {
  id: string;
  title: string;
  video_type: string;
  target_audience: string;
  tone: string;
  visual_style: string;
  duration_sec?: number | null;
  aspect_ratio: string;
  language: string;
  objective: string;
  concept: string;
  constraints?: AgentJsonValue;
  metadata?: AgentJsonValue;
  status: string;
  updated_at?: string;
}

export interface AgentCanvasProjectMemoryDetail {
  id: string;
  version: number;
  status: string;
  core_intent: string;
  soul: string;
  brand_facts?: AgentJsonValue;
  non_negotiables?: AgentJsonValue;
  visual_anchors?: AgentJsonValue;
  allowed?: AgentJsonValue;
  forbidden?: AgentJsonValue;
  prompt_injection_hints?: AgentJsonValue;
  source_refs?: AgentJsonValue;
}

export interface AgentCanvasSourceMaterialSummary {
  id: string;
  title: string;
  node_type: string;
  status: string;
}

export interface AgentCanvasKeyElementSummary {
  id: string;
  client_key: string;
  name: string;
  type: string;
  description?: string;
  source_type?: string;
  status: string;
}

export interface AgentCanvasKeyElementStateSummary {
  id: string;
  key_element_id: string;
  client_key: string;
  label: string;
  visual_description?: string;
  reference_status: string;
  reference_node_id?: string;
  reference_version_id?: string;
  status: string;
  is_default: boolean;
}

export interface AgentCanvasKeyElementDetail
  extends AgentCanvasKeyElementSummary {
  source_refs?: AgentJsonValue;
  states: AgentCanvasKeyElementStateSummary[];
  shot_refs: AgentCanvasShotKeyElementRefDetail[];
}

export interface AgentCanvasKeyElementStateDetail
  extends AgentCanvasKeyElementStateSummary {
  state_facts?: AgentJsonValue;
  source_refs?: AgentJsonValue;
  key_element?: AgentCanvasKeyElementSummary;
  reference_node?: AgentCanvasMediaNodeDetail;
  reference_version?: AgentCanvasArtifactVersion;
  dependent_shots: AgentCanvasShotSummary[];
  missing_reason?: string;
}

export interface AgentCanvasSceneDetail {
  id: string;
  client_key: string;
  sort_order: number;
  title: string;
  description: string;
  location: string;
  mood: string;
  status: string;
  shots: AgentCanvasShotSummary[];
  updated_at?: string;
}

export interface AgentCanvasShotSummary {
  id: string;
  client_key: string;
  title: string;
  status: string;
  sequence_index: number;
}

export interface AgentCanvasShotDetail {
  id: string;
  client_key: string;
  title: string;
  status: string;
  sequence_index: number;
  scene_id?: string;
  shot_kind?: string;
  duration_sec?: number | null;
  narrative_purpose?: string;
  brief?: AgentJsonValue;
  creative_text?: string;
  visual_intent?: string;
  action_text?: string;
  camera_intent?: string;
  dialogue?: string;
  narration?: string;
  audio_plan?: AgentJsonValue;
  dependencies: AgentCanvasShotDependencyDetail[];
  key_elements: AgentCanvasShotKeyElementRefDetail[];
  artifacts: AgentCanvasArtifactSlot[];
  render_plans: AgentCanvasRenderPlanSummary[];
  reviews: AgentCanvasReviewSummary[];
  issues: AgentCanvasIssueSummary[];
  updated_at?: string;
}

export interface AgentCanvasShotDependencyDetail {
  id: string;
  from_shot_id: string;
  to_shot_id: string;
  dependency_type: string;
  required_artifact?: string;
  injection_role?: string;
  blocking_phase?: string;
  stale_policy?: string;
  reason?: string;
}

export interface AgentCanvasShotKeyElementRefDetail {
  id: string;
  shot_id: string;
  shot_title?: string;
  key_element_id: string;
  key_element_name?: string;
  key_element_state_id?: string;
  state_label?: string;
  role: string;
  required: boolean;
  sort_order: number;
}

export interface AgentCanvasArtifactSlot {
  kind: string;
  status: string;
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

export interface AgentCanvasRenderPlanSummary {
  id: string;
  revision: number;
  target_phase: string;
  operation: string;
  status: string;
}

export interface AgentCanvasReviewSummary {
  id: string;
  review_task: string;
  target_phase: string;
  status: string;
  verdict: string;
  score?: number;
}

export interface AgentCanvasIssueSummary {
  id: string;
  title: string;
  severity: string;
  dimension: string;
  suggested_fix: string;
}

export interface AgentCanvasArtifactDetail {
  node: AgentCanvasMediaNodeDetail;
  asset?: AgentCanvasAssetRead;
  current_version?: AgentCanvasArtifactVersion;
  versions: AgentCanvasArtifactVersion[];
  generation_jobs: AgentCanvasGenerationJob[];
  render_plans: AgentCanvasRenderPlanSummary[];
  reviews: AgentCanvasReviewRecord[];
  issues: AgentCanvasIssueSummary[];
}

export interface AgentCanvasMediaNodeDetail {
  id: string;
  workspace_id: string;
  node_type: string;
  title: string;
  status: string;
  prompt?: string;
  source?: string;
  operation_type?: string;
  shot_id?: string;
  asset_id?: string;
  model_provider?: string;
  model_id?: string;
  model_params?: AgentJsonValue;
  current_version_id?: string;
  metadata?: AgentJsonValue;
  updated_at?: string;
}

export interface AgentCanvasAssetRead {
  id: string;
  type: string;
  mime: string;
  storage_url?: string;
  access_url?: string;
  text_content?: string;
  size_bytes?: number;
  metadata: Record<string, AgentJsonValue>;
}

export interface AgentCanvasArtifactVersion {
  id: string;
  workspace_id: string;
  node_id: string;
  job_id?: string;
  asset_id?: string;
  version_no: number;
  winner: boolean;
  output: Record<string, AgentJsonValue>;
  review_score?: number;
  input_hash: string;
  status: string;
  progress: number;
  error_code?: string;
  error_message?: string;
  provider_request: Record<string, AgentJsonValue>;
  provider_response: Record<string, AgentJsonValue>;
  asset?: AgentCanvasAssetRead;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface AgentCanvasGenerationJob {
  id: string;
  workspace_id: string;
  target_node_id: string;
  parent_job_id?: string;
  operation_type: string;
  provider: string;
  model_id: string;
  intent: Record<string, AgentJsonValue>;
  rendered_prompt: string;
  provider_request: Record<string, AgentJsonValue>;
  provider_response: Record<string, AgentJsonValue>;
  status: string;
  progress: number;
  attempt: number;
  max_attempts: number;
  error_code?: string;
  error_message?: string;
  requested_by_type: string;
  requested_by_id?: string;
  created_at: string;
}

export interface AgentCanvasRenderPlanDetail {
  id: string;
  scope_type: string;
  scope_id: string;
  target_phase: string;
  task_type: string;
  model_prompt_profile: string;
  operation: string;
  status: string;
  revision: number;
  render_plan_key?: string;
  reference_bindings?: AgentJsonValue;
  subject_bindings?: AgentJsonValue;
  prompt_parts?: AgentJsonValue;
  params?: AgentJsonValue;
  audit_hints?: AgentJsonValue;
  blocker?: AgentJsonValue;
  compiled_prompt?: string;
  compiled_request?: AgentJsonValue;
  prompt_audit?: AgentJsonValue;
  cost_estimate?: AgentJsonValue;
  rationale?: string;
  submitted_worker_task_id?: string;
  output_node?: AgentCanvasMediaNodeDetail;
  output_version?: AgentCanvasArtifactVersion;
  reviews: AgentCanvasReviewRecord[];
  issues: AgentCanvasIssueSummary[];
  created_at?: string;
  updated_at?: string;
  compiled_at?: string;
  submitted_at?: string;
  completed_at?: string;
}

export interface AgentCanvasReviewRecord {
  id: string;
  shot_id?: string;
  node_id: string;
  artifact_version_id: string;
  generation_job_id?: string;
  target_phase: string;
  status: string;
  attempt_no: number;
  max_attempts: number;
  overall_score?: number;
  rubric: Record<string, AgentJsonValue>;
  critique: string;
  retry_recommendation: Record<string, AgentJsonValue>;
  model_provider?: string;
  model_id?: string;
  error_code?: string;
  error_message?: string;
  created_at: string;
  completed_at?: string;
}

export interface AgentCanvasReviewDetail {
  review: AgentCanvasReviewRecord;
  issues: AgentCanvasIssueSummary[];
}

export interface AgentCanvasIssueDetail {
  id: string;
  review_record_id?: string;
  dimension: string;
  severity: string;
  status: string;
  target_object_type: string;
  target_object_id: string;
  title: string;
  description: string;
  evidence?: string;
  suggested_fix?: string;
  fix_hint?: string;
  requires_user_confirmation: boolean;
  review?: AgentCanvasReviewRecord;
  created_at?: string;
  updated_at?: string;
}

export interface PostAgentDecisionResponse {
  message: AgentMessage;
  decision_event: AgentEvent;
  resolved_event: AgentEvent;
  task: AgentTask;
}

export type AgentProductionPhase =
  | "planning"
  | "preview"
  | "review"
  | "video"
  | "final"
  | "waiting_confirmation"
  | "complete"
  | "needs_attention"
  | "error";

export type AgentProductionStatus =
  | "none"
  | "queued"
  | "running"
  | "ready"
  | "failed";

export interface AgentProductionCounts {
  shots_total: number;
  previews_ready: number;
  reviews_accepted: number;
  videos_ready: number;
  final_outputs: number;
  running_tasks: number;
  failed_tasks: number;
  waiting_decisions: number;
  audio_ready: number;
  audio_missing: number;
  final_reviews: number;
}

export interface AgentProductionAudioPlan {
  id: string;
  status: string;
  title: string;
  plan_kind?: string;
  language?: string;
  target_duration_sec?: number;
  voiceover_script?: string;
  voice_profile?: Record<string, unknown>;
  bgm_plan?: Record<string, unknown>;
  voiceover_node_id?: string;
  voiceover_status: AgentProductionStatus;
  bgm_node_id?: string;
  bgm_status: AgentProductionStatus;
  timeline_plan_id?: string;
  voiceover_render_plan_id?: string;
  bgm_render_plan_id?: string;
}

export interface AgentProductionShot {
  id: string;
  client_key: string;
  sort_order: number;
  title: string;
  duration_sec?: number;
  status: string;
  preview_status: AgentProductionStatus;
  review_status: AgentProductionStatus;
  video_status: AgentProductionStatus;
  preview_node_id?: string;
  video_node_id?: string;
  review_score?: number;
}

export interface AgentProductionTimelineItem {
  id: string;
  type: string;
  label: string;
  status: string;
  role?: string;
  scope?: Record<string, unknown>;
  diagnostics?: Record<string, unknown>;
  created_at?: string;
  completed_at?: string;
}

export interface AgentProductionFinalOutput {
  node_id: string;
  version_id?: string;
  asset_id?: string;
  title: string;
  status: AgentProductionStatus;
  operation: string;
  completed_at?: string;
}

export interface AgentProductionOverview {
  workspace_id: string;
  phase: AgentProductionPhase | string;
  counts: AgentProductionCounts;
  audio_plan?: AgentProductionAudioPlan | null;
  shots: AgentProductionShot[];
  timeline: AgentProductionTimelineItem[];
  final_outputs: AgentProductionFinalOutput[];
  diagnostics?: Record<string, unknown>;
  updated_at: string;
}

export function fetchAgentThread(workspaceId: string) {
  return apiFetch<AgentThreadResponse>(
    `/agent/workspaces/${workspaceId}/thread`,
  );
}

export function fetchAgentMessages(
  workspaceId: string,
  afterCreatedAt = "",
  limit = 1000,
) {
  const params = new URLSearchParams({
    limit: String(limit),
  });
  if (afterCreatedAt) {
    params.set("after_created_at", afterCreatedAt);
  }
  return apiFetch<AgentMessagesResponse>(
    `/agent/workspaces/${workspaceId}/messages?${params.toString()}`,
  );
}

export function fetchAgentThreads(workspaceId: string) {
  return apiFetch<AgentThreadsResponse>(
    `/agent/workspaces/${workspaceId}/threads`,
  );
}

export function fetchAgentThreadMessages(
  workspaceId: string,
  threadId: string,
  afterSeq = 0,
  limit = 1000,
) {
  const params = new URLSearchParams({
    limit: String(limit),
  });
  if (afterSeq > 0) {
    params.set("after_seq", String(afterSeq));
  }
  return apiFetch<AgentMessagesResponse>(
    `/agent/workspaces/${workspaceId}/threads/${threadId}/messages?${params.toString()}`,
  );
}

export function fetchAgentTasks(workspaceId: string) {
  return apiFetch<AgentTasksResponse>(
    `/agent/workspaces/${workspaceId}/tasks`,
  );
}

export function fetchAgentModelSelection(workspaceId: string) {
  return apiFetch<AgentModelSelectionResponse>(
    `/agent/workspaces/${workspaceId}/model-selection`,
  );
}

export function fetchAgentProductionOverview(workspaceId: string) {
  return apiFetch<AgentProductionOverview>(
    `/agent/workspaces/${workspaceId}/production-overview`,
  );
}

export function fetchAgentCanvasWorkbench(workspaceId: string) {
  return apiFetch<AgentWorkbenchProjection>(
    `/agent/workspaces/${workspaceId}/canvas/workbench`,
  );
}

export function fetchAgentCanvasDetail(
  workspaceId: string,
  selection: AgentWorkbenchSelection,
) {
  const params = new URLSearchParams({
    object_type: selection.objectType,
    object_id: selection.objectId,
  });
  return apiFetch<AgentCanvasDetail>(
    `/agent/workspaces/${workspaceId}/canvas/details?${params.toString()}`,
  );
}

export function putAgentCanvasLayout(
  workspaceId: string,
  input: { positions: AgentWorkbenchLayoutPosition[] },
) {
  return apiFetch<void>(`/agent/workspaces/${workspaceId}/canvas/layout`, {
    method: "PUT",
    body: JSON.stringify(input),
  });
}

export function putAgentModelSelection(
  workspaceId: string,
  input: { producer: AgentModelRef },
) {
  return apiFetch<AgentModelSelectionResponse>(
    `/agent/workspaces/${workspaceId}/model-selection`,
    {
      method: "PUT",
      body: JSON.stringify(input),
    },
  );
}

export function postAgentMessage(
  workspaceId: string,
  input: {
    text: string;
    client_message_id?: string;
    attachments?: AgentAttachment[];
  },
) {
  return apiFetch<PostAgentMessageResponse>(
    `/agent/workspaces/${workspaceId}/messages`,
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  );
}

export function postAgentDecision(
  workspaceId: string,
  eventId: string,
  input: {
    selected_option_id?: string;
    free_text?: string;
    client_response_id?: string;
  },
) {
  return apiFetch<PostAgentDecisionResponse>(
    `/agent/workspaces/${workspaceId}/decisions/${eventId}/respond`,
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  );
}

export function uploadAgentAttachment(workspaceId: string, file: File) {
  const form = new FormData();
  form.append("file", file);
  return apiFetch<PostAgentAttachmentResponse>(
    `/agent/workspaces/${workspaceId}/attachments`,
    {
      method: "POST",
      body: form,
    },
  );
}
