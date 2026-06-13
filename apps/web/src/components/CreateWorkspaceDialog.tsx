import { FormEvent, useEffect, useRef, useState } from "react";

interface CreateWorkspaceDialogProps {
  isOpen: boolean;
  isSubmitting: boolean;
  error: string;
  onClose: () => void;
  onSubmit: (name: string) => void;
}

export function CreateWorkspaceDialog({
  isOpen,
  isSubmitting,
  error,
  onClose,
  onSubmit,
}: CreateWorkspaceDialogProps) {
  const [name, setName] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      setName("");
      window.setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [isOpen]);

  if (!isOpen) {
    return null;
  }

  const handleSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    onSubmit(name);
  };

  return (
    <div className="modal-backdrop">
      <section className="modal-card">
        <div className="modal-header">
          <h2 className="modal-title">新建项目</h2>
          <p className="modal-description">给这支营销视频起一个工作名。</p>
        </div>

        <form className="auth-form" onSubmit={handleSubmit}>
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
