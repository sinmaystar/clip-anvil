import { apiFetch, type MediaAsset, type MediaNode } from "./api";

export interface AgentThread {
  id: string;
  workspace_id: string;
  role: "producer" | "craftsman" | "reviewer" | "composer";
  scope_type: "workspace" | "shot" | "final_output";
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
  task_type: "producer_turn" | "tool_call" | "decision_resume";
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
  afterSeq = 0,
  limit = 1000,
) {
  const params = new URLSearchParams({
    after_seq: String(afterSeq),
    limit: String(limit),
  });
  return apiFetch<AgentMessagesResponse>(
    `/agent/workspaces/${workspaceId}/messages?${params.toString()}`,
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
