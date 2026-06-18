import { FormEvent, useEffect, useRef, useState } from "react";
import type { WorkspaceMode } from "../lib/api";

interface CreateWorkspaceDialogProps {
  isOpen: boolean;
  isSubmitting: boolean;
  error: string;
  onClose: () => void;
  onSubmit: (input: { name: string; mode: WorkspaceMode }) => void;
}

export function CreateWorkspaceDialog({
  isOpen,
  isSubmitting,
  error,
  onClose,
  onSubmit,
}: CreateWorkspaceDialogProps) {
  const [name, setName] = useState("");
  const [mode, setMode] = useState<WorkspaceMode>("studio");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      setName("");
      setMode("studio");
      window.setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [isOpen]);

  if (!isOpen) {
    return null;
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onSubmit({ name, mode });
  };

  return (
    <div className="modal-backdrop">
      <section className="modal-card">
        <div className="modal-header">
          <h2 className="modal-title">新建项目</h2>
          <p className="modal-description">给这支营销视频起一个工作名。</p>
        </div>

        <form className="auth-form" onSubmit={handleSubmit}>
          <fieldset className="workspace-mode-picker">
            <legend>项目模式</legend>
            <label
              className="workspace-mode-option"
              data-selected={mode === "studio"}
            >
              <input
                checked={mode === "studio"}
                name="workspace-mode"
                onChange={() => setMode("studio")}
                type="radio"
                value="studio"
              />
              <span>
                <strong>Studio 手动模式</strong>
                <small>专业用户手动搭建节点、连线和运行。</small>
              </span>
            </label>
            <label
              className="workspace-mode-option"
              data-selected={mode === "agent"}
            >
              <input
                checked={mode === "agent"}
                name="workspace-mode"
                onChange={() => setMode("agent")}
                type="radio"
                value="agent"
              />
              <span>
                <strong>Agent 自动模式</strong>
                <small>通过对话驱动 Agent 规划和生产，画布只读。</small>
              </span>
            </label>
          </fieldset>

          <label className="field-label">
            项目名称
            <input
              className="field-input"
              onChange={(event) => setName(event.target.value)}
              ref={inputRef}
              required
              type="text"
              value={name}
            />
          </label>

          {error ? <p className="form-error">{error}</p> : null}

          <div className="form-actions">
            <button
              className="form-secondary-button"
              onClick={onClose}
              type="button"
            >
              取消
            </button>
            <button
              className="form-primary-button"
              disabled={isSubmitting}
              type="submit"
            >
              {isSubmitting ? "创建中" : "确认"}
            </button>
          </div>
        </form>
      </section>
    </div>
  );
}
