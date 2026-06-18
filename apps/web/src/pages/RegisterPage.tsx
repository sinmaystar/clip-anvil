import { FormEvent, useState, type CSSProperties } from "react";
import { Link, useNavigate } from "react-router";
import { register } from "../lib/api";
import { useAppearanceStore } from "../stores/appearance";
import { useAuthStore } from "../stores/auth";

export function RegisterPage() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.login);
  const toggleAppearance = useAppearanceStore((state) => state.toggleAppearance);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);

    try {
      const response = await register(email, password, name);
      setAuth(response.token, response.account);
      navigate("/workspaces", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "注册失败");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <main className="auth-page">
      <div className="auth-nav">
        <button
          className="app-brand-button"
          onClick={() => navigate("/workspaces")}
          type="button"
        >
          影砧
        </button>
        <button
          aria-label="切换明暗主题"
          className="app-theme-button"
          onClick={toggleAppearance}
          type="button"
        >
          ◐
        </button>
      </div>
      <section className="auth-stage">
        <div className="auth-card">
          <div className="auth-art" aria-hidden="true" />
          <div className="auth-content">
            <div className="auth-hero">
              <p className="auth-kicker">AI Video Studio</p>
              <h1>从灵感到分镜，再到可生成的视频画布。</h1>
              <p>
                影砧把提示词、参考图、声音、镜头和版本组织成一张可编辑的生成网络。
              </p>
              <div className="film-stack" aria-hidden="true">
                <span style={{ "--poster": "#dbeafe" } as CSSProperties}>
                  Reference
                </span>
                <span style={{ "--poster": "#ccfbf1" } as CSSProperties}>
                  Storyboard
                </span>
                <span style={{ "--poster": "#e9d5ff" } as CSSProperties}>
                  Generate
                </span>
                <span style={{ "--poster": "#dcfce7" } as CSSProperties}>
                  Final
                </span>
              </div>
            </div>

            <div className="login-panel">
              <div className="auth-heading">
                <p className="auth-kicker">Start creating</p>
                <h2 className="auth-title">创建账号</h2>
              </div>

              <form className="auth-form" onSubmit={handleSubmit}>
                <label className="field-label">
                  邮箱
                  <input
                    className="field-input"
                    onChange={(event) => setEmail(event.target.value)}
                    required
                    type="email"
                    value={email}
                  />
                </label>

                <label className="field-label">
                  昵称
                  <input
                    className="field-input"
                    onChange={(event) => setName(event.target.value)}
                    required
                    type="text"
                    value={name}
                  />
                </label>

                <label className="field-label">
                  密码
                  <input
                    className="field-input"
                    minLength={6}
                    onChange={(event) => setPassword(event.target.value)}
                    required
                    type="password"
                    value={password}
                  />
                </label>

                {error ? <p className="form-error">{error}</p> : null}

                <button
                  className="auth-submit"
                  disabled={isSubmitting}
                  type="submit"
                >
                  {isSubmitting ? "注册中" : "进入创作空间"}
                </button>
              </form>

              <p className="auth-switch">
                已有账号？
                <Link className="auth-link" to="/login">
                  登录
                </Link>
              </p>
            </div>
          </div>
        </div>
      </section>
    </main>
  );
}
