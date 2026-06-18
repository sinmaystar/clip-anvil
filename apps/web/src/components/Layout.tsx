import { Outlet, useNavigate } from "react-router";
import { useAppearanceStore } from "../stores/appearance";
import { useAuthStore } from "../stores/auth";

export function Layout() {
  const navigate = useNavigate();
  const account = useAuthStore((state) => state.account);
  const logout = useAuthStore((state) => state.logout);
  const toggleAppearance = useAppearanceStore((state) => state.toggleAppearance);

  const handleLogout = () => {
    logout();
    navigate("/login", { replace: true });
  };

  return (
    <div className="app-shell">
      <header className="app-topbar">
        <button
          className="app-brand-button"
          onClick={() => navigate("/workspaces")}
          type="button"
        >
          影砧
        </button>
        <div className="app-account-row">
          <button
            aria-label="切换明暗主题"
            className="app-theme-button"
            onClick={toggleAppearance}
            type="button"
          >
            ◐
          </button>
          <span className="app-account-name">{account?.name ?? "未登录"}</span>
          <button
            className="app-logout-button"
            onClick={handleLogout}
            type="button"
          >
            登出
          </button>
        </div>
      </header>
      <Outlet />
    </div>
  );
}
