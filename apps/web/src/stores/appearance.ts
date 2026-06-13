import { create } from "zustand";

export type Appearance = "dark" | "light";

const APPEARANCE_KEY = "clip-anvil-appearance";

function readStoredAppearance(): Appearance {
  if (typeof window === "undefined") {
    return "dark";
  }

  return window.localStorage.getItem(APPEARANCE_KEY) === "light"
    ? "light"
    : "dark";
}

interface AppearanceState {
  appearance: Appearance;
  toggleAppearance: () => void;
}

export const useAppearanceStore = create<AppearanceState>((set, get) => ({
  appearance: readStoredAppearance(),
  toggleAppearance: () => {
    const next = get().appearance === "dark" ? "light" : "dark";
    window.localStorage.setItem(APPEARANCE_KEY, next);
    set({ appearance: next });
  },
}));
