import { FormEvent, useState } from "react";
import { Link, useNavigate } from "react-router";
import { register } from "../lib/api";
import { useAuthStore } from "../stores/auth";

export function RegisterPage() {
  const navigate = useNavigate();
  const setAuth = useAuthStore((state) => state.login);
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
      <section className="auth-card">
        <div className="auth-heading">
          <p className="auth-kicker">Clip Anvil</p>
          <h1 className="auth-title">创建账号</h1>
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
            {isSubmitting ? "注册中" : "注册"}
          </button>
        </form>

        <p className="auth-switch">
          已有账号？
          <Link className="auth-link" to="/login">
            登录
          </Link>
        </p>
      </section>
    </main>
  );
}
