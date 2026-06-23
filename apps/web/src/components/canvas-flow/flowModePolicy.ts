import { createContext, useContext } from "react";
import type { CanvasFlowMode } from "./flowTypes";

export interface CanvasFlowPolicy {
  canPanZoom: boolean;
  canSelect: boolean;
  canDragNodes: boolean;
  canPersistViewport: boolean;
  canCreateNodes: boolean;
  canDeleteNodes: boolean;
  canCreateEdges: boolean;
  canDeleteEdges: boolean;
  canUploadAssets: boolean;
  canEditNodeContent: boolean;
  canRunNodes: boolean;
  canEditGroups: boolean;
}

export const studioFlowPolicy: CanvasFlowPolicy = {
  canPanZoom: true,
  canSelect: true,
  canDragNodes: true,
  canPersistViewport: true,
  canCreateNodes: true,
  canDeleteNodes: true,
  canCreateEdges: true,
  canDeleteEdges: true,
  canUploadAssets: true,
  canEditNodeContent: true,
  canRunNodes: true,
  canEditGroups: true,
};

export const agentFlowPolicy: CanvasFlowPolicy = {
  canPanZoom: true,
  canSelect: true,
  canDragNodes: true,
  canPersistViewport: true,
  canCreateNodes: false,
  canDeleteNodes: false,
  canCreateEdges: false,
  canDeleteEdges: false,
  canUploadAssets: false,
  canEditNodeContent: false,
  canRunNodes: false,
  canEditGroups: false,
};

export function policyForCanvasMode(mode: CanvasFlowMode): CanvasFlowPolicy {
  return mode === "studio" ? studioFlowPolicy : agentFlowPolicy;
}

const CanvasFlowPolicyContext =
  createContext<CanvasFlowPolicy>(studioFlowPolicy);

export const CanvasFlowPolicyProvider = CanvasFlowPolicyContext.Provider;

export function useCanvasFlowPolicy(): CanvasFlowPolicy {
  return useContext(CanvasFlowPolicyContext);
}
