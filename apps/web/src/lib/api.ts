import { useAuthStore } from "../stores/auth";
import { apiErrorMessage } from "./apiErrors";

export interface Account {
  id: string;
  email: string;
  name: string;
  avatar_url?: string;
}

export interface AuthResponse {
  token: string;
  account: Account;
}

export type WorkspaceMode = "studio" | "agent";

export interface Workspace {
  id: string;
  name: string;
  mode: WorkspaceMode;
  owner_id: string;
  settings?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export type MediaType = "text" | "image" | "video" | "audio" | "reference_pack";

export type AssetType = "text" | "image" | "video" | "audio" | "json";

export type OperationType =
  | "manual"
  | "upload"
  | "collect_references"
  | "text_generation"
  | "text_to_image"
  | "image_to_image"
  | "multi_image_to_image"
  | "text_to_video"
  | "image_to_video"
  | "video_to_video"
  | "multi_reference_to_video"
  | "extract_first_frame"
  | "extract_last_frame";

export type NodeStatus =
  | "draft"
  | "ready"
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "stale"
  | "user_editing";

export interface MediaNode {
  id: string;
  workspace_id: string;
  node_type: MediaType;
  title: string;
  prompt: string;
  asset_id?: string | null;
  asset_url?: string;
  thumbnail_url?: string;
  group_id?: string | null;
  status: NodeStatus;
  canvas_x: number;
  canvas_y: number;
  canvas_w: number;
  canvas_h: number;
  source?: "user" | "agent" | string;
  operation_type?: OperationType | string;
  prompt_template?: string;
  prompt_rich?: PromptRichDocument;
  prompt_refs?: PromptRefsDocument;
  model_provider?: string | null;
  model_id?: string | null;
  model_params?: unknown;
  current_version_id?: string | null;
  production_preview?: ProductionPreview;
  reference_pack_preview?: ReferencePackPreview;
  active_stale_reason_count?: number;
  metadata?: unknown;
  created_at: string;
  updated_at: string;
}

export interface PromptRef {
  node_id: string;
  label: string;
  node_type: MediaType | string;
}

export interface PromptRefsDocument {
  version: 1;
  refs: PromptRef[];
}

export interface PromptRichDocument {
  version: 1;
  source: "textarea-at" | string;
  text: string;
}

export interface CanvasCamera {
  x: number;
  y: number;
  zoom: number;
}

export interface CanvasPayload {
  camera: CanvasCamera;
  nodes: MediaNode[];
  edges: MediaEdge[];
  groups: MediaGroup[];
}

export interface MediaEdge {
  id: string;
  workspace_id: string;
  from_node_id: string;
  to_node_id: string;
  edge_type: "dependency" | "reference" | "sequence";
  source: string;
  created_at: string;
}

export interface MediaGroup {
  id: string;
  workspace_id: string;
  name: string;
  sort_order: number;
  node_ids: string[];
}

export interface MediaAsset {
  id: string;
  workspace_id: string;
  type: Exclude<AssetType, "json">;
  mime: string;
  storage_url: string;
  access_url?: string;
  thumbnail_url?: string;
  size_bytes?: number;
  created_at: string;
}

export interface ModelCapability {
  provider_id: string;
  model_id: string;
  display_name: string;
  output_types: string[];
  supported_operations: string[];
  supported_input_node_types: string[];
  limits: Record<string, unknown>;
  pricing: Record<string, unknown>;
  defaults: Record<string, unknown>;
  enabled: boolean;
}

export interface GenerationJob {
  id: string;
  workspace_id: string;
  target_node_id: string;
  parent_job_id?: string;
  operation_type: string;
  provider: string;
  model_id: string;
  intent: Record<string, unknown>;
  rendered_prompt: string;
  provider_request: Record<string, unknown>;
  provider_response: Record<string, unknown>;
  status:
    | "pending"
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "cancelled";
  progress: number;
  attempt: number;
  max_attempts: number;
  error_code?: string;
  error_message?: string;
  requested_by_type: string;
  requested_by_id?: string;
  created_at: string;
}

export interface ProductionAsset {
  id: string;
  type: AssetType;
  mime: string;
  storage_url?: string;
  access_url?: string;
  text_content?: string;
  size_bytes?: number;
  metadata: Record<string, unknown>;
}

export interface ProductionPreview {
  version_id: string;
  version_no: number;
  asset_id?: string;
  asset_type?: AssetType;
  mime?: string;
  access_url?: string;
  thumbnail_url?: string;
  text?: string;
  width?: number;
  height?: number;
  duration_ms?: number;
  input_hash?: string;
  created_at?: string;
}

export interface ReferencePackPreviewMember {
  id: string;
  node_type: MediaType;
  title: string;
  status: NodeStatus;
  operation_type?: OperationType | string;
  asset_id?: string | null;
}

export interface ReferencePackPreview {
  member_count: number;
  members: ReferencePackPreviewMember[];
}

export interface ArtifactVersion {
  id: string;
  workspace_id: string;
  node_id: string;
  job_id?: string;
  asset_id?: string;
  version_no: number;
  winner: boolean;
  output: Record<string, unknown>;
  review_score?: number;
  input_hash: string;
  status:
    | "pending"
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "cancelled";
  progress: number;
  error_code?: string;
  error_message?: string;
  provider_request: Record<string, unknown>;
  provider_response: Record<string, unknown>;
  asset?: ProductionAsset;
  created_at: string;
  started_at?: string;
  completed_at?: string;
}

export interface StaleReason {
  id: string;
  node_id: string;
  upstream_node_id: string;
  upstream_version_id?: string;
  reason_code: string;
  reason_message: string;
  details: Record<string, unknown>;
}

export interface SandboxJob {
  id: string;
  workspace_id: string;
  target_node_id?: string;
  generation_job_id?: string;
  job_type: string;
  operation_type: string;
  status:
    | "pending"
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "cancelled";
  sandbox_id?: string;
  command: string;
  cwd: string;
  input: Record<string, unknown>;
  output: Record<string, unknown>;
  exit_code?: number;
  stdout?: string;
  stderr?: string;
  duration_ms: number;
  error_code?: string;
  error_message?: string;
  created_at: string;
}

export interface NodeProductionState {
  node: MediaNode;
  current_version?: ArtifactVersion;
  versions: ArtifactVersion[];
  latest_job?: GenerationJob;
  active_stale_reasons: StaleReason[];
  capability?: ModelCapability;
  sandbox_jobs: SandboxJob[];
}

export interface RunNodeResponse {
  node?: MediaNode;
  job: GenerationJob;
  version?: ArtifactVersion;
}

export interface SelectArtifactVersionResponse {
  node: MediaNode;
  version: ArtifactVersion;
}

export interface ReferencePackItem {
  id: string;
  pack_node_id: string;
  member_node_id: string;
  position: number;
}

const API_BASE = "/api";

export class ApiError extends Error {
  status: number;
  data: unknown;

