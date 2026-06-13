import { useAuthStore } from "../stores/auth";

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

export interface Workspace {
  id: string;
  name: string;
  owner_id: string;
  settings?: Record<string, unknown>;
  created_at: string;
  updated_at: string;
}

export type MediaType = "text" | "image" | "video" | "audio";

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
  asset_url?: string;
  status: NodeStatus;
  canvas_x: number;
  canvas_y: number;
  canvas_w: number;
  canvas_h: number;
  created_at: string;
  updated_at: string;
}

export interface CanvasCamera {
  x: number;
  y: number;
  zoom: number;
}

export interface CanvasPayload {
  camera: CanvasCamera;
  nodes: MediaNode[];
}

const API_BASE = "/api";

export class ApiError extends Error {
  status: number;

  constructor(status: number, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
  }
}

export async function apiFetch<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const token = useAuthStore.getState().token;
  const headers = new Headers(options.headers);

  if (!headers.has("Content-Type") && options.body) {
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
    let message = "请求失败";
    try {
      const data = (await response.json()) as { error?: string };
      message = data.error ?? message;
    } catch {
      message = response.statusText || message;
    }
    throw new ApiError(response.status, message);
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

export function createWorkspace(name: string) {
  return apiFetch<Workspace>("/workspaces", {
    method: "POST",
    body: JSON.stringify({ name }),
  });
}

export function fetchCanvas(workspaceId: string) {
  return apiFetch<CanvasPayload>(`/workspaces/${workspaceId}/canvas`);
}

export function createMediaNode(input: {
  id?: string;
  workspace_id: string;
  node_type: MediaType;
  title: string;
  prompt?: string;
  status?: NodeStatus;
  canvas_x: number;
  canvas_y: number;
}) {
  return apiFetch<MediaNode>("/nodes", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function updateMediaNode(
  id: string,
  input: Partial<Pick<MediaNode, "title" | "prompt" | "status">>,
) {
  return apiFetch<MediaNode>(`/nodes/${id}`, {
    method: "PATCH",
    body: JSON.stringify(input),
  });
}

export function deleteMediaNode(id: string) {
  return apiFetch<void>(`/nodes/${id}`, {
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
