import type { AgentEvent, AgentMessage, AgentTask } from "./agentApi";

export type AgentConnectionStatus =
  | "connecting"
  | "connected"
  | "reconnecting"
  | "offline";

export type AgentSocketEvent =
  | {
      type: "agent.message.created";
      payload: {
        workspace_id: string;
        thread_id: string;
        message: AgentMessage;
        event: AgentEvent;
      };
    }
  | {
      type: "agent.message.updated";
      payload: {
        workspace_id: string;
        thread_id: string;
        message: AgentMessage;
        event: AgentEvent;
      };
    }
  | {
      type: "agent.message.delta";
      payload: {
        workspace_id: string;
        thread_id: string;
        task_id: string;
        message_id?: string;
        block_id: string;
        block_type: "markdown" | "thinking" | string;
        delta: string;
        sequence: number;
      };
    }
  | {
      type: "agent.event.created";
      payload: {
        workspace_id: string;
        thread_id?: string | null;
        task_id?: string | null;
        event: AgentEvent;
      };
    }
  | {
      type: "agent.task.updated";
      payload: {
        workspace_id: string;
        thread_id?: string | null;
        task: AgentTask;
      };
    };

interface ConnectAgentSocketInput {
  workspaceId: string;
  token: string;
  onEvent: (event: AgentSocketEvent) => void;
  onReconnect: () => void;
  onStatusChange: (status: AgentConnectionStatus) => void;
}

export function connectAgentSocket(input: ConnectAgentSocketInput) {
  let closed = false;
  let attempt = 0;
  let socket: WebSocket | null = null;
  let timer: number | undefined;

  const connect = () => {
    input.onStatusChange(attempt === 0 ? "connecting" : "reconnecting");
    const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";
    socket = new WebSocket(
      `${protocol}//${window.location.host}/ws/agent?workspaceId=${input.workspaceId}&token=${encodeURIComponent(input.token)}`,
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
      input.onEvent(JSON.parse(message.data as string) as AgentSocketEvent);
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
