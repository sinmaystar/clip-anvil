import type { CanvasConnectionStatus } from "../lib/ws";

const label: Record<CanvasConnectionStatus, string> = {
  connecting: "连接中",
  connected: "已连接",
  reconnecting: "重连中",
  offline: "离线",
};

export function ConnectionStatus({
  status,
}: {
  status: CanvasConnectionStatus;
}) {
  return (
    <span className="connection-status" data-status={status}>
      {label[status]}
    </span>
  );
}