  constructor(status: number, message: string, data?: unknown) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.data = data;
  }
}

export async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = useAuthStore.getState().token;
  const headers = new Headers(options.headers);

  if (
    !headers.has("Content-Type") &&
    options.body &&
    !(options.body instanceof FormData)
  ) {
    headers.set("Content-Type", "application/json");
  }
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers,
  });

  if (!response.ok) {
    let data: unknown;
    let message = "请求失败";
    try {
      data = await response.json();
      message = apiErrorMessage(data, message);
    } catch {
      message = response.statusText || message;
    }
    throw new ApiError(response.status, message, data);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export function login(email: string, password: string) {
  return apiFetch<AuthResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify({ email, password }),
  });
}

export function register(email: string, password: string, name: string) {
  return apiFetch<AuthResponse>("/auth/register", {
    method: "POST",
    body: JSON.stringify({ email, password, name }),
  });
}

export function fetchMe() {
  return apiFetch<Account>("/auth/me");
}

export function fetchWorkspaces() {
  return apiFetch<Workspace[]>("/workspaces");
}

export function fetchWorkspace(id: string) {
  return apiFetch<Workspace>(`/workspaces/${id}`);
}

export function createWorkspace(input: { name: string; mode: WorkspaceMode }) {
  return apiFetch<Workspace>("/workspaces", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function fetchCanvas(workspaceId: string) {
  return apiFetch<CanvasPayload>(`/workspaces/${workspaceId}/canvas`).then(
    normalizeCanvasPayload,
  );
}

export interface CreateMediaNodeRequest {
  id?: string;
  workspace_id: string;
  node_type: MediaType;
  title: string;
  prompt?: string;
  status?: NodeStatus;
  asset_id?: string;
  operation_type?: OperationType | string;
  model_provider?: string;
  model_id?: string;
  model_params?: Record<string, unknown>;
  canvas_x: number;
  canvas_y: number;
}

export function createMediaNode(input: CreateMediaNodeRequest) {
  return apiFetch<MediaNode>("/nodes", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function uploadMediaAsset(workspaceId: string, file: File) {
  const form = new FormData();
  form.append("workspace_id", workspaceId);
  form.append("file", file);
  return apiFetch<MediaAsset>("/upload", {
    method: "POST",
    body: form,
  });
}

export function updateMediaNode(
  id: string,
  input: Partial<
    Pick<
      MediaNode,
      | "title"
      | "prompt"
      | "status"
      | "group_id"
      | "operation_type"
      | "prompt_refs"
      | "prompt_rich"
      | "model_provider"
      | "model_id"
      | "model_params"
    >
  >,
) {
  return apiFetch<MediaNode>(`/nodes/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function fetchModelCapabilities() {
  return apiFetch<ModelCapability[]>("/model-capabilities");
}

export function fetchNodeProductionState(id: string) {
  return apiFetch<NodeProductionState>(`/nodes/${id}/production-state`);
}

export function runNode(id: string, input: { max_attempts?: number } = {}) {
  return apiFetch<RunNodeResponse>(`/nodes/${id}/run`, {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function selectNodeVersion(id: string, versionId: string) {
  return apiFetch<SelectArtifactVersionResponse>(
    `/nodes/${id}/versions/${versionId}/select`,
    {
      method: "POST",
    },
  );
}

export function retryJob(id: string) {
  return apiFetch<RunNodeResponse>(`/jobs/${id}/retry`, {
    method: "POST",
  });
}

export function fetchReferencePackItems(id: string) {
  return apiFetch<ReferencePackItem[]>(`/reference-packs/${id}/items`);
}

export function replaceReferencePackItems(
  id: string,
  member_node_ids: string[],
) {
  return apiFetch<ReferencePackItem[]>(`/reference-packs/${id}/items`, {
    method: "PUT",
    body: JSON.stringify({ member_node_ids }),
  });
}

export function createMediaGroup(input: {
  workspace_id: string;
  name: string;
  node_ids?: string[];
}) {
  return apiFetch<{ group: Omit<MediaGroup, "node_ids">; node_ids: string[] }>(
    "/groups",
    {
      method: "POST",
      body: JSON.stringify(input),
    },
  );
}

export function updateMediaGroup(
  id: string,
  input: Partial<Pick<MediaGroup, "name" | "sort_order">>,
) {
  return apiFetch<Omit<MediaGroup, "node_ids">>(`/groups/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function deleteMediaGroup(id: string) {
  return apiFetch<void>(`/groups/${id}`, {
    method: "DELETE",
  });
}

export function replaceMediaGroupNodes(id: string, node_ids: string[]) {
  return apiFetch<{
    group: Omit<MediaGroup, "node_ids">;
    node_ids: string[] | null;
  }>(
    `/groups/${id}/nodes`,
    {
      method: "PUT",
      body: JSON.stringify({ node_ids }),
    },
  );
}

function normalizeCanvasPayload(payload: CanvasPayload): CanvasPayload {
  return {
    ...payload,
    groups: payload.groups.map(normalizeMediaGroup),
  };
}

function normalizeMediaGroup(group: MediaGroup): MediaGroup {
  return {
    ...group,
    node_ids: Array.isArray(group.node_ids) ? group.node_ids : [],
  };
}

export function fetchNodeInputs(id: string) {
  return apiFetch<MediaNode[]>(`/nodes/${id}/inputs`);
}

export function deleteMediaNode(id: string) {
  return apiFetch<void>(`/nodes/${id}`, {
    method: "DELETE",
  });
}

export function createMediaEdge(input: {
  workspace_id: string;
  from_node_id: string;
  to_node_id: string;
}) {
  return apiFetch<MediaEdge>("/edges", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function deleteMediaEdge(id: string) {
  return apiFetch<void>(`/edges/${id}`, {
    method: "DELETE",
  });
}

export function batchUpdateNodePositions(
  positions: Array<{ id: string; canvas_x: number; canvas_y: number }>,
) {
  return apiFetch<void>("/nodes/batch-position", {
    method: "PATCH",
    body: JSON.stringify({ positions }),
  });
}

export function updateCamera(workspaceId: string, camera: CanvasCamera) {
  return apiFetch<void>(`/workspaces/${workspaceId}/camera`, {
    method: "PATCH",
    body: JSON.stringify(camera),
  });
}
