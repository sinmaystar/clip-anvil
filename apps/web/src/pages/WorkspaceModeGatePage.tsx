import { useQuery } from "@tanstack/react-query";
import { Navigate, useParams } from "react-router";
import { fetchWorkspace } from "../lib/api";
import { workspaceRoute } from "../lib/workspaceRoutes";

export function WorkspaceModeGatePage() {
  const { id } = useParams();
  const workspaceQuery = useQuery({
    queryKey: ["workspace", id],
    queryFn: () => fetchWorkspace(id ?? ""),
    enabled: Boolean(id),
  });

  if (workspaceQuery.isLoading) {
    return (
      <div className="app-route-loading" role="status" aria-label="正在加载" />
    );
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <main className="workspace-route-state">
        <p>项目加载失败</p>
      </main>
    );
  }

  return <Navigate to={workspaceRoute(workspaceQuery.data)} replace />;
}
