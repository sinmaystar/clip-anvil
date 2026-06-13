import { Navigate, Outlet } from "react-router";
import { useAuthStore } from "../stores/auth";

export function GuestRoute() {
  const token = useAuthStore((state) => state.token);

  if (token) {
    return <Navigate to="/workspaces" replace />;
  }

  return <Outlet />;
}
