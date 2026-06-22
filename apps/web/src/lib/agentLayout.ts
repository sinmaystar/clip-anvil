export const agentPanelWidthStorageKey = "clip-anvil-agent-panel-width";
export const agentPanelHeightStorageKey = "clip-anvil-agent-panel-height";
export const agentRestorePositionStorageKey =
  "clip-anvil-agent-restore-position";

export interface AgentFloatingPosition {
  x: number;
  y: number;
}

export type AgentPanelCorner = "top-left" | "bottom-left";

export interface AgentPanelCornerResizeInput {
  corner: AgentPanelCorner;
  startClientX: number;
  startClientY: number;
  clientX: number;
  clientY: number;
  startWidth: number;
  startHeight: number;
  viewportWidth: number;
  viewportHeight: number;
}

export function clampAgentPanelWidth(width: number, viewportWidth: number) {
  const max = Math.max(320, Math.min(720, viewportWidth - 80));
  return Math.min(Math.max(width, 360), max);
}

export function clampAgentPanelHeight(height: number, viewportHeight: number) {
  const max = Math.max(360, viewportHeight - 32);
  return Math.min(Math.max(height, 360), max);
}

export interface AgentMessageScrollState {
  scrollHeight: number;
  clientHeight: number;
  scrollTop: number;
}

export function isAgentMessageListNearBottom(
  state: AgentMessageScrollState,
  threshold = 64,
) {
  return state.scrollHeight - state.clientHeight - state.scrollTop <= threshold;
}

export function resizeAgentPanelFromCorner({
  corner,
  startClientX,
  startClientY,
  clientX,
  clientY,
  startWidth,
  startHeight,
  viewportWidth,
  viewportHeight,
}: AgentPanelCornerResizeInput) {
  const nextHeight =
    corner === "top-left"
      ? startHeight + startClientY - clientY
      : startHeight + clientY - startClientY;

  return {
    width: clampAgentPanelWidth(
      startWidth + startClientX - clientX,
      viewportWidth,
    ),
    height: clampAgentPanelHeight(nextHeight, viewportHeight),
  };
}

export function clampAgentRestorePosition(
  position: AgentFloatingPosition,
  viewportWidth: number,
  viewportHeight: number,
  size = 54,
  margin = 16,
): AgentFloatingPosition {
  return {
    x: Math.min(Math.max(position.x, margin), viewportWidth - size - margin),
    y: Math.min(Math.max(position.y, margin), viewportHeight - size - margin),
  };
}

export function defaultAgentRestorePosition(
  viewportWidth: number,
  viewportHeight: number,
  size = 54,
  margin = 16,
): AgentFloatingPosition {
  return {
    x: viewportWidth - size - margin,
    y: viewportHeight - size - margin,
  };
}
