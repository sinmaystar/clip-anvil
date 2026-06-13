import { create } from "zustand";
import type { Account } from "../lib/api";

const TOKEN_KEY = "clip-anvil-token";
const ACCOUNT_KEY = "clip-anvil-account";

interface AuthState {
  token: string | null;
  account: Account | null;
  login: (token: string, account: Account) => void;
  logout: () => void;
}

function readStoredToken() {
  return window.localStorage.getItem(TOKEN_KEY);
}

function readStoredAccount() {
  const raw = window.localStorage.getItem(ACCOUNT_KEY);
  if (!raw) {
    return null;
  }

  try {
    return JSON.parse(raw) as Account;
  } catch {
    window.localStorage.removeItem(ACCOUNT_KEY);
    return null;
  }
}

export const useAuthStore = create<AuthState>((set) => ({
  token: readStoredToken(),
  account: readStoredAccount(),
  login: (token, account) => {
    window.localStorage.setItem(TOKEN_KEY, token);
    window.localStorage.setItem(ACCOUNT_KEY, JSON.stringify(account));
    set({ token, account });
  },
  logout: () => {
    window.localStorage.removeItem(TOKEN_KEY);
    window.localStorage.removeItem(ACCOUNT_KEY);
    set({ token: null, account: null });
  },
}));
