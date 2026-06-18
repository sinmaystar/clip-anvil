import { useQuery } from "@tanstack/react-query";
import { Navigate, useNavigate, useParams } from "react-router";
import { fetchCanvas, fetchWorkspace } from "../lib/api";
import { workspaceModeRoute } from "../lib/workspaceRoutes";

export function AgentWorkspacePage() {
  const { id } = useParams();
  const navigate = useNavigate();
  const workspaceQuery = useQuery({
    queryKey: ["workspace", id],
    queryFn: () => fetchWorkspace(id ?? ""),
    enabled: Boolean(id),
  });
  const canvasQuery = useQuery({
    queryKey: ["workspace", id, "canvas"],
    queryFn: () => fetchCanvas(id ?? ""),
    enabled: Boolean(id),
  });

  if (workspaceQuery.isLoading) {
    return (
      <div className="app-route-loading" role="status" aria-label="正在加载" />
    );
  }

  if (workspaceQuery.isError || !workspaceQuery.data) {
    return (
      <main className="agent-workspace-shell">
        <p className="agent-empty-text">项目加载失败</p>
      </main>
    );
  }

  if (workspaceQuery.data.mode !== "agent") {
    return (
      <Navigate
        to={workspaceModeRoute(
          workspaceQuery.data.id,
          workspaceQuery.data.mode,
        )}
        replace
      />
    );
  }

  const canvas = canvasQuery.data;

  return (
    <main className="agent-workspace-shell">
      <header className="agent-topbar">
        <button
          className="studio-secondary-button"
          onClick={() => navigate("/workspaces")}
          type="button"
        >
          返回
        </button>
        <div>
          <p className="workspace-kicker">Agent Workspace</p>
          <h1>{workspaceQuery.data.name}</h1>
        </div>
      </header>

      <section className="agent-workbench">
        <aside className="agent-chat-panel">
          <div className="agent-panel-header">
            <h2>Producer</h2>
            <span>即将接入</span>
          </div>
          <div className="agent-chat-placeholder">
            <p>
              Agent 对话将在 M6 接入。当前工作区已进入 Agent
              模式，画布由 Agent 工具写入。
            </p>
          </div>
        </aside>

        <section className="agent-canvas-panel" aria-label="只读画布">
          <div className="agent-panel-header">
            <h2>只读画布</h2>
            <span>{canvas?.nodes.length ?? 0} 个节点</span>
          </div>
          {canvasQuery.isLoading ? (
            <p className="agent-empty-text">正在加载画布</p>
          ) : canvas && canvas.nodes.length > 0 ? (
            <div className="agent-node-list">
              {canvas.nodes.map((node) => (
                <article className="agent-node-card" key={node.id}>
                  <strong>{node.title || "未命名节点"}</strong>
                  <span>{node.node_type}</span>
                </article>
              ))}
            </div>
          ) : (
            <p className="agent-empty-text">Agent 尚未创建画布节点。</p>
          )}
        </section>
      </section>
    </main>
  );
}
