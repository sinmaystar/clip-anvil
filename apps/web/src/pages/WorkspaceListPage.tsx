import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useNavigate } from "react-router";
import { CreateWorkspaceDialog } from "../components/CreateWorkspaceDialog";
import { createWorkspace, fetchWorkspaces } from "../lib/api";

export function WorkspaceListPage() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [error, setError] = useState("");

  const workspacesQuery = useQuery({
    queryKey: ["workspaces"],
    queryFn: fetchWorkspaces,
  });

  const createMutation = useMutation({
    mutationFn: createWorkspace,
    onSuccess: async (workspace) => {
      await queryClient.invalidateQueries({ queryKey: ["workspaces"] });
      setIsDialogOpen(false);
      navigate(`/workspaces/${workspace.id}`);
    },
    onError: (err) => {
      setError(err instanceof Error ? err.message : "创建项目失败");
    },
  });

  const handleCreate = (name: string) => {
    setError("");
    createMutation.mutate(name);
  };

  return (
    <main className="workspace-page">
      <section className="workspace-inner">
        <div className="workspace-header">
          <div>
            <p className="workspace-kicker">Workspace</p>
            <h1 className="workspace-title">我的项目</h1>
          </div>
          <button
            className="workspace-create-button"
            onClick={() => {
              setError("");
              setIsDialogOpen(true);
            }}
            type="button"
          >
            + 新建项目
          </button>
        </div>

        {workspacesQuery.isLoading ? (
          <div className="workspace-state">
            加载中
          </div>
        ) : null}

        {workspacesQuery.isError ? (
          <div className="workspace-state" data-tone="danger">
            项目加载失败
          </div>
        ) : null}

        {workspacesQuery.data?.length === 0 ? (
          <div className="workspace-empty">
            <p className="workspace-empty-title">
              还没有项目
            </p>
            <p className="workspace-empty-copy">点击上方按钮创建第一个项目。</p>
          </div>
        ) : null}

        {workspacesQuery.data && workspacesQuery.data.length > 0 ? (
          <div className="workspace-grid">
            {workspacesQuery.data.map((workspace) => (
              <button
                className="workspace-card"
                key={workspace.id}
                onClick={() => navigate(`/workspaces/${workspace.id}`)}
                type="button"
              >
                <h2 className="workspace-card-title">
                  {workspace.name}
                </h2>
                <p className="workspace-card-meta">
                  {formatDate(workspace.created_at)} 创建
                </p>
              </button>
            ))}
          </div>
        ) : null}
      </section>

      <CreateWorkspaceDialog
        error={error}
        isOpen={isDialogOpen}
        isSubmitting={createMutation.isPending}
        onClose={() => setIsDialogOpen(false)}
        onSubmit={handleCreate}
      />
    </main>
  );
}

function formatDate(value: string) {
  return new Intl.DateTimeFormat("zh-CN", {
    month: "numeric",
    day: "numeric",
  }).format(new Date(value));
}
