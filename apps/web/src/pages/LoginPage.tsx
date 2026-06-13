import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router";
import { login } from "../lib/api";
import { useAuthStore } from "../stores/auth";

export function LoginPage() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.login);
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError("");
    setIsSubmitting(true);

    try {
      const response = await login(email, password);
      setAuth(response.token, response.account);
      navigate("/workspaces", { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : "登录失败");
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <main className="auth-page">
      <section className="auth-card">
        <div className="auth-heading">
          <p className="auth-kicker">Clip Anvil</p>
          <h1 className="auth-title">登录影砧</h1>
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
            密码
            <input
              className="field-input"
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
            {isSubmitting ? "登录中" : "登录"}
          </button>
        </form>

        <p className="auth-switch">
          没有账号？
          <Link className="auth-link" to="/register">
            注册
          </Link>
        </p>
      </section>
    </main>
  );
}
