import { createContext, useContext } from "react";
import type { ReactNode } from "react";
import type {
  AgentWorkbenchObjectType,
  AgentWorkbenchSelection,
} from "../../lib/agentWorkbenchSelection";
import { agentWorkbenchSelectionEquals } from "../../lib/agentWorkbenchSelection";

interface AgentWorkbenchSelectionContextValue {
  selected: AgentWorkbenchSelection | null;
  select: (selection: AgentWorkbenchSelection) => void;
  isSelected: (
    objectType: AgentWorkbenchObjectType,
    objectId: string | undefined | null,
  ) => boolean;
}

const AgentWorkbenchSelectionContext =
  createContext<AgentWorkbenchSelectionContextValue | null>(null);

export function AgentWorkbenchSelectionProvider({
  children,
  selected,
  onSelect,
}: {
  children: ReactNode;
  selected: AgentWorkbenchSelection | null;
  onSelect: (selection: AgentWorkbenchSelection) => void;
}) {
  return (
    <AgentWorkbenchSelectionContext.Provider
      value={{
        selected,
        select: onSelect,
        isSelected: (objectType, objectId) =>
          agentWorkbenchSelectionEquals(selected, objectType, objectId),
      }}
    >
      {children}
    </AgentWorkbenchSelectionContext.Provider>
  );
}

export function useAgentWorkbenchSelection() {
  const context = useContext(AgentWorkbenchSelectionContext);
  if (!context) {
    throw new Error(
      "useAgentWorkbenchSelection must be used inside AgentWorkbenchSelectionProvider",
    );
  }
  return context;
}
