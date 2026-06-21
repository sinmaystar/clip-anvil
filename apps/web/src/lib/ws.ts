export type CanvasConnectionStatus =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "offline";

export type CanvasEvent =
  | { type: "NodeCreated"; payload: { node: unknown } }
  | { type: "NodeUpdated"; payload: { node: unknown } }
  | { type: "NodeDeleted"; payload: { node_id: string } }
  | { type: "EdgeCreated"; payload: { edge: unknown } }
  | { type: "EdgeDeleted"; payload: { edge_id: string } }
  | { type: "GroupCreated"; payload: { group: unknown } }
  | { type: "GroupUpdated"; payload: { group: unknown } }
  | { type: "GroupDeleted"; payload: { group_id: string } }
  | { type: "production.job.updated"; payload: ProductionJobEventPayload }
  | { type: "production.model.delta"; payload: ProductionJobEventPayload };

export interface ProductionJobEventPayload {
  workspace_id: string;
  node_id: string;
  job_id: string;
  status:
    | "pending"
    | "queued"
    | "running"
    | "succeeded"
    | "failed"
    | "cancelled";
  progress: number;
  delta?: string;
  [key: string]: unknown;
}

interface ConnectCanvasSocketInput {
  workspaceId: string;
  token: string;
  onEvent: (event: CanvasEvent) => void;
  onReconnect: () => void;
  onStatusChange: (status: CanvasConnectionStatus) => void;
}

export function connectCanvasSocket(input: ConnectCanvasSocketInput) {
  let closed = false;
  let attempt = 0;
  let socket: WebSocket | null = null;
  let timer: number | undefined;

  const connect = () => {
    input.onStatusChange(attempt === 0 ? "connecting" : "reconnecting");
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    socket = new WebSocket(
      `${protocol}//${window.location.host}/ws/canvas?workspaceId=${input.workspaceId}&token=${encodeURIComponent(input.token)}`,
    );

    socket.onopen = () => {
      const wasReconnect = attempt > 0;
      attempt = 0;
      input.onStatusChange("connected");
      if (wasReconnect) {
        input.onReconnect();
      }
    };
    socket.onmessage = (message) => {
      input.onEvent(JSON.parse(message.data as string) as CanvasEvent);
    };
    socket.onclose = () => {
      if (closed) {
        input.onStatusChange("offline");
        return;
      }
      const delay = Math.min(30_000, 1_000 * 2 ** attempt);
      attempt += 1;
      timer = window.setTimeout(connect, delay);
    };
    socket.onerror = () => {
      socket?.close();
    };
  };

  connect();

  return () => {
    closed = true;
    window.clearTimeout(timer);
    socket?.close();
  };
}
